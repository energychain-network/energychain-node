package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	markets := map[uint64]Market{}
	denoms := map[string]bool{}
	var maxMarketID uint64
	for _, m := range gs.Markets {
		if m.Id == 0 {
			return fmt.Errorf("genesis: market id must be > 0")
		}
		if _, dup := markets[m.Id]; dup {
			return fmt.Errorf("genesis: duplicate market id %d", m.Id)
		}
		if err := ValidateDenom(m.Denom); err != nil {
			return fmt.Errorf("genesis: market %d denom: %w", m.Id, err)
		}
		if denoms[m.Denom] {
			return fmt.Errorf("genesis: duplicate market denom %q", m.Denom)
		}
		denoms[m.Denom] = true
		if _, err := sdk.AccAddressFromBech32(m.Admin); err != nil {
			return fmt.Errorf("genesis: market %d admin: %w", m.Id, err)
		}
		if err := ValidateDenom(m.SettlementDenom); err != nil {
			return fmt.Errorf("genesis: market %d settlement_denom: %w", m.Id, err)
		}
		if m.InitialPrice == 0 {
			return fmt.Errorf("genesis: market %d initial_price must be > 0", m.Id)
		}
		if m.MintFeeBps > Bps || m.MeltFeeBps > Bps {
			return fmt.Errorf("genesis: market %d fees out of range", m.Id)
		}
		if !MarketStatusValid(m.Status) {
			return fmt.Errorf("genesis: market %d invalid status", m.Id)
		}
		if m.Supply > 0 && m.Treasury == 0 {
			return fmt.Errorf("genesis: market %d has supply but empty treasury", m.Id)
		}
		// Orphan treasury (backing with no supply) is the bootstrap-mint skim
		// precondition: the next bootstrap minter would inherit the whole
		// treasury for free. The runtime never produces this state (a full
		// melt drains the treasury), so reject it on import too.
		if m.Supply == 0 && m.Treasury > 0 {
			return fmt.Errorf("genesis: market %d has treasury but no supply (orphan backing)", m.Id)
		}
		if m.FloorPrice != FloorPrice(m.Treasury, m.Supply) {
			return fmt.Errorf("genesis: market %d floor_price %d != treasury/supply %d", m.Id, m.FloorPrice, FloorPrice(m.Treasury, m.Supply))
		}
		markets[m.Id] = m
		if m.Id > maxMarketID {
			maxMarketID = m.Id
		}
	}
	if uint32(len(gs.Markets)) > gs.Params.MaxMarkets {
		return fmt.Errorf("genesis: markets exceed max")
	}

	// balances: sum per market (incl. escrow) must equal the market supply.
	balSum := map[uint64]uint64{}
	escrow := map[uint64]uint64{}
	seen := map[string]bool{}
	for _, b := range gs.Balances {
		if _, ok := markets[b.MarketId]; !ok {
			return fmt.Errorf("genesis: balance references unknown market %d", b.MarketId)
		}
		if b.Holder != InvestEscrow {
			if _, err := sdk.AccAddressFromBech32(b.Holder); err != nil {
				return fmt.Errorf("genesis: balance holder invalid: %w", err)
			}
		}
		key := fmt.Sprintf("%d|%s", b.MarketId, b.Holder)
		if seen[key] {
			return fmt.Errorf("genesis: duplicate balance %s", key)
		}
		seen[key] = true
		if b.Amount == 0 {
			return fmt.Errorf("genesis: zero balance row %s", key)
		}
		s, err := SafeAdd(balSum[b.MarketId], b.Amount)
		if err != nil {
			return fmt.Errorf("genesis: market %d balance overflow", b.MarketId)
		}
		balSum[b.MarketId] = s
		if b.Holder == InvestEscrow {
			escrow[b.MarketId] = b.Amount
		}
	}
	for id, m := range markets {
		if balSum[id] != m.Supply {
			return fmt.Errorf("genesis: market %d supply %d != sum of balances %d", id, m.Supply, balSum[id])
		}
	}

	// invests: active locks must reconcile with the escrow balance.
	investEscrow := map[uint64]uint64{}
	investIDs := map[uint64]bool{}
	var maxInvestID uint64
	for _, iv := range gs.Invests {
		if iv.Id == 0 {
			return fmt.Errorf("genesis: invest id must be > 0")
		}
		if investIDs[iv.Id] {
			return fmt.Errorf("genesis: duplicate invest id %d", iv.Id)
		}
		investIDs[iv.Id] = true
		if _, ok := markets[iv.MarketId]; !ok {
			return fmt.Errorf("genesis: invest %d references unknown market %d", iv.Id, iv.MarketId)
		}
		if _, err := sdk.AccAddressFromBech32(iv.Investor); err != nil {
			return fmt.Errorf("genesis: invest %d investor invalid: %w", iv.Id, err)
		}
		if iv.PrincipalUnits == 0 {
			return fmt.Errorf("genesis: invest %d principal_units must be > 0", iv.Id)
		}
		if iv.ApyBps > gs.Params.MaxApyBps {
			return fmt.Errorf("genesis: invest %d apy_bps %d exceeds max %d", iv.Id, iv.ApyBps, gs.Params.MaxApyBps)
		}
		switch iv.Status {
		case InvestStatus_INVEST_STATUS_ACTIVE:
			if iv.Maturity <= iv.OpenedAt {
				return fmt.Errorf("genesis: invest %d maturity before open", iv.Id)
			}
			// Bound the owed yield: the floor only rises, so the current
			// floor-valued principal is an upper bound on the open-time value,
			// and the prorated rate over the lock term caps the payable yield.
			// This stops a crafted genesis from over-funding a future
			// CloseInvest from the reward pool.
			mk := markets[iv.MarketId]
			principalValue, err := MeltGross(mk.Treasury, mk.Supply, iv.PrincipalUnits)
			if err != nil {
				return fmt.Errorf("genesis: invest %d principal value: %w", iv.Id, err)
			}
			maxYield, err := ProrateYield(principalValue, iv.ApyBps, iv.Maturity-iv.OpenedAt, gs.Params.YearSeconds)
			if err != nil {
				return fmt.Errorf("genesis: invest %d yield bound: %w", iv.Id, err)
			}
			if iv.Yield > maxYield {
				return fmt.Errorf("genesis: invest %d yield %d exceeds bound %d", iv.Id, iv.Yield, maxYield)
			}
			s, err := SafeAdd(investEscrow[iv.MarketId], iv.PrincipalUnits)
			if err != nil {
				return fmt.Errorf("genesis: market %d invest escrow overflow", iv.MarketId)
			}
			investEscrow[iv.MarketId] = s
		case InvestStatus_INVEST_STATUS_REDEEMED, InvestStatus_INVEST_STATUS_CANCELLED:
			if iv.ResolvedAt == 0 {
				return fmt.Errorf("genesis: resolved invest %d missing resolved_at", iv.Id)
			}
		default:
			return fmt.Errorf("genesis: invest %d invalid status", iv.Id)
		}
		if iv.Id > maxInvestID {
			maxInvestID = iv.Id
		}
	}
	// The InvestEscrow balance for each market must equal the sum of its
	// ACTIVE locked principal — no phantom escrowed units, none missing.
	mset := map[uint64]bool{}
	for id := range escrow {
		mset[id] = true
	}
	for id := range investEscrow {
		mset[id] = true
	}
	for id := range mset {
		if escrow[id] != investEscrow[id] {
			return fmt.Errorf("genesis: market %d escrow balance %d != active invest units %d", id, escrow[id], investEscrow[id])
		}
	}

	if gs.MarketIdSeq < maxMarketID {
		return fmt.Errorf("genesis: market_id_seq %d < max id %d", gs.MarketIdSeq, maxMarketID)
	}
	if gs.InvestIdSeq < maxInvestID {
		return fmt.Errorf("genesis: invest_id_seq %d < max id %d", gs.InvestIdSeq, maxInvestID)
	}
	return nil
}
