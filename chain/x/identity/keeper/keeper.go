package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/identity/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	Schema collections.Schema

	Params     collections.Item[types.Params]
	Accounts   collections.Map[string, types.Account]
	Registrars collections.Map[string, types.Registrar]
	Sanctions  collections.Map[string, types.Sanction]
	Policies   collections.Map[string, types.Policy]

	Audit         collections.Map[uint64, types.AuditEntry]
	AuditByModule collections.KeySet[collections.Pair[string, uint64]]
	AuditSeq      collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("identity: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,

		Params:     collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Accounts:   collections.NewMap(sb, types.AccountPrefix, "accounts", collections.StringKey, codec.CollValue[types.Account](cdc)),
		Registrars: collections.NewMap(sb, types.RegistrarPrefix, "registrars", collections.StringKey, codec.CollValue[types.Registrar](cdc)),
		Sanctions:  collections.NewMap(sb, types.SanctionPrefix, "sanctions", collections.StringKey, codec.CollValue[types.Sanction](cdc)),
		Policies:   collections.NewMap(sb, types.PolicyPrefix, "policies", collections.StringKey, codec.CollValue[types.Policy](cdc)),

		Audit: collections.NewMap(sb, types.AuditPrefix, "audit", collections.Uint64Key, codec.CollValue[types.AuditEntry](cdc)),
		AuditByModule: collections.NewKeySet(sb, types.AuditByModulePrefix, "audit_by_module",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		AuditSeq: collections.NewSequence(sb, types.AuditSeqPrefix, "audit_seq"),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params ---------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return p, nil
}

// ---- accounts -------------------------------------------------------------

func (k Keeper) GetAccountRaw(ctx context.Context, addr string) (types.Account, bool, error) {
	a, err := k.Accounts.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Account{}, false, nil
		}
		return types.Account{}, false, err
	}
	return a, true, nil
}

func (k Keeper) CountRegistrars(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Registrars.Walk(ctx, nil, func(string, types.Registrar) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

func (k Keeper) CountPolicies(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Policies.Walk(ctx, nil, func(string, types.Policy) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

func (k Keeper) IsRegistrar(ctx context.Context, addr string) (bool, error) {
	return k.Registrars.Has(ctx, addr)
}

func (k Keeper) GetPolicy(ctx context.Context, id string) (types.Policy, bool, error) {
	p, err := k.Policies.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Policy{}, false, nil
		}
		return types.Policy{}, false, err
	}
	return p, true, nil
}

// ---- audit ----------------------------------------------------------------

// appendAudit writes one monotonic audit entry. It never fails the
// caller's primary flow on a non-storage error: detail is truncated to
// the configured cap rather than rejected.
func (k Keeper) appendAudit(ctx context.Context, module, action, actor, subject, detail string) error {
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if uint32(len(detail)) > params.AuditMaxDetailLen {
		detail = detail[:params.AuditMaxDetailLen]
	}
	seq, err := k.AuditSeq.Next(ctx)
	if err != nil {
		return err
	}
	seq++ // 1-indexed; 0 is reserved as "unset"
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	entry := types.AuditEntry{
		Seq:     seq,
		Time:    sdkCtx.BlockTime().Unix(),
		Height:  sdkCtx.BlockHeight(),
		Module:  module,
		Action:  action,
		Actor:   actor,
		Subject: subject,
		Detail:  detail,
	}
	if err := k.Audit.Set(ctx, seq, entry); err != nil {
		return err
	}
	return k.AuditByModule.Set(ctx, collections.Join(module, seq))
}

// RecordAction is the cross-module hook other keepers call to append a
// compliance audit entry. Errors are swallowed (logged) so an audit
// write can never roll back a successful business transaction.
func (k Keeper) RecordAction(ctx sdk.Context, module, action, actor, subject, detail string) {
	if err := types.ValidateModuleTag(module); err != nil {
		module = "unknown"
	}
	if err := k.appendAudit(ctx, module, action, actor, subject, detail); err != nil {
		ctx.Logger().Error("identity: audit write failed", "module", module, "action", action, "err", err)
	}
}

// ---- sanctions (cross-module read) ----------------------------------------

// IsSanctioned reports whether addr is on the sanctions list. Used by
// every value-bearing module's transfer gate.
func (k Keeper) IsSanctioned(ctx sdk.Context, addr string) bool {
	has, err := k.Sanctions.Has(ctx, addr)
	if err != nil {
		// fail closed: treat storage errors as sanctioned to avoid
		// leaking value past an unreadable list.
		ctx.Logger().Error("identity: sanctions read failed", "addr", addr, "err", err)
		return true
	}
	return has
}

// ---- compliance gate ------------------------------------------------------

// RequireKYC enforces that addr has a kyc_cleared, unexpired, non-frozen
// Account. Used by modules that gate mint/receipt on KYC.
func (k Keeper) RequireKYC(ctx sdk.Context, addr string) error {
	if addr == "" {
		return nil
	}
	acc, found, err := k.GetAccountRaw(ctx, addr)
	if err != nil {
		return err
	}
	if !found {
		return types.ErrKYCRequired.Wrapf("account %s has no identity record", addr)
	}
	if acc.Status == types.AccountStatus_ACCOUNT_STATUS_FROZEN {
		return types.ErrAccountFrozen.Wrap(addr)
	}
	if !acc.KycCleared {
		return types.ErrKYCRequired.Wrap(addr)
	}
	if acc.KycExpiresAt != 0 && ctx.BlockTime().Unix() >= acc.KycExpiresAt {
		return types.ErrKYCRequired.Wrapf("%s kyc expired", addr)
	}
	return nil
}

// EvaluateTransfer is the unified compliance gate. It checks, in order:
// global sanctions on both parties, then the named policy's rules
// (paused, frozen, KYC, accreditation, jurisdiction allow/deny lists).
// An empty policyID means "no policy bound" and only the sanctions gate
// applies. from=="" or to=="" skips that party (mint/burn legs).
func (k Keeper) EvaluateTransfer(ctx sdk.Context, module, assetRef, policyID, from, to string, amount uint64) error {
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear {
		if from != "" && k.IsSanctioned(ctx, from) {
			return types.ErrSanctioned.Wrapf("sender %s", from)
		}
		if to != "" && k.IsSanctioned(ctx, to) {
			return types.ErrSanctioned.Wrapf("receiver %s", to)
		}
	}
	if policyID == "" {
		return nil
	}
	pol, found, err := k.GetPolicy(ctx, policyID)
	if err != nil {
		return err
	}
	if !found {
		return types.ErrNotFound.Wrapf("policy %q", policyID)
	}
	if pol.Paused {
		return types.ErrPolicyPaused.Wrap(policyID)
	}
	if err := k.checkParty(ctx, pol, from, false); err != nil {
		return err
	}
	if err := k.checkParty(ctx, pol, to, true); err != nil {
		return err
	}
	return nil
}

// checkParty applies a policy's rules to a single party. isReceiver
// gates the accreditation requirement (only receivers must be
// accredited; senders may always divest).
//
// A missing Account is treated as zero-valued (not kyc, not accredited,
// not frozen, empty jurisdiction). Each rule below then independently
// fails closed where proof of compliance is required, and fails open
// only where a rule cannot logically apply to an unregistered wallet:
//
//   - deny_frozen           : blocks only a known, FROZEN account; an
//                             unregistered wallet is by definition not
//                             frozen and is not blocked by this rule alone.
//   - require_kyc           : an unknown / un-cleared / expired party is denied.
//   - require_accredited    : (receiver) an unknown / un-accredited party is denied.
//   - allowed_jurisdictions : the party MUST be in the allow-list; unknown
//                             account or empty jurisdiction is denied.
//   - denied_jurisdictions  : the party must be PROVABLY outside the deny
//                             list; an unknown account or empty jurisdiction
//                             cannot be proven clear, so it is denied. This
//                             closes the "use a fresh / jurisdiction-less
//                             wallet to evade the deny list" bypass.
func (k Keeper) checkParty(ctx sdk.Context, pol types.Policy, addr string, isReceiver bool) error {
	if addr == "" {
		return nil
	}
	acc, found, err := k.GetAccountRaw(ctx, addr)
	if err != nil {
		return err
	}

	if pol.DenyFrozen && found && acc.Status == types.AccountStatus_ACCOUNT_STATUS_FROZEN {
		return types.ErrAccountFrozen.Wrap(addr)
	}
	if pol.RequireKyc {
		if !found || !acc.KycCleared {
			return types.ErrKYCRequired.Wrap(addr)
		}
		if acc.KycExpiresAt != 0 && ctx.BlockTime().Unix() >= acc.KycExpiresAt {
			return types.ErrKYCRequired.Wrapf("%s kyc expired", addr)
		}
	}
	if isReceiver && pol.RequireAccredited && (!found || !acc.Accredited) {
		return types.ErrAccreditedReq.Wrap(addr)
	}
	if len(pol.DeniedJurisdictions) > 0 {
		if !found || acc.Jurisdiction == "" {
			return types.ErrJurisdictionDenied.Wrapf("%s has no jurisdiction on record", addr)
		}
		if contains(pol.DeniedJurisdictions, acc.Jurisdiction) {
			return types.ErrJurisdictionDenied.Wrapf("%s in %s", addr, acc.Jurisdiction)
		}
	}
	if len(pol.AllowedJurisdictions) > 0 {
		if !found || !contains(pol.AllowedJurisdictions, acc.Jurisdiction) {
			return types.ErrJurisdictionDenied.Wrapf("%s jurisdiction %q not in allow-list", addr, acc.Jurisdiction)
		}
	}
	return nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
