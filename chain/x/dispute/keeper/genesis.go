package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/dispute/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("dispute: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextArbitratorId > 0 {
		if err := k.ArbitratorIDSeq.Set(ctx, gs.NextArbitratorId-1); err != nil {
			return err
		}
	}
	if gs.NextDisputeId > 0 {
		if err := k.DisputeIDSeq.Set(ctx, gs.NextDisputeId-1); err != nil {
			return err
		}
	}
	for _, a := range gs.Arbitrators {
		if err := k.SetArbitrator(ctx, a); err != nil {
			return err
		}
		if err := k.ArbitratorByDID.Set(ctx, a.Did, a.Id); err != nil {
			return err
		}
		if err := k.ArbitratorBySigner.Set(ctx, a.SignerAddress, a.Id); err != nil {
			return err
		}
	}
	for _, d := range gs.Disputes {
		// Bypass SetDispute (which assumes prior-row deltas) and
		// write directly so the indexes are rebuilt cleanly.
		if err := k.Disputes.Set(ctx, d.Id, d); err != nil {
			return err
		}
		if err := k.DisputeByStatus.Set(ctx, collections.Join(uint32(d.Status), d.Id)); err != nil {
			return err
		}
		if err := k.DisputeBySubject.Set(ctx, collections.Join3(uint32(d.SubjectKind), d.SubjectRef, d.Id)); err != nil {
			return err
		}
	}
	for _, tm := range gs.TribunalMembers {
		if err := k.Tribunal.Set(ctx, collections.Join(tm.DisputeId, tm.ArbitratorId), tm); err != nil {
			return err
		}
	}
	for _, v := range gs.Votes {
		if err := k.Votes.Set(ctx, collections.Join(v.DisputeId, v.ArbitratorId), v); err != nil {
			return err
		}
	}
	for _, e := range gs.Evidence {
		if err := k.Evidence.Set(ctx, collections.Join(e.DisputeId, e.Id), e); err != nil {
			return err
		}
	}
	for _, es := range gs.EvidenceSeqs {
		if err := k.EvidenceSeq.Set(ctx, es.DisputeId, es.NextEvidenceId); err != nil {
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
	aNext, err := k.ArbitratorIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	dNext, err := k.DisputeIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{
		Params:           p,
		NextArbitratorId: aNext + 1,
		NextDisputeId:    dNext + 1,
	}
	if err := k.Arbitrators.Walk(ctx, nil, func(_ uint64, v types.Arbitrator) (bool, error) {
		out.Arbitrators = append(out.Arbitrators, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Disputes.Walk(ctx, nil, func(_ uint64, v types.Dispute) (bool, error) {
		out.Disputes = append(out.Disputes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Tribunal.Walk(ctx, nil, func(_ collections.Pair[uint64, uint64], v types.TribunalMember) (bool, error) {
		out.TribunalMembers = append(out.TribunalMembers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Pair[uint64, uint64], v types.Vote) (bool, error) {
		out.Votes = append(out.Votes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Evidence.Walk(ctx, nil, func(_ collections.Pair[uint64, uint64], v types.Evidence) (bool, error) {
		out.Evidence = append(out.Evidence, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.EvidenceSeq.Walk(ctx, nil, func(disputeID, next uint64) (bool, error) {
		out.EvidenceSeqs = append(out.EvidenceSeqs, types.EvidenceSeqEntry{
			DisputeId: disputeID, NextEvidenceId: next,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
