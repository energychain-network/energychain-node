package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/bridge/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, c := range gs.Chains {
		if err := k.Chains.Set(ctx, c.Id, c); err != nil {
			return err
		}
	}
	for _, a := range gs.Assets {
		if err := k.Assets.Set(ctx, a.Id, a); err != nil {
			return err
		}
		if err := k.AssetByDenom.Set(ctx, a.Denom, a.Id); err != nil {
			return err
		}
	}
	denomOf := map[uint64]string{}
	for _, a := range gs.Assets {
		denomOf[a.Id] = a.Denom
	}
	for _, o := range gs.Outbounds {
		if err := k.Outbounds.Set(ctx, o.Nonce, o); err != nil {
			return err
		}
		if err := k.recordBurn(ctx, denomOf[o.AssetId], o.Amount); err != nil {
			return err
		}
	}
	for _, in := range gs.Inbounds {
		if err := k.Inbounds.Set(ctx, in.Id, in); err != nil {
			return err
		}
		if err := k.InboundBySrc.Set(ctx, collections.Join(in.SrcChainId, in.SrcNonce), in.Id); err != nil {
			return err
		}
		// Rebuild the derived release-side state (window index + minted
		// counters) so rate limits and net-outstanding survive an export/
		// import round-trip without being part of the genesis document.
		if in.Status == types.InboundStatus_INBOUND_STATUS_RELEASED {
			if err := k.recordRelease(ctx, in, denomOf[in.AssetId]); err != nil {
				return err
			}
		}
	}
	// No escrow reconciliation in the mint/burn model: inbound releases MINT
	// the native stablecoin and outbound locks BURN it, so there is no escrow
	// pool whose balance must match a net locked-minus-released position.
	if err := k.ChainIDSeq.Set(ctx, gs.ChainIdSeq); err != nil {
		return err
	}
	if err := k.AssetIDSeq.Set(ctx, gs.AssetIdSeq); err != nil {
		return err
	}
	if err := k.OutboundSeq.Set(ctx, gs.OutboundNonceSeq); err != nil {
		return err
	}
	return k.InboundIDSeq.Set(ctx, gs.InboundIdSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Chains.Walk(ctx, nil, func(_ uint64, c types.ExternalChain) (bool, error) {
		gs.Chains = append(gs.Chains, c)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Assets.Walk(ctx, nil, func(_ uint64, a types.Asset) (bool, error) {
		gs.Assets = append(gs.Assets, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Outbounds.Walk(ctx, nil, func(_ uint64, o types.Outbound) (bool, error) {
		gs.Outbounds = append(gs.Outbounds, o)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Inbounds.Walk(ctx, nil, func(_ uint64, in types.Inbound) (bool, error) {
		gs.Inbounds = append(gs.Inbounds, in)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.ChainIdSeq, err = k.ChainIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.AssetIdSeq, err = k.AssetIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.OutboundNonceSeq, err = k.OutboundSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.InboundIdSeq, err = k.InboundIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
