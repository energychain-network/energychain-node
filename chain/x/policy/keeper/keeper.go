package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/internal/dsl"
	"energychain/x/policy/types"
)

// Keeper is the policy module's persistent state surface. It registers
// governance-managed Policies (rule sets), tracks PolicyBindings that
// link assets to policies, and maintains an optional ring-buffer of
// EvaluationLog rows for forensics.
//
// The hot path is EvaluateTransfer, which asset modules (rwa,
// stablecoin, eac, …) call to ask "may this transfer happen?". The
// keeper composes a dsl.Context from cross-module dependencies (DID,
// sanctions) and runs the registered Policy.
//
// All cross-module dependencies (did, sanctions, audit) are nil-safe —
// dev/test wiring may pass nil and the engine falls back to permissive
// defaults documented per dependency.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	did       types.DIDKeeper
	sanctions types.SanctionsKeeper
	audit     types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	Policies            collections.Map[string, types.Policy]
	Bindings            collections.Map[collections.Pair[string, string], types.PolicyBinding]
	BindingByPolicy     collections.KeySet[collections.Triple[string, string, string]]
	EvaluationLogs      collections.Map[uint64, types.EvaluationLog]
	EvalLogByPolicy     collections.KeySet[collections.Pair[string, uint64]]
	EvalLogByAssetClass collections.KeySet[collections.Pair[string, uint64]]
	EvalLogIDSeq        collections.Sequence
	// EvalLogOldest tracks the smallest unevicted log id. Eviction reads
	// and bumps it in O(1). When the buffer is empty (right after init
	// genesis or after a wipe) the value is 0 and the next log starts
	// the buffer fresh; the keeper recovers the head lazily.
	EvalLogOldest collections.Item[uint64]
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	authority string,
	did types.DIDKeeper,
	sanctions types.SanctionsKeeper,
	audit types.AuditKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		did:          did,
		sanctions:    sanctions,
		audit:        audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),

		Policies: collections.NewMap(sb, types.PolicyCollectionPrefix, "policies",
			collections.StringKey, codec.CollValue[types.Policy](cdc)),

		Bindings: collections.NewMap(sb, types.BindingCollectionPrefix, "bindings",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.PolicyBinding](cdc)),

		BindingByPolicy: collections.NewKeySet(sb, types.BindingByPolicyPrefix, "binding_by_policy",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey)),

		EvaluationLogs: collections.NewMap(sb, types.EvaluationLogCollectionPrefix, "evaluation_logs",
			collections.Uint64Key, codec.CollValue[types.EvaluationLog](cdc)),

		EvalLogByPolicy: collections.NewKeySet(sb, types.EvaluationLogByPolicyPrefix, "eval_log_by_policy",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		EvalLogByAssetClass: collections.NewKeySet(sb, types.EvaluationLogByAssetClassPrefix, "eval_log_by_asset_class",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		EvalLogIDSeq: collections.NewSequence(sb, types.EvaluationLogIDSeqPrefix, "eval_log_seq"),

		EvalLogOldest: collections.NewItem(sb, types.EvaluationLogOldestPrefix, "eval_log_oldest",
			collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("policy keeper: build schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error { return k.Params.Set(ctx, p) }

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("policy params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// Policy CRUD
// ---------------------------------------------------------------------------

func (k Keeper) SetPolicy(ctx sdk.Context, p types.Policy) error {
	return k.Policies.Set(ctx, p.Id, p)
}

func (k Keeper) GetPolicy(ctx sdk.Context, id string) (types.Policy, bool) {
	p, err := k.Policies.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("policy decode", "id", id, "err", err)
		}
		return types.Policy{}, false
	}
	return p, true
}

func (k Keeper) HasPolicy(ctx sdk.Context, id string) bool {
	has, _ := k.Policies.Has(ctx, id)
	return has
}

// CountPolicies walks the primary collection. Cheap because the per-block
// cap (Params.MaxPolicies) keeps the size bounded; we use it during
// MsgRegisterPolicy to enforce the cap.
func (k Keeper) CountPolicies(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Policies.Walk(ctx, nil, func(_ string, _ types.Policy) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

func (k Keeper) SetBinding(ctx sdk.Context, b types.PolicyBinding) error {
	if err := k.Bindings.Set(ctx, collections.Join(b.AssetClass, b.AssetId), b); err != nil {
		return err
	}
	return k.BindingByPolicy.Set(ctx, collections.Join3(b.PolicyId, b.AssetClass, b.AssetId))
}

func (k Keeper) GetBinding(ctx sdk.Context, assetClass, assetID string) (types.PolicyBinding, bool) {
	b, err := k.Bindings.Get(ctx, collections.Join(assetClass, assetID))
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("binding decode", "class", assetClass, "id", assetID, "err", err)
		}
		return types.PolicyBinding{}, false
	}
	return b, true
}

func (k Keeper) RemoveBinding(ctx sdk.Context, assetClass, assetID string) error {
	b, ok := k.GetBinding(ctx, assetClass, assetID)
	if !ok {
		return collections.ErrNotFound
	}
	if err := k.Bindings.Remove(ctx, collections.Join(assetClass, assetID)); err != nil {
		return err
	}
	if err := k.BindingByPolicy.Remove(ctx, collections.Join3(b.PolicyId, b.AssetClass, b.AssetId)); err != nil &&
		!errors.Is(err, collections.ErrNotFound) {
		return err
	}
	return nil
}

// CountBindings is the binding-table equivalent of CountPolicies.
func (k Keeper) CountBindings(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Bindings.Walk(ctx, nil, func(_ collections.Pair[string, string], _ types.PolicyBinding) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Evaluation logs
// ---------------------------------------------------------------------------

// NextEvalLogID issues a fresh, monotonically increasing eval-log id.
// IDs start at 1 (id 0 is reserved as the "uninitialised" sentinel
// used by validators and the EvalLogOldest cursor); this matches the
// convention in x/audit. The collections.Sequence library returns 0
// on the first call, so we use Peek+1 semantics rather than Next().
func (k Keeper) NextEvalLogID(ctx sdk.Context) (uint64, error) {
	cur, err := k.EvalLogIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	next := cur + 1
	return next, k.EvalLogIDSeq.Set(ctx, next)
}

// recordEvaluationLog persists an EvaluationLog and indexes it by
// policy + asset_class. When the buffer fills, the oldest entry is
// evicted in O(1) using the EvalLogOldest cursor.
//
// SECURITY: the eviction path is bounded to amortised O(1) per write.
// Earlier prototypes walked the full collection on every denial; that
// turned a sustained denial flood (which an attacker can trigger via
// Evaluate or by repeatedly retrying a transfer) into an O(N) DoS
// vector. The cursor approach keeps gas predictable regardless of
// EvaluationLogMax.
func (k Keeper) recordEvaluationLog(ctx sdk.Context, l types.EvaluationLog) {
	if err := k.EvaluationLogs.Set(ctx, l.Id, l); err != nil {
		ctx.Logger().Error("policy: persist eval log", "err", err)
		return
	}
	if l.PolicyId != "" {
		_ = k.EvalLogByPolicy.Set(ctx, collections.Join(l.PolicyId, l.Id))
	}
	if l.AssetClass != "" {
		_ = k.EvalLogByAssetClass.Set(ctx, collections.Join(l.AssetClass, l.Id))
	}
	k.evictExcessLogsO1(ctx)
}

// evictExcessLogsO1 is the O(1) replacement for the legacy walk-based
// evictor. It uses the EvalLogOldest cursor to know which log to evict
// next; on first call (cursor==0) we re-derive the head with a single
// Walk-with-limit-1 call.
//
// Bounded eviction: at most maxEvictPerCall entries removed per write.
// Sustained denial floods catch up across subsequent calls.
const maxEvictPerCall = 4

func (k Keeper) evictExcessLogsO1(ctx sdk.Context) {
	params := k.GetParams(ctx)
	maxLogs := uint64(params.EvaluationLogMax)
	if maxLogs == 0 {
		return
	}
	// Peek returns the largest id ever issued (0 == none).
	largestID, err := k.EvalLogIDSeq.Peek(ctx)
	if err != nil || largestID == 0 {
		return
	}
	oldest, _ := k.EvalLogOldest.Get(ctx)
	if oldest == 0 {
		// Lazy initialisation: walk to find the smallest stored id
		// (the head). Runs exactly once after first denial.
		var first uint64
		_ = k.EvaluationLogs.Walk(ctx, nil, func(id uint64, _ types.EvaluationLog) (bool, error) {
			first = id
			return true, nil
		})
		if first == 0 {
			return
		}
		oldest = first
	}
	// IDs are dense (no gaps) so the live count is largestID-oldest+1.
	currentSize := largestID - oldest + 1
	if currentSize <= maxLogs {
		_ = k.EvalLogOldest.Set(ctx, oldest)
		return
	}
	excess := currentSize - maxLogs
	if excess > maxEvictPerCall {
		excess = maxEvictPerCall
	}
	for i := uint64(0); i < excess; i++ {
		l, ok := k.getLog(ctx, oldest)
		oldest++
		if !ok {
			continue
		}
		_ = k.EvaluationLogs.Remove(ctx, l.Id)
		if l.PolicyId != "" {
			_ = k.EvalLogByPolicy.Remove(ctx, collections.Join(l.PolicyId, l.Id))
		}
		if l.AssetClass != "" {
			_ = k.EvalLogByAssetClass.Remove(ctx, collections.Join(l.AssetClass, l.Id))
		}
	}
	_ = k.EvalLogOldest.Set(ctx, oldest)
}

func (k Keeper) getLog(ctx sdk.Context, id uint64) (types.EvaluationLog, bool) {
	l, err := k.EvaluationLogs.Get(ctx, id)
	if err != nil {
		return types.EvaluationLog{}, false
	}
	return l, true
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for i, p := range gs.Policies {
		if err := k.SetPolicy(ctx, p); err != nil {
			return fmt.Errorf("policy %d: %w", i, err)
		}
	}
	for i, b := range gs.Bindings {
		if err := k.SetBinding(ctx, b); err != nil {
			return fmt.Errorf("binding %d: %w", i, err)
		}
	}
	for i, l := range gs.EvaluationLogs {
		if err := k.EvaluationLogs.Set(ctx, l.Id, l); err != nil {
			return fmt.Errorf("eval log %d: %w", i, err)
		}
		if l.PolicyId != "" {
			if err := k.EvalLogByPolicy.Set(ctx, collections.Join(l.PolicyId, l.Id)); err != nil {
				return err
			}
		}
		if l.AssetClass != "" {
			if err := k.EvalLogByAssetClass.Set(ctx, collections.Join(l.AssetClass, l.Id)); err != nil {
				return err
			}
		}
	}
	if gs.EvaluationLogCounter > 0 {
		if err := k.EvalLogIDSeq.Set(ctx, gs.EvaluationLogCounter); err != nil {
			return fmt.Errorf("seed eval log counter: %w", err)
		}
	}
	// Restore the eviction head cursor by walking the smallest stored
	// id. Cheap (one read) on InitGenesis since we walk anyway above
	// while loading EvaluationLogs.
	if len(gs.EvaluationLogs) > 0 {
		var head uint64
		_ = k.EvaluationLogs.Walk(ctx, nil, func(id uint64, _ types.EvaluationLog) (bool, error) {
			head = id
			return true, nil
		})
		if head > 0 {
			if err := k.EvalLogOldest.Set(ctx, head); err != nil {
				return fmt.Errorf("seed eval log oldest cursor: %w", err)
			}
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	_ = k.Policies.Walk(ctx, nil, func(_ string, p types.Policy) (bool, error) {
		gs.Policies = append(gs.Policies, p)
		return false, nil
	})
	_ = k.Bindings.Walk(ctx, nil, func(_ collections.Pair[string, string], b types.PolicyBinding) (bool, error) {
		gs.Bindings = append(gs.Bindings, b)
		return false, nil
	})
	_ = k.EvaluationLogs.Walk(ctx, nil, func(_ uint64, l types.EvaluationLog) (bool, error) {
		gs.EvaluationLogs = append(gs.EvaluationLogs, l)
		return false, nil
	})
	if seq, err := k.EvalLogIDSeq.Peek(ctx); err == nil {
		gs.EvaluationLogCounter = seq
	}
	return gs
}

// ---------------------------------------------------------------------------
// Engine entry point
// ---------------------------------------------------------------------------

// EvaluateRequest bundles every input EvaluateTransfer / EvaluateDryRun
// take. Optional fields default to their zero-value semantics
// documented in the dsl.Subject doc-comment.
//
// Holdings are POST-transfer projections — callers MUST pre-subtract
// the amount from sender and use the pre-transfer balance for the
// receiver. RuleMaxPerHolder adds Amount to Receiver.Holdings to
// derive the cap check.
type EvaluateRequest struct {
	AssetClass string
	AssetID    string
	Sender     string
	Receiver   string
	Amount     uint64

	// Override fields. When non-empty / non-zero they take precedence
	// over the on-chain DIDKeeper / SanctionsKeeper data.
	SenderJurisdictionOverride   string
	ReceiverJurisdictionOverride string
	SenderCredsOverride          []string
	ReceiverCredsOverride        []string
	SenderHoldings               uint64
	ReceiverHoldings             uint64
}

// EvaluateTransfer is the hot path called by asset modules at the
// pre-transfer hook. Returns nil iff the transfer is allowed.
//
// Resolution order:
//
//  1. If no binding exists for (assetClass, assetID) → ALLOW
//     (asset is opt-in; absence of binding = no policy, no constraint).
//  2. If binding's policy is missing → FAIL CLOSED. The binding pointed
//     at a policy that has been removed; the safe default is to deny
//     so a registry-corruption bug cannot silently weaken enforcement.
//  3. If policy.Status == DISABLED → FAIL CLOSED.
//  4. Build dsl.Context from on-chain state + caller-provided overrides
//     and evaluate the policy.
func (k Keeper) EvaluateTransfer(ctx sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error {
	res, _ := k.evaluate(ctx, EvaluateRequest{
		AssetClass: assetClass, AssetID: assetID,
		Sender: sender, Receiver: receiver, Amount: amount,
	}, true /* writeLogs */)
	return res.Error()
}

// EvaluateTransferDetailed is the rich entry point asset modules call
// when they need to convey holdings or pre-checked credentials. It
// records denial logs and emits audit events.
func (k Keeper) EvaluateTransferDetailed(ctx sdk.Context, req EvaluateRequest) (dsl.Result, string) {
	return k.evaluate(ctx, req, true /* writeLogs */)
}

// EvaluateDryRun is the read-only counterpart used by the query server.
// It MUST NOT mutate state — otherwise free queries become a denial-of-
// service vector against the EvaluationLog ring buffer.
//
// SECURITY: see the writeLogs gate in the implementation. Earlier
// prototypes always wrote denial logs from this path; that turned the
// gRPC Evaluate query into an unauthenticated state-mutation primitive.
func (k Keeper) EvaluateDryRun(ctx sdk.Context, req EvaluateRequest) (dsl.Result, string) {
	return k.evaluate(ctx, req, false /* writeLogs */)
}

// evaluate is the shared engine path. The writeLogs flag separates the
// transfer hot path (writes EvaluationLog rows + emits audit hooks)
// from the dry-run query path (read-only).
func (k Keeper) evaluate(ctx sdk.Context, req EvaluateRequest, writeLogs bool) (dsl.Result, string) {
	binding, ok := k.GetBinding(ctx, req.AssetClass, req.AssetID)
	if !ok {
		return dsl.Result{Allowed: true}, ""
	}
	policy, ok := k.GetPolicy(ctx, binding.PolicyId)
	if !ok {
		denial := dsl.Result{
			Allowed: false,
			Code:    dsl.CodeBindingMissing,
			Detail:  "binding references missing policy " + binding.PolicyId,
		}
		if writeLogs {
			k.maybeRecordDenial(ctx, binding.PolicyId, req.AssetClass, req.AssetID,
				req.Sender, req.Receiver, req.Amount, denial)
		}
		return denial, binding.PolicyId
	}
	if policy.Status == types.PolicyStatus_POLICY_STATUS_DISABLED {
		denial := dsl.Result{
			Allowed: false,
			Code:    dsl.CodePolicyDisabled,
			Detail:  "policy " + policy.Id + " is DISABLED",
		}
		if writeLogs {
			k.maybeRecordDenial(ctx, policy.Id, req.AssetClass, req.AssetID,
				req.Sender, req.Receiver, req.Amount, denial)
		}
		return denial, policy.Id
	}

	dctx := k.buildDSLContext(ctx, req)
	res := types.ToDSLPolicy(policy).Evaluate(dctx)
	if !res.Allowed && writeLogs {
		k.maybeRecordDenial(ctx, policy.Id, req.AssetClass, req.AssetID,
			req.Sender, req.Receiver, req.Amount, res)
	}
	return res, policy.Id
}

// buildDSLContext composes a dsl.Context from cross-module dependencies
// and the caller-supplied overrides. Each dependency is queried with a
// single narrow call so the engine never accidentally pulls in
// unneeded data.
//
// Override precedence: any non-empty override field wins over the
// on-chain value. Empty / zero values fall through to the keeper. This
// lets the Evaluate query implement counter-factual ("what if Alice
// WERE in CA?") simulations.
func (k Keeper) buildDSLContext(ctx sdk.Context, req EvaluateRequest) dsl.Context {
	now := ctx.BlockTime().Unix()
	dctx := dsl.Context{
		Sender: dsl.Subject{
			DID:         req.Sender,
			Credentials: credentialSet(req.SenderCredsOverride),
			Holdings:    req.SenderHoldings,
		},
		Receiver: dsl.Subject{
			DID:         req.Receiver,
			Credentials: credentialSet(req.ReceiverCredsOverride),
			Holdings:    req.ReceiverHoldings,
		},
		Amount:       req.Amount,
		Now:          now,
		Sanctioned:   map[string]bool{},
		Frozen:       map[string]bool{},
		LockupExpiry: map[string]int64{},
	}
	dctx.Sender.Jurisdiction = req.SenderJurisdictionOverride
	dctx.Receiver.Jurisdiction = req.ReceiverJurisdictionOverride
	if k.did != nil {
		if dctx.Sender.Jurisdiction == "" {
			dctx.Sender.Jurisdiction = k.did.Jurisdiction(ctx, req.Sender)
		}
		if dctx.Receiver.Jurisdiction == "" {
			dctx.Receiver.Jurisdiction = k.did.Jurisdiction(ctx, req.Receiver)
		}
	}
	if k.sanctions != nil {
		if k.sanctions.IsSanctioned(ctx, req.Sender) {
			dctx.Sanctioned[req.Sender] = true
		}
		if k.sanctions.IsSanctioned(ctx, req.Receiver) {
			dctx.Sanctioned[req.Receiver] = true
		}
	}
	return dctx
}

func credentialSet(creds []string) map[string]bool {
	if len(creds) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(creds))
	for _, c := range creds {
		out[c] = true
	}
	return out
}

// maybeRecordDenial conditionally writes a row to the EvaluationLog
// ring-buffer, gated by Params.LogDenials, AND emits a structured
// audit-keeper event when an x/audit dependency is wired.
func (k Keeper) maybeRecordDenial(
	ctx sdk.Context,
	policyID, assetClass, assetID, sender, receiver string,
	amount uint64,
	res dsl.Result,
) {
	params := k.GetParams(ctx)
	if !params.LogDenials || params.EvaluationLogMax == 0 {
		// Even when logging is off, still emit the audit hook so
		// downstream regulators don't lose visibility.
		k.maybeAuditDenial(ctx, policyID, assetClass, assetID, sender, receiver, res)
		return
	}
	id, err := k.NextEvalLogID(ctx)
	if err != nil {
		ctx.Logger().Error("policy: log seq", "err", err)
		k.maybeAuditDenial(ctx, policyID, assetClass, assetID, sender, receiver, res)
		return
	}
	detail := res.Detail
	if len(detail) > types.DetailMaxLen {
		detail = detail[:types.DetailMaxLen]
	}
	k.recordEvaluationLog(ctx, types.EvaluationLog{
		Id:              id,
		PolicyId:        policyID,
		AssetClass:      assetClass,
		AssetId:         assetID,
		Sender:          sender,
		Receiver:        receiver,
		Amount:          amount,
		ResultCode:      uint32(res.Code),
		FailedRuleIndex: uint32(res.FailedRule),
		Detail:          detail,
		Timestamp:       ctx.BlockTime().Unix(),
		Height:          ctx.BlockHeight(),
	})
	k.maybeAuditDenial(ctx, policyID, assetClass, assetID, sender, receiver, res)
}

func (k Keeper) maybeAuditDenial(
	ctx sdk.Context,
	policyID, assetClass, assetID, sender, receiver string,
	res dsl.Result,
) {
	if k.audit == nil {
		return
	}
	k.audit.RecordPolicyDenial(ctx, policyID, assetClass, assetID, sender, receiver, uint32(res.Code), res.Detail)
}
