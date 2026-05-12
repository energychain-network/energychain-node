package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/did/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

// emitDIDEvent writes a structured event to the chain's event manager
// using the canonical attribute keys. The audit module subscribes to
// these so a single source of truth (the keeper) feeds both the live
// websocket stream and the on-chain audit log.
func emitDIDEvent(ctx sdk.Context, eventType, did, actor, reason string) {
	attrs := []sdk.Attribute{
		sdk.NewAttribute("did", did),
		sdk.NewAttribute("actor", actor),
	}
	if reason != "" {
		attrs = append(attrs, sdk.NewAttribute("reason", reason))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(eventType, attrs...))
}

// ---------------------------------------------------------------------------
// DID lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) CreateDID(goCtx context.Context, msg *types.MsgCreateDID) (*types.MsgCreateDIDResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, exists := m.GetDocument(ctx, msg.Subject); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("did %s already exists", msg.Subject)
	}

	now := ctx.BlockTime().Unix()
	controllers := msg.Controllers
	if len(controllers) == 0 {
		// Default: subject controls themselves. Saves boilerplate at
		// the call site and matches W3C's "implicit controller is the
		// subject" convention.
		controllers = []string{msg.Subject}
	}
	doc := types.DIDDocument{
		Id:           msg.Subject,
		Controllers:  controllers,
		Verification: msg.Verification,
		Service:      msg.Service,
		Status:       types.StatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
		Version:      1,
	}
	for i := range doc.Verification {
		if doc.Verification[i].CreatedAt == 0 {
			doc.Verification[i].CreatedAt = now
		}
		if doc.Verification[i].Controller == "" {
			doc.Verification[i].Controller = msg.Subject
		}
	}

	params := m.GetParams(ctx)
	if err := types.ValidateDocumentShape(&doc, params); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid document: %s", err)
	}
	if err := m.SetDocument(ctx, doc); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}

	emitDIDEvent(ctx, "did_created", msg.Subject, msg.Creator, "")
	return &types.MsgCreateDIDResponse{Version: 1}, nil
}

func (m msgServer) UpdateDID(goCtx context.Context, msg *types.MsgUpdateDID) (*types.MsgUpdateDIDResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	doc, ok := m.GetDocument(ctx, msg.Subject)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("did %s", msg.Subject)
	}
	if doc.Status != types.StatusActive {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("cannot update a deactivated DID")
	}
	if !m.IsController(ctx, msg.Subject, msg.Controller) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("%s is not a controller of %s", msg.Controller, msg.Subject)
	}
	if msg.ExpectedVersion != doc.Version {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("stale update: expected v%d, current v%d",
			msg.ExpectedVersion, doc.Version)
	}

	// Clear the old controller-index pairs before mutating doc.Controllers
	// so we never leave dangling (oldController, subject) entries.
	if err := m.ClearControllerIndex(ctx, doc.Controllers, doc.Id); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("clear index: %s", err)
	}

	if len(msg.Controllers) > 0 {
		doc.Controllers = msg.Controllers
	}
	if len(msg.Verification) > 0 {
		doc.Verification = msg.Verification
	}
	doc.Service = msg.Service // service list MAY be cleared; that's OK
	doc.UpdatedAt = ctx.BlockTime().Unix()
	doc.Version++

	params := m.GetParams(ctx)
	if err := types.ValidateDocumentShape(&doc, params); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid document: %s", err)
	}
	if err := m.SetDocument(ctx, doc); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}

	emitDIDEvent(ctx, "did_updated", msg.Subject, msg.Controller, "")
	return &types.MsgUpdateDIDResponse{Version: doc.Version}, nil
}

func (m msgServer) DeactivateDID(goCtx context.Context, msg *types.MsgDeactivateDID) (*types.MsgDeactivateDIDResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	doc, ok := m.GetDocument(ctx, msg.Subject)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("did %s", msg.Subject)
	}
	if doc.Status == types.StatusDeactivated {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already deactivated")
	}
	if !m.IsController(ctx, msg.Subject, msg.Controller) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("not a controller")
	}

	doc.Status = types.StatusDeactivated
	doc.UpdatedAt = ctx.BlockTime().Unix()
	doc.Version++
	if err := m.SetDocument(ctx, doc); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDIDEvent(ctx, "did_deactivated", msg.Subject, msg.Controller, "")
	return &types.MsgDeactivateDIDResponse{}, nil
}

func (m msgServer) RotateKey(goCtx context.Context, msg *types.MsgRotateKey) (*types.MsgRotateKeyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	doc, ok := m.GetDocument(ctx, msg.Subject)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("did %s", msg.Subject)
	}
	if doc.Status != types.StatusActive {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("cannot rotate key on inactive DID")
	}
	if !m.IsController(ctx, msg.Subject, msg.Controller) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("not a controller")
	}

	now := ctx.BlockTime().Unix()
	found := false
	for i, vm := range doc.Verification {
		if vm.Id == msg.OldKeyId {
			if vm.RevokedAt != 0 {
				return nil, sdkerrors.ErrInvalidRequest.Wrapf("key %s already revoked", msg.OldKeyId)
			}
			doc.Verification[i].RevokedAt = now
			found = true
			break
		}
	}
	if !found {
		return nil, sdkerrors.ErrNotFound.Wrapf("verification method %s", msg.OldKeyId)
	}

	newVM := msg.NewKey
	if newVM.CreatedAt == 0 {
		newVM.CreatedAt = now
	}
	if newVM.Controller == "" {
		newVM.Controller = msg.Subject
	}
	for _, existing := range doc.Verification {
		if existing.Id == newVM.Id {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("duplicate key id %s", newVM.Id)
		}
	}
	doc.Verification = append(doc.Verification, newVM)
	doc.UpdatedAt = now
	doc.Version++

	params := m.GetParams(ctx)
	if err := types.ValidateDocumentShape(&doc, params); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid document after rotate: %s", err)
	}
	if err := m.SetDocument(ctx, doc); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDIDEvent(ctx, "did_key_rotated", msg.Subject, msg.Controller, msg.OldKeyId)
	return &types.MsgRotateKeyResponse{Version: doc.Version}, nil
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

func (m msgServer) IssueCredential(goCtx context.Context, msg *types.MsgIssueCredential) (*types.MsgIssueCredentialResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.IsActiveAnchor(ctx, msg.Issuer, msg.Type) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("%s is not a trust anchor for %s", msg.Issuer, msg.Type)
	}
	if !m.IsActive(ctx, msg.Subject) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("subject DID %s is not active", msg.Subject)
	}
	if _, exists := m.GetCredential(ctx, msg.Id); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("credential %s already exists", msg.Id)
	}
	params := m.GetParams(ctx)
	if uint32(len(msg.Hash)) > params.MaxCredentialHashLen {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("hash too long (max %d)", params.MaxCredentialHashLen)
	}

	now := ctx.BlockTime().Unix()
	issuedAt := msg.IssuedAt
	if issuedAt == 0 {
		issuedAt = now
	}
	c := types.CredentialStatus{
		Id:        msg.Id,
		Issuer:    msg.Issuer,
		Subject:   msg.Subject,
		Type:      msg.Type,
		Hash:      msg.Hash,
		Status:    types.CredentialValid,
		IssuedAt:  issuedAt,
		ExpiresAt: msg.ExpiresAt,
	}
	if err := m.SetCredential(ctx, c); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDIDEvent(ctx, "did_credential_issued", msg.Subject, msg.Issuer, msg.Type)
	return &types.MsgIssueCredentialResponse{}, nil
}

func (m msgServer) RevokeCredential(goCtx context.Context, msg *types.MsgRevokeCredential) (*types.MsgRevokeCredentialResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	c, ok := m.GetCredential(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("credential %s", msg.Id)
	}
	if c.Issuer != msg.Issuer {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("only issuer %s may revoke", c.Issuer)
	}
	if c.Status == types.CredentialRevoked {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already revoked")
	}
	if err := m.MarkCredentialRevoked(ctx, c, msg.Reason); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("revoke: %s", err)
	}
	emitDIDEvent(ctx, "did_credential_revoked", c.Subject, msg.Issuer, msg.Reason)
	return &types.MsgRevokeCredentialResponse{}, nil
}

// ---------------------------------------------------------------------------
// Trust anchors (governance only)
// ---------------------------------------------------------------------------

func (m msgServer) RegisterAnchor(goCtx context.Context, msg *types.MsgRegisterAnchor) (*types.MsgRegisterAnchorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	switch msg.Tier {
	case types.AnchorTierRoot:
		if msg.Authority != m.GetAuthority() {
			return nil, sdkerrors.ErrUnauthorized.Wrap("root anchor: only governance authority may register")
		}
	case types.AnchorTierIntermediate:
		// Intermediate must be delegated by an existing active root.
		parent, ok := m.GetAnchor(ctx, msg.ParentDid)
		if !ok || !parent.Active || parent.Tier != types.AnchorTierRoot {
			return nil, sdkerrors.ErrUnauthorized.Wrap("parent must be an active root anchor")
		}
		if msg.Authority != msg.ParentDid {
			return nil, sdkerrors.ErrUnauthorized.Wrap("intermediate anchor: authority must equal parent_did")
		}
	default:
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("unknown tier %q", msg.Tier)
	}
	if _, exists := m.GetAnchor(ctx, msg.Did); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("anchor already registered")
	}

	a := types.TrustAnchor{
		Did:             msg.Did,
		Tier:            msg.Tier,
		ParentDid:       msg.ParentDid,
		CredentialTypes: msg.CredentialTypes,
		Active:          true,
		AddedAt:         ctx.BlockTime().Unix(),
	}
	if err := m.SetAnchor(ctx, a); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDIDEvent(ctx, "did_anchor_registered", msg.Did, msg.Authority, msg.Tier)
	return &types.MsgRegisterAnchorResponse{}, nil
}

func (m msgServer) DeactivateAnchor(goCtx context.Context, msg *types.MsgDeactivateAnchor) (*types.MsgDeactivateAnchorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	a, ok := m.GetAnchor(ctx, msg.Did)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("anchor %s", msg.Did)
	}
	switch a.Tier {
	case types.AnchorTierRoot:
		if msg.Authority != m.GetAuthority() {
			return nil, sdkerrors.ErrUnauthorized.Wrap("root anchor: only governance authority may deactivate")
		}
	case types.AnchorTierIntermediate:
		// Either the parent root or the chain authority can deactivate
		// an intermediate. Lets governance pull the plug on a whole
		// branch in emergencies without tracing each delegation.
		if msg.Authority != a.ParentDid && msg.Authority != m.GetAuthority() {
			return nil, sdkerrors.ErrUnauthorized.Wrap("only parent root or governance may deactivate")
		}
	}
	a.Active = false
	if err := m.SetAnchor(ctx, a); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDIDEvent(ctx, "did_anchor_deactivated", msg.Did, msg.Authority, "")
	return &types.MsgDeactivateAnchorResponse{}, nil
}

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("expected authority %s, got %s", m.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid params: %s", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
