package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/rwa/types"
)

// RWAEscrowPoolAccount is the deterministic 20-byte module address
// that holds RWA-token units locked into escrows owned by the
// x/escrow (or any cross-module) flow. The address is owned and
// minted-into by x/rwa itself so that holder-side compliance can
// continue to be enforced (KYC / per-holder cap) at the release
// leg without piercing into another module's keeper.
//
// Using a fixed bech32 derived from a well-known seed keeps it
// reproducible across genesis reseeds and exempts it from any
// chain prefix configuration change.
var rwaEscrowPoolAccount = authtypes.NewModuleAddress("rwa_escrow_pool").String()

// EscrowPoolAccount returns the canonical RWA-side escrow pool
// address. Exposed as a function (vs. a const var) so the wider
// codebase always reads from the single source of truth.
func EscrowPoolAccount() string { return rwaEscrowPoolAccount }

// _ keeps the unused import quiet when the file is reduced.
var _ = context.Background

// EscrowLock is the public primitive other modules (notably
// x/escrow) call to debit `depositor`'s transferable balance into
// the escrow pool. The pool is identified by a deterministic
// 20-byte module address owned by the calling module — passed in
// as `pool` rather than baked into x/rwa so different in-flight
// holding accounts (escrow / clearing / etc.) can co-exist.
//
// Compliance posture for the lock-leg:
//   - Token MUST be ACTIVE. PAUSED / TERMINATED tokens cannot have
//     new units leave their holder, period.
//   - Sender (depositor) is sanctions-checked when
//     RequireSanctionsClear is on.
//   - Transferable check (frozen + lockup) is honored — module-
//     controlled escrows cannot launder a frozen / locked balance.
//   - The pool is exempt from KYC / per-holder cap because it is
//     not a real holder; the credit is a temporary book entry that
//     EscrowRelease unwinds on payout.
//
// Callers MUST validate amount > 0; this method also guards.
func (k Keeper) EscrowLock(ctx sdk.Context, tokenID uint64, depositor string, amount uint64) error {
	if amount == 0 {
		return fmt.Errorf("escrow lock amount must be > 0")
	}
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("token %d not found", tokenID)
	}
	if t.Status != types.TokenStatus_TOKEN_STATUS_ACTIVE {
		return fmt.Errorf("token %d not active (status=%s)", t.Id, t.Status)
	}
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.sanctions != nil {
		if k.sanctions.IsSanctioned(ctx, depositor) {
			return fmt.Errorf("depositor %s is sanctioned", depositor)
		}
	}
	now := ctx.BlockTime().Unix()
	_, _, _, transferable, err := k.transferable(ctx, t.Id, depositor, now)
	if err != nil {
		return err
	}
	if transferable < amount {
		return fmt.Errorf("insufficient transferable balance: have %d, need %d (frozen / lockups apply)",
			transferable, amount)
	}
	if err := k.debitBalance(ctx, t.Id, depositor, amount); err != nil {
		return err
	}
	if err := k.creditBalance(ctx, t.Id, EscrowPoolAccount(), amount); err != nil {
		return err
	}
	k.recordAudit(ctx, t.Id, t.IssuerId, "escrow_lock", depositor, EscrowPoolAccount(), "")
	return nil
}

// EscrowRelease debits the escrow pool and credits `recipient`
// with full holder-side compliance.
//
// Compliance posture for the release-leg:
//   - Token MAY be ACTIVE or TERMINATED — we want winding-down
//     escrows to be able to settle even after the issuer marked the
//     token terminal. PAUSED is refused so a freeze can hold all
//     in-flight escrows.
//   - Recipient is sanctions-checked when RequireSanctionsClear is
//     on.
//   - KYC is enforced when the token requires KYC holders.
//   - Per-holder cap is enforced.
//   - Policy DSL is intentionally NOT re-evaluated on release — the
//     evaluation happened at the originating module's gate (e.g.
//     escrow create/release threshold met). Re-evaluating here
//     would let a policy churn block a settled trade and expose
//     funds to indefinite limbo.
func (k Keeper) EscrowRelease(ctx sdk.Context, tokenID uint64, recipient string, amount uint64) error {
	if amount == 0 {
		return fmt.Errorf("escrow release amount must be > 0")
	}
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("token %d not found", tokenID)
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_PAUSED {
		return fmt.Errorf("token %d is paused", t.Id)
	}
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.sanctions != nil {
		if k.sanctions.IsSanctioned(ctx, recipient) {
			return fmt.Errorf("recipient %s is sanctioned", recipient)
		}
	}
	if t.RequireKycHolders {
		if err := k.requireKYC(ctx, t.Id, recipient); err != nil {
			return err
		}
	}
	if err := k.requirePerHolderCap(ctx, t, recipient, amount); err != nil {
		return err
	}
	pool := EscrowPoolAccount()
	cur, err := k.GetBalance(ctx, t.Id, pool)
	if err != nil {
		return err
	}
	if cur < amount {
		return fmt.Errorf("escrow pool short: have %d, need %d for token %d", cur, amount, t.Id)
	}
	if err := k.debitBalance(ctx, t.Id, pool, amount); err != nil {
		return err
	}
	if err := k.creditBalance(ctx, t.Id, recipient, amount); err != nil {
		return err
	}
	k.recordAudit(ctx, t.Id, t.IssuerId, "escrow_release", pool, recipient, "")
	return nil
}

// EscrowBalanceOf returns the units the rwa-side escrow pool holds
// for the given token. Useful for x/escrow audit invariants.
func (k Keeper) EscrowBalanceOf(ctx sdk.Context, tokenID uint64) (uint64, error) {
	return k.GetBalance(ctx, tokenID, EscrowPoolAccount())
}
