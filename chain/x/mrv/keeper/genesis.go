package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/mrv/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("mrv: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextSchemaId > 0 {
		if err := k.SchemaIDSeq.Set(ctx, gs.NextSchemaId-1); err != nil {
			return err
		}
	}
	if gs.NextVerifierId > 0 {
		if err := k.VerifierIDSeq.Set(ctx, gs.NextVerifierId-1); err != nil {
			return err
		}
	}
	if gs.NextReportId > 0 {
		if err := k.ReportIDSeq.Set(ctx, gs.NextReportId-1); err != nil {
			return err
		}
	}
	if gs.NextGrantId > 0 {
		if err := k.GrantIDSeq.Set(ctx, gs.NextGrantId-1); err != nil {
			return err
		}
	}
	for _, s := range gs.Schemas {
		if err := k.SetSchema(ctx, s); err != nil {
			return err
		}
	}
	for _, v := range gs.Verifiers {
		if err := k.SetVerifier(ctx, v); err != nil {
			return err
		}
		if err := k.VerifierByDID.Set(ctx, v.Did, v.Id); err != nil {
			return err
		}
		if err := k.VerifierBySigner.Set(ctx, v.SignerAddress, v.Id); err != nil {
			return err
		}
	}
	// Reports + indexes; rebuild the per-subject count from
	// the loaded data so the runtime counter stays consistent.
	reportCount := map[string]uint64{}
	for _, r := range gs.Reports {
		if err := k.SetReport(ctx, r); err != nil {
			return err
		}
		if err := k.ReportBySubject.Set(ctx, collections.Join(r.Subject, r.Id)); err != nil {
			return err
		}
		if err := k.ReportBySchema.Set(ctx, collections.Join(r.SchemaId, r.Id)); err != nil {
			return err
		}
		reportCount[r.Subject]++
	}
	for sub, c := range reportCount {
		if err := k.ReportCountBySubject.Set(ctx, sub, c); err != nil {
			return err
		}
	}
	for _, g := range gs.Grants {
		if err := k.SetGrant(ctx, g); err != nil {
			return err
		}
		if err := k.GrantByGranter.Set(ctx, collections.Join(g.Granter, g.Id)); err != nil {
			return err
		}
		if err := k.GrantByGrantee.Set(ctx, collections.Join(g.GranteeDid, g.Id)); err != nil {
			return err
		}
		if err := k.GrantByExpiry.Set(ctx, collections.Join(g.ExpiresAt, g.Id)); err != nil {
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
	sNext, err := k.SchemaIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	vNext, err := k.VerifierIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	rNext, err := k.ReportIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gNext, err := k.GrantIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{
		Params:         p,
		NextSchemaId:   sNext + 1,
		NextVerifierId: vNext + 1,
		NextReportId:   rNext + 1,
		NextGrantId:    gNext + 1,
	}
	if err := k.Schemas.Walk(ctx, nil, func(_ uint64, v types.Schema) (bool, error) {
		out.Schemas = append(out.Schemas, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Verifiers.Walk(ctx, nil, func(_ uint64, v types.Verifier) (bool, error) {
		out.Verifiers = append(out.Verifiers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reports.Walk(ctx, nil, func(_ uint64, v types.Report) (bool, error) {
		out.Reports = append(out.Reports, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Grants.Walk(ctx, nil, func(_ uint64, v types.ViewKeyGrant) (bool, error) {
		out.Grants = append(out.Grants, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
