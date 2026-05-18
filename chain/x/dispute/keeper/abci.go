package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dispute/types"
)

// EndBlock runs two bounded sweeps each block:
//
//  1. Lapsed OPEN disputes: the respondent failed to post bond
//     before respond_deadline. Auto-cancel and refund the
//     plaintiff. This keeps the active-dispute table bounded
//     against a flood of unanswered openings.
//
//  2. Lapsed DELIBERATING disputes: the tribunal failed to
//     reach a ruling before deliberation_deadline. Auto-
//     finalize using the current vote tally (NO_FAULT if no
//     clear winner). Bonds are disbursed identically to a
//     manual FinalizeRuling.
//
// Both passes are capped by params.MaxFinalizationsPerBlock so
// a backlog cannot expand block time without bound. Errors on a
// single dispute are logged and skipped — they do not abort
// the block, so a misbehaving counterparty (e.g. blocked
// account) cannot wedge the whole sweep.
func (k Keeper) EndBlock(goCtx context.Context) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	now := ctx.BlockTime().Unix()
	p, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	budget := uint32(p.MaxFinalizationsPerBlock)
	if budget == 0 {
		return nil
	}

	// Phase 1: lapsed OPEN — collect ids first so mutation
	// during iteration doesn't invalidate cursors.
	openIDs, err := k.collectStatusUpTo(ctx, types.DisputeStatus_DISPUTE_STATUS_OPEN, budget)
	if err != nil {
		return err
	}
	used := uint32(0)
	for _, id := range openIDs {
		if used >= budget {
			break
		}
		d, ok, err := k.GetDispute(ctx, id)
		if err != nil {
			ctx.Logger().Error("dispute: end-block load", "id", id, "err", err)
			continue
		}
		if !ok || d.Status != types.DisputeStatus_DISPUTE_STATUS_OPEN {
			continue
		}
		if now < d.RespondDeadline {
			continue
		}
		if err := k.autoCancelLapsed(ctx, d); err != nil {
			ctx.Logger().Error("dispute: auto-cancel", "id", id, "err", err)
			continue
		}
		used++
	}

	// Phase 2: lapsed DELIBERATING — same pattern.
	remaining := budget - used
	if remaining == 0 {
		return nil
	}
	delibIDs, err := k.collectStatusUpTo(ctx, types.DisputeStatus_DISPUTE_STATUS_DELIBERATING, remaining)
	if err != nil {
		return err
	}
	for _, id := range delibIDs {
		if used >= budget {
			break
		}
		d, ok, err := k.GetDispute(ctx, id)
		if err != nil {
			ctx.Logger().Error("dispute: end-block load", "id", id, "err", err)
			continue
		}
		if !ok || d.Status != types.DisputeStatus_DISPUTE_STATUS_DELIBERATING {
			continue
		}
		if now < d.DeliberationDeadline {
			continue
		}
		if err := k.autoFinalizeLapsed(ctx, d, p.DefaultSlashBps); err != nil {
			ctx.Logger().Error("dispute: auto-finalize", "id", id, "err", err)
			continue
		}
		used++
	}
	return nil
}

func (k Keeper) collectStatusUpTo(ctx context.Context, status types.DisputeStatus, limit uint32) ([]uint64, error) {
	ids := []uint64{}
	rng := collections.NewPrefixedPairRange[uint32, uint64](uint32(status))
	it, err := k.DisputeByStatus.Iterate(ctx, rng)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	for ; it.Valid() && uint32(len(ids)) < limit; it.Next() {
		key, err := it.Key()
		if err != nil {
			return nil, err
		}
		ids = append(ids, key.K2())
	}
	return ids, nil
}

func (k Keeper) autoCancelLapsed(ctx context.Context, d types.Dispute) error {
	// Best-effort refund: if the plaintiff is blocked at the
	// stablecoin layer or the denom is paused, the bond is
	// forfeited to the pool rather than blocking auto-cancel.
	// This is the chain's only path to drain stuck OPEN
	// disputes — any other approach lets a blocked
	// counterparty wedge the end-block sweep indefinitely.
	k.tryRefundOrForfeit(ctx, d.Id, d.BondDenom, d.Plaintiff, d.PlaintiffBond, "auto_cancel_plaintiff")
	d.Status = types.DisputeStatus_DISPUTE_STATUS_CANCELLED
	d.ResolvedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := k.SetDispute(ctx, d); err != nil {
		return err
	}
	k.emit(ctx, "dispute.auto_cancelled",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("reason", "respond_deadline_passed"),
	)
	k.recordAudit(ctx, d.Id, "dispute.auto_cancel", k.authority, "", "respond_deadline_passed")
	return nil
}

// autoFinalizeLapsed mirrors the in-line FinalizeRuling path
// but is called by the chain itself when no party finalizes
// within the deliberation window. We tally votes, decide the
// outcome, and disburse bonds identically. If the tally is
// empty or tied, decideOutcome returns NO_FAULT — both sides
// are refunded in full, which is the conservative choice when
// the tribunal failed to rule.
func (k Keeper) autoFinalizeLapsed(ctx context.Context, d types.Dispute, defaultSlashBps uint32) error {
	tally, err := k.tallyForDispute(ctx, d.Id)
	if err != nil {
		return err
	}
	outcome := decideOutcome(tally)
	var refundPlaintiff, refundRespondent uint64
	switch outcome {
	case types.RulingOutcome_RULING_OUTCOME_PLAINTIFF:
		_, refundRespondent = types.SlashAmount(d.RespondentBond, defaultSlashBps)
		refundPlaintiff = d.PlaintiffBond
	case types.RulingOutcome_RULING_OUTCOME_RESPONDENT:
		_, refundPlaintiff = types.SlashAmount(d.PlaintiffBond, defaultSlashBps)
		refundRespondent = d.RespondentBond
	case types.RulingOutcome_RULING_OUTCOME_SPLIT:
		_, refundPlaintiff = types.SlashAmount(d.PlaintiffBond, defaultSlashBps)
		_, refundRespondent = types.SlashAmount(d.RespondentBond, defaultSlashBps)
	default:
		outcome = types.RulingOutcome_RULING_OUTCOME_NO_FAULT
		refundPlaintiff = d.PlaintiffBond
		refundRespondent = d.RespondentBond
	}
	// Same best-effort rationale as autoCancelLapsed: a
	// blocked or sanctioned counterparty cannot wedge the
	// auto-finalize sweep. Forfeited amounts surface as
	// dispute.bond.forfeited events for off-chain reconciliation
	// or a future governance-driven treasury sweep.
	k.tryRefundOrForfeit(ctx, d.Id, d.BondDenom, d.Plaintiff, refundPlaintiff, "auto_finalize_plaintiff")
	k.tryRefundOrForfeit(ctx, d.Id, d.BondDenom, d.Respondent, refundRespondent, "auto_finalize_respondent")
	d.Status = types.DisputeStatus_DISPUTE_STATUS_RESOLVED
	d.Outcome = outcome
	if outcome == types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		d.SlashBps = 0
	} else {
		d.SlashBps = defaultSlashBps
	}
	d.ResolvedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := k.SetDispute(ctx, d); err != nil {
		return err
	}
	k.emit(ctx, "dispute.auto_finalized",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("outcome", outcome.String()),
	)
	k.recordAudit(ctx, d.Id, "dispute.auto_finalize", k.authority, outcome.String(), "deliberation_deadline_passed")
	return nil
}

func (k Keeper) tallyForDispute(ctx context.Context, disputeID uint64) (voteTally, error) {
	var t voteTally
	rng := collections.NewPrefixedPairRange[uint64, uint64](disputeID)
	it, err := k.Votes.Iterate(ctx, rng)
	if err != nil {
		return t, err
	}
	defer it.Close()
	for ; it.Valid(); it.Next() {
		v, err := it.Value()
		if err != nil {
			return t, err
		}
		t.total++
		switch v.Choice {
		case types.VoteChoice_VOTE_CHOICE_PLAINTIFF:
			t.plaintiff++
		case types.VoteChoice_VOTE_CHOICE_RESPONDENT:
			t.respondent++
		case types.VoteChoice_VOTE_CHOICE_SPLIT:
			t.split++
		case types.VoteChoice_VOTE_CHOICE_NO_FAULT:
			t.noFault++
		case types.VoteChoice_VOTE_CHOICE_ABSTAIN:
			t.abstain++
		}
	}
	return t, nil
}
