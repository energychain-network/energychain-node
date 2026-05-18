package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxAuctions:               DefaultMaxAuctions,
		MaxAuctionsPerSeller:      DefaultMaxAuctionsPerSeller,
		MaxBidsPerAuction:         DefaultMaxBidsPerAuction,
		MinAuctionDurationSeconds: DefaultMinAuctionDurationSeconds,
		MaxAuctionDurationSeconds: DefaultMaxAuctionDurationSeconds,
		MemoMaxLen:                DefaultMemoMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxAuctions == 0 || p.MaxAuctions > HardMaxAuctions {
		return fmt.Errorf("max_auctions out of range")
	}
	if p.MaxAuctionsPerSeller == 0 || p.MaxAuctionsPerSeller > HardMaxAuctionsPerSeller {
		return fmt.Errorf("max_auctions_per_seller out of range")
	}
	if p.MaxBidsPerAuction == 0 || p.MaxBidsPerAuction > HardMaxBidsPerAuction {
		return fmt.Errorf("max_bids_per_auction out of range")
	}
	if p.MinAuctionDurationSeconds < HardMinAuctionDurationSeconds {
		return fmt.Errorf("min_auction_duration_seconds must be >= %d", HardMinAuctionDurationSeconds)
	}
	if p.MaxAuctionDurationSeconds <= 0 || p.MaxAuctionDurationSeconds > HardMaxAuctionDurationSeconds {
		return fmt.Errorf("max_auction_duration_seconds out of range")
	}
	if p.MinAuctionDurationSeconds > p.MaxAuctionDurationSeconds {
		return fmt.Errorf("min > max auction duration")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len must be <= %d", HardMemoMaxLen)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:         DefaultParams(),
		NextAuctionId:  1,
		NextBidId:      1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Auctions)) > gs.Params.MaxAuctions {
		return fmt.Errorf("genesis: %d auctions > max %d", len(gs.Auctions), gs.Params.MaxAuctions)
	}
	ids := map[uint64]bool{}
	perSeller := map[string]uint32{}
	for _, a := range gs.Auctions {
		if a.Id == 0 {
			return fmt.Errorf("genesis: auction id must be > 0")
		}
		if ids[a.Id] {
			return fmt.Errorf("genesis: duplicate auction id %d", a.Id)
		}
		ids[a.Id] = true
		if a.Id >= gs.NextAuctionId {
			return fmt.Errorf("genesis: auction id %d >= next_auction_id %d", a.Id, gs.NextAuctionId)
		}
		if !KindValid(a.Kind) {
			return fmt.Errorf("genesis: auction %d invalid kind", a.Id)
		}
		if !StatusValid(a.Status) {
			return fmt.Errorf("genesis: auction %d invalid status", a.Id)
		}
		if err := ValidateAddr("seller", a.Seller); err != nil {
			return err
		}
		if err := ValidateDenom(a.PaymentDenom); err != nil {
			return err
		}
		if err := ValidateAssetRef(a.AssetRef); err != nil {
			return err
		}
		perSeller[a.Seller]++
		if perSeller[a.Seller] > gs.Params.MaxAuctionsPerSeller {
			return fmt.Errorf("genesis: per-seller cap exceeded")
		}
	}
	bidIDs := map[uint64]bool{}
	perAuctionBids := map[uint64]uint32{}
	for _, b := range gs.Bids {
		if b.Id == 0 {
			return fmt.Errorf("genesis: bid id must be > 0")
		}
		if bidIDs[b.Id] {
			return fmt.Errorf("genesis: duplicate bid id %d", b.Id)
		}
		bidIDs[b.Id] = true
		if b.Id >= gs.NextBidId {
			return fmt.Errorf("genesis: bid id %d >= next_bid_id %d", b.Id, gs.NextBidId)
		}
		if !ids[b.AuctionId] {
			return fmt.Errorf("genesis: bid %d references unknown auction %d", b.Id, b.AuctionId)
		}
		if err := ValidateAddr("bidder", b.Bidder); err != nil {
			return err
		}
		perAuctionBids[b.AuctionId]++
		if perAuctionBids[b.AuctionId] > gs.Params.MaxBidsPerAuction {
			return fmt.Errorf("genesis: per-auction bid cap exceeded for auction %d", b.AuctionId)
		}
	}
	return nil
}
