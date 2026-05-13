package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxIssuers:                     DefaultMaxIssuers,
		MaxCategoriesPerIssuer:         DefaultMaxCategoriesPerIssuer,
		MaxUnitsPerBatch:               DefaultMaxUnitsPerBatch,
		MaxCcpLabels:                   DefaultMaxCCPLabels,
		RequireSanctionsClear:          true,
		RequireArticle6ForCrossBorder:  true,
	}
}

func (p Params) Validate() error {
	if p.MaxIssuers == 0 || p.MaxIssuers > MaxIssuersUpper {
		return fmt.Errorf("max_issuers %d out of range (1..=%d)", p.MaxIssuers, MaxIssuersUpper)
	}
	if p.MaxCategoriesPerIssuer == 0 || p.MaxCategoriesPerIssuer > MaxCategoriesPerIssuerUpper {
		return fmt.Errorf("max_categories_per_issuer %d out of range (1..=%d)",
			p.MaxCategoriesPerIssuer, MaxCategoriesPerIssuerUpper)
	}
	if p.MaxUnitsPerBatch == 0 {
		return fmt.Errorf("max_units_per_batch must be > 0")
	}
	if p.MaxCcpLabels > 64 {
		return fmt.Errorf("max_ccp_labels %d > 64", p.MaxCcpLabels)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
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

	assets := map[uint64]Asset{}
	eacClaimedSums := map[uint64]uint64{}
	for _, a := range gs.Assets {
		if a.Id == 0 {
			return fmt.Errorf("asset id 0 is reserved")
		}
		if a.Id > gs.AssetIdSeq {
			return fmt.Errorf("asset id %d exceeds id seq %d", a.Id, gs.AssetIdSeq)
		}
		if _, dup := assets[a.Id]; dup {
			return fmt.Errorf("duplicate asset id %d", a.Id)
		}
		assets[a.Id] = a
		if !issuers[a.IssuerId] {
			return fmt.Errorf("asset %d references unknown issuer %q", a.Id, a.IssuerId)
		}
		if !AssetCategoryValid(a.Category) {
			return fmt.Errorf("asset %d invalid category", a.Id)
		}
		if !AssetStatusValid(a.Status) {
			return fmt.Errorf("asset %d invalid status", a.Id)
		}
		if !Article6StatusValid(a.Article6Status) {
			return fmt.Errorf("asset %d invalid article6_status", a.Id)
		}
		if a.RetiredUnits > a.IssuedUnits {
			return fmt.Errorf("asset %d retired %d > issued %d", a.Id, a.RetiredUnits, a.IssuedUnits)
		}
		if a.LinkedEacCertificateId != 0 {
			v, err := SafeAdd(eacClaimedSums[a.LinkedEacCertificateId], a.IssuedUnits)
			if err != nil {
				return fmt.Errorf("eac claim sum overflow for cert %d: %w", a.LinkedEacCertificateId, err)
			}
			eacClaimedSums[a.LinkedEacCertificateId] = v
		}
	}

	balanceSums := map[uint64]uint64{}
	for _, b := range gs.Balances {
		if _, ok := assets[b.AssetId]; !ok {
			return fmt.Errorf("balance references unknown asset %d", b.AssetId)
		}
		if b.Amount == 0 {
			return fmt.Errorf("balance for asset %d/%s is zero (should be pruned)", b.AssetId, b.Account)
		}
		v, err := SafeAdd(balanceSums[b.AssetId], b.Amount)
		if err != nil {
			return fmt.Errorf("balance sum overflow for asset %d: %w", b.AssetId, err)
		}
		balanceSums[b.AssetId] = v
	}
	for id, a := range assets {
		want, err := SafeSub(a.IssuedUnits, a.RetiredUnits)
		if err != nil {
			return err
		}
		if balanceSums[id] != want {
			return fmt.Errorf("asset %d invariant: sum(balances)=%d vs issued-retired=%d",
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
		if _, ok := assets[r.AssetId]; !ok {
			return fmt.Errorf("retirement %d references unknown asset %d", r.Id, r.AssetId)
		}
		if r.Amount == 0 {
			return fmt.Errorf("retirement %d has zero amount", r.Id)
		}
	}

	for _, br := range gs.Bridges {
		if _, ok := assets[br.AssetId]; !ok {
			return fmt.Errorf("bridge references unknown asset %d", br.AssetId)
		}
		if err := ValidateOracleTopic(br.OracleTopicId); err != nil {
			return err
		}
	}

	// EAC claims must reconcile with the per-asset linked EAC totals.
	claimsByEAC := map[uint64]uint64{}
	for _, c := range gs.EacClaims {
		if c.EacCertificateId == 0 {
			return fmt.Errorf("eac_claim references zero certificate id")
		}
		claimsByEAC[c.EacCertificateId] = c.ClaimedUnits
	}
	for ec, sum := range eacClaimedSums {
		if claimsByEAC[ec] != sum {
			return fmt.Errorf("eac_claim mismatch for certificate %d: claims=%d, derived=%d",
				ec, claimsByEAC[ec], sum)
		}
	}
	return nil
}
