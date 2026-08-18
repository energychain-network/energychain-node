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

	denoms := map[string]bool{}
	for _, d := range gs.Denoms {
		if err := ValidateDenomID(d.Id); err != nil {
			return fmt.Errorf("genesis: denom %q: %w", d.Id, err)
		}
		if denoms[d.Id] {
			return fmt.Errorf("genesis: duplicate denom %q", d.Id)
		}
		denoms[d.Id] = true
		if _, err := sdk.AccAddressFromBech32(d.Admin); err != nil {
			return fmt.Errorf("genesis: denom %q admin invalid: %w", d.Id, err)
		}
		if !DenomStatusValid(d.Status) {
			return fmt.Errorf("genesis: denom %q invalid status", d.Id)
		}
		if uint32(len(d.Minters)) > gs.Params.MaxMintersPerDenom {
			return fmt.Errorf("genesis: denom %q exceeds max minters", d.Id)
		}
		for _, mnt := range d.Minters {
			if _, err := sdk.AccAddressFromBech32(mnt); err != nil {
				return fmt.Errorf("genesis: denom %q minter %q invalid: %w", d.Id, mnt, err)
			}
		}
	}
	if uint32(len(gs.Denoms)) > gs.Params.MaxDenoms {
		return fmt.Errorf("genesis: denoms exceed max")
	}

	// Balances: every row references a known denom; accumulate per-denom
	// totals so they can be checked against the declared supply.
	computed := map[string]uint64{}
	escrow := map[string]uint64{} // per-denom escrow balance
	seenBal := map[string]bool{}
	for _, b := range gs.Balances {
		if !denoms[b.DenomId] {
			return fmt.Errorf("genesis: balance references unknown denom %q", b.DenomId)
		}
		// The reserved redemption-escrow account is module-owned and not a
		// bech32 address; its balance must equal the sum of PENDING
		// redemption amounts (checked below).
		if b.Account == RedemptionEscrow {
			escrow[b.DenomId] += b.Amount
		} else if _, err := sdk.AccAddressFromBech32(b.Account); err != nil {
			return fmt.Errorf("genesis: balance account %q invalid: %w", b.Account, err)
		}
		key := b.DenomId + "|" + b.Account
		if seenBal[key] {
			return fmt.Errorf("genesis: duplicate balance %s", key)
		}
		seenBal[key] = true
		if b.Amount == 0 {
			return fmt.Errorf("genesis: zero balance row %s should be omitted", key)
		}
		sum, err := SafeAdd(computed[b.DenomId], b.Amount)
		if err != nil {
			return fmt.Errorf("genesis: denom %q balance overflow", b.DenomId)
		}
		computed[b.DenomId] = sum
	}

	declared := map[string]uint64{}
	for _, s := range gs.Supplies {
		if !denoms[s.DenomId] {
			return fmt.Errorf("genesis: supply references unknown denom %q", s.DenomId)
		}
		if _, dup := declared[s.DenomId]; dup {
			return fmt.Errorf("genesis: duplicate supply for %q", s.DenomId)
		}
		declared[s.DenomId] = s.Amount
	}
	for denom, sum := range computed {
		if declared[denom] != sum {
			return fmt.Errorf("genesis: denom %q supply %d != sum of balances %d", denom, declared[denom], sum)
		}
	}
	for denom, total := range declared {
		if total != 0 && computed[denom] == 0 {
			return fmt.Errorf("genesis: denom %q supply %d but no balances", denom, total)
		}
	}

	for _, a := range gs.Allowances {
		if !denoms[a.DenomId] {
			return fmt.Errorf("genesis: allowance references unknown denom %q", a.DenomId)
		}
		if _, err := sdk.AccAddressFromBech32(a.Owner); err != nil {
			return fmt.Errorf("genesis: allowance owner invalid: %w", err)
		}
		if _, err := sdk.AccAddressFromBech32(a.Spender); err != nil {
			return fmt.Errorf("genesis: allowance spender invalid: %w", err)
		}
		if a.Amount == 0 {
			return fmt.Errorf("genesis: zero allowance row should be omitted")
		}
	}

	for _, f := range gs.Flags {
		if !denoms[f.DenomId] {
			return fmt.Errorf("genesis: flags reference unknown denom %q", f.DenomId)
		}
		if _, err := sdk.AccAddressFromBech32(f.Account); err != nil {
			return fmt.Errorf("genesis: flags account invalid: %w", err)
		}
		if !f.Frozen && !f.Blacklisted {
			return fmt.Errorf("genesis: cleared flags row for %s should be omitted", f.Account)
		}
	}

	var maxID uint64
	ids := map[uint64]bool{}
	pendingEscrow := map[string]uint64{} // per-denom sum of PENDING amounts
	for _, r := range gs.Redemptions {
		if r.Id == 0 {
			return fmt.Errorf("genesis: redemption id must be > 0")
		}
		if ids[r.Id] {
			return fmt.Errorf("genesis: duplicate redemption id %d", r.Id)
		}
		ids[r.Id] = true
		if r.Id > maxID {
			maxID = r.Id
		}
		if !denoms[r.DenomId] {
			return fmt.Errorf("genesis: redemption %d references unknown denom %q", r.Id, r.DenomId)
		}
		if _, err := sdk.AccAddressFromBech32(r.Holder); err != nil {
			return fmt.Errorf("genesis: redemption %d holder invalid: %w", r.Id, err)
		}
		if r.Status == RedemptionStatus_REDEMPTION_STATUS_PENDING {
			if r.Amount == 0 {
				return fmt.Errorf("genesis: pending redemption %d amount must be > 0", r.Id)
			}
			sum, err := SafeAdd(pendingEscrow[r.DenomId], r.Amount)
			if err != nil {
				return fmt.Errorf("genesis: denom %q pending escrow overflow", r.DenomId)
			}
			pendingEscrow[r.DenomId] = sum
		}
	}
	if gs.RedemptionIdSeq < maxID {
		return fmt.Errorf("genesis: redemption_id_seq %d < max id %d", gs.RedemptionIdSeq, maxID)
	}

	// Escrow invariant: the reserved escrow balance for each denom must
	// exactly equal the sum of its PENDING redemption amounts. This stops a
	// crafted genesis from seeding redeemable "phantom supply" (a pending
	// redemption with no escrowed tokens) that a later cancel would release.
	denomsWithEscrow := map[string]bool{}
	for denom := range escrow {
		denomsWithEscrow[denom] = true
	}
	for denom := range pendingEscrow {
		denomsWithEscrow[denom] = true
	}
	for denom := range denomsWithEscrow {
		if escrow[denom] != pendingEscrow[denom] {
			return fmt.Errorf("genesis: denom %q escrow balance %d != pending redemptions %d",
				denom, escrow[denom], pendingEscrow[denom])
		}
	}

	return nil
}
