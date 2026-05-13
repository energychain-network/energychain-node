package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/escrow/types"
)

// InitGenesis writes the genesis snapshot into module state.
// Counts on each escrow are taken from the genesis row directly
// (already validated by GenesisState.Validate to match the sum of
// per-signer Approval rows).
func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, e := range gs.Escrows {
		if err := k.SetEscrow(ctx, e); err != nil {
			return err
		}
		if err := k.EscrowByDepositor.Set(ctx, collections.Join(e.Depositor, e.Id)); err != nil {
			return err
		}
		if err := k.EscrowByBeneficiary.Set(ctx, collections.Join(e.Beneficiary, e.Id)); err != nil {
			return err
		}
	}
	for _, a := range gs.Approvals {
		if err := k.SetApproval(ctx, a); err != nil {
			return err
		}
	}
	if gs.NextEscrowId > 1 {
		if err := k.IDSeq.Set(ctx, gs.NextEscrowId-1); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: p}

	if err := k.Escrows.Walk(ctx, nil, func(_ uint64, v types.Escrow) (bool, error) {
		gs.Escrows = append(gs.Escrows, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Approvals.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.Approval) (bool, error) {
		gs.Approvals = append(gs.Approvals, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextEscrowId = seq + 1
	return gs, nil
}
