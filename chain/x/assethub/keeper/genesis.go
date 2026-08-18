package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/assethub/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, p := range gs.Providers {
		if err := k.Providers.Set(ctx, p.Address, p); err != nil {
			return err
		}
	}
	for _, d := range gs.Devices {
		if err := k.Devices.Set(ctx, d.Id, d); err != nil {
			return err
		}
		if err := k.DeviceByOperator.Set(ctx, collections.Join(d.Operator, d.Id)); err != nil {
			return err
		}
	}
	for _, r := range gs.Readings {
		if err := k.Readings.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.ReadingByDevice.Set(ctx, collections.Join(r.DeviceId, r.Id)); err != nil {
			return err
		}
	}
	for _, tpc := range gs.Topics {
		if err := k.Topics.Set(ctx, tpc.Id, tpc); err != nil {
			return err
		}
	}
	for _, s := range gs.Submissions {
		if err := k.Submissions.Set(ctx, collections.Join(s.TopicId, s.Provider), s); err != nil {
			return err
		}
	}
	if err := k.ReadingIDSeq.Set(ctx, gs.ReadingIdSeq); err != nil {
		return err
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Providers.Walk(ctx, nil, func(_ string, v types.Provider) (bool, error) {
		gs.Providers = append(gs.Providers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Devices.Walk(ctx, nil, func(_ string, v types.Device) (bool, error) {
		gs.Devices = append(gs.Devices, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Readings.Walk(ctx, nil, func(_ uint64, v types.MeteringReading) (bool, error) {
		gs.Readings = append(gs.Readings, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Topics.Walk(ctx, nil, func(_ string, v types.OracleTopic) (bool, error) {
		gs.Topics = append(gs.Topics, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Submissions.Walk(ctx, nil, func(_ collections.Pair[string, string], v types.OracleSubmission) (bool, error) {
		gs.Submissions = append(gs.Submissions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.ReadingIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.ReadingIdSeq = seq
	return gs, nil
}
