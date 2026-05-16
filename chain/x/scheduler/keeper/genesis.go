package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/scheduler/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, j := range gs.Jobs {
		if err := k.Jobs.Set(ctx, j.Id, j); err != nil {
			return err
		}
		if err := k.JobByOwner.Set(ctx, collections.Join(j.Owner, j.Id)); err != nil {
			return err
		}
		if j.Status == types.Status_STATUS_ACTIVE {
			if err := k.JobByNextRun.Set(ctx, collections.Join(j.NextRunTime, j.Id)); err != nil {
				return err
			}
		}
	}
	if gs.NextJobId > 1 {
		if err := k.IDSeq.Set(ctx, gs.NextJobId-1); err != nil {
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
	if err := k.Jobs.Walk(ctx, nil, func(_ uint64, v types.Job) (bool, error) {
		gs.Jobs = append(gs.Jobs, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextJobId = seq + 1
	return gs, nil
}
