package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxContracts:                 DefaultMaxContracts,
		MaxContractsPerParty:         DefaultMaxContractsPerParty,
		MinSettlementPeriodSeconds:   DefaultMinSettlementPeriod,
		MaxSettlementPeriodSeconds:   DefaultMaxSettlementPeriod,
		DefaultGracePeriodSeconds:    DefaultGracePeriodSeconds,
		MaxOracleStalenessSecondsCap: DefaultMaxOracleStalenessCap,
		MaxCatchupPeriodsPerSettle:   DefaultMaxCatchupPerSettle,
		MemoMaxLen:                   DefaultMemoMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxContracts == 0 || p.MaxContracts > HardMaxContracts {
		return fmt.Errorf("max_contracts must be in (0, %d]", HardMaxContracts)
	}
	if p.MaxContractsPerParty == 0 || p.MaxContractsPerParty > HardMaxContractsPerParty {
		return fmt.Errorf("max_contracts_per_party must be in (0, %d]", HardMaxContractsPerParty)
	}
	if p.MinSettlementPeriodSeconds < HardMinSettlementPeriod {
		return fmt.Errorf("min_settlement_period_seconds must be >= %d", HardMinSettlementPeriod)
	}
	if p.MaxSettlementPeriodSeconds <= 0 || p.MaxSettlementPeriodSeconds > HardMaxSettlementPeriod {
		return fmt.Errorf("max_settlement_period_seconds out of range")
	}
	if p.MinSettlementPeriodSeconds > p.MaxSettlementPeriodSeconds {
		return fmt.Errorf("min > max settlement_period_seconds")
	}
	if p.DefaultGracePeriodSeconds <= 0 {
		return fmt.Errorf("default_grace_period_seconds must be > 0")
	}
	if p.MaxOracleStalenessSecondsCap <= 0 || p.MaxOracleStalenessSecondsCap > HardMaxOracleStalenessCap {
		return fmt.Errorf("max_oracle_staleness_seconds_cap out of range")
	}
	if p.MaxCatchupPeriodsPerSettle == 0 || p.MaxCatchupPeriodsPerSettle > HardMaxCatchupPerSettle {
		return fmt.Errorf("max_catchup_periods_per_settle out of range")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len must be <= %d", HardMemoMaxLen)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:         DefaultParams(),
		NextContractId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Contracts)) > gs.Params.MaxContracts {
		return fmt.Errorf("genesis: %d contracts > max %d", len(gs.Contracts), gs.Params.MaxContracts)
	}
	ids := map[uint64]bool{}
	perParty := map[string]uint32{}
	for _, c := range gs.Contracts {
		if c.Id == 0 {
			return fmt.Errorf("genesis: contract id must be > 0")
		}
		if ids[c.Id] {
			return fmt.Errorf("genesis: duplicate contract id %d", c.Id)
		}
		ids[c.Id] = true
		if c.Id >= gs.NextContractId {
			return fmt.Errorf("genesis: contract id %d >= next_contract_id %d", c.Id, gs.NextContractId)
		}
		if !KindValid(c.Kind) {
			return fmt.Errorf("genesis: contract %d invalid kind", c.Id)
		}
		if !StatusValid(c.Status) {
			return fmt.Errorf("genesis: contract %d invalid status", c.Id)
		}
		if err := ValidateAddr("buyer", c.Buyer); err != nil {
			return err
		}
		if err := ValidateAddr("seller", c.Seller); err != nil {
			return err
		}
		if c.Buyer == c.Seller {
			return fmt.Errorf("genesis: contract %d buyer == seller", c.Id)
		}
		if err := ValidateDenom(c.AssetDenom); err != nil {
			return fmt.Errorf("genesis: contract %d: %w", c.Id, err)
		}
		if c.SettlementPeriodSeconds < gs.Params.MinSettlementPeriodSeconds || c.SettlementPeriodSeconds > gs.Params.MaxSettlementPeriodSeconds {
			return fmt.Errorf("genesis: contract %d settlement_period out of range", c.Id)
		}
		if c.MaxOracleStalenessSeconds < 0 || c.MaxOracleStalenessSeconds > gs.Params.MaxOracleStalenessSecondsCap {
			return fmt.Errorf("genesis: contract %d max_oracle_staleness_seconds out of cap", c.Id)
		}
		if c.EndTime != 0 && c.EndTime <= c.StartTime {
			return fmt.Errorf("genesis: contract %d end_time <= start_time", c.Id)
		}
		if c.MarginBuyer > c.MarginRequirement*4 || c.MarginSeller > c.MarginRequirement*4 {
			// Loose sanity bound — a margin balance many times
			// the requirement signals likely import corruption.
			// Multiplier of 4 is intentionally generous to allow
			// large excess deposits that haven't been withdrawn.
			return fmt.Errorf("genesis: contract %d margin balance implausibly large", c.Id)
		}
		perParty[c.Buyer]++
		perParty[c.Seller]++
		if perParty[c.Buyer] > gs.Params.MaxContractsPerParty || perParty[c.Seller] > gs.Params.MaxContractsPerParty {
			return fmt.Errorf("genesis: party cap exceeded")
		}
	}
	return nil
}
