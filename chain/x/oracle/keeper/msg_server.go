package keeper

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/oracle/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

func (m msgServer) onlyAuthority(authority string) error {
	if authority != m.GetAuthority() {
		return sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	return nil
}

// ---------------------------------------------------------------------------
// Topic lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterTopic(goCtx context.Context, msg *types.MsgRegisterTopic) (*types.MsgRegisterTopicResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if m.HasTopic(ctx, msg.Topic.Id) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("topic %s already exists", msg.Topic.Id)
	}
	now := ctx.BlockTime().Unix()
	t := msg.Topic
	t.CreatedAt = now
	t.UpdatedAt = now
	if err := m.SetTopic(ctx, t); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_topic_registered",
		"topic_id", t.Id, "kind", t.Kind.String(), "agg", t.Aggregation.String())
	return &types.MsgRegisterTopicResponse{}, nil
}

func (m msgServer) UpdateTopic(goCtx context.Context, msg *types.MsgUpdateTopic) (*types.MsgUpdateTopicResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	prior, ok := m.GetTopic(ctx, msg.Topic.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("topic %s", msg.Topic.Id)
	}
	t := msg.Topic
	// Preserve immutable fields: created_at + paused state remain under
	// MsgPause/Resume control; the update flow is for params + allow_list.
	t.CreatedAt = prior.CreatedAt
	t.Paused = prior.Paused
	t.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetTopic(ctx, t); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_topic_updated", "topic_id", t.Id)
	return &types.MsgUpdateTopicResponse{}, nil
}

func (m msgServer) PauseTopic(goCtx context.Context, msg *types.MsgPauseTopic) (*types.MsgPauseTopicResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	t, ok := m.GetTopic(ctx, msg.TopicId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("topic %s", msg.TopicId)
	}
	t.Paused = true
	t.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetTopic(ctx, t); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_topic_paused", "topic_id", t.Id, "reason", msg.Reason)
	return &types.MsgPauseTopicResponse{}, nil
}

func (m msgServer) ResumeTopic(goCtx context.Context, msg *types.MsgResumeTopic) (*types.MsgResumeTopicResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	t, ok := m.GetTopic(ctx, msg.TopicId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("topic %s", msg.TopicId)
	}
	t.Paused = false
	t.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetTopic(ctx, t); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_topic_resumed", "topic_id", t.Id)
	return &types.MsgResumeTopicResponse{}, nil
}

// ---------------------------------------------------------------------------
// Provider lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterProvider(goCtx context.Context, msg *types.MsgRegisterProvider) (*types.MsgRegisterProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, ok := m.GetProvider(ctx, msg.Address); ok {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("provider already registered")
	}
	params := m.GetParams(ctx)
	if msg.Bond.Denom != params.BondDenom {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("bond denom %s != %s", msg.Bond.Denom, params.BondDenom)
	}
	if msg.Bond.Amount.LT(params.BondMinInt()) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("bond %s below minimum %s%s",
			msg.Bond.Amount, params.BondMinInt(), params.BondDenom)
	}
	addr, _ := sdk.AccAddressFromBech32(msg.Address)
	if err := m.EscrowFrom(ctx, addr, sdk.NewCoins(msg.Bond)); err != nil {
		return nil, sdkerrors.ErrInsufficientFunds.Wrapf("escrow: %s", err)
	}
	now := ctx.BlockTime().Unix()
	p := types.Provider{
		Address:       msg.Address,
		Name:          msg.Name,
		Status:        types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond:          msg.Bond,
		JoinedAt:      now,
		Ed25519Pubkey: msg.Ed25519Pubkey,
		ContactUri:    msg.ContactUri,
	}
	if err := m.SetProvider(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_provider_registered",
		"address", msg.Address, "bond", msg.Bond.String())
	return &types.MsgRegisterProviderResponse{}, nil
}

func (m msgServer) TopUpBond(goCtx context.Context, msg *types.MsgTopUpBond) (*types.MsgTopUpBondResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProvider(ctx, msg.Address)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrap("provider not registered")
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_RETIRED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("provider retired")
	}
	if msg.Amount.Denom != p.Bond.Denom {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("denom mismatch %s vs %s", msg.Amount.Denom, p.Bond.Denom)
	}
	addr, _ := sdk.AccAddressFromBech32(msg.Address)
	if err := m.EscrowFrom(ctx, addr, sdk.NewCoins(msg.Amount)); err != nil {
		return nil, sdkerrors.ErrInsufficientFunds.Wrapf("escrow: %s", err)
	}
	p.Bond = p.Bond.Add(msg.Amount)
	if err := m.SetProvider(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_bond_topped_up",
		"address", msg.Address, "amount", msg.Amount.String(), "new_bond", p.Bond.String())
	return &types.MsgTopUpBondResponse{}, nil
}

func (m msgServer) RequestWithdrawBond(goCtx context.Context, msg *types.MsgRequestWithdrawBond) (*types.MsgRequestWithdrawBondResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProvider(ctx, msg.Address)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrap("provider not registered")
	}
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("only ACTIVE providers may begin withdrawal")
	}
	params := m.GetParams(ctx)
	releaseHeight := ctx.BlockHeight() + params.BondReleaseCooldown
	p.Status = types.ProviderStatus_PROVIDER_STATUS_WITHDRAWING
	p.BondReleaseHeight = releaseHeight
	if err := m.SetProvider(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	if err := m.EnqueueBondRelease(ctx, releaseHeight, p.Address); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("enqueue: %s", err)
	}
	emit(ctx, "oracle_bond_withdraw_requested",
		"address", msg.Address, "release_height", fmt.Sprintf("%d", releaseHeight))
	return &types.MsgRequestWithdrawBondResponse{ReleaseHeight: releaseHeight}, nil
}

func (m msgServer) WithdrawBond(goCtx context.Context, msg *types.MsgWithdrawBond) (*types.MsgWithdrawBondResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProvider(ctx, msg.Address)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrap("provider not registered")
	}
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_WITHDRAWING {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("provider is not in WITHDRAWING status")
	}
	if ctx.BlockHeight() < p.BondReleaseHeight {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"cooldown not elapsed: current height %d, release height %d",
			ctx.BlockHeight(), p.BondReleaseHeight)
	}
	addr, _ := sdk.AccAddressFromBech32(msg.Address)
	returned := p.Bond
	// SECURITY: a fully-slashed bond (Bond.Amount == 0) still progresses
	// through the withdrawal flow but skips the bank send. Without this
	// guard, sdk.NewCoins(zero) yields an empty coin set whose Send
	// behaviour in some bank implementations is undefined. Recording the
	// retire transition unconditionally keeps the audit trail intact.
	if p.Bond.IsPositive() {
		if err := m.ReleaseTo(ctx, addr, sdk.NewCoins(p.Bond)); err != nil {
			return nil, sdkerrors.ErrInsufficientFunds.Wrapf("release: %s", err)
		}
	}
	// Retire the provider rather than delete the row, so audit trails
	// can still resolve historical submissions to a known identity.
	p.Status = types.ProviderStatus_PROVIDER_STATUS_RETIRED
	p.Bond = sdk.NewCoin(p.Bond.Denom, sdkmath.ZeroInt())
	p.BondReleaseHeight = 0
	if err := m.SetProvider(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_bond_withdrawn",
		"address", msg.Address, "returned", returned.String())
	return &types.MsgWithdrawBondResponse{Returned: returned}, nil
}

func (m msgServer) SuspendProvider(goCtx context.Context, msg *types.MsgSuspendProvider) (*types.MsgSuspendProviderResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProvider(ctx, msg.Address)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrap("provider not registered")
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_RETIRED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("provider already retired")
	}
	now := ctx.BlockTime().Unix()
	if msg.Until <= now {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("until must be in the future")
	}
	p.Status = types.ProviderStatus_PROVIDER_STATUS_SUSPENDED
	p.SuspendedAt = now
	p.SuspendedUntil = msg.Until
	p.SuspendedReason = msg.Reason
	if err := m.SetProvider(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_provider_suspended",
		"address", msg.Address, "until", fmt.Sprintf("%d", msg.Until),
		"reason", msg.Reason)
	return &types.MsgSuspendProviderResponse{}, nil
}

// ---------------------------------------------------------------------------
// Submissions
// ---------------------------------------------------------------------------

func (m msgServer) SubmitValue(goCtx context.Context, msg *types.MsgSubmitValue) (*types.MsgSubmitValueResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	t, ok := m.GetTopic(ctx, msg.TopicId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("topic %s", msg.TopicId)
	}
	if !t.Enabled {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("topic disabled")
	}
	if t.Paused {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("topic paused")
	}
	if !m.IsActiveProvider(ctx, msg.Provider, t.Id) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("provider not allowed for this topic")
	}
	params := m.GetParams(ctx)
	if uint32(len(msg.Metadata)) > params.MaxMetadataSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metadata size %d exceeds %d",
			len(msg.Metadata), params.MaxMetadataSize)
	}
	now := ctx.BlockTime().Unix()
	maxAge := t.MaxDataAgeSeconds
	if maxAge == 0 {
		maxAge = params.DataMaxAge
	}
	if maxAge > 0 && now-msg.Timestamp > maxAge {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"submission too old: %d vs now %d (max age %d)",
			msg.Timestamp, now, maxAge)
	}
	if msg.Timestamp > now+60 {
		// 60-second positive skew tolerates clock drift; anything
		// further into the future is almost certainly a bug or a
		// griefing attempt.
		return nil, sdkerrors.ErrInvalidRequest.Wrap("submission timestamp too far in the future")
	}
	s := types.Submission{
		TopicId:     msg.TopicId,
		Provider:    msg.Provider,
		Value:       msg.Value,
		Metadata:    msg.Metadata,
		Timestamp:   msg.Timestamp,
		Height:      ctx.BlockHeight(),
		Attestation: msg.Attestation,
	}
	if err := m.SetSubmission(ctx, s); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_value_submitted",
		"topic_id", msg.TopicId, "provider", msg.Provider,
		"value", fmt.Sprintf("%d", msg.Value), "ts", fmt.Sprintf("%d", msg.Timestamp))
	return &types.MsgSubmitValueResponse{}, nil
}

func (m msgServer) SubmitReserveAttestation(goCtx context.Context, msg *types.MsgSubmitReserveAttestation) (*types.MsgSubmitReserveAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, exists := m.GetReserveAttestation(ctx, msg.Id); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("attestation id %s exists", msg.Id)
	}
	r := types.ReserveAttestation{
		Id:              msg.Id,
		Asset:           msg.Asset,
		Custodian:       msg.Custodian,
		Amount:          msg.Amount,
		AttestationUri:  msg.AttestationUri,
		AttestationHash: msg.AttestationHash,
		Signers:         msg.Signers,
		Threshold:       msg.Threshold,
		SubmittedAt:     ctx.BlockTime().Unix(),
		SubmittedHeight: ctx.BlockHeight(),
	}
	if err := m.SetReserveAttestation(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "oracle_reserve_attested",
		"id", msg.Id, "asset", msg.Asset, "amount", msg.Amount,
		"threshold", fmt.Sprintf("%d/%d", msg.Threshold, len(msg.Signers)))
	return &types.MsgSubmitReserveAttestationResponse{}, nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("params: %s", err)
	}
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// emit is a tiny helper that builds a string-attribute event without
// dragging the eventattr package import everywhere; the audit / dataslash
// modules consume the emitted attributes verbatim.
func emit(ctx sdk.Context, kind string, kvs ...string) {
	if len(kvs)%2 != 0 {
		ctx.Logger().Error("oracle: emit got odd kvs", "kind", kind)
		return
	}
	attrs := make([]sdk.Attribute, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(kvs[i], kvs[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(kind, attrs...))
}
