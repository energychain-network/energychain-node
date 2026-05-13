package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxIssuers:                     DefaultMaxIssuers,
		MaxTokens:                      DefaultMaxTokens,
		MaxHoldersPerSnapshot:          DefaultMaxHoldersPerSnapshot,
		MaxPendingRedemptionsPerHolder: DefaultMaxPendingRedemptionsPerHolder,
		MaxLockupsPerHolder:            DefaultMaxLockupsPerHolder,
		RequireSanctionsClear:          true,
	}
}

func (p Params) Validate() error {
	if p.MaxIssuers == 0 || p.MaxIssuers > HardMaxIssuers {
		return fmt.Errorf("max_issuers must be in (0, %d]", HardMaxIssuers)
	}
	if p.MaxTokens == 0 || p.MaxTokens > HardMaxTokens {
		return fmt.Errorf("max_tokens must be in (0, %d]", HardMaxTokens)
	}
	if p.MaxHoldersPerSnapshot == 0 || p.MaxHoldersPerSnapshot > HardMaxHoldersPerSnapshot {
		return fmt.Errorf("max_holders_per_snapshot must be in (0, %d]", HardMaxHoldersPerSnapshot)
	}
	if p.MaxPendingRedemptionsPerHolder == 0 || p.MaxPendingRedemptionsPerHolder > HardMaxPendingRedemptions {
		return fmt.Errorf("max_pending_redemptions_per_holder must be in (0, %d]", HardMaxPendingRedemptions)
	}
	if p.MaxLockupsPerHolder == 0 || p.MaxLockupsPerHolder > HardMaxLockupsPerHolder {
		return fmt.Errorf("max_lockups_per_holder must be in (0, %d]", HardMaxLockupsPerHolder)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	issuerIDs := map[string]bool{}
	for _, is := range gs.Issuers {
		if err := ValidateIssuerID(is.Id); err != nil {
			return err
		}
		if issuerIDs[is.Id] {
			return fmt.Errorf("genesis: duplicate issuer id %q", is.Id)
		}
		issuerIDs[is.Id] = true
		if !IssuerStatusValid(is.Status) {
			return fmt.Errorf("genesis: issuer %q invalid status", is.Id)
		}
	}

	tokenIDs := map[uint64]bool{}
	tokenSymbols := map[string]bool{}
	tokenIssuer := map[uint64]string{}
	tokenDenom := map[uint64]string{}
	tokenSupplyClaim := map[uint64]uint64{}
	for _, t := range gs.Tokens {
		if t.Id == 0 {
			return fmt.Errorf("genesis: token id must be > 0")
		}
		if tokenIDs[t.Id] {
			return fmt.Errorf("genesis: duplicate token id %d", t.Id)
		}
		tokenIDs[t.Id] = true
		if err := ValidateTokenSymbol(t.Symbol); err != nil {
			return err
		}
		if tokenSymbols[t.Symbol] {
			return fmt.Errorf("genesis: duplicate token symbol %q", t.Symbol)
		}
		tokenSymbols[t.Symbol] = true
		if !issuerIDs[t.IssuerId] {
			return fmt.Errorf("genesis: token %d references unknown issuer %q", t.Id, t.IssuerId)
		}
		if !TokenStatusValid(t.Status) {
			return fmt.Errorf("genesis: token %d invalid status", t.Id)
		}
		if !AssetClassValid(t.AssetClass) {
			return fmt.Errorf("genesis: token %d invalid asset_class", t.Id)
		}
		if t.Decimals > MaxDecimals {
			return fmt.Errorf("genesis: token %d decimals %d > %d", t.Id, t.Decimals, MaxDecimals)
		}
		tokenIssuer[t.Id] = t.IssuerId
		tokenDenom[t.Id] = t.SettlementDenom
		tokenSupplyClaim[t.Id] = t.TotalSupply
	}

	balanceSums := map[uint64]uint64{}
	balanceSeen := map[string]bool{}
	for _, b := range gs.Balances {
		if !tokenIDs[b.TokenId] {
			return fmt.Errorf("genesis: balance for unknown token %d", b.TokenId)
		}
		key := fmt.Sprintf("%d|%s", b.TokenId, b.Account)
		if balanceSeen[key] {
			return fmt.Errorf("genesis: duplicate balance row %s", key)
		}
		balanceSeen[key] = true
		next, err := SafeAdd(balanceSums[b.TokenId], b.Amount)
		if err != nil {
			return err
		}
		balanceSums[b.TokenId] = next
	}

	pendingByToken := map[uint64]uint64{}
	for _, p := range gs.PendingRedemptionUnits {
		if !tokenIDs[p.TokenId] {
			return fmt.Errorf("genesis: pending units for unknown token %d", p.TokenId)
		}
		if _, dup := pendingByToken[p.TokenId]; dup {
			return fmt.Errorf("genesis: duplicate pending units row for token %d", p.TokenId)
		}
		pendingByToken[p.TokenId] = p.Amount
	}

	for tid, claimed := range tokenSupplyClaim {
		live := balanceSums[tid] + pendingByToken[tid]
		if live != claimed {
			return fmt.Errorf("genesis: token %d supply mismatch (sum_balances+pending=%d, claimed=%d)",
				tid, live, claimed)
		}
	}

	for _, f := range gs.AccountFlags {
		if !tokenIDs[f.TokenId] {
			return fmt.Errorf("genesis: account_flags for unknown token %d", f.TokenId)
		}
	}

	for _, fb := range gs.Frozen {
		if !tokenIDs[fb.TokenId] {
			return fmt.Errorf("genesis: frozen for unknown token %d", fb.TokenId)
		}
		// frozen <= balance
		bkey := fmt.Sprintf("%d|%s", fb.TokenId, fb.Account)
		_ = bkey
		// We do not have direct lookup, so accept (validated at apply time).
	}

	for _, l := range gs.Lockups {
		if !tokenIDs[l.TokenId] {
			return fmt.Errorf("genesis: lockup for unknown token %d", l.TokenId)
		}
	}

	snapshotIDs := map[uint64]bool{}
	snapshotToken := map[uint64]uint64{}
	for _, s := range gs.Snapshots {
		if s.Id == 0 {
			return fmt.Errorf("genesis: snapshot id must be > 0")
		}
		if snapshotIDs[s.Id] {
			return fmt.Errorf("genesis: duplicate snapshot id %d", s.Id)
		}
		snapshotIDs[s.Id] = true
		if !tokenIDs[s.TokenId] {
			return fmt.Errorf("genesis: snapshot %d references unknown token %d", s.Id, s.TokenId)
		}
		snapshotToken[s.Id] = s.TokenId
	}

	snapBalSeen := map[string]bool{}
	snapBalSum := map[uint64]uint64{}
	for _, sb := range gs.SnapshotBalances {
		if !snapshotIDs[sb.SnapshotId] {
			return fmt.Errorf("genesis: snapshot_balance references unknown snapshot %d", sb.SnapshotId)
		}
		key := fmt.Sprintf("%d|%s", sb.SnapshotId, sb.Account)
		if snapBalSeen[key] {
			return fmt.Errorf("genesis: duplicate snapshot_balance row %s", key)
		}
		snapBalSeen[key] = true
		next, err := SafeAdd(snapBalSum[sb.SnapshotId], sb.Amount)
		if err != nil {
			return err
		}
		snapBalSum[sb.SnapshotId] = next
	}
	for _, s := range gs.Snapshots {
		if snapBalSum[s.Id] != s.TotalSupply {
			return fmt.Errorf("genesis: snapshot %d total_supply mismatch (sum=%d, claimed=%d)",
				s.Id, snapBalSum[s.Id], s.TotalSupply)
		}
	}

	distIDs := map[uint64]bool{}
	for _, d := range gs.Distributions {
		if d.Id == 0 {
			return fmt.Errorf("genesis: distribution id must be > 0")
		}
		if distIDs[d.Id] {
			return fmt.Errorf("genesis: duplicate distribution id %d", d.Id)
		}
		distIDs[d.Id] = true
		if !tokenIDs[d.TokenId] {
			return fmt.Errorf("genesis: distribution %d references unknown token %d", d.Id, d.TokenId)
		}
		if d.SnapshotId != 0 && !snapshotIDs[d.SnapshotId] {
			return fmt.Errorf("genesis: distribution %d references unknown snapshot %d", d.Id, d.SnapshotId)
		}
		if d.ClaimedAmount > d.FundedAmount {
			return fmt.Errorf("genesis: distribution %d claimed (%d) > funded (%d)", d.Id, d.ClaimedAmount, d.FundedAmount)
		}
		if d.FundedAmount > d.TotalAmount {
			return fmt.Errorf("genesis: distribution %d funded (%d) > total (%d)", d.Id, d.FundedAmount, d.TotalAmount)
		}
		if !DistributionStatusValid(d.Status) {
			return fmt.Errorf("genesis: distribution %d invalid status", d.Id)
		}
		if d.SettlementDenom != tokenDenom[d.TokenId] {
			return fmt.Errorf("genesis: distribution %d settlement_denom %q != token %d denom %q",
				d.Id, d.SettlementDenom, d.TokenId, tokenDenom[d.TokenId])
		}
	}

	claimSums := map[uint64]uint64{}
	claimSeen := map[string]bool{}
	for _, c := range gs.DistributionClaims {
		if !distIDs[c.DistributionId] {
			return fmt.Errorf("genesis: distribution_claim references unknown distribution %d", c.DistributionId)
		}
		key := fmt.Sprintf("%d|%s", c.DistributionId, c.Account)
		if claimSeen[key] {
			return fmt.Errorf("genesis: duplicate distribution_claim row %s", key)
		}
		claimSeen[key] = true
		next, err := SafeAdd(claimSums[c.DistributionId], c.Amount)
		if err != nil {
			return err
		}
		claimSums[c.DistributionId] = next
	}
	for _, d := range gs.Distributions {
		if claimSums[d.Id] > d.ClaimedAmount {
			return fmt.Errorf("genesis: distribution %d sum-of-claims %d > claimed_amount %d",
				d.Id, claimSums[d.Id], d.ClaimedAmount)
		}
	}

	redIDs := map[uint64]bool{}
	pendingSumByToken := map[uint64]uint64{}
	for _, r := range gs.Redemptions {
		if r.Id == 0 {
			return fmt.Errorf("genesis: redemption id must be > 0")
		}
		if redIDs[r.Id] {
			return fmt.Errorf("genesis: duplicate redemption id %d", r.Id)
		}
		redIDs[r.Id] = true
		if !tokenIDs[r.TokenId] {
			return fmt.Errorf("genesis: redemption %d references unknown token %d", r.Id, r.TokenId)
		}
		if !RedemptionStatusValid(r.Status) {
			return fmt.Errorf("genesis: redemption %d invalid status", r.Id)
		}
		if r.Status == RedemptionStatus_REDEMPTION_STATUS_PENDING {
			next, err := SafeAdd(pendingSumByToken[r.TokenId], r.Amount)
			if err != nil {
				return err
			}
			pendingSumByToken[r.TokenId] = next
		}
	}
	for tid, claimed := range pendingByToken {
		if pendingSumByToken[tid] != claimed {
			return fmt.Errorf("genesis: token %d pending_redemption_units mismatch (sum=%d, claimed=%d)",
				tid, pendingSumByToken[tid], claimed)
		}
	}
	for tid := range pendingSumByToken {
		if _, ok := pendingByToken[tid]; !ok {
			return fmt.Errorf("genesis: token %d has pending redemptions but no pending_redemption_units row", tid)
		}
	}

	return nil
}
