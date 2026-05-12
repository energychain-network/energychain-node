// Package dsl is the deterministic policy DSL shared by x/policy, x/sanctions,
// x/rwa, and x/stablecoin. It implements the "transfer-precondition" rules
// described in docs/native-modules.md §4.18:
//
//	a composable set of predicates over identity (DID), holdings,
//	jurisdictions, lock-ups, and per-account caps; rules can be attached
//	to any asset contract and evolved through governance.
//
// Design notes:
//
//   - Rules are *data*, not code. They serialize as protobuf so they can be
//     stored, governance-upgraded, and queried like any other module state.
//
//   - Evaluation is *pure*: the engine only reads from the Context the
//     caller passes in. Side effects (freezing balances, emitting audit
//     events) are the caller's responsibility.
//
//   - Failure modes are *typed*: every rejection returns a Result with a
//     stable RuleCode so client SDKs can map machine-readable error codes
//     to user-friendly messages.
//
// This package deliberately does NOT depend on cosmos-sdk types. The engine
// is plain Go so it can be unit-tested without spinning up a chain context.
package dsl

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// RuleCode is the machine-readable identifier returned by the engine when a
// rule denies a transfer. Codes are stable across module upgrades; to
// retire a code, mark it deprecated but never reuse the integer.
type RuleCode uint32

const (
	CodeAllow RuleCode = 0 // sentinel: evaluation succeeded.

	CodeRequireKYC          RuleCode = 1001
	CodeRequireAccredited   RuleCode = 1002
	CodeRequireKYBPaperwork RuleCode = 1003

	CodeJurisdictionDenied RuleCode = 2001
	CodeJurisdictionUnknown RuleCode = 2002

	CodeSanctionsHit       RuleCode = 3001
	CodeFreezeActive       RuleCode = 3002
	CodeBlacklistedAddress RuleCode = 3003

	CodeLockupActive       RuleCode = 4001
	CodeVestingNotMature   RuleCode = 4002
	CodePerHolderCapHit    RuleCode = 4003
	CodeMaxHoldersExceeded RuleCode = 4004

	CodeMissingCredential RuleCode = 5001
	CodeRevokedCredential RuleCode = 5002
)

// Result captures the outcome of evaluating a Policy. Success returns
// {Allowed: true, Code: CodeAllow}. Failure returns the first rule that
// fired, with both a stable Code and a human-readable Detail.
//
// The struct is deliberately not an `error`: callers frequently want to
// return both the underlying error (for ABCI logs) and the structured Code
// (for the client envelope), so we keep the two channels separate.
type Result struct {
	Allowed bool
	Code    RuleCode
	Detail  string
	// FailedRule is the index of the policy rule that triggered the deny;
	// useful for governance UIs that highlight "which clause blocked me".
	// 0 when Allowed.
	FailedRule int
}

// Error returns a non-nil error iff the Result is a denial. Saves the
// `if !res.Allowed { return errors.New(res.Detail) }` boilerplate at
// callsites.
func (r Result) Error() error {
	if r.Allowed {
		return nil
	}
	return fmt.Errorf("policy [%d]: %s", r.Code, r.Detail)
}

// Subject is the actor under evaluation. Two subjects are passed to
// Evaluate (sender + receiver) so a Policy can express asymmetric rules
// like "receiver must be accredited but sender does not need to be".
type Subject struct {
	// DID is the bech32 address (acts as a primary key into x/did).
	DID string
	// Jurisdiction is the ISO-3166 alpha-2 country code; "" when
	// unknown. Engine treats unknown as "not in any allow-list".
	Jurisdiction string
	// Credentials is the set of VC types the subject currently holds in
	// non-revoked state. Type strings follow the W3C-VC convention
	// `urn:vc:<issuer>:<class>` but are opaque to the engine.
	Credentials map[string]bool
	// Holdings is the subject's current balance of the asset under
	// evaluation. 0 for new holders.
	Holdings uint64
}

// Context bundles every external observation the engine needs to evaluate
// the rules. Callers fill it in by querying x/did, x/sanctions, etc.
type Context struct {
	Sender   Subject
	Receiver Subject
	// Amount is the transfer quantity. The engine never subtracts it from
	// Sender.Holdings; that's the caller's responsibility (we only see
	// the post-state via Holdings). Amount is consulted by per-holder cap
	// checks against `Receiver.Holdings + Amount`.
	Amount uint64
	// Now is the chain's BlockTime in Unix seconds; used by lockup /
	// vesting rules. Engine never reads time.Now() to stay deterministic.
	Now int64
	// Sanctioned is the set of addresses currently flagged by x/sanctions.
	// Membership is by DID string (bech32). Bool value is irrelevant; the
	// presence of the key is the signal.
	Sanctioned map[string]bool
	// Frozen is the set of addresses for which x/rwa or x/stablecoin has
	// active freezes. Same membership convention as Sanctioned.
	Frozen map[string]bool
	// LockupExpiry maps DID -> Unix seconds at which the holder's lockup
	// terminates. Missing DID == no lockup.
	LockupExpiry map[string]int64
}

// RuleKind enumerates the operators supported by the DSL. Renumbering is a
// breaking change for stored Policies; only append new values.
type RuleKind uint32

const (
	RuleUnspecified RuleKind = 0

	RuleRequireCredential   RuleKind = 1  // params: type
	RuleRequireKYC          RuleKind = 2
	RuleRequireAccredited   RuleKind = 3
	RuleRequireKYB          RuleKind = 4
	RuleJurisdictionAllow   RuleKind = 10 // params: list of ISO codes
	RuleJurisdictionDeny    RuleKind = 11
	RuleNotSanctioned       RuleKind = 20
	RuleNotFrozen           RuleKind = 21
	RuleLockupExpired       RuleKind = 30
	RuleMaxPerHolder        RuleKind = 40 // params: max
	RuleMaxHolders          RuleKind = 41 // params: max + currentHolders
)

// Side selects which subject(s) a rule applies to. Lets a single Policy
// express "sender + receiver must both be KYCd" in one row.
type Side uint8

const (
	SideReceiver Side = 0
	SideSender   Side = 1
	SideBoth     Side = 2
)

// Rule is a single predicate. Params is a free-form map so the protobuf
// schema does not need bumping every time a new operator lands; the engine
// validates required keys per Kind.
type Rule struct {
	Kind   RuleKind
	Side   Side
	Params map[string]string
}

// Policy is an ordered list of rules. AND semantics: a transfer is allowed
// only when every rule passes. We deliberately do not expose OR / NOT
// composition at the DSL level — they are sources of policy ambiguity
// disliked by compliance reviewers; instead, callers should split into
// multiple Policies and pick one per branch in client code.
type Policy struct {
	// Name is a human-readable label, e.g. "us-reg-d-506c". Stored on
	// chain so audit logs can name the policy that approved a transfer.
	Name  string
	Rules []Rule
}

// Evaluate runs the rules in order and returns the first denial, or the
// success sentinel. O(len(rules)).
func (p Policy) Evaluate(ctx Context) Result {
	for i, r := range p.Rules {
		if res := evalRule(r, ctx); !res.Allowed {
			res.FailedRule = i
			return res
		}
	}
	return Result{Allowed: true}
}

// Validate ensures the Policy is structurally well-formed: every rule has
// a known kind and the required params. It does NOT execute the rules.
func (p Policy) Validate() error {
	if p.Name == "" {
		return errors.New("dsl: policy name must be set")
	}
	if len(p.Name) > 64 {
		return fmt.Errorf("dsl: policy name too long (%d > 64)", len(p.Name))
	}
	for i, r := range p.Rules {
		if err := validateRule(r); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
}

// validateRule checks the static shape of a single rule.
func validateRule(r Rule) error {
	switch r.Kind {
	case RuleUnspecified:
		return errors.New("kind must be set")
	case RuleRequireCredential:
		if _, ok := r.Params["type"]; !ok {
			return errors.New("require_credential: missing param 'type'")
		}
	case RuleJurisdictionAllow, RuleJurisdictionDeny:
		if _, ok := r.Params["countries"]; !ok {
			return errors.New("jurisdiction rule: missing param 'countries'")
		}
	case RuleMaxPerHolder, RuleMaxHolders:
		if _, ok := r.Params["max"]; !ok {
			return errors.New("max_* rule: missing param 'max'")
		}
	}
	return nil
}

// evalRule dispatches to the per-kind logic. Centralised here so adding
// a new RuleKind is a single switch arm + a helper.
//
// SECURITY NOTE: a few rule kinds are inherently single-sided regardless
// of the caller's declared Side. RuleMaxPerHolder is the canonical
// example — it caps how much a holder may end up with after the
// transfer, which only ever applies to the receiver (the sender's
// balance shrinks). Forcing the side here prevents a misconfigured
// policy from incorrectly blocking the sender or, worse, allowing a
// misconfigured `Side=SideBoth` to be silently weakened by both
// subjects passing the wrong arithmetic.
func evalRule(r Rule, ctx Context) Result {
	side := effectiveSide(r)
	subjects := selectSubjects(side, ctx)
	for _, s := range subjects {
		if res := evalRuleForSubject(r, s, ctx); !res.Allowed {
			return res
		}
	}
	return Result{Allowed: true}
}

// effectiveSide overrides the rule-declared Side for kinds whose
// semantics are unambiguously single-sided.
func effectiveSide(r Rule) Side {
	if r.Kind == RuleMaxPerHolder {
		// Receiver-only by definition; sender's holdings shrink, not
		// grow, so cap math doesn't apply to them.
		return SideReceiver
	}
	return r.Side
}

func selectSubjects(side Side, ctx Context) []Subject {
	switch side {
	case SideSender:
		return []Subject{ctx.Sender}
	case SideReceiver:
		return []Subject{ctx.Receiver}
	default:
		return []Subject{ctx.Sender, ctx.Receiver}
	}
}

// evalRuleForSubject contains the actual predicate logic. Splitting it
// from the dispatch above keeps each operator's intent legible.
func evalRuleForSubject(r Rule, s Subject, ctx Context) Result {
	switch r.Kind {
	case RuleRequireCredential:
		ct := r.Params["type"]
		if !s.Credentials[ct] {
			return deny(CodeMissingCredential, fmt.Sprintf("subject %s missing credential %s", s.DID, ct))
		}
	case RuleRequireKYC:
		if !s.Credentials["urn:vc:kyc:passed"] {
			return deny(CodeRequireKYC, fmt.Sprintf("subject %s lacks KYC", s.DID))
		}
	case RuleRequireAccredited:
		if !s.Credentials["urn:vc:investor:accredited"] {
			return deny(CodeRequireAccredited, fmt.Sprintf("subject %s lacks accredited-investor credential", s.DID))
		}
	case RuleRequireKYB:
		if !s.Credentials["urn:vc:kyb:passed"] {
			return deny(CodeRequireKYBPaperwork, fmt.Sprintf("subject %s lacks KYB", s.DID))
		}
	case RuleJurisdictionAllow:
		if !inCountrySet(s.Jurisdiction, r.Params["countries"], true) {
			return deny(CodeJurisdictionDenied, fmt.Sprintf("subject %s jurisdiction %q not in allow-list", s.DID, s.Jurisdiction))
		}
	case RuleJurisdictionDeny:
		if inCountrySet(s.Jurisdiction, r.Params["countries"], false) {
			return deny(CodeJurisdictionDenied, fmt.Sprintf("subject %s jurisdiction %q in deny-list", s.DID, s.Jurisdiction))
		}
	case RuleNotSanctioned:
		if ctx.Sanctioned[s.DID] {
			return deny(CodeSanctionsHit, fmt.Sprintf("subject %s is sanctioned", s.DID))
		}
	case RuleNotFrozen:
		if ctx.Frozen[s.DID] {
			return deny(CodeFreezeActive, fmt.Sprintf("subject %s account is frozen", s.DID))
		}
	case RuleLockupExpired:
		if exp, ok := ctx.LockupExpiry[s.DID]; ok && exp > ctx.Now {
			return deny(CodeLockupActive, fmt.Sprintf("subject %s lockup until %d (now %d)", s.DID, exp, ctx.Now))
		}
	case RuleMaxPerHolder:
		max, err := parseUint(r.Params["max"])
		if err != nil {
			return deny(CodePerHolderCapHit, fmt.Sprintf("malformed max param: %v", err))
		}
		// Receiver-only: enforced by effectiveSide(). We additionally
		// guard against uint overflow when summing.
		sum, overflow := safeAddU64(s.Holdings, ctx.Amount)
		if overflow {
			return deny(CodePerHolderCapHit, fmt.Sprintf("subject %s post-transfer balance overflows uint64", s.DID))
		}
		if sum > max {
			return deny(CodePerHolderCapHit, fmt.Sprintf("subject %s would exceed per-holder cap %d", s.DID, max))
		}
	}
	return Result{Allowed: true}
}

// deny is the canonical short-form for returning a denial Result.
func deny(code RuleCode, detail string) Result {
	return Result{Allowed: false, Code: code, Detail: detail}
}

// inCountrySet checks whether code matches any entry in the comma-delimited
// list. Empty code never matches an allow list (defaults to deny) but
// always escapes a deny list (defaults to allow); the caller picks the
// stricter of the two via Rule.Kind.
func inCountrySet(code string, list string, isAllow bool) bool {
	if code == "" {
		return !isAllow
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, c := range strings.Split(list, ",") {
		if strings.ToUpper(strings.TrimSpace(c)) == code {
			return true
		}
	}
	return false
}

// parseUint is a sandbox-friendly atoi: rejects empty / signed / too-long
// strings, and detects overflow during the digit fold so attacker-supplied
// "9999...999" cannot wrap to a permissive small cap.
func parseUint(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("empty value")
	}
	if len(s) > 20 {
		return 0, errors.New("too long")
	}
	var v uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q", c)
		}
		next := v*10 + uint64(c-'0')
		if next < v {
			return 0, errors.New("overflow")
		}
		v = next
	}
	return v, nil
}

// safeAddU64 returns a+b and a true overflow flag iff the sum wrapped.
// Used by predicate evaluators that compare post-transfer balances
// against caps — without this an attacker-supplied large amount could
// wrap to a small number and slip past a cap that should have rejected.
func safeAddU64(a, b uint64) (uint64, bool) {
	sum := a + b
	if sum < a {
		return 0, true
	}
	return sum, false
}

// SortedRules returns the policy rules in a deterministic, hash-stable
// order. Used by genesis encoders that must round-trip without depending
// on map iteration order.
func (p Policy) SortedRules() []Rule {
	out := make([]Rule, len(p.Rules))
	copy(out, p.Rules)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Side < out[j].Side
	})
	return out
}
