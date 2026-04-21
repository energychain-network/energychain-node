package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/types"
)

type msgServer struct {
	keeper Keeper
}

var _ types.MsgServer = &msgServer{}

func NewMsgServerImpl(keeper Keeper) *msgServer {
	return &msgServer{keeper: keeper}
}

func (m *msgServer) SubmitData(goCtx context.Context, msg *types.MsgSubmitData) (*types.MsgSubmitDataResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.keeper.IsAuthorizedOracle(ctx, msg.Submitter, msg.Category) {
		return nil, fmt.Errorf("address %s is not an authorized oracle for category %s",
			msg.Submitter, msg.Category)
	}

	params := m.keeper.GetParams(ctx)
	if params.MaxMetadataSize > 0 && uint32(len(msg.Metadata)) > params.MaxMetadataSize {
		return nil, fmt.Errorf("metadata size %d exceeds maximum %d", len(msg.Metadata), params.MaxMetadataSize)
	}

	blockTime := ctx.BlockTime().Unix()
	maxDrift := params.DataMaxAge
	if maxDrift <= 0 {
		maxDrift = 3600
	}
	if msg.Timestamp > blockTime+maxDrift {
		return nil, fmt.Errorf("timestamp %d is too far in the future (block time %d, max drift %ds)",
			msg.Timestamp, blockTime, maxDrift)
	}
	if msg.Timestamp < blockTime-maxDrift {
		return nil, fmt.Errorf("timestamp %d is too far in the past (block time %d, max drift %ds)",
			msg.Timestamp, blockTime, maxDrift)
	}

	data := types.OracleData{
		Category:    msg.Category,
		Value:       msg.Value,
		Metadata:    msg.Metadata,
		Timestamp:   msg.Timestamp,
		Submitter:   msg.Submitter,
		BlockHeight: ctx.BlockHeight(),
	}

	if err := m.keeper.SetOracleData(ctx, data); err != nil {
		return nil, fmt.Errorf("storing oracle data: %w", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_data_submitted",
		sdk.NewAttribute("submitter", msg.Submitter),
		sdk.NewAttribute("category", msg.Category),
		sdk.NewAttribute("value", msg.Value),
		sdk.NewAttribute("metadata", msg.Metadata),
		sdk.NewAttribute("timestamp", fmt.Sprintf("%d", msg.Timestamp)),
	))

	return &types.MsgSubmitDataResponse{}, nil
}

func (m *msgServer) AddOracle(goCtx context.Context, msg *types.MsgAddOracle) (*types.MsgAddOracleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	oracle := types.OracleInfo{
		Address:              msg.OracleAddress,
		Name:                 msg.Name,
		Active:               true,
		AuthorizedCategories: msg.AuthorizedCategories,
	}

	if err := m.keeper.AddOracle(ctx, oracle); err != nil {
		return nil, fmt.Errorf("storing oracle: %w", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_added",
		sdk.NewAttribute("address", msg.OracleAddress),
		sdk.NewAttribute("name", msg.Name),
	))

	return &types.MsgAddOracleResponse{}, nil
}

func (m *msgServer) RemoveOracle(goCtx context.Context, msg *types.MsgRemoveOracle) (*types.MsgRemoveOracleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	if _, found := m.keeper.GetOracle(ctx, msg.OracleAddress); !found {
		return nil, fmt.Errorf("oracle not found: %s", msg.OracleAddress)
	}

	m.keeper.RemoveOracle(ctx, msg.OracleAddress)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_removed",
		sdk.NewAttribute("address", msg.OracleAddress),
	))

	return &types.MsgRemoveOracleResponse{}, nil
}

// UpdateParams replaces the entire oracle module Params. Authority is the
// gov module address; any other signer is rejected.
func (m *msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("invalid authority: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.keeper.SetParams(ctx, msg.Params); err != nil {
		return nil, fmt.Errorf("setting params: %w", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
