package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/internal/commitment"
	"energychain/x/meter/types"
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
// Metering point lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterMeteringPoint(goCtx context.Context, msg *types.MsgRegisterMeteringPoint) (*types.MsgRegisterMeteringPointResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if m.HasMeteringPoint(ctx, msg.Id) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metering_point %s already registered", msg.Id)
	}
	if msg.Submitter != msg.OwnerAddress {
		// We deliberately require submitter == owner at registration so
		// the on-chain audit trail unambiguously names the entity that
		// brought the meter into existence. Delegated registrations
		// must use governance.
		return nil, sdkerrors.ErrUnauthorized.Wrap("submitter must equal owner_address")
	}
	if err := m.requireOwnerHasDID(ctx, msg.OwnerAddress); err != nil {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("owner DID gate: %s", err)
	}
	params := m.GetParams(ctx)
	if uint32(len(msg.MetadataUri)) > params.MaxMetadataUriSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metadata_uri exceeds %d", params.MaxMetadataUriSize)
	}

	now := ctx.BlockTime().Unix()
	mp := types.MeteringPoint{
		Id:              msg.Id,
		DeviceDid:       msg.DeviceDid,
		OwnerAddress:    msg.OwnerAddress,
		GridZone:        msg.GridZone,
		GridOperator:    msg.GridOperator,
		Tariff:          msg.Tariff,
		MeasurementType: msg.MeasurementType,
		Unit:            msg.Unit,
		Timezone:        msg.Timezone,
		MetadataUri:     msg.MetadataUri,
		Active:          true,
		RegisteredAt:    now,
		UpdatedAt:       now,
	}
	if err := m.SetMeteringPoint(ctx, mp); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "meter_point_registered",
		"id", mp.Id, "owner", mp.OwnerAddress, "device", mp.DeviceDid,
		"zone", mp.GridZone, "type", mp.MeasurementType.String())
	return &types.MsgRegisterMeteringPointResponse{}, nil
}

func (m msgServer) UpdateMeteringPoint(goCtx context.Context, msg *types.MsgUpdateMeteringPoint) (*types.MsgUpdateMeteringPointResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	prior, ok := m.GetMeteringPoint(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s", msg.Id)
	}
	if msg.Submitter != prior.OwnerAddress && msg.Submitter != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner or governance may update")
	}
	if !prior.Active && msg.Submitter != m.GetAuthority() {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("metering_point inactive; only governance may update")
	}
	params := m.GetParams(ctx)
	if uint32(len(msg.MetadataUri)) > params.MaxMetadataUriSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metadata_uri exceeds %d", params.MaxMetadataUriSize)
	}

	next := prior
	if msg.GridZone != "" {
		next.GridZone = msg.GridZone
	}
	if msg.GridOperator != "" {
		next.GridOperator = msg.GridOperator
	}
	if msg.Tariff != "" {
		next.Tariff = msg.Tariff
	}
	if msg.Unit != "" {
		next.Unit = msg.Unit
	}
	if msg.Timezone != "" {
		next.Timezone = msg.Timezone
	}
	if msg.MetadataUri != "" {
		next.MetadataUri = msg.MetadataUri
	}
	next.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.updateMeteringPoint(ctx, prior, next); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("update: %s", err)
	}
	emit(ctx, "meter_point_updated", "id", next.Id)
	return &types.MsgUpdateMeteringPointResponse{}, nil
}

func (m msgServer) DeactivateMeteringPoint(goCtx context.Context, msg *types.MsgDeactivateMeteringPoint) (*types.MsgDeactivateMeteringPointResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	prior, ok := m.GetMeteringPoint(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s", msg.Id)
	}
	if msg.Submitter != prior.OwnerAddress && msg.Submitter != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner or governance may deactivate")
	}
	if !prior.Active {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already inactive")
	}
	next := prior
	next.Active = false
	next.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.updateMeteringPoint(ctx, prior, next); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("deactivate: %s", err)
	}
	emit(ctx, "meter_point_deactivated", "id", next.Id, "reason", msg.Reason)
	return &types.MsgDeactivateMeteringPointResponse{}, nil
}

// ---------------------------------------------------------------------------
// Readings
// ---------------------------------------------------------------------------

func (m msgServer) SubmitReading(goCtx context.Context, msg *types.MsgSubmitReading) (*types.MsgSubmitReadingResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	mp, ok := m.GetMeteringPoint(ctx, msg.MeteringPointId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s", msg.MeteringPointId)
	}
	if !mp.Active {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("metering_point inactive")
	}
	if !m.isAuthorizedSubmitter(ctx, mp, msg.Submitter) {
		return nil, sdkerrors.ErrUnauthorized.Wrap(
			"submitter is neither the owner nor an active stream-authorised delegate")
	}
	if err := m.requireAttestedDevice(ctx, mp.DeviceDid); err != nil {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("attestation gate: %s", err)
	}

	params := m.GetParams(ctx)
	if uint32(len(msg.CommitmentBytes)) == 0 || uint32(len(msg.CommitmentBytes)) > params.MaxCommitmentSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("commitment_bytes length out of range (0, %d]", params.MaxCommitmentSize)
	}
	if uint32(len(msg.DeviceSignature)) > params.MaxSignatureSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("device_signature exceeds %d", params.MaxSignatureSize)
	}

	// Defense-in-depth: re-validate the period invariant in the keeper.
	// ValidateBasic catches it earlier in the antehandler path, but msg
	// servers SHOULD NOT trust that path exclusively because alternative
	// invocation routes (genesis, internal tooling, IBC bridges) may
	// skip ValidateBasic.
	period, ok := types.ReadingPeriodSeconds(msg.Period)
	if !ok {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("invalid period")
	}
	if msg.StartTime <= 0 || msg.EndTime <= 0 {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("non-positive timestamps")
	}
	if msg.EndTime-msg.StartTime != period {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"end_time-start_time (%d) must equal period seconds (%d)",
			msg.EndTime-msg.StartTime, period)
	}

	now := ctx.BlockTime().Unix()
	if params.FutureSkewSeconds > 0 && msg.StartTime > now+params.FutureSkewSeconds {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("start_time too far in the future")
	}

	// Structurally validate the commitment bytes against the named scheme.
	// SchemePedersenSecp256k1 verification is intentionally NOT performed
	// here — the chain only enforces the wire shape; binding (i.e. that
	// the commitment opens to a real value) is checked off-chain by view-
	// key holders.
	cm := commitment.Commitment{
		Scheme: types.CommitmentSchemeToInternal(msg.CommitmentScheme),
		Domain: commitmentDomain(mp.Id),
		Value:  msg.CommitmentBytes,
	}
	if err := cm.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("commitment: %s", err)
	}

	// Idempotency: re-submitting the exact (mp, start_time) bucket is
	// rejected outright. A correction must arrive as a new reading with
	// data_quality=ESTIMATED on a different bucket, or as a governance-
	// driven rewrite later.
	if _, exists := m.GetReading(ctx, msg.MeteringPointId, msg.StartTime); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"reading already exists for (%s, %d)", msg.MeteringPointId, msg.StartTime)
	}

	r := types.Reading{
		MeteringPointId:  msg.MeteringPointId,
		Period:           msg.Period,
		StartTime:        msg.StartTime,
		EndTime:          msg.EndTime,
		CommitmentScheme: msg.CommitmentScheme,
		CommitmentBytes:  msg.CommitmentBytes,
		PlaintextValue:   msg.PlaintextValue,
		DeviceSignature:  msg.DeviceSignature,
		DataQuality:      msg.DataQuality,
		SubmittedBy:      msg.Submitter,
		SubmittedAt:      now,
		Height:           ctx.BlockHeight(),
	}
	if err := m.SetReading(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "meter_reading_submitted",
		"mp", mp.Id, "start", fmt.Sprintf("%d", r.StartTime),
		"period", r.Period.String(), "scheme", r.CommitmentScheme.String(),
		"quality", r.DataQuality.String(), "by", msg.Submitter)
	return &types.MsgSubmitReadingResponse{}, nil
}

// ---------------------------------------------------------------------------
// Batches
// ---------------------------------------------------------------------------

func (m msgServer) SubmitBatch(goCtx context.Context, msg *types.MsgSubmitBatch) (*types.MsgSubmitBatchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	mp, ok := m.GetMeteringPoint(ctx, msg.MeteringPointId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s", msg.MeteringPointId)
	}
	if !mp.Active {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("metering_point inactive")
	}
	if !m.isAuthorizedSubmitter(ctx, mp, msg.Submitter) {
		return nil, sdkerrors.ErrUnauthorized.Wrap(
			"submitter is neither the owner nor an active stream-authorised delegate")
	}
	if err := m.requireAttestedDevice(ctx, mp.DeviceDid); err != nil {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("attestation gate: %s", err)
	}

	params := m.GetParams(ctx)
	if msg.Count == 0 {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("count must be > 0")
	}
	if msg.Count > params.MaxBatchCount {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("count %d exceeds %d",
			msg.Count, params.MaxBatchCount)
	}
	if uint32(len(msg.DeviceSignature)) > params.MaxSignatureSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("device_signature exceeds %d", params.MaxSignatureSize)
	}
	if msg.MerkleAlgorithm != types.MerkleAlgoSHA256 {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("merkle_algorithm %q unsupported", msg.MerkleAlgorithm)
	}
	if len(msg.MerkleRoot) != types.MerkleHashHexLen {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("merkle_root must be %d hex chars", types.MerkleHashHexLen)
	}
	if _, err := hex.DecodeString(msg.MerkleRoot); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("merkle_root must be hex")
	}
	if _, ok := types.ReadingPeriodSeconds(msg.Period); !ok {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("invalid period")
	}
	if msg.StartTime <= 0 || msg.EndTime <= 0 || msg.StartTime > msg.EndTime {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("invalid time range")
	}
	now := ctx.BlockTime().Unix()
	if params.FutureSkewSeconds > 0 && msg.StartTime > now+params.FutureSkewSeconds {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("start_time too far in the future")
	}

	id := deriveBatchID(msg.Submitter, msg.MeteringPointId, msg.MerkleRoot, ctx.BlockHeight())
	if _, exists := m.GetBatch(ctx, id); exists {
		// Genuine retries (same submitter, same root, same height) hit
		// this branch and we treat them as idempotent success — repeated
		// network attempts shouldn't fail noisy.
		return &types.MsgSubmitBatchResponse{BatchId: id}, nil
	}

	b := types.Batch{
		Id:              id,
		MeteringPointId: msg.MeteringPointId,
		MerkleRoot:      msg.MerkleRoot,
		MerkleAlgorithm: msg.MerkleAlgorithm,
		Count:           msg.Count,
		Period:          msg.Period,
		StartTime:       msg.StartTime,
		EndTime:         msg.EndTime,
		Uri:             msg.Uri,
		DeviceSignature: msg.DeviceSignature,
		SubmittedBy:     msg.Submitter,
		SubmittedAt:     now,
		Height:          ctx.BlockHeight(),
	}
	if err := m.SetBatch(ctx, b); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "meter_batch_submitted",
		"id", b.Id, "mp", mp.Id, "merkle_root", b.MerkleRoot,
		"count", fmt.Sprintf("%d", b.Count), "uri", b.Uri)
	return &types.MsgSubmitBatchResponse{BatchId: id}, nil
}

// ---------------------------------------------------------------------------
// Stream authorisation
// ---------------------------------------------------------------------------

func (m msgServer) AuthorizeStream(goCtx context.Context, msg *types.MsgAuthorizeStream) (*types.MsgAuthorizeStreamResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	mp, ok := m.GetMeteringPoint(ctx, msg.MeteringPointId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s", msg.MeteringPointId)
	}
	if msg.Authorizer != mp.OwnerAddress && msg.Authorizer != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner or governance may authorise streams")
	}
	if !mp.Active {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("metering_point inactive")
	}

	now := ctx.BlockTime().Unix()
	if msg.ExpiresAt <= now {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("expires_at must be in the future")
	}
	params := m.GetParams(ctx)
	if params.StreamMaxValidity > 0 && msg.ExpiresAt-now > params.StreamMaxValidity {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"validity window %d exceeds max %d seconds",
			msg.ExpiresAt-now, params.StreamMaxValidity)
	}

	seq, err := m.NextStreamID(ctx)
	if err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("seq: %s", err)
	}
	a := types.StreamAuthorization{
		Id:              formatStreamID(seq),
		MeteringPointId: msg.MeteringPointId,
		DeviceDid:       msg.DeviceDid,
		AuthorizedBy:    msg.Authorizer,
		IssuedAt:        now,
		ExpiresAt:       msg.ExpiresAt,
		Metadata:        msg.Metadata,
	}
	if err := m.SetStreamAuth(ctx, a); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "meter_stream_authorized",
		"id", a.Id, "mp", mp.Id, "device", a.DeviceDid,
		"by", a.AuthorizedBy, "expires_at", fmt.Sprintf("%d", a.ExpiresAt))
	return &types.MsgAuthorizeStreamResponse{Id: a.Id}, nil
}

func (m msgServer) RevokeStream(goCtx context.Context, msg *types.MsgRevokeStream) (*types.MsgRevokeStreamResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, ok := m.GetStreamAuth(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("stream %s", msg.Id)
	}
	if a.Revoked {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already revoked")
	}
	mp, ok := m.GetMeteringPoint(ctx, a.MeteringPointId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("metering_point %s referenced by stream", a.MeteringPointId)
	}
	if msg.Authorizer != a.AuthorizedBy && msg.Authorizer != mp.OwnerAddress && msg.Authorizer != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only original authoriser, current owner, or governance may revoke")
	}
	if err := m.MarkStreamRevoked(ctx, a, msg.Reason); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("revoke: %s", err)
	}
	emit(ctx, "meter_stream_revoked", "id", a.Id, "by", msg.Authorizer, "reason", msg.Reason)
	return &types.MsgRevokeStreamResponse{}, nil
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

// commitmentDomain mirrors the convention in internal/commitment so an
// off-chain verifier can rebuild the same domain string from the on-
// chain row alone.
func commitmentDomain(mpID string) string {
	return "meter.read.v1:" + mpID
}

// deriveBatchID hashes (submitter, mp, merkle_root, height) into a
// 16-hex-char id. Stable for genuine retries (same inputs → same id) and
// collision-resistant across distinct submitters because the submitter
// is part of the digest.
func deriveBatchID(submitter, mpID, merkleRoot string, height int64) string {
	h := sha256.New()
	h.Write([]byte(submitter))
	h.Write([]byte{0})
	h.Write([]byte(mpID))
	h.Write([]byte{0})
	h.Write([]byte(merkleRoot))
	var hb [8]byte
	binary.BigEndian.PutUint64(hb[:], uint64(height))
	h.Write(hb[:])
	return "batch/" + hex.EncodeToString(h.Sum(nil)[:8])
}

// isAuthorizedSubmitter is true when submitter matches the MP owner or
// when the submitter address equals an active (non-revoked, non-expired)
// stream authorisation's device_did. Today the device_did is stored as a
// string match; once x/did exposes a "resolve verification method to
// bech32 controller" hook this can resolve through a DID Document.
func (k Keeper) isAuthorizedSubmitter(ctx sdk.Context, mp types.MeteringPoint, submitter string) bool {
	if submitter == mp.OwnerAddress {
		return true
	}
	now := ctx.BlockTime().Unix()
	authorized := false
	rng := streamRangeForMP(mp.Id)
	_ = k.StreamByMP.Walk(ctx, rng, func(key streamMPKey) (bool, error) {
		a, ok := k.GetStreamAuth(ctx, key.K2())
		if !ok {
			return false, nil
		}
		if a.Revoked {
			return false, nil
		}
		if a.ExpiresAt > 0 && a.ExpiresAt <= now {
			return false, nil
		}
		// Two ways the stream can authorise the submitter:
		//   1. submitter address literally equals the device DID
		//      (simple integration: device-controlled keypair = bech32).
		//   2. submitter equals the original authoriser (owner-delegated
		//      pattern where owner gives a sidecar account streaming
		//      rights for this MP).
		if submitter == a.DeviceDid || submitter == a.AuthorizedBy {
			authorized = true
			return true, nil
		}
		return false, nil
	})
	return authorized
}
