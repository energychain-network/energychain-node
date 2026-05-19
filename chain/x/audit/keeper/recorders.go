package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/audit/types"
)

// Cross-module audit recorders.
//
// Every consumer module (x/stablecoin, x/eac, x/carbon, x/rwa,
// x/escrow, x/contract, x/market, x/clearing, x/auction,
// x/scheduler, x/streampay, x/cfe247, x/sanctions, x/mrv,
// x/dispute, x/dataslash, x/policy) declares its OWN narrow
// AuditKeeper interface in its expected_keepers.go. This file
// implements every one of those interfaces against the single
// underlying RecordAuditLog primitive so that ONE concrete
// auditkeeper.Keeper satisfies all 17 surfaces at once.
//
// Design:
//   - Each Record*Action helper builds a structured EventType
//     of the form "<module>.<action>" so downstream consumers
//     can filter by module without parsing data.
//   - subject_id / asset_id / contract_id / dispute_id / etc.
//     all flow into the Target field as a "kind:id" string —
//     keeping Target a string preserves the existing
//     ByTarget indexes and avoids a proto schema bump.
//   - Severity defaults to INFO; compliance-critical actions
//     (force_transfer / freeze / ban / slash / sanctions hit)
//     are promoted to NOTICE / WARN so off-chain alerters can
//     subscribe via the BySeverity index.
//   - Failure to write an audit row is swallowed and logged at
//     Error level. Audit is a side-channel — losing one row
//     must never abort the originating Msg, otherwise a buggy
//     audit codec could DoS the chain.

// record is the shared low-level recorder. Module-specific
// helpers below all funnel through it so the data envelope is
// uniform.
func (k Keeper) record(ctx sdk.Context, eventType, actor, target, action, detail string, sev types.Severity) {
	id := k.GetNextID(ctx)
	log := types.AuditLog{
		ID:          id,
		EventType:   eventType,
		Actor:       actor,
		Target:      target,
		Action:      action,
		Data:        detail,
		BlockHeight: ctx.BlockHeight(),
		Timestamp:   ctx.BlockTime().Unix(),
		Severity:    sev,
	}
	if err := k.RecordAuditLog(ctx, log); err != nil {
		ctx.Logger().Error("audit: record cross-module action",
			"event_type", eventType, "actor", actor, "target", target,
			"action", action, "err", err)
	}
}

// severityForAction picks a default Severity based on the
// action verb. Compliance-critical verbs escalate so an
// alerter subscribing to BySeverity(>=NOTICE) catches them
// without having to enumerate every event_type.
//
// Callers may always override by recording directly via
// RecordAuditLog with an explicit Severity.
func severityForAction(action string) types.Severity {
	switch action {
	case "force_transfer", "freeze", "unfreeze", "blacklist", "unblacklist",
		"ban", "unban", "jail", "unjail", "slash", "expel",
		"sanctions_hit", "policy_denied", "bond_forfeit",
		"dispute_slash", "infraction_report":
		return types.Severity_SEVERITY_NOTICE
	case "ruling_finalized", "dispute_finalized", "auto_ban", "auto_jail":
		return types.Severity_SEVERITY_WARN
	default:
		return types.Severity_SEVERITY_INFO
	}
}

// ---- stablecoin -------------------------------------------------------

// RecordStablecoinAction satisfies x/stablecoin/types.AuditKeeper.
// Target encodes denom + issuer so a single index lookup can
// scope to "all events for issuer X under denom Y".
func (k Keeper) RecordStablecoinAction(ctx sdk.Context, denomID, issuerID, action, actor, subject, detail string) {
	target := fmt.Sprintf("stablecoin:%s:%s|subject:%s", denomID, issuerID, subject)
	k.record(ctx, "stablecoin."+action, actor, target, action, detail, severityForAction(action))
}

// ---- eac --------------------------------------------------------------

func (k Keeper) RecordEACAction(ctx sdk.Context, certificateID uint64, issuerID, action, actor, beneficiary, detail string) {
	target := fmt.Sprintf("eac:cert:%d|issuer:%s|beneficiary:%s", certificateID, issuerID, beneficiary)
	k.record(ctx, "eac."+action, actor, target, action, detail, severityForAction(action))
}

// ---- carbon -----------------------------------------------------------

func (k Keeper) RecordCarbonAction(ctx sdk.Context, assetID uint64, issuerID, action, actor, beneficiary, detail string) {
	target := fmt.Sprintf("carbon:asset:%d|issuer:%s|beneficiary:%s", assetID, issuerID, beneficiary)
	k.record(ctx, "carbon."+action, actor, target, action, detail, severityForAction(action))
}

// ---- cfe247 -----------------------------------------------------------

func (k Keeper) RecordCFEAction(ctx sdk.Context, subjectID, action, actor, detail string) {
	target := fmt.Sprintf("cfe247:%s", subjectID)
	k.record(ctx, "cfe247."+action, actor, target, action, detail, severityForAction(action))
}

// ---- rwa --------------------------------------------------------------

func (k Keeper) RecordRWAAction(ctx sdk.Context, tokenID uint64, issuerID, action, actor, subject, detail string) {
	target := fmt.Sprintf("rwa:token:%d|issuer:%s|subject:%s", tokenID, issuerID, subject)
	k.record(ctx, "rwa."+action, actor, target, action, detail, severityForAction(action))
}

// ---- escrow -----------------------------------------------------------

func (k Keeper) RecordEscrowAction(ctx sdk.Context, escrowID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("escrow:%d|subject:%s", escrowID, subject)
	k.record(ctx, "escrow."+action, actor, target, action, detail, severityForAction(action))
}

// ---- scheduler --------------------------------------------------------

func (k Keeper) RecordSchedulerAction(ctx sdk.Context, jobID uint64, action, actor, detail string) {
	target := fmt.Sprintf("scheduler:job:%d", jobID)
	k.record(ctx, "scheduler."+action, actor, target, action, detail, severityForAction(action))
}

// ---- streampay --------------------------------------------------------

func (k Keeper) RecordStreamPayAction(ctx sdk.Context, streamID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("streampay:%d|subject:%s", streamID, subject)
	k.record(ctx, "streampay."+action, actor, target, action, detail, severityForAction(action))
}

// ---- contract ---------------------------------------------------------

func (k Keeper) RecordContractAction(ctx sdk.Context, contractID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("contract:%d|subject:%s", contractID, subject)
	k.record(ctx, "contract."+action, actor, target, action, detail, severityForAction(action))
}

// ---- auction ----------------------------------------------------------

func (k Keeper) RecordAuctionAction(ctx sdk.Context, auctionID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("auction:%d|subject:%s", auctionID, subject)
	k.record(ctx, "auction."+action, actor, target, action, detail, severityForAction(action))
}

// ---- market -----------------------------------------------------------

func (k Keeper) RecordMarketAction(ctx sdk.Context, pairID, orderID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("market:pair:%d|order:%d|subject:%s", pairID, orderID, subject)
	k.record(ctx, "market."+action, actor, target, action, detail, severityForAction(action))
}

// ---- clearing ---------------------------------------------------------

func (k Keeper) RecordClearingAction(ctx sdk.Context, cycleID, memberID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("clearing:cycle:%d|member:%d|subject:%s", cycleID, memberID, subject)
	k.record(ctx, "clearing."+action, actor, target, action, detail, severityForAction(action))
}

// ---- mrv --------------------------------------------------------------

func (k Keeper) RecordMRVAction(ctx sdk.Context, reportID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("mrv:report:%d|subject:%s", reportID, subject)
	k.record(ctx, "mrv."+action, actor, target, action, detail, severityForAction(action))
}

// ---- dispute ----------------------------------------------------------

func (k Keeper) RecordDisputeAction(ctx sdk.Context, disputeID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("dispute:%d|subject:%s", disputeID, subject)
	k.record(ctx, "dispute."+action, actor, target, action, detail, severityForAction(action))
}

// ---- dataslash --------------------------------------------------------

func (k Keeper) RecordDataslashAction(ctx sdk.Context, providerID uint64, action, actor, subject, detail string) {
	target := fmt.Sprintf("dataslash:provider:%d|subject:%s", providerID, subject)
	k.record(ctx, "dataslash."+action, actor, target, action, detail, severityForAction(action))
}

// ---- sanctions --------------------------------------------------------

func (k Keeper) RecordSanctionsAction(ctx sdk.Context, listID, subject, action, detail string) {
	target := fmt.Sprintf("sanctions:list:%s|subject:%s", listID, subject)
	k.record(ctx, "sanctions."+action, "" /* actor unknown at hook */, target,
		action, detail, severityForAction("sanctions_hit"))
}

// ---- policy -----------------------------------------------------------

// RecordPolicyDenial is shaped slightly differently because x/policy's
// expected_keeper signature carries a numeric denial code instead of a
// free-text action. We synthesise the action as "deny:<code>" so all
// other recorder conventions still apply.
func (k Keeper) RecordPolicyDenial(ctx sdk.Context, policyID, assetClass, assetID, sender, receiver string, code uint32, detail string) {
	action := fmt.Sprintf("deny:%d", code)
	target := fmt.Sprintf("policy:%s|asset:%s/%s|to:%s", policyID, assetClass, assetID, receiver)
	k.record(ctx, "policy."+action, sender, target, action, detail,
		types.Severity_SEVERITY_NOTICE)
}
