package keeper

import (
	"context"
	"crypto/sha256"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

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

func (m msgServer) RecordAudit(goCtx context.Context, msg *types.MsgRecordAudit) (*types.MsgRecordAuditResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.isAllowedAuditor(ctx, msg.Creator) {
		return nil, fmt.Errorf("address %s is not an allowed auditor", msg.Creator)
	}

	params := m.GetParams(ctx)
	if params.MaxDataSize > 0 && uint32(len(msg.Data)) > params.MaxDataSize {
		return nil, fmt.Errorf("data size %d exceeds maximum %d", len(msg.Data), params.MaxDataSize)
	}

	if err := m.CheckAndIncrAuditCount(ctx, msg.Creator); err != nil {
		return nil, err
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
		ID:          id,
		EventType:   msg.EventType,
		Actor:       msg.Creator,
		Target:      msg.Target,
		Action:      msg.Action,
		Data:        msg.Data,
		BlockHeight: ctx.BlockHeight(),
		Timestamp:   ctx.BlockTime().Unix(),
		TxHash:      txHash,
	}

	if err := m.RecordAuditLog(ctx, log); err != nil {
		return nil, fmt.Errorf("storing audit log: %w", err)
	}
	m.IncrementCounter(ctx, id)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"audit_recorded",
		sdk.NewAttribute("id", fmt.Sprintf("%d", id)),
		sdk.NewAttribute("event_type", msg.EventType),
		sdk.NewAttribute("actor", msg.Creator),
		sdk.NewAttribute("target", msg.Target),
		sdk.NewAttribute("action", msg.Action),
		sdk.NewAttribute("data", msg.Data),
	))

	return &types.MsgRecordAuditResponse{}, nil
}

// UpdateParams replaces the entire audit module Params. Authority is the
// gov module address; any other signer is rejected.
func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, fmt.Errorf("invalid authority: expected %s, got %s", m.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, fmt.Errorf("setting params: %w", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
