package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/sanctions/types"
)

type msgServer struct{ Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

func (m msgServer) onlyAuthority(authority string) error {
	if authority != m.GetAuthority() {
		return sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	return nil
}

// ---------------------------------------------------------------------------
// List lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterList(goCtx context.Context, msg *types.MsgRegisterList) (*types.MsgRegisterListResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if m.HasList(ctx, msg.Id) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("list %s already exists", msg.Id)
	}
	params := m.GetParams(ctx)
	if m.CountLists(ctx) >= params.MaxLists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("max_lists cap %d reached", params.MaxLists)
	}
	now := ctx.BlockTime().Unix()
	l := types.SanctionList{
		Id:           msg.Id,
		Name:         msg.Name,
		Description:  msg.Description,
		SourceUri:    msg.SourceUri,
		Jurisdiction: msg.Jurisdiction,
		Authority:    msg.ListAuthority,
		Status:       types.ListStatus_LIST_STATUS_ACTIVE,
		CreatedBy:    msg.Authority,
		CreatedAt:    now,
		UpdatedAt:    now,
		EntryCount:   0,
	}
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_list_registered", "list_id", l.Id, "name", l.Name)
	return &types.MsgRegisterListResponse{}, nil
}

func (m msgServer) UpdateList(goCtx context.Context, msg *types.MsgUpdateList) (*types.MsgUpdateListResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := m.GetList(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", msg.Id)
	}
	// SECURITY: ARCHIVED lists are write-locked. Operators must
	// re-register under a new id rather than mutate an archived list.
	if l.Status == types.ListStatus_LIST_STATUS_ARCHIVED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("cannot update ARCHIVED list")
	}
	if msg.Name != "" {
		l.Name = msg.Name
	}
	if msg.Description != "" {
		l.Description = msg.Description
	}
	if msg.SourceUri != "" {
		l.SourceUri = msg.SourceUri
	}
	if msg.ListAuthority != "" {
		l.Authority = msg.ListAuthority
	}
	l.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_list_updated", "list_id", l.Id)
	return &types.MsgUpdateListResponse{}, nil
}

func (m msgServer) DeprecateList(goCtx context.Context, msg *types.MsgDeprecateList) (*types.MsgDeprecateListResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := m.GetList(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", msg.Id)
	}
	l.Status = types.ListStatus_LIST_STATUS_DEPRECATED
	l.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_list_deprecated", "list_id", l.Id, "reason", msg.Reason)
	return &types.MsgDeprecateListResponse{}, nil
}

func (m msgServer) ArchiveList(goCtx context.Context, msg *types.MsgArchiveList) (*types.MsgArchiveListResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := m.GetList(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", msg.Id)
	}
	l.Status = types.ListStatus_LIST_STATUS_ARCHIVED
	l.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_list_archived", "list_id", l.Id, "reason", msg.Reason)
	return &types.MsgArchiveListResponse{}, nil
}

// ---------------------------------------------------------------------------
// Entries
// ---------------------------------------------------------------------------

func (m msgServer) AddEntry(goCtx context.Context, msg *types.MsgAddEntry) (*types.MsgAddEntryResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := m.GetList(ctx, msg.Entry.ListId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", msg.Entry.ListId)
	}
	// SECURITY: only ACTIVE lists accept new entries. DEPRECATED is
	// read-only for adds (existing entries continue to match).
	if l.Status != types.ListStatus_LIST_STATUS_ACTIVE {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"list %s status %s does not accept new entries", l.Id, l.Status.String())
	}
	params := m.GetParams(ctx)
	// SECURITY: when require_proposal_for_adds=true, direct adds are
	// universally refused. This is the audit-strict mode some chains
	// will want; routing all sanctions through propose+confirm leaves
	// a trail in the Proposal collection.
	if params.RequireProposalForAdds {
		return nil, sdkerrors.ErrInvalidRequest.Wrap(
			"direct adds disabled (require_proposal_for_adds=true); use MsgProposeDelta")
	}
	// AuthZ: governance OR the list's registered authority may add.
	if msg.Authority != m.GetAuthority() && msg.Authority != l.Authority {
		return nil, sdkerrors.ErrUnauthorized.Wrapf(
			"only governance or list authority %s may add to list %s", l.Authority, l.Id)
	}
	if m.CountEntries(ctx, l.Id) >= params.MaxEntriesPerList {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"list %s already at cap %d entries", l.Id, params.MaxEntriesPerList)
	}
	// SECURITY: dedupe across both raw and canonical (lowercase) forms
	// so an attacker can't sneak in "cosmos1ABC..." after the
	// lowercase form was already added.
	if m.LookupHasEntry(ctx, l.Id, msg.Entry.Subject) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"entry (%s, %s) already exists; use propose+confirm to update", l.Id, msg.Entry.Subject)
	}
	now := ctx.BlockTime().Unix()
	e := msg.Entry
	e.Status = types.EntryStatus_ENTRY_STATUS_ACTIVE
	e.AddedAt = now
	e.AddedBy = msg.Authority
	if err := m.SetEntry(ctx, e); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	l.EntryCount++
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set list: %s", err)
	}
	if m.audit != nil {
		m.audit.RecordSanctionsAction(ctx, l.Id, e.Subject, "add_entry", e.Reason)
	}
	emit(ctx, "sanctions_entry_added",
		"list_id", l.Id, "subject", e.Subject, "program", e.Program)
	return &types.MsgAddEntryResponse{}, nil
}

func (m msgServer) RemoveEntry(goCtx context.Context, msg *types.MsgRemoveEntry) (*types.MsgRemoveEntryResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, _, ok := m.LookupEntry(ctx, msg.ListId, msg.Subject)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("entry (%s, %s)", msg.ListId, msg.Subject)
	}
	if e.Status == types.EntryStatus_ENTRY_STATUS_REMOVED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("entry already REMOVED")
	}
	now := ctx.BlockTime().Unix()
	e.Status = types.EntryStatus_ENTRY_STATUS_REMOVED
	e.RemovedAt = now
	e.RemovedBy = msg.Authority
	// SECURITY: reason accumulates across remove cycles (a future
	// "re-add then remove" would extend the trail). Cap at
	// ReasonMaxLen so the row size stays bounded.
	if e.Reason != "" && msg.Reason != "" {
		e.Reason = types.TruncateReason(e.Reason + " | removed: " + msg.Reason)
	} else if msg.Reason != "" {
		e.Reason = types.TruncateReason("removed: " + msg.Reason)
	}
	if err := m.SetEntry(ctx, e); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	if m.audit != nil {
		m.audit.RecordSanctionsAction(ctx, e.ListId, e.Subject, "remove_entry", msg.Reason)
	}
	emit(ctx, "sanctions_entry_removed", "list_id", e.ListId, "subject", e.Subject, "reason", msg.Reason)
	return &types.MsgRemoveEntryResponse{}, nil
}

// ---------------------------------------------------------------------------
// Propose + confirm
// ---------------------------------------------------------------------------

func (m msgServer) ProposeDelta(goCtx context.Context, msg *types.MsgProposeDelta) (*types.MsgProposeDeltaResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := m.GetList(ctx, msg.ListId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", msg.ListId)
	}
	if l.Status != types.ListStatus_LIST_STATUS_ACTIVE {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"list %s status %s does not accept proposals", l.Id, l.Status.String())
	}
	params := m.GetParams(ctx)
	if uint32(len(msg.Actions)) > params.MaxProposalActions {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"%d actions exceed cap %d", len(msg.Actions), params.MaxProposalActions)
	}
	if m.CountPendingProposals(ctx) >= params.MaxPendingProposals {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"max_pending_proposals cap %d reached", params.MaxPendingProposals)
	}
	id, err := m.NextProposalID(ctx)
	if err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("seq: %s", err)
	}
	// Normalise list_id on every action so the confirm path doesn't
	// have to second-guess.
	for i := range msg.Actions {
		msg.Actions[i].Entry.ListId = msg.ListId
	}
	p := types.Proposal{
		Id:           id,
		ListId:       msg.ListId,
		Proposer:     msg.Proposer,
		Status:       types.ProposalStatus_PROPOSAL_STATUS_PENDING,
		Actions:      msg.Actions,
		SourceUri:    msg.SourceUri,
		SourceDigest: msg.SourceDigest,
		SubmittedAt:  ctx.BlockTime().Unix(),
	}
	if err := m.SetProposal(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_proposal_submitted",
		"proposal_id", fmt.Sprintf("%d", id), "list_id", msg.ListId,
		"proposer", msg.Proposer, "actions", fmt.Sprintf("%d", len(msg.Actions)))
	return &types.MsgProposeDeltaResponse{ProposalId: id}, nil
}

func (m msgServer) ConfirmProposal(goCtx context.Context, msg *types.MsgConfirmProposal) (*types.MsgConfirmProposalResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProposal(ctx, msg.ProposalId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("proposal %d", msg.ProposalId)
	}
	if p.Status != types.ProposalStatus_PROPOSAL_STATUS_PENDING {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"proposal %d status %s not PENDING", p.Id, p.Status.String())
	}
	l, ok := m.GetList(ctx, p.ListId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("list %s", p.ListId)
	}
	// SECURITY: re-check list status at confirm time. A proposal
	// submitted while the list was ACTIVE must NOT apply if governance
	// has since deprecated/archived the list.
	if l.Status != types.ListStatus_LIST_STATUS_ACTIVE {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"list %s status %s no longer accepts confirmations", l.Id, l.Status.String())
	}
	params := m.GetParams(ctx)
	now := ctx.BlockTime().Unix()
	added, removed := uint32(0), uint32(0)
	// Pre-flight: validate every action so we either apply all or
	// none. Per-action validation:
	//   - ADD: refuse if the entry already exists (use REMOVE then ADD).
	//   - REMOVE: refuse if the entry doesn't exist or is already REMOVED.
	type plan struct {
		action types.ProposalAction
	}
	plans := make([]plan, 0, len(p.Actions))
	for i, a := range p.Actions {
		switch a.Kind {
		case types.ProposalAction_KIND_ADD:
			if m.LookupHasEntry(ctx, p.ListId, a.Entry.Subject) {
				return nil, sdkerrors.ErrInvalidRequest.Wrapf(
					"action %d: entry (%s, %s) already exists", i, p.ListId, a.Entry.Subject)
			}
		case types.ProposalAction_KIND_REMOVE:
			e, _, ok := m.LookupEntry(ctx, p.ListId, a.Entry.Subject)
			if !ok {
				return nil, sdkerrors.ErrInvalidRequest.Wrapf(
					"action %d: entry (%s, %s) does not exist", i, p.ListId, a.Entry.Subject)
			}
			if e.Status != types.EntryStatus_ENTRY_STATUS_ACTIVE {
				return nil, sdkerrors.ErrInvalidRequest.Wrapf(
					"action %d: entry (%s, %s) already REMOVED", i, p.ListId, a.Entry.Subject)
			}
		default:
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("action %d: invalid kind", i)
		}
		plans = append(plans, plan{action: a})
	}
	// Cap check happens AFTER the existence pre-flight so a proposal
	// to remove + add in equal numbers does not bump into the per-list
	// cap unnecessarily.
	currentEntries := m.CountEntries(ctx, l.Id)
	netAdd := int64(0)
	for _, pp := range plans {
		switch pp.action.Kind {
		case types.ProposalAction_KIND_ADD:
			netAdd++
		case types.ProposalAction_KIND_REMOVE:
			// Removed entries stay on chain (status flip), so the
			// count does not shrink. netAdd is unchanged.
		}
	}
	if uint32(int64(currentEntries)+netAdd) > params.MaxEntriesPerList {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"applying proposal would push list %s past cap %d", l.Id, params.MaxEntriesPerList)
	}

	for _, pp := range plans {
		a := pp.action
		switch a.Kind {
		case types.ProposalAction_KIND_ADD:
			e := a.Entry
			e.Status = types.EntryStatus_ENTRY_STATUS_ACTIVE
			e.AddedAt = now
			e.AddedBy = p.Proposer
			if err := m.SetEntry(ctx, e); err != nil {
				return nil, sdkerrors.ErrLogic.Wrapf("set entry: %s", err)
			}
			added++
		case types.ProposalAction_KIND_REMOVE:
			e, _, _ := m.LookupEntry(ctx, p.ListId, a.Entry.Subject)
			e.Status = types.EntryStatus_ENTRY_STATUS_REMOVED
			e.RemovedAt = now
			e.RemovedBy = msg.Authority
			if err := m.SetEntry(ctx, e); err != nil {
				return nil, sdkerrors.ErrLogic.Wrapf("set entry: %s", err)
			}
			removed++
		}
	}
	l.EntryCount += uint64(added)
	if err := m.SetList(ctx, l); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set list: %s", err)
	}
	p.Status = types.ProposalStatus_PROPOSAL_STATUS_APPLIED
	p.ResolvedAt = now
	p.Resolver = msg.Authority
	if err := m.SetProposal(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set proposal: %s", err)
	}
	if m.audit != nil {
		m.audit.RecordSanctionsAction(ctx, l.Id, "", "confirm_proposal",
			fmt.Sprintf("id=%d add=%d remove=%d", p.Id, added, removed))
	}
	emit(ctx, "sanctions_proposal_confirmed",
		"proposal_id", fmt.Sprintf("%d", p.Id),
		"added", fmt.Sprintf("%d", added),
		"removed", fmt.Sprintf("%d", removed))
	return &types.MsgConfirmProposalResponse{AddedCount: added, RemovedCount: removed}, nil
}

func (m msgServer) RejectProposal(goCtx context.Context, msg *types.MsgRejectProposal) (*types.MsgRejectProposalResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetProposal(ctx, msg.ProposalId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("proposal %d", msg.ProposalId)
	}
	if p.Status != types.ProposalStatus_PROPOSAL_STATUS_PENDING {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"proposal %d status %s not PENDING", p.Id, p.Status.String())
	}
	now := ctx.BlockTime().Unix()
	p.Status = types.ProposalStatus_PROPOSAL_STATUS_REJECTED
	p.ResolvedAt = now
	p.Resolver = msg.Authority
	p.Reason = msg.Reason
	if err := m.SetProposal(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "sanctions_proposal_rejected",
		"proposal_id", fmt.Sprintf("%d", p.Id), "reason", msg.Reason)
	return &types.MsgRejectProposalResponse{}, nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("params: %s", err)
	}
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
