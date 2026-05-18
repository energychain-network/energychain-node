package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/auction/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("auction: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextAuctionId > 0 {
		if err := k.AuctionIDSeq.Set(ctx, gs.NextAuctionId-1); err != nil {
			return err
		}
	}
	if gs.NextBidId > 0 {
		if err := k.BidIDSeq.Set(ctx, gs.NextBidId-1); err != nil {
			return err
		}
	}
	for _, a := range gs.Auctions {
		if err := k.Auctions.Set(ctx, a.Id, a); err != nil {
			return err
		}
		if err := k.AuctionBySeller.Set(ctx, collections.Join(a.Seller, a.Id)); err != nil {
			return err
		}
	}
	for _, b := range gs.Bids {
		if err := k.Bids.Set(ctx, b.Id, b); err != nil {
			return err
		}
		if err := k.BidByAuction.Set(ctx, collections.Join(b.AuctionId, b.Id)); err != nil {
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
	aNext, err := k.AuctionIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	bNext, err := k.BidIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{
		Params:        p,
		NextAuctionId: aNext + 1,
		NextBidId:     bNext + 1,
	}
	if err := k.Auctions.Walk(ctx, nil, func(_ uint64, v types.Auction) (bool, error) {
		out.Auctions = append(out.Auctions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Bids.Walk(ctx, nil, func(_ uint64, v types.Bid) (bool, error) {
		out.Bids = append(out.Bids, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
