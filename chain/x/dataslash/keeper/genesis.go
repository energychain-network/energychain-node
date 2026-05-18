package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/dataslash/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	// Seed each sequence one *behind* the genesis "next id"
	// so that NextXxxID() returns the genesis value on first
	// call (since k.next does Peek-then-Next, returning v+1
	// after persisting v).
	if err := setSeq(ctx, k.ProviderIDSeq, gs.NextProviderId); err != nil {
		return err
	}
	if err := setSeq(ctx, k.InfractionIDSeq, gs.NextInfractionId); err != nil {
		return err
	}
	if err := setSeq(ctx, k.JailRecordIDSeq, gs.NextJailRecordId); err != nil {
		return err
	}
	if err := setSeq(ctx, k.BanRecordIDSeq, gs.NextBanRecordId); err != nil {
		return err
	}
	for _, p := range gs.Providers {
		if err := k.SetProvider(ctx, p); err != nil {
			return err
		}
		if err := k.ProviderByDID.Set(ctx, p.Did, p.Id); err != nil {
			return err
		}
		if err := k.ProviderBySigner.Set(ctx, p.SignerAddress, p.Id); err != nil {
			return err
		}
	}
	for _, inf := range gs.Infractions {
		if err := k.Infractions.Set(ctx, inf.Id, inf); err != nil {
			return err
		}
		if err := k.InfractionByProvider.Set(ctx, collections.Join(inf.ProviderId, inf.Id)); err != nil {
			return err
		}
		if err := k.InfractionByKind.Set(ctx, collections.Join(uint32(inf.Kind), inf.Id)); err != nil {
			return err
		}
	}
	for _, jr := range gs.JailRecords {
		if err := k.JailRecords.Set(ctx, jr.Id, jr); err != nil {
			return err
		}
		if err := k.JailRecordByProvider.Set(ctx, collections.Join(jr.ProviderId, jr.Id)); err != nil {
			return err
		}
	}
	for _, br := range gs.BanRecords {
		if err := k.BanRecords.Set(ctx, br.Id, br); err != nil {
			return err
		}
		if err := k.BanRecordByProvider.Set(ctx, collections.Join(br.ProviderId, br.Id)); err != nil {
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

	// Peek returns the last-assigned id; NextXxxId in the
	// genesis schema means the id that *will* be assigned to
	// the next row, so we add 1.
	provLast, err := k.ProviderIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextProviderId = provLast + 1

	infLast, err := k.InfractionIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextInfractionId = infLast + 1

	jrLast, err := k.JailRecordIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextJailRecordId = jrLast + 1

	brLast, err := k.BanRecordIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextBanRecordId = brLast + 1

	if err := k.Providers.Walk(ctx, nil, func(_ uint64, v types.Provider) (bool, error) {
		gs.Providers = append(gs.Providers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Infractions.Walk(ctx, nil, func(_ uint64, v types.Infraction) (bool, error) {
		gs.Infractions = append(gs.Infractions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.JailRecords.Walk(ctx, nil, func(_ uint64, v types.JailRecord) (bool, error) {
		gs.JailRecords = append(gs.JailRecords, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.BanRecords.Walk(ctx, nil, func(_ uint64, v types.BanRecord) (bool, error) {
		gs.BanRecords = append(gs.BanRecords, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return gs, nil
}

// setSeq sets the sequence so that the next call to
// Sequence.Next returns `next`. Sequence stores the *last
// assigned* value and Next advances it by one, so we need to
// persist (next - 1).
func setSeq(ctx context.Context, seq collections.Sequence, next uint64) error {
	if next == 0 {
		return fmt.Errorf("next sequence value must be >= 1")
	}
	return seq.Set(ctx, next-1)
}
