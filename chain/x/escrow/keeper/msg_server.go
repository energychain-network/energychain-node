package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/escrow/types"
)

type msgServer struct{ k Keeper }

// NewMsgServerImpl constructs the auto-generated MsgServer interface
// over the Keeper.
func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

// ---- helpers -------------------------------------------------------------

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

func (s msgServer) loadAndCheck(ctx context.Context, id uint64, allowed ...types.Status) (types.Escrow, types.Params, error) {
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return types.Escrow{}, types.Params{}, err
	}
	e, err := s.k.MustGetEscrow(ctx, id)
	if err != nil {
		return types.Escrow{}, types.Params{}, err
	}
	if len(allowed) == 0 {
		return e, p, nil
	}
	for _, a := range allowed {
		if e.Status == a {
			return e, p, nil
		}
	}
	return types.Escrow{}, types.Params{}, fmt.Errorf("escrow %d in status %s, expected one of %v", id, e.Status, allowed)
}

// resetApprovalsAfterTerminal removes per-signer approval rows on
// terminal transitions. Approval count fields on the escrow stay
// at their final tally so the audit trail / event stream still
// reflects what happened, but the per-signer index is wiped to
// keep the bounded-state guarantee tight.
func (s msgServer) resetApprovalsAfterTerminal(ctx context.Context, e types.Escrow) error {
	rng := collections.NewPrefixedPairRange[uint64, string](e.Id)
	var keys []collections.Pair[uint64, string]
	if err := s.k.Approvals.Walk(ctx, rng, func(key collections.Pair[uint64, string], _ types.Approval) (bool, error) {
		keys = append(keys, key)
		return false, nil
	}); err != nil {
		return err
	}
	for _, kk := range keys {
		if err := s.k.Approvals.Remove(ctx, kk); err != nil {
			return err
		}
	}
	return nil
}

// ---- CreateEscrow / Fund / Cancel ----------------------------------------

func (s msgServer) CreateEscrow(goCtx context.Context, m *types.MsgCreateEscrow) (*types.MsgCreateEscrowResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	if uint32(len(m.Committee)) > p.MaxSignersPerEscrow {
		return nil, fmt.Errorf("committee (%d) exceeds max_signers_per_escrow (%d)", len(m.Committee), p.MaxSignersPerEscrow)
	}

	now := ctx.BlockTime().Unix()
	if m.Triggers.ReleaseAfter != 0 && m.Triggers.ReleaseAfter > now+p.MaxReleaseHorizonSeconds {
		return nil, fmt.Errorf("release_after %d beyond horizon %d", m.Triggers.ReleaseAfter, now+p.MaxReleaseHorizonSeconds)
	}
	if m.Triggers.RefundAfter != 0 && m.Triggers.RefundAfter > now+p.MaxReleaseHorizonSeconds {
		return nil, fmt.Errorf("refund_after %d beyond horizon %d", m.Triggers.RefundAfter, now+p.MaxReleaseHorizonSeconds)
	}

	// Bound state growth.
	count, err := s.k.CountEscrows(ctx)
	if err != nil {
		return nil, err
	}
	if count >= p.MaxEscrows {
		return nil, fmt.Errorf("escrow count cap reached: %d", p.MaxEscrows)
	}

	// Fail-fast: depositor must be unblocked at create time so a
	// blocked party cannot tie up beneficiary in pending state.
	if err := s.k.requireNotSanctioned(ctx, p, "depositor", m.Depositor); err != nil {
		return nil, err
	}

	// Cross-module asset existence checks (cheap reads).
	switch m.Kind {
	case types.AssetKind_ASSET_KIND_STABLECOIN:
		if s.k.stablecoin == nil {
			return nil, fmt.Errorf("stablecoin keeper not wired")
		}
		if !s.k.stablecoin.HasDenom(ctx, m.StablecoinDenom) {
			return nil, fmt.Errorf("denom %q not registered", m.StablecoinDenom)
		}
	case types.AssetKind_ASSET_KIND_RWA:
		if s.k.rwa == nil {
			return nil, fmt.Errorf("rwa keeper not wired")
		}
		if !s.k.rwa.HasToken(ctx, m.RwaTokenId) {
			return nil, fmt.Errorf("rwa token %d not found", m.RwaTokenId)
		}
	}

	id, err := s.k.NextEscrowID(ctx)
	if err != nil {
		return nil, err
	}

	e := types.Escrow{
		Id:                 id,
		Depositor:          m.Depositor,
		Beneficiary:        m.Beneficiary,
		FallbackAddr:       m.FallbackAddr,
		Committee:          append([]string(nil), m.Committee...),
		ApprovalThreshold:  m.ApprovalThreshold,
		Arbiter:            m.Arbiter,
		Kind:               m.Kind,
		StablecoinDenom:    m.StablecoinDenom,
		RwaTokenId:         m.RwaTokenId,
		Amount:             m.Amount,
		Triggers:           m.Triggers,
		Status:             types.Status_STATUS_DRAFT,
		Memo:               m.Memo,
		CreatedAt:          now,
	}

	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.k.EscrowByDepositor.Set(ctx, collections.Join(e.Depositor, e.Id)); err != nil {
		return nil, err
	}
	if err := s.k.EscrowByBeneficiary.Set(ctx, collections.Join(e.Beneficiary, e.Id)); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "create", e.Depositor, e.Beneficiary, e.Memo)
	s.k.emit(ctx, "create",
		"id", u64s(e.Id),
		"depositor", e.Depositor,
		"beneficiary", e.Beneficiary,
		"kind", e.Kind.String(),
		"amount", u64s(e.Amount),
	)
	return &types.MsgCreateEscrowResponse{EscrowId: e.Id}, nil
}

func (s msgServer) Fund(goCtx context.Context, m *types.MsgFund) (*types.MsgFundResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_DRAFT)
	if err != nil {
		return nil, err
	}
	if m.Depositor != e.Depositor {
		return nil, fmt.Errorf("only depositor %s may fund escrow %d", e.Depositor, e.Id)
	}
	if err := s.k.requireNotSanctioned(ctx, p, "depositor", e.Depositor); err != nil {
		return nil, err
	}

	if err := s.k.payIn(ctx, e); err != nil {
		return nil, err
	}

	now := ctx.BlockTime().Unix()
	e.Status = types.Status_STATUS_FUNDED
	e.FundedAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "fund", e.Depositor, EscrowPoolAccount, "")
	s.k.emit(ctx, "fund", "id", u64s(e.Id), "amount", u64s(e.Amount))
	return &types.MsgFundResponse{}, nil
}

func (s msgServer) Cancel(goCtx context.Context, m *types.MsgCancel) (*types.MsgCancelResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, _, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_DRAFT)
	if err != nil {
		return nil, err
	}
	if m.Depositor != e.Depositor {
		return nil, fmt.Errorf("only depositor %s may cancel a draft escrow", e.Depositor)
	}
	now := ctx.BlockTime().Unix()
	e.Status = types.Status_STATUS_CANCELLED
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, e.Id, "cancel", m.Depositor, e.FallbackAddr, m.Reason)
	s.k.emit(ctx, "cancel", "id", u64s(e.Id), "reason", m.Reason)
	return &types.MsgCancelResponse{}, nil
}

// ---- Approve / Revoke ----------------------------------------------------

func (s msgServer) Approve(goCtx context.Context, m *types.MsgApprove) (*types.MsgApproveResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, _, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED)
	if err != nil {
		return nil, err
	}
	if !s.k.IsSigner(e, m.Signer) {
		return nil, fmt.Errorf("%s not on escrow %d signer committee", m.Signer, e.Id)
	}

	prev, ok, err := s.k.GetApproval(ctx, e.Id, m.Signer)
	if err != nil {
		return nil, err
	}
	if ok && prev.Intent == m.Intent {
		// Idempotent re-approval; refresh memo/timestamp but no
		// counter change. Avoids both double-count and "no-op
		// looks like an error" surprises for retried txs.
		prev.ApprovedAt = ctx.BlockTime().Unix()
		prev.Memo = m.Memo
		if err := s.k.SetApproval(ctx, prev); err != nil {
			return nil, err
		}
		return &types.MsgApproveResponse{
			ReleaseCount: e.ApprovalCountRelease,
			RefundCount:  e.ApprovalCountRefund,
		}, nil
	}

	if ok {
		// Switching sides: vacate the previous tally.
		switch prev.Intent {
		case types.Intent_INTENT_RELEASE:
			c, err := types.SafeSubU32(e.ApprovalCountRelease, 1)
			if err != nil {
				return nil, err
			}
			e.ApprovalCountRelease = c
		case types.Intent_INTENT_REFUND:
			c, err := types.SafeSubU32(e.ApprovalCountRefund, 1)
			if err != nil {
				return nil, err
			}
			e.ApprovalCountRefund = c
		}
	}

	now := ctx.BlockTime().Unix()
	a := types.Approval{
		EscrowId:   e.Id,
		Signer:     m.Signer,
		Intent:     m.Intent,
		ApprovedAt: now,
		Memo:       m.Memo,
	}
	if err := s.k.SetApproval(ctx, a); err != nil {
		return nil, err
	}
	switch m.Intent {
	case types.Intent_INTENT_RELEASE:
		c, err := types.SafeAddU32(e.ApprovalCountRelease, 1)
		if err != nil {
			return nil, err
		}
		e.ApprovalCountRelease = c
	case types.Intent_INTENT_REFUND:
		c, err := types.SafeAddU32(e.ApprovalCountRefund, 1)
		if err != nil {
			return nil, err
		}
		e.ApprovalCountRefund = c
	}
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "approve", m.Signer, "", m.Intent.String())
	s.k.emit(ctx, "approve",
		"id", u64s(e.Id),
		"signer", m.Signer,
		"intent", m.Intent.String(),
	)
	return &types.MsgApproveResponse{
		ReleaseCount: e.ApprovalCountRelease,
		RefundCount:  e.ApprovalCountRefund,
	}, nil
}

func (s msgServer) RevokeApproval(goCtx context.Context, m *types.MsgRevokeApproval) (*types.MsgRevokeApprovalResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, _, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED)
	if err != nil {
		return nil, err
	}
	prev, ok, err := s.k.GetApproval(ctx, e.Id, m.Signer)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no approval to revoke for signer %s on escrow %d", m.Signer, e.Id)
	}
	switch prev.Intent {
	case types.Intent_INTENT_RELEASE:
		c, err := types.SafeSubU32(e.ApprovalCountRelease, 1)
		if err != nil {
			return nil, err
		}
		e.ApprovalCountRelease = c
	case types.Intent_INTENT_REFUND:
		c, err := types.SafeSubU32(e.ApprovalCountRefund, 1)
		if err != nil {
			return nil, err
		}
		e.ApprovalCountRefund = c
	}
	if err := s.k.RemoveApproval(ctx, e.Id, m.Signer); err != nil {
		return nil, err
	}
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "revoke_approval", m.Signer, "", prev.Intent.String())
	s.k.emit(ctx, "revoke_approval",
		"id", u64s(e.Id),
		"signer", m.Signer,
	)
	return &types.MsgRevokeApprovalResponse{}, nil
}

// ---- Release / Refund ----------------------------------------------------

func (s msgServer) Release(goCtx context.Context, m *types.MsgRelease) (*types.MsgReleaseResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	if e.ApprovalCountRelease < e.ApprovalThreshold {
		return nil, fmt.Errorf("release threshold not met: have %d, need %d", e.ApprovalCountRelease, e.ApprovalThreshold)
	}
	if e.Triggers.ReleaseAfter != 0 && now < e.Triggers.ReleaseAfter {
		return nil, fmt.Errorf("release_after %d not yet reached (now=%d)", e.Triggers.ReleaseAfter, now)
	}
	if err := s.k.checkOracleTrigger(ctx, e.Triggers, now); err != nil {
		return nil, err
	}
	if err := s.k.requireNotSanctioned(ctx, p, "beneficiary", e.Beneficiary); err != nil {
		return nil, err
	}

	if err := s.k.payOut(ctx, e, e.Beneficiary); err != nil {
		return nil, err
	}

	e.Status = types.Status_STATUS_RELEASED
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.resetApprovalsAfterTerminal(ctx, e); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "release", m.Actor, e.Beneficiary, "")
	s.k.emit(ctx, "release", "id", u64s(e.Id), "beneficiary", e.Beneficiary, "amount", u64s(e.Amount))
	return &types.MsgReleaseResponse{}, nil
}

func (s msgServer) Refund(goCtx context.Context, m *types.MsgRefund) (*types.MsgRefundResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	if e.ApprovalCountRefund < e.ApprovalThreshold {
		return nil, fmt.Errorf("refund threshold not met: have %d, need %d", e.ApprovalCountRefund, e.ApprovalThreshold)
	}
	if e.Triggers.RefundAfter != 0 && now < e.Triggers.RefundAfter {
		return nil, fmt.Errorf("refund_after %d not yet reached (now=%d)", e.Triggers.RefundAfter, now)
	}
	if err := s.k.requireNotSanctioned(ctx, p, "fallback_addr", e.FallbackAddr); err != nil {
		return nil, err
	}

	if err := s.k.payOut(ctx, e, e.FallbackAddr); err != nil {
		return nil, err
	}

	e.Status = types.Status_STATUS_REFUNDED
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.resetApprovalsAfterTerminal(ctx, e); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, e.Id, "refund", m.Actor, e.FallbackAddr, "")
	s.k.emit(ctx, "refund", "id", u64s(e.Id), "fallback", e.FallbackAddr, "amount", u64s(e.Amount))
	return &types.MsgRefundResponse{}, nil
}

// ---- Arbiter overrides ---------------------------------------------------

func (s msgServer) ArbiterRelease(goCtx context.Context, m *types.MsgArbiterRelease) (*types.MsgArbiterReleaseResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED, types.Status_STATUS_DISPUTED)
	if err != nil {
		return nil, err
	}
	if e.Arbiter == "" || m.Arbiter != e.Arbiter {
		return nil, fmt.Errorf("only arbiter %s may force-release escrow %d", e.Arbiter, e.Id)
	}
	if err := s.k.requireNotSanctioned(ctx, p, "beneficiary", e.Beneficiary); err != nil {
		return nil, err
	}
	if err := s.k.payOut(ctx, e, e.Beneficiary); err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	e.Status = types.Status_STATUS_RELEASED
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.resetApprovalsAfterTerminal(ctx, e); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, e.Id, "arbiter_release", m.Arbiter, e.Beneficiary, m.Reason)
	s.k.emit(ctx, "arbiter_release", "id", u64s(e.Id), "beneficiary", e.Beneficiary, "reason", m.Reason)
	return &types.MsgArbiterReleaseResponse{}, nil
}

func (s msgServer) ArbiterRefund(goCtx context.Context, m *types.MsgArbiterRefund) (*types.MsgArbiterRefundResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED, types.Status_STATUS_DISPUTED)
	if err != nil {
		return nil, err
	}
	if e.Arbiter == "" || m.Arbiter != e.Arbiter {
		return nil, fmt.Errorf("only arbiter %s may force-refund escrow %d", e.Arbiter, e.Id)
	}
	if err := s.k.requireNotSanctioned(ctx, p, "fallback_addr", e.FallbackAddr); err != nil {
		return nil, err
	}
	if err := s.k.payOut(ctx, e, e.FallbackAddr); err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	e.Status = types.Status_STATUS_REFUNDED
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.resetApprovalsAfterTerminal(ctx, e); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, e.Id, "arbiter_refund", m.Arbiter, e.FallbackAddr, m.Reason)
	s.k.emit(ctx, "arbiter_refund", "id", u64s(e.Id), "fallback", e.FallbackAddr, "reason", m.Reason)
	return &types.MsgArbiterRefundResponse{}, nil
}

// ---- Dispute hooks -------------------------------------------------------

func (s msgServer) MarkDisputed(goCtx context.Context, m *types.MsgMarkDisputed) (*types.MsgMarkDisputedResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, _, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_FUNDED)
	if err != nil {
		return nil, err
	}
	// An arbiter-less escrow has no path out of DISPUTED — it would
	// be permanently locked. Until x/dispute lands and provides the
	// hook-driven resolution path, we refuse to enter DISPUTED for
	// arbiter-less escrows. Parties of such escrows can still settle
	// by approving release / refund.
	if e.Arbiter == "" {
		return nil, fmt.Errorf("escrow %d has no arbiter; dispute path unavailable until x/dispute is wired", e.Id)
	}
	if m.Actor != e.Depositor && m.Actor != e.Beneficiary && m.Actor != e.Arbiter {
		return nil, fmt.Errorf("actor %s not party to escrow %d (depositor / beneficiary / arbiter)", m.Actor, e.Id)
	}
	now := ctx.BlockTime().Unix()
	e.Status = types.Status_STATUS_DISPUTED
	e.DisputedAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, e.Id, "dispute_mark", m.Actor, "", m.Reason)
	s.k.emit(ctx, "dispute_mark", "id", u64s(e.Id), "actor", m.Actor, "reason", m.Reason)
	return &types.MsgMarkDisputedResponse{}, nil
}

func (s msgServer) ResolveDispute(goCtx context.Context, m *types.MsgResolveDispute) (*types.MsgResolveDisputeResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, p, err := s.loadAndCheck(ctx, m.EscrowId, types.Status_STATUS_DISPUTED)
	if err != nil {
		return nil, err
	}
	if e.Arbiter == "" || m.Arbiter != e.Arbiter {
		return nil, fmt.Errorf("only arbiter %s may resolve dispute on escrow %d", e.Arbiter, e.Id)
	}

	dest := e.FallbackAddr
	field := "fallback_addr"
	if m.Release {
		dest = e.Beneficiary
		field = "beneficiary"
	}
	if err := s.k.requireNotSanctioned(ctx, p, field, dest); err != nil {
		return nil, err
	}
	if err := s.k.payOut(ctx, e, dest); err != nil {
		return nil, err
	}

	now := ctx.BlockTime().Unix()
	if m.Release {
		e.Status = types.Status_STATUS_RELEASED
	} else {
		e.Status = types.Status_STATUS_REFUNDED
	}
	e.SettledAt = now
	if err := s.k.SetEscrow(ctx, e); err != nil {
		return nil, err
	}
	if err := s.resetApprovalsAfterTerminal(ctx, e); err != nil {
		return nil, err
	}
	action := "dispute_resolve_refund"
	if m.Release {
		action = "dispute_resolve_release"
	}
	s.k.recordAudit(ctx, e.Id, action, m.Arbiter, dest, m.Reason)
	s.k.emit(ctx, action,
		"id", u64s(e.Id),
		"dest", dest,
		"reason", m.Reason,
	)
	return &types.MsgResolveDisputeResponse{}, nil
}

// ---- UpdateParams --------------------------------------------------------

func (s msgServer) UpdateParams(goCtx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
