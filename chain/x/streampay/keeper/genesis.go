package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/streampay/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, s := range gs.Streams {
		if err := k.SetStream(ctx, s); err != nil {
			return err
		}
		if err := k.StreamBySender.Set(ctx, collections.Join(s.Sender, s.Id)); err != nil {
			return err
		}
		if err := k.StreamByReceiver.Set(ctx, collections.Join(s.Receiver, s.Id)); err != nil {
			return err
		}
	}
	if gs.NextStreamId > 1 {
		if err := k.IDSeq.Set(ctx, gs.NextStreamId-1); err != nil {
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
	if err := k.Streams.Walk(ctx, nil, func(_ uint64, v types.Stream) (bool, error) {
		gs.Streams = append(gs.Streams, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextStreamId = seq + 1
	return gs, nil
}
