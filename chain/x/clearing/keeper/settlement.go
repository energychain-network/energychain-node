package keeper

import (
	"context"
	"sort"
	"strings"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/clearing/types"
)

// settle is the workhorse for MsgSettleCycle. It:
//
//  1. Walks every obligation in the cycle and aggregates a
//     signed net per (member, denom). Internally uses a
//     symmetric pair of incoming/outgoing uint64 buckets to
//     avoid signed integers (cosmos-sdk avoids them broadly).
//  2. Persists a NetPosition row for each (member, denom)
//     with non-zero magnitude.
//  3. For each net-payer, attempts to draw down their margin
//     up to `magnitude`. Any shortfall triggers the default
//     waterfall (defaulter margin → default fund). Net
//     receivers are paid out of the pool exactly the amount
//     they're owed (less any pro-rata haircut if the
//     waterfall could not absorb the full shortfall — this is
//     a "loss-allocation" mode rather than a strict atomic
//     revert, because the per-denom mutualised default fund
//     is the explicit second-tier protection promised by the
//     blueprint).
//
// Returns (settledCycle, defaultEventsCount). A non-zero
// defaultEventsCount with uncovered > 0 in any row marks
// the cycle as DEFAULTED rather than SETTLED; otherwise the
// cycle is SETTLED.
//
// Sanctioned counterparties are tolerated INSIDE settle: the
// per-leg stablecoin freeze surface (drainPool / fundPool)
// already refuses transfers into a frozen account, so the
// settlement still happens but the payee's owed amount is
// recorded as `uncovered`, accruing to the receiver's claim
// against the system; offline this is what would route to
// x/dispute. Without this tolerance a single sanctioned
// counterparty could veto an entire multilateral cycle.
func (k Keeper) settle(ctx context.Context, cycle types.Cycle) (types.Cycle, []types.DefaultEvent, error) {
	type key struct {
		member uint64
		denom  string
	}
	incoming := map[key]uint64{}
	outgoing := map[key]uint64{}
	denomSet := map[string]struct{}{}
	memberSet := map[uint64]struct{}{}

	// 1. Aggregate.
	rng := collections.NewPrefixedPairRange[uint64, uint64](cycle.Id)
	if err := k.ObligationByCycle.Walk(ctx, rng, func(p collections.Pair[uint64, uint64]) (bool, error) {
		o, ok, err := k.GetObligation(ctx, p.K2())
		if err != nil {
			return true, err
		}
		if !ok {
			return false, nil
		}
		denomSet[o.Denom] = struct{}{}
		memberSet[o.FromMemberId] = struct{}{}
		memberSet[o.ToMemberId] = struct{}{}
		kIn := key{member: o.ToMemberId, denom: o.Denom}
		kOut := key{member: o.FromMemberId, denom: o.Denom}
		nin, err := types.SafeAdd(incoming[kIn], o.Amount)
		if err != nil {
			return true, err
		}
		incoming[kIn] = nin
		nout, err := types.SafeAdd(outgoing[kOut], o.Amount)
		if err != nil {
			return true, err
		}
		outgoing[kOut] = nout
		return false, nil
	}); err != nil {
		return cycle, nil, err
	}

	// Release the gross reservation we held at submit time —
	// the cycle is about to consume its margin draws and the
	// (member, denom) exposure is no longer outstanding.
	// We do this BEFORE the loss-allocation pass so that
	// `setMargin` calls below correctly reflect the available
	// balance (margin minus zero reservation, post-release).
	for kOut, amount := range outgoing {
		if _, err := k.subReservation(ctx, kOut.member, kOut.denom, amount); err != nil {
			return cycle, nil, err
		}
	}

	// 2. Compute deterministic iteration order.
	denoms := make([]string, 0, len(denomSet))
	for d := range denomSet {
		denoms = append(denoms, d)
	}
	sort.Strings(denoms)
	members := make([]uint64, 0, len(memberSet))
	for m := range memberSet {
		members = append(members, m)
	}
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })

	// 3. Build net positions and persist.
	var netCount uint32
	for _, d := range denoms {
		for _, m := range members {
			in := incoming[key{member: m, denom: d}]
			out := outgoing[key{member: m, denom: d}]
			if in == out {
				continue
			}
			var mag uint64
			var short bool
			if out > in {
				mag = out - in
				short = true
			} else {
				mag = in - out
			}
			np := types.NetPosition{
				CycleId:   cycle.Id,
				MemberId:  m,
				Denom:     d,
				Magnitude: mag,
				IsShort:   short,
			}
			if err := k.NetPositions.Set(ctx, collections.Join3(cycle.Id, m, d), np); err != nil {
				return cycle, nil, err
			}
			netCount++
		}
	}

	// 4. Loss allocation per (denom): collect payers, drain
	// margin first then default fund. Then disburse to
	// receivers in pro-rata fashion if the pool came up short.
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	defaultEvents := []types.DefaultEvent{}
	hasDefault := false

	for _, d := range denoms {
		var (
			payers    []uint64
			receivers []uint64
			owedTotal uint64
			paidTotal uint64
		)
		for _, m := range members {
			in := incoming[key{member: m, denom: d}]
			out := outgoing[key{member: m, denom: d}]
			if out > in {
				payers = append(payers, m)
			} else if in > out {
				receivers = append(receivers, m)
				diff := in - out
				owedTotal, _ = types.SafeAdd(owedTotal, diff)
			}
		}

		// 4a. Collect payments.
		for _, payer := range payers {
			due := outgoing[key{member: payer, denom: d}] - incoming[key{member: payer, denom: d}]
			// Pay from margin first.
			margin, err := k.GetMargin(ctx, payer, d)
			if err != nil {
				return cycle, nil, err
			}
			fromMargin := minU64(due, margin)
			if fromMargin > 0 {
				if err := k.setMargin(ctx, payer, d, margin-fromMargin); err != nil {
					return cycle, nil, err
				}
				paidTotal, _ = types.SafeAdd(paidTotal, fromMargin)
			}
			short := due - fromMargin
			if short == 0 {
				continue
			}
			// Tap the mutualised default fund.
			df, err := k.GetDefaultFund(ctx, d)
			if err != nil {
				return cycle, nil, err
			}
			fromFund := minU64(short, df)
			if fromFund > 0 {
				if err := k.setDefaultFund(ctx, d, df-fromFund); err != nil {
					return cycle, nil, err
				}
				paidTotal, _ = types.SafeAdd(paidTotal, fromFund)
			}
			uncov := short - fromFund
			if uncov > 0 {
				hasDefault = true
			}
			ev := types.DefaultEvent{
				CycleId:         cycle.Id,
				MemberId:        payer,
				Denom:           d,
				Shortfall:       due,
				FromMargin:      fromMargin,
				FromDefaultFund: fromFund,
				Uncovered:       uncov,
				At:              now,
			}
			if err := k.DefaultEvents.Set(ctx, collections.Join3(cycle.Id, payer, d), ev); err != nil {
				return cycle, nil, err
			}
			defaultEvents = append(defaultEvents, ev)
		}

		// 4b. Pay receivers. If paidTotal < owedTotal we apply a
		// pro-rata haircut. Otherwise pay in full.
		for _, receiver := range receivers {
			owed := incoming[key{member: receiver, denom: d}] - outgoing[key{member: receiver, denom: d}]
			payout := owed
			if owedTotal > 0 && paidTotal < owedTotal {
				// Pro-rata: payout = owed * paidTotal / owedTotal
				// Using big-int-free arithmetic; safe because
				// uint64 max is ~1.8e19 and individual cycles
				// are bounded.
				p, err := safeMulDiv(owed, paidTotal, owedTotal)
				if err != nil {
					return cycle, nil, err
				}
				payout = p
			}
			if payout == 0 {
				continue
			}
			mem, err := k.MustGetMember(ctx, receiver)
			if err != nil {
				return cycle, nil, err
			}
			// Receiver-side sanctions check: refuse to pay a
			// sanctioned receiver, accumulating their owed
			// amount as an explicit uncovered DefaultEvent for
			// off-chain resolution.
			if err := k.requireUnsanctioned(ctx, mem.Address, "receiver"); err != nil {
				hasDefault = true
				ev := types.DefaultEvent{
					CycleId:    cycle.Id,
					MemberId:   receiver,
					Denom:      d,
					Shortfall:  payout,
					Uncovered:  payout,
					At:         now,
				}
				if err := k.DefaultEvents.Set(ctx, collections.Join3(cycle.Id, receiver, d), ev); err != nil {
					return cycle, nil, err
				}
				defaultEvents = append(defaultEvents, ev)
				continue
			}
			if err := k.drainPool(ctx, d, mem.Address, payout); err != nil {
				// Per-denom freeze / pause encountered at
				// payout time: same loss-allocation as the
				// sanctions case above. Record uncovered and
				// proceed so a single frozen counterparty can
				// not veto the entire multilateral cycle.
				hasDefault = true
				ev := types.DefaultEvent{
					CycleId:    cycle.Id,
					MemberId:   receiver,
					Denom:      d,
					Shortfall:  payout,
					Uncovered:  payout,
					At:         now,
				}
				if err := k.DefaultEvents.Set(ctx, collections.Join3(cycle.Id, receiver, d), ev); err != nil {
					return cycle, nil, err
				}
				defaultEvents = append(defaultEvents, ev)
				continue
			}
			k.emit(ctx, "settle_payout",
				"cycle_id", u64s(cycle.Id),
				"receiver_id", u64s(receiver),
				"denom", d,
				"amount", u64s(payout),
				"owed", u64s(owed),
			)
		}
	}

	if hasDefault {
		cycle.Status = types.CycleStatus_CYCLE_STATUS_DEFAULTED
	} else {
		cycle.Status = types.CycleStatus_CYCLE_STATUS_SETTLED
	}
	cycle.SettledTime = now
	if err := k.SetCycle(ctx, cycle); err != nil {
		return cycle, defaultEvents, err
	}
	k.emit(ctx, "settle_done",
		"cycle_id", u64s(cycle.Id),
		"status", cycle.Status.String(),
		"net_positions", u64s(uint64(netCount)),
		"defaults", u64s(uint64(len(defaultEvents))),
		"denoms", strings.Join(denoms, ","),
	)
	return cycle, defaultEvents, nil
}

// cancel walks the obligations and removes them, then marks
// the cycle CANCELLED. Used by MsgCancelCycle. Reservations
// held against from-side margins are released exactly per
// obligation (no double release because every obligation is
// removed on the same pass).
func (k Keeper) cancel(ctx context.Context, cycle types.Cycle) (types.Cycle, error) {
	rng := collections.NewPrefixedPairRange[uint64, uint64](cycle.Id)
	keysToRemove := []collections.Pair[uint64, uint64]{}
	idsToRemove := []uint64{}
	releases := map[struct {
		member uint64
		denom  string
	}]uint64{}
	if err := k.ObligationByCycle.Walk(ctx, rng, func(p collections.Pair[uint64, uint64]) (bool, error) {
		o, ok, err := k.GetObligation(ctx, p.K2())
		if err != nil {
			return true, err
		}
		keysToRemove = append(keysToRemove, p)
		idsToRemove = append(idsToRemove, p.K2())
		if ok {
			rk := struct {
				member uint64
				denom  string
			}{member: o.FromMemberId, denom: o.Denom}
			s, err := types.SafeAdd(releases[rk], o.Amount)
			if err != nil {
				return true, err
			}
			releases[rk] = s
		}
		return false, nil
	}); err != nil {
		return cycle, err
	}
	for _, k2 := range keysToRemove {
		if err := k.ObligationByCycle.Remove(ctx, k2); err != nil {
			return cycle, err
		}
	}
	for _, id := range idsToRemove {
		if err := k.Obligations.Remove(ctx, id); err != nil {
			return cycle, err
		}
	}
	for rk, amount := range releases {
		if _, err := k.subReservation(ctx, rk.member, rk.denom, amount); err != nil {
			return cycle, err
		}
	}
	cycle.Status = types.CycleStatus_CYCLE_STATUS_CANCELLED
	cycle.SettledTime = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return cycle, k.SetCycle(ctx, cycle)
}

// minU64 returns the smaller of two uint64 values.
func minU64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// safeMulDiv computes (a * b) / c without overflow when
// a*b > 2^64. Used by the receiver-side pro-rata haircut.
// Falls back to (a/c)*b when a is divisible by c to preserve
// precision in the common case; otherwise uses the standard
// (a*b)/c with an overflow guard.
func safeMulDiv(a, b, c uint64) (uint64, error) {
	if c == 0 {
		return 0, nil
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	// fast path: no overflow possible.
	if a <= ^uint64(0)/b {
		return (a * b) / c, nil
	}
	// slow path: split a around c.
	// (a*b)/c = ((a/c)*b) + ((a%c)*b)/c
	hi := (a / c) * b
	lo := ((a % c) * b) / c
	out, err := types.SafeAdd(hi, lo)
	if err != nil {
		return 0, err
	}
	return out, nil
}
