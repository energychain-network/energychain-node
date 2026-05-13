package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxDenoms:                       DefaultMaxDenoms,
		MaxIssuers:                      DefaultMaxIssuers,
		MaxMintAuthoritiesPerIssuer:     DefaultMaxMintAuthoritiesPerIssuer,
		MaxRedemptionsPendingPerHolder:  DefaultMaxRedemptionsPendingPerHolder,
		MintRequiresReserve:             true,
	}
}

func (p Params) Validate() error {
	if p.MaxDenoms == 0 || p.MaxDenoms > MaxDenomsUpper {
		return fmt.Errorf("max_denoms %d out of range (1..=%d)", p.MaxDenoms, MaxDenomsUpper)
	}
	if p.MaxIssuers == 0 || p.MaxIssuers > MaxIssuersUpper {
		return fmt.Errorf("max_issuers %d out of range (1..=%d)", p.MaxIssuers, MaxIssuersUpper)
	}
	if p.MaxMintAuthoritiesPerIssuer == 0 || p.MaxMintAuthoritiesPerIssuer > MaxMintAuthoritiesPerIssuerUpper {
		return fmt.Errorf("max_mint_authorities_per_issuer %d out of range (1..=%d)", p.MaxMintAuthoritiesPerIssuer, MaxMintAuthoritiesPerIssuerUpper)
	}
	if p.MaxRedemptionsPendingPerHolder == 0 || p.MaxRedemptionsPendingPerHolder > MaxRedemptionsPendingPerHolderUpper {
		return fmt.Errorf("max_redemptions_pending_per_holder %d out of range (1..=%d)", p.MaxRedemptionsPendingPerHolder, MaxRedemptionsPendingPerHolderUpper)
	}
	if p.RequireKycCredential != "" {
		if err := ValidateCredentialType(p.RequireKycCredential); err != nil {
			return err
		}
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}

	denoms := map[string]bool{}
	for _, d := range gs.Denoms {
		if err := ValidateDenomID(d.Id); err != nil {
			return err
		}
		if denoms[d.Id] {
			return fmt.Errorf("duplicate denom %q", d.Id)
		}
		denoms[d.Id] = true
		if !DenomStatusValid(d.Status) {
			return fmt.Errorf("denom %q has invalid status", d.Id)
		}
	}

	issuers := map[string]bool{}
	for _, is := range gs.Issuers {
		if err := ValidateIssuerID(is.Id); err != nil {
			return err
		}
		if issuers[is.Id] {
			return fmt.Errorf("duplicate issuer %q", is.Id)
		}
		issuers[is.Id] = true
		if !IssuerStatusValid(is.Status) {
			return fmt.Errorf("issuer %q has invalid status", is.Id)
		}
	}

	outstandingByDenom := map[string]uint64{}
	quotaSeen := map[string]bool{}
	for _, q := range gs.Quotas {
		if !issuers[q.IssuerId] {
			return fmt.Errorf("quota references unknown issuer %q", q.IssuerId)
		}
		if !denoms[q.DenomId] {
			return fmt.Errorf("quota references unknown denom %q", q.DenomId)
		}
		k := q.IssuerId + "|" + q.DenomId
		if quotaSeen[k] {
			return fmt.Errorf("duplicate quota (%s, %s)", q.IssuerId, q.DenomId)
		}
		quotaSeen[k] = true
		if q.Outstanding > q.Ceiling {
			return fmt.Errorf("quota (%s, %s) outstanding %d > ceiling %d",
				q.IssuerId, q.DenomId, q.Outstanding, q.Ceiling)
		}
		v, err := SafeAdd(outstandingByDenom[q.DenomId], q.Outstanding)
		if err != nil {
			return fmt.Errorf("outstanding sum overflow for denom %q: %w", q.DenomId, err)
		}
		outstandingByDenom[q.DenomId] = v
	}

	for _, r := range gs.Reserves {
		if !denoms[r.DenomId] {
			return fmt.Errorf("reserve references unknown denom %q", r.DenomId)
		}
		if err := ValidateOracleTopic(r.OracleTopicId); err != nil {
			return err
		}
		if r.RequiredRatioBps == 0 {
			return fmt.Errorf("reserve %q has zero required_ratio_bps", r.DenomId)
		}
	}

	for _, b := range gs.Balances {
		if !denoms[b.DenomId] {
			return fmt.Errorf("balance references unknown denom %q", b.DenomId)
		}
		if b.Amount == 0 {
			return fmt.Errorf("balance for %s/%s is zero (should be pruned)", b.DenomId, b.Account)
		}
	}

	supplies := map[string]uint64{}
	for _, s := range gs.Supplies {
		if !denoms[s.DenomId] {
			return fmt.Errorf("supply references unknown denom %q", s.DenomId)
		}
		if _, ok := supplies[s.DenomId]; ok {
			return fmt.Errorf("duplicate supply for denom %q", s.DenomId)
		}
		supplies[s.DenomId] = s.TotalSupply
	}

	// Cross-check supply against summed balances. This protects against
	// a malformed export where supply and balance ledgers drift apart.
	balanceSums := map[string]uint64{}
	for _, b := range gs.Balances {
		v, err := SafeAdd(balanceSums[b.DenomId], b.Amount)
		if err != nil {
			return fmt.Errorf("balance sum overflow for denom %q: %w", b.DenomId, err)
		}
		balanceSums[b.DenomId] = v
	}
	for d, sum := range balanceSums {
		if supplies[d] != sum {
			return fmt.Errorf("supply mismatch for denom %q: balances sum %d vs supply %d", d, sum, supplies[d])
		}
	}

	// Invariant: per denom, sum(outstanding) MUST equal supply when any
	// quota row exists for the denom. Skipping the check when no quota
	// row exists keeps the door open for greenfield genesis bootstraps
	// that seed balances directly without minting through an issuer
	// (e.g. test harnesses), but the burn path will then refuse the
	// first burn until a quota row is established.
	denomHasQuota := map[string]bool{}
	for _, q := range gs.Quotas {
		denomHasQuota[q.DenomId] = true
	}
	for d, sup := range supplies {
		if !denomHasQuota[d] {
			continue
		}
		if outstandingByDenom[d] != sup {
			return fmt.Errorf("invariant broken for denom %q: sum(outstanding)=%d vs supply=%d",
				d, outstandingByDenom[d], sup)
		}
	}

	for _, a := range gs.Allowances {
		if !denoms[a.DenomId] {
			return fmt.Errorf("allowance references unknown denom %q", a.DenomId)
		}
	}

	for _, f := range gs.Flags {
		if !denoms[f.DenomId] {
			return fmt.Errorf("flags references unknown denom %q", f.DenomId)
		}
	}

	for _, r := range gs.Redemptions {
		if !denoms[r.DenomId] {
			return fmt.Errorf("redemption %d references unknown denom %q", r.Id, r.DenomId)
		}
		if !issuers[r.IssuerId] {
			return fmt.Errorf("redemption %d references unknown issuer %q", r.Id, r.IssuerId)
		}
		if !RedemptionStatusValid(r.Status) {
			return fmt.Errorf("redemption %d has invalid status", r.Id)
		}
		if r.Id > gs.RedemptionIdSeq {
			return fmt.Errorf("redemption id %d exceeds id seq %d", r.Id, gs.RedemptionIdSeq)
		}
	}

	return nil
}
