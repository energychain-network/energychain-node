package keeper

import (
	"bytes"
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/auction/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// statusForCurrentPhase returns the right "open" status given
// the auction's kind and the current time. Used during PlaceBid
// / CommitBid / RevealBid to enforce phase boundaries.
func statusForKindAtTime(a types.Auction, now int64) types.Status {
	switch a.Kind {
	case types.Kind_KIND_ENGLISH, types.Kind_KIND_DUTCH:
		if now < a.StartTime {
			return types.Status_STATUS_DRAFT
		}
		if now >= a.EndTime {
			return types.Status_STATUS_CLOSED
		}
		return types.Status_STATUS_OPEN
	case types.Kind_KIND_SEALED_FIRST, types.Kind_KIND_SEALED_SECOND:
		if now < a.StartTime {
			return types.Status_STATUS_DRAFT
		}
		if now < a.CommitEndTime {
			return types.Status_STATUS_COMMIT
		}
		if now < a.RevealEndTime {
			return types.Status_STATUS_REVEAL
		}
		return types.Status_STATUS_CLOSED
	}
	return types.Status_STATUS_UNSPECIFIED
}

// ---- CreateAuction ----------------------------------------------------

func (s msgServer) CreateAuction(goCtx context.Context, m *types.MsgCreateAuction) (*types.MsgCreateAuctionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}

	now := ctx.BlockTime().Unix()
	startTime := m.StartTime
	if startTime == 0 {
		startTime = now
	}
	// Duration bound — defined relative to the latest gate
	// (reveal_end_time for sealed, end_time for english / dutch).
	var endRef int64
	switch m.Kind {
	case types.Kind_KIND_ENGLISH, types.Kind_KIND_DUTCH:
		endRef = m.EndTime
	case types.Kind_KIND_SEALED_FIRST, types.Kind_KIND_SEALED_SECOND:
		endRef = m.RevealEndTime
	}
	dur := endRef - startTime
	if dur < p.MinAuctionDurationSeconds || dur > p.MaxAuctionDurationSeconds {
		return nil, fmt.Errorf("auction duration %d out of [%d, %d]", dur, p.MinAuctionDurationSeconds, p.MaxAuctionDurationSeconds)
	}

	count, err := s.k.CountAuctions(ctx)
	if err != nil {
		return nil, err
	}
	if count >= p.MaxAuctions {
		return nil, fmt.Errorf("auction count cap reached: %d", p.MaxAuctions)
	}
	n, err := s.k.CountAuctionsForSeller(ctx, m.Seller)
	if err != nil {
		return nil, err
	}
	if n >= p.MaxAuctionsPerSeller {
		return nil, fmt.Errorf("seller %s already holds %d auctions (cap %d)", m.Seller, n, p.MaxAuctionsPerSeller)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Seller, "seller"); err != nil {
		return nil, err
	}

	id, err := s.k.NextAuctionID(ctx)
	if err != nil {
		return nil, err
	}
	a := types.Auction{
		Id:                id,
		Kind:              m.Kind,
		Status:            types.Status_STATUS_DRAFT,
		Seller:            m.Seller,
		AssetRef:          m.AssetRef,
		PaymentDenom:      m.PaymentDenom,
		ReservePrice:      m.ReservePrice,
		MinBidIncrement:   m.MinBidIncrement,
		DutchStartPrice:   m.DutchStartPrice,
		DutchFloorPrice:   m.DutchFloorPrice,
		DutchDecaySeconds: m.DutchDecaySeconds,
		StartTime:         startTime,
		CommitEndTime:     m.CommitEndTime,
		RevealEndTime:     m.RevealEndTime,
		EndTime:           m.EndTime,
		CreatedAt:         now,
		Memo:              m.Memo,
	}
	// If start_time is already in the past, jump into the
	// initial open / commit phase eagerly so the first bid does
	// not need to wait for an explicit transition.
	a.Status = statusForKindAtTime(a, now)
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	if err := s.k.AuctionBySeller.Set(ctx, collections.Join(a.Seller, a.Id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "create", m.Seller, "", fmt.Sprintf("kind=%s", a.Kind))
	s.k.emit(ctx, "create",
		"id", u64s(a.Id),
		"kind", a.Kind.String(),
		"seller", a.Seller,
		"asset_ref", a.AssetRef,
		"payment_denom", a.PaymentDenom,
		"start_time", i64s(a.StartTime),
		"status", a.Status.String(),
	)
	return &types.MsgCreateAuctionResponse{AuctionId: a.Id}, nil
}

// ---- CancelAuction ----------------------------------------------------

func (s msgServer) CancelAuction(goCtx context.Context, m *types.MsgCancelAuction) (*types.MsgCancelAuctionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, err := s.k.MustGetAuction(ctx, m.AuctionId)
	if err != nil {
		return nil, err
	}
	if a.Seller != m.Seller {
		return nil, fmt.Errorf("only seller can cancel")
	}
	if a.Status == types.Status_STATUS_SETTLED || a.Status == types.Status_STATUS_CLOSED || a.Status == types.Status_STATUS_CANCELLED {
		return nil, fmt.Errorf("auction %d not cancellable (status=%s)", a.Id, a.Status)
	}
	// Refuse cancellation if any binding bid has been placed.
	// Otherwise the seller could cancel after seeing competitive
	// bids, defeating the auction's price-discovery purpose.
	n, err := s.k.CountBidsForAuction(ctx, a.Id)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("auction %d has %d bids; cannot cancel", a.Id, n)
	}
	a.Status = types.Status_STATUS_CANCELLED
	a.ClosedAt = ctx.BlockTime().Unix()
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "cancel", m.Seller, "", m.Reason)
	s.k.emit(ctx, "cancel", "id", u64s(a.Id), "reason", m.Reason)
	return &types.MsgCancelAuctionResponse{}, nil
}

// ---- PlaceBid (English / Dutch) ---------------------------------------

func (s msgServer) PlaceBid(goCtx context.Context, m *types.MsgPlaceBid) (*types.MsgPlaceBidResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, err := s.k.MustGetAuction(ctx, m.AuctionId)
	if err != nil {
		return nil, err
	}
	if a.Kind != types.Kind_KIND_ENGLISH && a.Kind != types.Kind_KIND_DUTCH {
		return nil, fmt.Errorf("PlaceBid only valid for english / dutch; use CommitBid for sealed")
	}
	// Refuse bids on auctions whose persisted status has already
	// progressed past OPEN. Without this gate a Dutch auction
	// that already crowned a winner (status=CLOSED via the
	// first PlaceBid) would still pass the phase-by-time check
	// — `statusForKindAtTime` returns OPEN until end_time — and
	// a second bidder could overwrite the winner. Same gate
	// blocks PlaceBid on a CANCELLED draft.
	switch a.Status {
	case types.Status_STATUS_CLOSED, types.Status_STATUS_SETTLED, types.Status_STATUS_CANCELLED:
		return nil, fmt.Errorf("auction %d not open (status=%s)", a.Id, a.Status)
	}
	now := ctx.BlockTime().Unix()
	phase := statusForKindAtTime(a, now)
	if phase != types.Status_STATUS_OPEN {
		return nil, fmt.Errorf("auction %d not OPEN (phase=%s)", a.Id, phase)
	}
	// Persist the transition so reads see OPEN.
	if a.Status != types.Status_STATUS_OPEN {
		a.Status = types.Status_STATUS_OPEN
	}
	if err := s.k.requireUnsanctioned(ctx, m.Bidder, "bidder"); err != nil {
		return nil, err
	}
	if m.Bidder == a.Seller {
		return nil, fmt.Errorf("seller cannot bid in their own auction")
	}
	// Per-auction bid cap.
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	bn, err := s.k.CountBidsForAuction(ctx, a.Id)
	if err != nil {
		return nil, err
	}
	if bn >= p.MaxBidsPerAuction {
		return nil, fmt.Errorf("auction %d bid cap %d reached", a.Id, p.MaxBidsPerAuction)
	}

	switch a.Kind {
	case types.Kind_KIND_ENGLISH:
		if m.Price < a.ReservePrice {
			return nil, fmt.Errorf("bid %d < reserve %d", m.Price, a.ReservePrice)
		}
		// must beat current high by min_bid_increment
		if a.WinningBidId != 0 {
			cur, err := s.k.MustGetBid(ctx, a.WinningBidId)
			if err != nil {
				return nil, err
			}
			minNew, err := types.SafeAdd(cur.Price, a.MinBidIncrement)
			if err != nil {
				return nil, err
			}
			if m.Price < minNew {
				return nil, fmt.Errorf("bid %d < min increment-adjusted %d", m.Price, minNew)
			}
			// Demote the previous high. Their deposit is now
			// refundable via WithdrawRefund — we do NOT auto-
			// push the refund so a sanctioned previous high
			// bidder cannot DOS the new bid (the bid moves
			// forward; the OUTBID deposit stays in the pool
			// pending self-service withdrawal).
			cur.Status = types.BidStatus_BID_STATUS_OUTBID
			if err := s.k.SetBid(ctx, cur); err != nil {
				return nil, err
			}
		}
	case types.Kind_KIND_DUTCH:
		px := types.DutchPriceAt(a, now)
		if m.Price < px {
			return nil, fmt.Errorf("dutch bid %d < current curve price %d", m.Price, px)
		}
		if m.Price < a.ReservePrice {
			return nil, fmt.Errorf("bid %d < reserve %d", m.Price, a.ReservePrice)
		}
	}
	// Escrow bid price into pool.
	if err := s.k.fundPool(ctx, a.PaymentDenom, m.Bidder, m.Price); err != nil {
		return nil, err
	}
	bidID, err := s.k.NextBidID(ctx)
	if err != nil {
		return nil, err
	}
	b := types.Bid{
		Id:         bidID,
		AuctionId:  a.Id,
		Bidder:     m.Bidder,
		Price:      m.Price,
		Deposit:    m.Price,
		PlacedAt:   now,
	}
	switch a.Kind {
	case types.Kind_KIND_ENGLISH:
		b.Status = types.BidStatus_BID_STATUS_ACTIVE
		a.WinningBidId = b.Id
	case types.Kind_KIND_DUTCH:
		// Dutch is single-shot: this bid wins immediately and
		// the auction transitions to CLOSED. clearing_price is
		// the bidder's stated price (which may be higher than
		// the curve price — the surplus is still owed by the
		// bidder, matching real-world over-bidder convention).
		b.Status = types.BidStatus_BID_STATUS_WON
		a.WinningBidId = b.Id
		a.Winner = m.Bidder
		a.ClearingPrice = m.Price
		a.Status = types.Status_STATUS_CLOSED
		a.ClosedAt = now
	}
	if err := s.k.SetBid(ctx, b); err != nil {
		return nil, err
	}
	if err := s.k.BidByAuction.Set(ctx, collections.Join(a.Id, b.Id)); err != nil {
		return nil, err
	}
	a.BidCount++
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "place_bid", m.Bidder, "", fmt.Sprintf("bid=%d price=%d", b.Id, m.Price))
	s.k.emit(ctx, "place_bid",
		"auction_id", u64s(a.Id),
		"bid_id", u64s(b.Id),
		"bidder", m.Bidder,
		"price", u64s(m.Price),
		"status", b.Status.String(),
	)
	return &types.MsgPlaceBidResponse{BidId: b.Id}, nil
}

// ---- CommitBid (Sealed) ----------------------------------------------

func (s msgServer) CommitBid(goCtx context.Context, m *types.MsgCommitBid) (*types.MsgCommitBidResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, err := s.k.MustGetAuction(ctx, m.AuctionId)
	if err != nil {
		return nil, err
	}
	if !types.IsSealed(a.Kind) {
		return nil, fmt.Errorf("CommitBid only valid for sealed kinds")
	}
	// Defense-in-depth: cancel currently requires zero bids, so
	// a CANCELLED sealed auction can never reach commit phase
	// — but the persisted-status gate makes that contract
	// explicit and survives any future change to cancel rules.
	switch a.Status {
	case types.Status_STATUS_CLOSED, types.Status_STATUS_SETTLED, types.Status_STATUS_CANCELLED:
		return nil, fmt.Errorf("auction %d not open (status=%s)", a.Id, a.Status)
	}
	now := ctx.BlockTime().Unix()
	phase := statusForKindAtTime(a, now)
	if phase != types.Status_STATUS_COMMIT {
		return nil, fmt.Errorf("auction %d not in COMMIT phase (phase=%s)", a.Id, phase)
	}
	a.Status = types.Status_STATUS_COMMIT
	if err := s.k.requireUnsanctioned(ctx, m.Bidder, "bidder"); err != nil {
		return nil, err
	}
	if m.Bidder == a.Seller {
		return nil, fmt.Errorf("seller cannot bid in their own auction")
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	bn, err := s.k.CountBidsForAuction(ctx, a.Id)
	if err != nil {
		return nil, err
	}
	if bn >= p.MaxBidsPerAuction {
		return nil, fmt.Errorf("auction %d bid cap %d reached", a.Id, p.MaxBidsPerAuction)
	}
	if err := s.k.fundPool(ctx, a.PaymentDenom, m.Bidder, m.Deposit); err != nil {
		return nil, err
	}
	bidID, err := s.k.NextBidID(ctx)
	if err != nil {
		return nil, err
	}
	b := types.Bid{
		Id:         bidID,
		AuctionId:  a.Id,
		Bidder:     m.Bidder,
		Status:     types.BidStatus_BID_STATUS_COMMITTED,
		Deposit:    m.Deposit,
		CommitHash: m.CommitHash,
		PlacedAt:   now,
	}
	if err := s.k.SetBid(ctx, b); err != nil {
		return nil, err
	}
	if err := s.k.BidByAuction.Set(ctx, collections.Join(a.Id, b.Id)); err != nil {
		return nil, err
	}
	a.BidCount++
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "commit_bid", m.Bidder, "", fmt.Sprintf("bid=%d deposit=%d", b.Id, m.Deposit))
	s.k.emit(ctx, "commit_bid",
		"auction_id", u64s(a.Id),
		"bid_id", u64s(b.Id),
		"bidder", m.Bidder,
		"deposit", u64s(m.Deposit),
	)
	return &types.MsgCommitBidResponse{BidId: b.Id}, nil
}

// ---- RevealBid (Sealed) ----------------------------------------------

func (s msgServer) RevealBid(goCtx context.Context, m *types.MsgRevealBid) (*types.MsgRevealBidResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	b, err := s.k.MustGetBid(ctx, m.BidId)
	if err != nil {
		return nil, err
	}
	if b.Bidder != m.Bidder {
		return nil, fmt.Errorf("only bidder can reveal")
	}
	if b.Status != types.BidStatus_BID_STATUS_COMMITTED {
		return nil, fmt.Errorf("bid %d not in COMMITTED status (status=%s)", b.Id, b.Status)
	}
	a, err := s.k.MustGetAuction(ctx, b.AuctionId)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	phase := statusForKindAtTime(a, now)
	if phase != types.Status_STATUS_REVEAL {
		return nil, fmt.Errorf("auction %d not in REVEAL phase (phase=%s)", a.Id, phase)
	}
	a.Status = types.Status_STATUS_REVEAL
	if m.Price > b.Deposit {
		return nil, fmt.Errorf("revealed price %d > committed deposit %d", m.Price, b.Deposit)
	}
	want := types.ComputeCommitHash(m.Price, m.Salt)
	if !bytes.Equal(want, b.CommitHash) {
		return nil, fmt.Errorf("commit hash mismatch")
	}
	b.Price = m.Price
	b.RevealSalt = m.Salt
	b.Status = types.BidStatus_BID_STATUS_REVEALED
	b.RevealedAt = now
	if err := s.k.SetBid(ctx, b); err != nil {
		return nil, err
	}
	a.ValidRevealCount++
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "reveal_bid", m.Bidder, "", fmt.Sprintf("bid=%d price=%d", b.Id, m.Price))
	s.k.emit(ctx, "reveal_bid",
		"auction_id", u64s(a.Id),
		"bid_id", u64s(b.Id),
		"price", u64s(m.Price),
	)
	return &types.MsgRevealBidResponse{}, nil
}

// ---- Close ------------------------------------------------------------

func (s msgServer) Close(goCtx context.Context, m *types.MsgClose) (*types.MsgCloseResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, err := s.k.MustGetAuction(ctx, m.AuctionId)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	switch a.Status {
	case types.Status_STATUS_CLOSED, types.Status_STATUS_SETTLED, types.Status_STATUS_CANCELLED:
		return nil, fmt.Errorf("auction %d already closed (status=%s)", a.Id, a.Status)
	}
	switch a.Kind {
	case types.Kind_KIND_ENGLISH:
		if now < a.EndTime {
			return nil, fmt.Errorf("auction %d not yet ended (end=%d now=%d)", a.Id, a.EndTime, now)
		}
		// English already tracks winning_bid_id; promote it
		// (or mark "no winner" if reserve not met).
		if a.WinningBidId != 0 {
			b, err := s.k.MustGetBid(ctx, a.WinningBidId)
			if err != nil {
				return nil, err
			}
			if b.Price >= a.ReservePrice {
				b.Status = types.BidStatus_BID_STATUS_WON
				if err := s.k.SetBid(ctx, b); err != nil {
					return nil, err
				}
				a.Winner = b.Bidder
				a.ClearingPrice = b.Price
			} else {
				// reserve not met — flag winner-candidate as
				// LOST and emit no-winner.
				b.Status = types.BidStatus_BID_STATUS_LOST
				if err := s.k.SetBid(ctx, b); err != nil {
					return nil, err
				}
				a.WinningBidId = 0
			}
		}
	case types.Kind_KIND_DUTCH:
		// Dutch closes implicitly on first PlaceBid; explicit
		// Close after end_time only handles the "no taker" case.
		if a.Winner == "" {
			if now < a.EndTime {
				return nil, fmt.Errorf("auction %d not yet ended (end=%d now=%d)", a.Id, a.EndTime, now)
			}
		}
	case types.Kind_KIND_SEALED_FIRST, types.Kind_KIND_SEALED_SECOND:
		if now < a.RevealEndTime {
			return nil, fmt.Errorf("auction %d still in reveal (until=%d now=%d)", a.Id, a.RevealEndTime, now)
		}
		// Walk all bids; sort by revealed price desc.
		var revealed []types.Bid
		rng := collections.NewPrefixedPairRange[uint64, uint64](a.Id)
		if err := s.k.BidByAuction.Walk(ctx, rng, func(k collections.Pair[uint64, uint64]) (bool, error) {
			b, err := s.k.MustGetBid(ctx, k.K2())
			if err != nil {
				return true, err
			}
			switch b.Status {
			case types.BidStatus_BID_STATUS_REVEALED:
				revealed = append(revealed, b)
			case types.BidStatus_BID_STATUS_COMMITTED:
				// Forfeit unrevealed commits.
				b.Status = types.BidStatus_BID_STATUS_FORFEITED
				if err := s.k.SetBid(ctx, b); err != nil {
					return true, err
				}
			}
			return false, nil
		}); err != nil {
			return nil, err
		}
		// Sort revealed bids: highest price first; tie-break
		// by lowest bid id for determinism.
		for i := 1; i < len(revealed); i++ {
			for j := i; j > 0; j-- {
				if revealed[j].Price > revealed[j-1].Price ||
					(revealed[j].Price == revealed[j-1].Price && revealed[j].Id < revealed[j-1].Id) {
					revealed[j-1], revealed[j] = revealed[j], revealed[j-1]
				} else {
					break
				}
			}
		}
		// Find winner: highest revealed price that clears reserve.
		var winner *types.Bid
		for i := range revealed {
			if revealed[i].Price >= a.ReservePrice {
				winner = &revealed[i]
				break
			}
		}
		if winner == nil {
			for i := range revealed {
				revealed[i].Status = types.BidStatus_BID_STATUS_LOST
				if err := s.k.SetBid(ctx, revealed[i]); err != nil {
					return nil, err
				}
			}
		} else {
			// Determine clearing price by kind.
			var clearing uint64
			if a.Kind == types.Kind_KIND_SEALED_FIRST {
				clearing = winner.Price
			} else {
				// SEALED_SECOND: pay second-highest valid bid,
				// or reserve_price if there is no second-highest.
				clearing = a.ReservePrice
				for i := range revealed {
					if revealed[i].Id == winner.Id {
						continue
					}
					if revealed[i].Price >= a.ReservePrice {
						clearing = revealed[i].Price
						break
					}
				}
			}
			winner.Status = types.BidStatus_BID_STATUS_WON
			if err := s.k.SetBid(ctx, *winner); err != nil {
				return nil, err
			}
			// Mark losers (revealed bids that aren't the winner)
			// as LOST.
			for i := range revealed {
				if revealed[i].Id == winner.Id {
					continue
				}
				revealed[i].Status = types.BidStatus_BID_STATUS_LOST
				if err := s.k.SetBid(ctx, revealed[i]); err != nil {
					return nil, err
				}
			}
			a.Winner = winner.Bidder
			a.WinningBidId = winner.Id
			a.ClearingPrice = clearing
		}
	}
	a.Status = types.Status_STATUS_CLOSED
	a.ClosedAt = now
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "close", m.Caller, "", fmt.Sprintf("winner=%s price=%d", a.Winner, a.ClearingPrice))
	s.k.emit(ctx, "close",
		"id", u64s(a.Id),
		"winner", a.Winner,
		"clearing_price", u64s(a.ClearingPrice),
		"winning_bid_id", u64s(a.WinningBidId),
	)
	return &types.MsgCloseResponse{Winner: a.Winner, ClearingPrice: a.ClearingPrice}, nil
}

// ---- Settle -----------------------------------------------------------

func (s msgServer) Settle(goCtx context.Context, m *types.MsgSettle) (*types.MsgSettleResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, err := s.k.MustGetAuction(ctx, m.AuctionId)
	if err != nil {
		return nil, err
	}
	if a.Status == types.Status_STATUS_SETTLED {
		return &types.MsgSettleResponse{PaidToSeller: 0}, nil
	}
	if a.Status != types.Status_STATUS_CLOSED {
		return nil, fmt.Errorf("auction %d not CLOSED (status=%s)", a.Id, a.Status)
	}
	// Sanctions re-check on seller AT settlement time. If
	// seller is now sanctioned, the funds stay in the pool;
	// losers can still self-serve refunds.
	if err := s.k.requireUnsanctioned(ctx, a.Seller, "seller"); err != nil {
		return nil, err
	}

	// Total to pay seller = clearing_price + sum(deposit) of
	// FORFEITED (unrevealed sealed) bids.
	paid := a.ClearingPrice
	rng := collections.NewPrefixedPairRange[uint64, uint64](a.Id)
	if err := s.k.BidByAuction.Walk(ctx, rng, func(k collections.Pair[uint64, uint64]) (bool, error) {
		b, err := s.k.MustGetBid(ctx, k.K2())
		if err != nil {
			return true, err
		}
		if b.Status == types.BidStatus_BID_STATUS_FORFEITED && !b.Refunded {
			np, err := types.SafeAdd(paid, b.Deposit)
			if err != nil {
				return true, err
			}
			paid = np
			b.Refunded = true
			if err := s.k.SetBid(ctx, b); err != nil {
				return true, err
			}
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if a.WinningBidId != 0 && a.Winner != "" {
		// Mark winner deposit as "consumed". Surplus over
		// clearing_price (e.g. SEALED_SECOND, or DUTCH over-bid)
		// is refundable to the winner.
		wb, err := s.k.MustGetBid(ctx, a.WinningBidId)
		if err != nil {
			return nil, err
		}
		if wb.Deposit < a.ClearingPrice {
			return nil, fmt.Errorf("BUG: winning bid deposit %d < clearing %d", wb.Deposit, a.ClearingPrice)
		}
		// Mark refunded=false so winner can self-claim surplus
		// via WithdrawRefund. They get back (deposit - clearing).
		_ = wb
	}
	if paid > 0 {
		if err := s.k.drainPool(ctx, a.PaymentDenom, a.Seller, paid); err != nil {
			return nil, err
		}
	}
	a.Status = types.Status_STATUS_SETTLED
	a.SettledAt = ctx.BlockTime().Unix()
	if err := s.k.SetAuction(ctx, a); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, a.Id, "settle", m.Caller, a.Seller, fmt.Sprintf("paid=%d", paid))
	s.k.emit(ctx, "settle",
		"id", u64s(a.Id),
		"paid_to_seller", u64s(paid),
	)
	return &types.MsgSettleResponse{PaidToSeller: paid}, nil
}

// ---- WithdrawRefund ---------------------------------------------------

func (s msgServer) WithdrawRefund(goCtx context.Context, m *types.MsgWithdrawRefund) (*types.MsgWithdrawRefundResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	b, err := s.k.MustGetBid(ctx, m.BidId)
	if err != nil {
		return nil, err
	}
	if b.Bidder != m.Bidder {
		return nil, fmt.Errorf("only bidder can withdraw")
	}
	if b.Refunded {
		return nil, fmt.Errorf("bid %d already refunded", b.Id)
	}
	a, err := s.k.MustGetAuction(ctx, b.AuctionId)
	if err != nil {
		return nil, err
	}

	// Determine refundable amount per bid status.
	var refund uint64
	switch b.Status {
	case types.BidStatus_BID_STATUS_OUTBID,
		types.BidStatus_BID_STATUS_LOST:
		refund = b.Deposit
	case types.BidStatus_BID_STATUS_COMMITTED, types.BidStatus_BID_STATUS_REVEALED:
		// If auction was cancelled (no bids accepted) or
		// is still pre-close, refusing the refund.
		if a.Status != types.Status_STATUS_CANCELLED {
			return nil, fmt.Errorf("bid %d not yet refundable (auction status=%s)", b.Id, a.Status)
		}
		refund = b.Deposit
	case types.BidStatus_BID_STATUS_ACTIVE:
		// ACTIVE = current English high. Not refundable unless
		// the auction was cancelled.
		if a.Status != types.Status_STATUS_CANCELLED {
			return nil, fmt.Errorf("bid %d still ACTIVE; cannot refund", b.Id)
		}
		refund = b.Deposit
	case types.BidStatus_BID_STATUS_WON:
		// Winner can claim the surplus over clearing_price.
		if a.Status != types.Status_STATUS_SETTLED {
			return nil, fmt.Errorf("auction %d not settled; winner cannot withdraw surplus yet", a.Id)
		}
		if b.Deposit > a.ClearingPrice {
			refund = b.Deposit - a.ClearingPrice
		}
		// refund may be 0 — that's a hard refusal so the caller
		// realises there's nothing to claim.
		if refund == 0 {
			return nil, fmt.Errorf("no surplus to claim")
		}
	case types.BidStatus_BID_STATUS_FORFEITED:
		return nil, fmt.Errorf("bid %d forfeited; deposit paid to seller", b.Id)
	default:
		return nil, fmt.Errorf("bid %d has unsupported status %s", b.Id, b.Status)
	}
	// Sanctions re-check on the receiving party.
	if err := s.k.requireUnsanctioned(ctx, m.Bidder, "bidder"); err != nil {
		return nil, err
	}
	if err := s.k.drainPool(ctx, a.PaymentDenom, m.Bidder, refund); err != nil {
		return nil, err
	}
	b.Refunded = true
	if err := s.k.SetBid(ctx, b); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "refund",
		"auction_id", u64s(a.Id),
		"bid_id", u64s(b.Id),
		"bidder", m.Bidder,
		"amount", u64s(refund),
	)
	return &types.MsgWithdrawRefundResponse{Refunded: refund}, nil
}

// ---- UpdateParams -----------------------------------------------------

func (s msgServer) UpdateParams(goCtx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
