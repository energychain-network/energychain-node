package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/identity/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, a := range gs.Accounts {
		if err := k.Accounts.Set(ctx, a.Address, a); err != nil {
			return err
		}
	}
	for _, r := range gs.Registrars {
		if err := k.Registrars.Set(ctx, r.Address, r); err != nil {
			return err
		}
	}
	for _, s := range gs.Sanctions {
		if err := k.Sanctions.Set(ctx, s.Address, s); err != nil {
			return err
		}
	}
	for _, p := range gs.Policies {
		if err := k.Policies.Set(ctx, p.Id, p); err != nil {
			return err
		}
	}
	for _, e := range gs.AuditEntries {
		if err := k.Audit.Set(ctx, e.Seq, e); err != nil {
			return err
		}
		if err := k.AuditByModule.Set(ctx, collections.Join(e.Module, e.Seq)); err != nil {
			return err
		}
	}
	if err := k.AuditSeq.Set(ctx, gs.AuditSeq); err != nil {
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

	if err := k.Accounts.Walk(ctx, nil, func(_ string, v types.Account) (bool, error) {
		gs.Accounts = append(gs.Accounts, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Registrars.Walk(ctx, nil, func(_ string, v types.Registrar) (bool, error) {
		gs.Registrars = append(gs.Registrars, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Sanctions.Walk(ctx, nil, func(_ string, v types.Sanction) (bool, error) {
		gs.Sanctions = append(gs.Sanctions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Policies.Walk(ctx, nil, func(_ string, v types.Policy) (bool, error) {
		gs.Policies = append(gs.Policies, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Audit.Walk(ctx, nil, func(_ uint64, v types.AuditEntry) (bool, error) {
		gs.AuditEntries = append(gs.AuditEntries, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.AuditSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.AuditSeq = seq
	return gs, nil
}
