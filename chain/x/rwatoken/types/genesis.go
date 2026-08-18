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

	// ---- tokens -------------------------------------------------------------
	tokens := map[uint64]Token{}
	symbols := map[string]bool{}
	var maxTokenID uint64
	for _, t := range gs.Tokens {
		if t.Id == 0 {
			return fmt.Errorf("genesis: token id must be > 0")
		}
		if _, dup := tokens[t.Id]; dup {
			return fmt.Errorf("genesis: duplicate token id %d", t.Id)
		}
		if err := ValidateSymbol(t.Symbol); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if symbols[t.Symbol] {
			return fmt.Errorf("genesis: duplicate token symbol %q", t.Symbol)
		}
		if err := ValidateAssetClass(t.AssetClass); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if _, err := sdk.AccAddressFromBech32(t.Admin); err != nil {
			return fmt.Errorf("genesis: token %d admin invalid: %w", t.Id, err)
		}
		if !TokenStatusValid(t.Status) {
			return fmt.Errorf("genesis: token %d invalid status", t.Id)
		}
		if err := ValidateSettlementDenom(t.SettlementDenom); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if t.Decimals > MaxDecimals {
			return fmt.Errorf("genesis: token %d decimals > %d", t.Id, MaxDecimals)
		}
		if err := ValidateOptionalPolicyID(t.PolicyId); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if err := ValidateRedemptionDelay(t.RedemptionDelaySeconds); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if err := ValidateMaturityTime(t.MaturityTime); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		if err := ValidateDeviceIDs(t.DeviceIds); err != nil {
			return fmt.Errorf("genesis: token %d: %w", t.Id, err)
		}
		tokens[t.Id] = t
		symbols[t.Symbol] = true
		if t.Id > maxTokenID {
			maxTokenID = t.Id
		}
	}
	if uint32(len(gs.Tokens)) > gs.Params.MaxTokens {
		return fmt.Errorf("genesis: tokens exceed max_tokens")
	}

	// ---- balances -----------------------------------------------------------
	computed := map[uint64]uint64{} // tokenID -> sum of balances
	escrow := map[uint64]uint64{}   // tokenID -> RedemptionEscrow balance
	seenBal := map[string]bool{}
	for _, b := range gs.Balances {
		if _, ok := tokens[b.TokenId]; !ok {
			return fmt.Errorf("genesis: balance references unknown token %d", b.TokenId)
		}
		if b.Holder == RedemptionEscrow {
			escrow[b.TokenId] += b.Amount
		} else if _, err := sdk.AccAddressFromBech32(b.Holder); err != nil {
			return fmt.Errorf("genesis: balance holder %q invalid: %w", b.Holder, err)
		}
		key := fmt.Sprintf("%d|%s", b.TokenId, b.Holder)
		if seenBal[key] {
			return fmt.Errorf("genesis: duplicate balance %s", key)
		}
		seenBal[key] = true
		if b.Amount == 0 {
			return fmt.Errorf("genesis: zero balance row %s should be omitted", key)
		}
		sum, err := SafeAdd(computed[b.TokenId], b.Amount)
		if err != nil {
			return fmt.Errorf("genesis: token %d balance overflow", b.TokenId)
		}
		computed[b.TokenId] = sum
	}
	for id, t := range tokens {
		if t.TotalSupply != computed[id] {
			return fmt.Errorf("genesis: token %d total_supply %d != sum of balances %d", id, t.TotalSupply, computed[id])
		}
	}

	// ---- flags --------------------------------------------------------------
	for _, f := range gs.Flags {
		if _, ok := tokens[f.TokenId]; !ok {
			return fmt.Errorf("genesis: flags reference unknown token %d", f.TokenId)
		}
		if _, err := sdk.AccAddressFromBech32(f.Holder); err != nil {
			return fmt.Errorf("genesis: flags holder invalid: %w", err)
		}
		if !f.Frozen {
			return fmt.Errorf("genesis: cleared flags row for %s should be omitted", f.Holder)
		}
	}

	// ---- snapshots ----------------------------------------------------------
	snapshots := map[uint64]Snapshot{}
	var maxSnapID uint64
	for _, s := range gs.Snapshots {
		if s.Id == 0 {
			return fmt.Errorf("genesis: snapshot id must be > 0")
		}
		if _, dup := snapshots[s.Id]; dup {
			return fmt.Errorf("genesis: duplicate snapshot id %d", s.Id)
		}
		if _, ok := tokens[s.TokenId]; !ok {
			return fmt.Errorf("genesis: snapshot %d references unknown token %d", s.Id, s.TokenId)
		}
		snapshots[s.Id] = s
		if s.Id > maxSnapID {
			maxSnapID = s.Id
		}
	}
	snapComputed := map[uint64]uint64{}
	snapCount := map[uint64]uint32{}
	seenSnapBal := map[string]bool{}
	for _, sb := range gs.SnapshotBalances {
		if _, ok := snapshots[sb.SnapshotId]; !ok {
			return fmt.Errorf("genesis: snapshot_balance references unknown snapshot %d", sb.SnapshotId)
		}
		if _, err := sdk.AccAddressFromBech32(sb.Holder); err != nil {
			return fmt.Errorf("genesis: snapshot_balance holder invalid: %w", err)
		}
		key := fmt.Sprintf("%d|%s", sb.SnapshotId, sb.Holder)
		if seenSnapBal[key] {
			return fmt.Errorf("genesis: duplicate snapshot_balance %s", key)
		}
		seenSnapBal[key] = true
		if sb.Amount == 0 {
			return fmt.Errorf("genesis: zero snapshot_balance row %s should be omitted", key)
		}
		sum, err := SafeAdd(snapComputed[sb.SnapshotId], sb.Amount)
		if err != nil {
			return fmt.Errorf("genesis: snapshot %d balance overflow", sb.SnapshotId)
		}
		snapComputed[sb.SnapshotId] = sum
		snapCount[sb.SnapshotId]++
	}
	for id, s := range snapshots {
		if s.TotalSupply != snapComputed[id] {
			return fmt.Errorf("genesis: snapshot %d total_supply %d != sum %d", id, s.TotalSupply, snapComputed[id])
		}
		if s.HolderCount != snapCount[id] {
			return fmt.Errorf("genesis: snapshot %d holder_count %d != rows %d", id, s.HolderCount, snapCount[id])
		}
	}

	// ---- distributions ------------------------------------------------------
	dists := map[uint64]Distribution{}
	var maxDistID uint64
	for _, d := range gs.Distributions {
		if d.Id == 0 {
			return fmt.Errorf("genesis: distribution id must be > 0")
		}
		if _, dup := dists[d.Id]; dup {
			return fmt.Errorf("genesis: duplicate distribution id %d", d.Id)
		}
		if _, ok := tokens[d.TokenId]; !ok {
			return fmt.Errorf("genesis: distribution %d references unknown token %d", d.Id, d.TokenId)
		}
		snap, ok := snapshots[d.SnapshotId]
		if !ok {
			return fmt.Errorf("genesis: distribution %d references unknown snapshot %d", d.Id, d.SnapshotId)
		}
		// The distribution must price against a snapshot of its OWN token,
		// otherwise pro-rata claims would draw token A's dividend pool using
		// token B's snapshot balances.
		if snap.TokenId != d.TokenId {
			return fmt.Errorf("genesis: distribution %d snapshot %d belongs to token %d not %d", d.Id, d.SnapshotId, snap.TokenId, d.TokenId)
		}
		if d.TotalAmount == 0 {
			return fmt.Errorf("genesis: distribution %d total_amount must be > 0", d.Id)
		}
		if d.ClaimedAmount > d.TotalAmount {
			return fmt.Errorf("genesis: distribution %d claimed %d > total %d", d.Id, d.ClaimedAmount, d.TotalAmount)
		}
		dists[d.Id] = d
		if d.Id > maxDistID {
			maxDistID = d.Id
		}
	}
	claimSum := map[uint64]uint64{}
	seenClaim := map[string]bool{}
	for _, c := range gs.DistributionClaims {
		if _, ok := dists[c.DistributionId]; !ok {
			return fmt.Errorf("genesis: claim references unknown distribution %d", c.DistributionId)
		}
		if _, err := sdk.AccAddressFromBech32(c.Holder); err != nil {
			return fmt.Errorf("genesis: claim holder invalid: %w", err)
		}
		key := fmt.Sprintf("%d|%s", c.DistributionId, c.Holder)
		if seenClaim[key] {
			return fmt.Errorf("genesis: duplicate claim %s", key)
		}
		seenClaim[key] = true
		sum, err := SafeAdd(claimSum[c.DistributionId], c.Amount)
		if err != nil {
			return fmt.Errorf("genesis: distribution %d claim overflow", c.DistributionId)
		}
		claimSum[c.DistributionId] = sum
	}
	for id, d := range dists {
		if claimSum[id] != d.ClaimedAmount {
			return fmt.Errorf("genesis: distribution %d claimed_amount %d != sum of claims %d", id, d.ClaimedAmount, claimSum[id])
		}
	}

	// ---- redemptions --------------------------------------------------------
	var maxRedID uint64
	redIDs := map[uint64]bool{}
	pendingUnits := map[uint64]uint64{} // tokenID -> sum of pending units
	for _, r := range gs.Redemptions {
		if r.Id == 0 {
			return fmt.Errorf("genesis: redemption id must be > 0")
		}
		if redIDs[r.Id] {
			return fmt.Errorf("genesis: duplicate redemption id %d", r.Id)
		}
		redIDs[r.Id] = true
		if _, ok := tokens[r.TokenId]; !ok {
			return fmt.Errorf("genesis: redemption %d references unknown token %d", r.Id, r.TokenId)
		}
		if _, err := sdk.AccAddressFromBech32(r.Holder); err != nil {
			return fmt.Errorf("genesis: redemption %d holder invalid: %w", r.Id, err)
		}
		switch r.Status {
		case RedemptionStatus_REDEMPTION_STATUS_PENDING:
			if r.Units == 0 {
				return fmt.Errorf("genesis: pending redemption %d units must be > 0", r.Id)
			}
			sum, err := SafeAdd(pendingUnits[r.TokenId], r.Units)
			if err != nil {
				return fmt.Errorf("genesis: token %d pending units overflow", r.TokenId)
			}
			pendingUnits[r.TokenId] = sum
		case RedemptionStatus_REDEMPTION_STATUS_SETTLED, RedemptionStatus_REDEMPTION_STATUS_CANCELLED:
			// Resolved redemptions are inert history: their units were already
			// burned (settled) or returned (cancelled) and are NOT escrowed,
			// so they must not contribute to the escrow/pending invariant
			// below. resolved_at must be set so they cannot be re-driven.
			if r.ResolvedAt == 0 {
				return fmt.Errorf("genesis: resolved redemption %d missing resolved_at", r.Id)
			}
		default:
			return fmt.Errorf("genesis: redemption %d has invalid status %s", r.Id, r.Status)
		}
		if r.Id > maxRedID {
			maxRedID = r.Id
		}
	}

	// Escrow invariant: the RedemptionEscrow balance for each token must
	// equal the sum of its PENDING redemption units. This stops a crafted
	// genesis from seeding redeemable units with no escrowed balance (which
	// a later cancel would credit out of thin air).
	tokensWithEscrow := map[uint64]bool{}
	for id := range escrow {
		tokensWithEscrow[id] = true
	}
	for id := range pendingUnits {
		tokensWithEscrow[id] = true
	}
	for id := range tokensWithEscrow {
		if escrow[id] != pendingUnits[id] {
			return fmt.Errorf("genesis: token %d escrow balance %d != pending redemption units %d", id, escrow[id], pendingUnits[id])
		}
	}

	// ---- issuers --------------------------------------------------------------
	seenIssuer := map[string]bool{}
	for _, iss := range gs.Issuers {
		if _, err := sdk.AccAddressFromBech32(iss); err != nil {
			return fmt.Errorf("genesis: issuer %q invalid: %w", iss, err)
		}
		if seenIssuer[iss] {
			return fmt.Errorf("genesis: duplicate issuer %s", iss)
		}
		seenIssuer[iss] = true
	}

	// ---- sequences ----------------------------------------------------------
	if gs.TokenIdSeq < maxTokenID {
		return fmt.Errorf("genesis: token_id_seq %d < max id %d", gs.TokenIdSeq, maxTokenID)
	}
	if gs.SnapshotIdSeq < maxSnapID {
		return fmt.Errorf("genesis: snapshot_id_seq %d < max id %d", gs.SnapshotIdSeq, maxSnapID)
	}
	if gs.DistributionIdSeq < maxDistID {
		return fmt.Errorf("genesis: distribution_id_seq %d < max id %d", gs.DistributionIdSeq, maxDistID)
	}
	if gs.RedemptionIdSeq < maxRedID {
		return fmt.Errorf("genesis: redemption_id_seq %d < max id %d", gs.RedemptionIdSeq, maxRedID)
	}

	return nil
}
