package keeper

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/audit/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// isAllowedAuditor returns true when the address may record audit logs.
// Permissionless mode lets any address through; otherwise the address must
// appear in AllowedAuditors (empty list => deny all).
func (m msgServer) isAllowedAuditor(ctx sdk.Context, address string) bool {
	params := m.GetParams(ctx)
	if params.Permissionless {
		return true
	}
	for _, allowed := range params.AllowedAuditors {
		if allowed == address {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// RecordAudit
// ---------------------------------------------------------------------------

func (m msgServer) RecordAudit(goCtx context.Context, msg *types.MsgRecordAudit) (*types.MsgRecordAuditResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.isAllowedAuditor(ctx, msg.Creator) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("address %s is not an allowed auditor", msg.Creator)
	}

	params := m.GetParams(ctx)
	if params.MaxDataSize > 0 && uint32(len(msg.Data)) > params.MaxDataSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("data size %d exceeds maximum %d",
			len(msg.Data), params.MaxDataSize)
	}

	// Schema gating: when the writer claims a schema_id, it must point at
	// a registered, non-deprecated SchemaDescriptor. This is what makes
	// the on-chain log self-describing — without it the regulator has to
	// guess what the bytes meant. Empty schema_id remains legal so ad-hoc
	// events can flow without governance pre-approval.
	if msg.SchemaId != "" {
		d, ok := m.GetSchema(ctx, msg.SchemaId)
		if !ok {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("schema_id %q not registered", msg.SchemaId)
		}
		if d.Deprecated {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("schema_id %q deprecated since %d",
				msg.SchemaId, d.DeprecatedAt)
		}
	}

	if err := m.CheckAndIncrAuditCount(ctx, msg.Creator); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}

	id := m.GetNextID(ctx)

	// TxHash records the SHA-256 of the raw tx bytes for this message, which
	// CometBFT/Cosmos SDK also use as the canonical tx hash. This deliberately
	// does NOT match the EVM tx hash (keccak256 of the RLP-encoded ETH tx)
	// because audit entries are produced from native Cosmos messages.
	txHash := ""
	if txBytes := ctx.TxBytes(); len(txBytes) > 0 {
		h := sha256.Sum256(txBytes)
		txHash = fmt.Sprintf("%X", h[:])
	}

	log := types.AuditLog{
		ID:               id,
		EventType:        msg.EventType,
		Actor:            msg.Creator,
		Target:           msg.Target,
		Action:           msg.Action,
		Data:             msg.Data,
		BlockHeight:      ctx.BlockHeight(),
		Timestamp:        ctx.BlockTime().Unix(),
		TxHash:           txHash,
		Severity:         msg.Severity,
		SchemaId:         msg.SchemaId,
		PayloadEncrypted: msg.PayloadEncrypted,
		PayloadDigest:    msg.PayloadDigest,
	}

	if err := m.RecordAuditLog(ctx, log); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("storing audit log: %s", err)
	}
	m.IncrementCounter(ctx, id)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_recorded",
		sdk.NewAttribute("id", fmt.Sprintf("%d", id)),
		sdk.NewAttribute("event_type", msg.EventType),
		sdk.NewAttribute("actor", msg.Creator),
		sdk.NewAttribute("target", msg.Target),
		sdk.NewAttribute("action", msg.Action),
		sdk.NewAttribute("severity", msg.Severity.String()),
		sdk.NewAttribute("schema_id", msg.SchemaId),
		sdk.NewAttribute("encrypted", fmt.Sprintf("%t", msg.PayloadEncrypted)),
	))

	return &types.MsgRecordAuditResponse{ID: id}, nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("expected authority %s, got %s", m.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid params: %s", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("setting params: %s", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---------------------------------------------------------------------------
// Schema registry
// ---------------------------------------------------------------------------

func (m msgServer) RegisterSchema(goCtx context.Context, msg *types.MsgRegisterSchema) (*types.MsgRegisterSchemaResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	d := msg.Descriptor_
	d.RegisteredBy = msg.Authority
	d.RegisteredAt = ctx.BlockTime().Unix()

	// Bump version automatically if a prior descriptor exists. The caller
	// MAY supply an explicit version (for genesis-driven re-registration);
	// otherwise we increment from the prior on-chain value. This keeps
	// version numbers monotonic without requiring callers to know the
	// current state.
	prev, exists := m.GetSchema(ctx, d.EventType)
	if exists {
		if d.Version == 0 {
			d.Version = prev.Version + 1
		}
		if d.Version <= prev.Version {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("version must exceed current %d", prev.Version)
		}
		// Preserve the deprecation status: re-registering a deprecated
		// schema un-deprecates it (governance signalled they want it
		// active again with new content).
		d.Deprecated = false
		d.DeprecatedAt = 0
	} else if d.Version == 0 {
		d.Version = 1
	}

	if err := m.SetSchema(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store schema: %s", err)
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_schema_registered",
		sdk.NewAttribute("event_type", d.EventType),
		sdk.NewAttribute("version", fmt.Sprintf("%d", d.Version)),
		sdk.NewAttribute("uri", d.Uri),
		sdk.NewAttribute("hash", d.Hash),
	))
	return &types.MsgRegisterSchemaResponse{}, nil
}

func (m msgServer) DeprecateSchema(goCtx context.Context, msg *types.MsgDeprecateSchema) (*types.MsgDeprecateSchemaResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := m.GetSchema(ctx, msg.EventType)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("schema %s", msg.EventType)
	}
	if d.Deprecated {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already deprecated")
	}
	d.Deprecated = true
	d.DeprecatedAt = ctx.BlockTime().Unix()
	if err := m.SetSchema(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_schema_deprecated",
		sdk.NewAttribute("event_type", d.EventType),
	))
	return &types.MsgDeprecateSchemaResponse{}, nil
}

// ---------------------------------------------------------------------------
// Archive
// ---------------------------------------------------------------------------

// Archive seals a contiguous span of audit logs into an off-chain batch
// and removes the originals from the live store. The block proof is two
// pieces of state: an ArchiveSegment record (immutable) and the absence
// of the originals (verifiable by querying the segment range).
func (m msgServer) Archive(goCtx context.Context, msg *types.MsgArchive) (*types.MsgArchiveResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if !m.IsArchiveAuthority(ctx, msg.Authority) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("not an archive authority")
	}

	params := m.GetParams(ctx)
	currentHeight := ctx.BlockHeight()

	// SECURITY: cap the requested range so a rogue (or fat-fingered)
	// authority cannot ask for [1, uint64.max] and force the validation
	// + eviction loops into 2^64 iterations. The eviction itself is
	// already capped, but the pre-flight walk would still spin the CPU
	// without this bound.
	span := msg.ToLogId - msg.FromLogId + 1
	if span > uint64(params.ArchiveMaxBatch) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"requested span %d exceeds archive_max_batch %d",
			span, params.ArchiveMaxBatch)
	}

	// Ensure the entire requested range is old enough. We check against
	// the highest log id (max BlockHeight ≈ now), so any log whose
	// BlockHeight is closer than ArchiveMinAgeBlocks is rejected.
	for id := msg.FromLogId; id <= msg.ToLogId; id++ {
		log, ok := m.GetAuditLog(ctx, id)
		if !ok {
			continue
		}
		if currentHeight-log.BlockHeight < params.ArchiveMinAgeBlocks {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf(
				"log %d too young (height %d, current %d, min age %d)",
				id, log.BlockHeight, currentHeight, params.ArchiveMinAgeBlocks)
		}
	}

	// Compute earliest / latest timestamps over the surviving rows; lets
	// regulators bound queries against the archive without re-fetching
	// the off-chain payload.
	earliest := int64(0)
	latest := int64(0)
	for id := msg.FromLogId; id <= msg.ToLogId; id++ {
		log, ok := m.GetAuditLog(ctx, id)
		if !ok {
			continue
		}
		if earliest == 0 || log.Timestamp < earliest {
			earliest = log.Timestamp
		}
		if log.Timestamp > latest {
			latest = log.Timestamp
		}
	}

	evicted, err := m.EvictLogsForArchive(ctx, msg.FromLogId, msg.ToLogId, params.ArchiveMaxBatch)
	if err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("evict: %s", err)
	}
	if evicted == 0 {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("no logs in requested range")
	}

	algo := strings.TrimSpace(msg.MerkleAlgorithm)
	if algo == "" {
		algo = types.MerkleAlgorithmSHA256
	}

	segID := m.NextArchiveID(ctx)
	segment := types.ArchiveSegment{
		ID:                segID,
		FromLogId:         msg.FromLogId,
		ToLogId:           msg.ToLogId,
		LogCount:          evicted,
		MerkleRoot:        msg.MerkleRoot,
		MerkleAlgorithm:   algo,
		Uri:               msg.Uri,
		ArchivedBy:        msg.Authority,
		ArchivedAt:        ctx.BlockTime().Unix(),
		EarliestTimestamp: earliest,
		LatestTimestamp:   latest,
	}
	if err := m.SetArchiveSegment(ctx, segment); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store segment: %s", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_archived",
		sdk.NewAttribute("segment_id", fmt.Sprintf("%d", segID)),
		sdk.NewAttribute("from_log_id", fmt.Sprintf("%d", msg.FromLogId)),
		sdk.NewAttribute("to_log_id", fmt.Sprintf("%d", msg.ToLogId)),
		sdk.NewAttribute("evicted", fmt.Sprintf("%d", evicted)),
		sdk.NewAttribute("merkle_root", msg.MerkleRoot),
		sdk.NewAttribute("uri", msg.Uri),
	))
	return &types.MsgArchiveResponse{SegmentId: segID, Evicted: evicted}, nil
}

// ---------------------------------------------------------------------------
// View-key grants
// ---------------------------------------------------------------------------

func (m msgServer) GrantViewKey(goCtx context.Context, msg *types.MsgGrantViewKey) (*types.MsgGrantViewKeyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// SECURITY: refuse to issue grants to addresses without an active
	// DID. Without this check, governance could (accidentally or not)
	// hand the encrypted key to an address with no on-chain identity to
	// hold it accountable.
	if !m.CheckGranteeHasDID(ctx, msg.Grantee) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("grantee %s has no active DID", msg.Grantee)
	}

	now := ctx.BlockTime().Unix()
	params := m.GetParams(ctx)

	expires := msg.ExpiresAt
	if params.ViewKeyMaxValidity > 0 && expires > 0 {
		// Cap the expires_at to the grantor's policy ceiling.
		maxExpires := now + params.ViewKeyMaxValidity
		if expires > maxExpires {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf(
				"expires_at %d exceeds policy ceiling %d (now+%d)",
				expires, maxExpires, params.ViewKeyMaxValidity)
		}
	}
	if expires > 0 && expires <= now {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("expires_at must be in the future")
	}

	g := types.ViewKeyGrant{
		Id:           m.NextGrantID(ctx),
		Grantor:      msg.Grantor,
		Grantee:      msg.Grantee,
		Scope:        msg.Scope,
		EncryptedKey: msg.EncryptedKey,
		GrantedAt:    now,
		ExpiresAt:    expires,
	}
	if err := m.SetGrant(ctx, g); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_view_key_granted",
		sdk.NewAttribute("grant_id", g.Id),
		sdk.NewAttribute("grantor", msg.Grantor),
		sdk.NewAttribute("grantee", msg.Grantee),
		sdk.NewAttribute("expires_at", fmt.Sprintf("%d", expires)),
	))
	return &types.MsgGrantViewKeyResponse{GrantId: g.Id}, nil
}

func (m msgServer) RevokeViewKey(goCtx context.Context, msg *types.MsgRevokeViewKey) (*types.MsgRevokeViewKeyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	g, ok := m.GetGrant(ctx, msg.GrantId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("grant %s", msg.GrantId)
	}
	// Either the original grantor or chain governance can revoke. We
	// allow governance because compromised grantor keys must not lock
	// the chain out of revoking the leak.
	if msg.Actor != g.Grantor && msg.Actor != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only grantor or governance may revoke")
	}
	if g.Revoked {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already revoked")
	}
	if err := m.MarkGrantRevoked(ctx, g, msg.Reason); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("revoke: %s", err)
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_view_key_revoked",
		sdk.NewAttribute("grant_id", g.Id),
		sdk.NewAttribute("actor", msg.Actor),
		sdk.NewAttribute("reason", msg.Reason),
	))
	return &types.MsgRevokeViewKeyResponse{}, nil
}
