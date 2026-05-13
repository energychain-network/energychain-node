package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxIssuers:            DefaultMaxIssuers,
		MaxKindsPerIssuer:     DefaultMaxKindsPerIssuer,
		MaxUnitsPerBatch:      uint32(min64(DefaultMaxUnitsPerBatch, uint64(^uint32(0)))),
		HourWindowSeconds:     DefaultHourWindowSeconds,
		RequireSanctionsClear: true,
	}
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func (p Params) Validate() error {
	if p.MaxIssuers == 0 || p.MaxIssuers > MaxIssuersUpper {
		return fmt.Errorf("max_issuers %d out of range (1..=%d)", p.MaxIssuers, MaxIssuersUpper)
	}
	if p.MaxKindsPerIssuer == 0 || p.MaxKindsPerIssuer > MaxKindsPerIssuerUpper {
		return fmt.Errorf("max_kinds_per_issuer %d out of range (1..=%d)", p.MaxKindsPerIssuer, MaxKindsPerIssuerUpper)
	}
	if p.MaxUnitsPerBatch == 0 {
		return fmt.Errorf("max_units_per_batch must be > 0")
	}
	// hour_window_seconds == 0 is allowed (disables the per-batch
	// window check) but values larger than a day are almost certainly
	// a misconfiguration.
	if p.HourWindowSeconds > 86_400 {
		return fmt.Errorf("hour_window_seconds %d > 86400 (one day)", p.HourWindowSeconds)
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
			return fmt.Errorf("issuer %q invalid status", is.Id)
		}
	}

	certs := map[uint64]Certificate{}
	for _, c := range gs.Certificates {
		if c.Id == 0 {
			return fmt.Errorf("certificate id 0 is reserved")
		}
		if c.Id > gs.CertificateIdSeq {
			return fmt.Errorf("certificate id %d exceeds id seq %d", c.Id, gs.CertificateIdSeq)
		}
		if _, dup := certs[c.Id]; dup {
			return fmt.Errorf("duplicate certificate id %d", c.Id)
		}
		certs[c.Id] = c
		if !issuers[c.IssuerId] {
			return fmt.Errorf("certificate %d references unknown issuer %q", c.Id, c.IssuerId)
		}
		if !CertificateKindValid(c.Kind) {
			return fmt.Errorf("certificate %d invalid kind", c.Id)
		}
		if !TechnologyValid(c.Technology) {
			return fmt.Errorf("certificate %d invalid technology", c.Id)
		}
		if !CertificateStatusValid(c.Status) {
			return fmt.Errorf("certificate %d invalid status", c.Id)
		}
		if c.RetiredUnits > c.IssuedUnits {
			return fmt.Errorf("certificate %d retired %d > issued %d", c.Id, c.RetiredUnits, c.IssuedUnits)
		}
	}

	balanceSums := map[uint64]uint64{}
	for _, b := range gs.Balances {
		c, ok := certs[b.CertificateId]
		if !ok {
			return fmt.Errorf("balance references unknown certificate %d", b.CertificateId)
		}
		if b.Amount == 0 {
			return fmt.Errorf("balance for cert %d/%s is zero (should be pruned)", b.CertificateId, b.Account)
		}
		v, err := SafeAdd(balanceSums[b.CertificateId], b.Amount)
		if err != nil {
			return fmt.Errorf("balance sum overflow for cert %d: %w", b.CertificateId, err)
		}
		balanceSums[b.CertificateId] = v
		_ = c
	}
	// Invariant: per certificate, sum(balances) MUST equal
	// issued_units - retired_units. Catches export drift early.
	for id, c := range certs {
		want, err := SafeSub(c.IssuedUnits, c.RetiredUnits)
		if err != nil {
			return fmt.Errorf("certificate %d retired > issued", id)
		}
		if balanceSums[id] != want {
			return fmt.Errorf("certificate %d invariant: sum(balances)=%d vs issued-retired=%d",
				id, balanceSums[id], want)
		}
	}

	for _, r := range gs.Retirements {
		if r.Id == 0 {
			return fmt.Errorf("retirement id 0 is reserved")
		}
		if r.Id > gs.RetirementIdSeq {
			return fmt.Errorf("retirement id %d exceeds id seq %d", r.Id, gs.RetirementIdSeq)
		}
		if _, ok := certs[r.CertificateId]; !ok {
			return fmt.Errorf("retirement %d references unknown certificate %d", r.Id, r.CertificateId)
		}
		if r.Amount == 0 {
			return fmt.Errorf("retirement %d has zero amount", r.Id)
		}
	}

	for _, br := range gs.Bridges {
		if _, ok := certs[br.CertificateId]; !ok {
			return fmt.Errorf("bridge references unknown certificate %d", br.CertificateId)
		}
		if err := ValidateOracleTopic(br.OracleTopicId); err != nil {
			return err
		}
	}
	return nil
}
