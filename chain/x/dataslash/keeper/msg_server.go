package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dataslash/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k} }

// ---- Provider lifecycle ----------------------------------------------

func (s msgServer) RegisterProvider(ctx context.Context, m *types.MsgRegisterProvider) (*types.MsgRegisterProviderResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint32(len(m.Jurisdictions)) > p.MaxJurisdictionsPerProvider {
		return nil, fmt.Errorf("too many jurisdictions")
	}
	cur, err := s.k.countProviders(ctx)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxProviders) {
		return nil, fmt.Errorf("max_providers reached: %d", p.MaxProviders)
	}
	// Uniqueness gates — DID and signer must each map to at
	// most one provider. Sharing a signer across providers
	// would let a single operator switch identity silently.
	if _, ok, err := s.k.GetProviderByDID(ctx, m.Did); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("DID %s already registered", m.Did)
	}
	if _, ok, err := s.k.GetProviderBySigner(ctx, m.SignerAddress); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("signer_address %s already bound to another provider", m.SignerAddress)
	}
	denom := m.BondDenom
	if denom == "" {
		denom = p.BondDenomDefault
	}
	if err := s.k.requireStablecoin(ctx, denom); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.BondOwner); err != nil {
		return nil, err
	}
	id, err := s.k.NextProviderID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	prov := types.Provider{
		Id: id, Did: m.Did, SignerAddress: m.SignerAddress, Name: m.Name,
		Role: m.Role, BondDenom: denom, BondAmount: 0,
		RequiredBond: types.MinBondFor(p, m.Role),
		BondOwner:    m.BondOwner,
		// Brand-new providers START in ACTIVE status but with
		// zero bond; the IsActiveSigner check above requires
		// bond_amount >= required_bond, so no data is accepted
		// until PostBond is called. This keeps the registration
		// flow simple while still gating data acceptance on
		// the bond invariant.
		Status:        types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		CreatedAt:     now,
		UpdatedAt:     now,
		Jurisdictions: m.Jurisdictions,
	}
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	if err := s.k.ProviderByDID.Set(ctx, m.Did, id); err != nil {
		return nil, err
	}
	if err := s.k.ProviderBySigner.Set(ctx, m.SignerAddress, id); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.provider.registered",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("did", m.Did),
		sdk.NewAttribute("signer", m.SignerAddress),
		sdk.NewAttribute("role", m.Role.String()),
		sdk.NewAttribute("required_bond", u64s(prov.RequiredBond)),
	)
	s.k.recordAudit(ctx, id, "provider.register", m.Authority, m.Did, m.Name)
	return &types.MsgRegisterProviderResponse{ProviderId: id}, nil
}

func (s msgServer) UpdateProviderInfo(ctx context.Context, m *types.MsgUpdateProviderInfo) (*types.MsgUpdateProviderInfoResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if types.ProviderIsTerminal(prov.Status) {
		return nil, fmt.Errorf("provider %d is terminal (%s); rotate via new registration", prov.Id, prov.Status)
	}
	if m.NewName != "" {
		prov.Name = m.NewName
	}
	if m.NewBondOwner != "" && m.NewBondOwner != prov.BondOwner {
		// Sanctions must be re-checked on the incoming
		// bond_owner so a banned operator can't piggy-back
		// on a re-assignment.
		if err := s.k.requireUnsanctioned(ctx, m.NewBondOwner); err != nil {
			return nil, err
		}
		prov.BondOwner = m.NewBondOwner
	}
	if m.NewJurisdictions != nil {
		p, err := s.k.GetParams(ctx)
		if err != nil {
			return nil, err
		}
		if uint32(len(m.NewJurisdictions)) > p.MaxJurisdictionsPerProvider {
			return nil, fmt.Errorf("too many jurisdictions")
		}
		prov.Jurisdictions = m.NewJurisdictions
	}
	prov.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.provider.updated",
		sdk.NewAttribute("id", u64s(prov.Id)),
	)
	s.k.recordAudit(ctx, prov.Id, "provider.update", m.Authority, prov.Did, prov.Name)
	return &types.MsgUpdateProviderInfoResponse{}, nil
}

// ---- Bond lifecycle ---------------------------------------------------

func (s msgServer) PostBond(ctx context.Context, m *types.MsgPostBond) (*types.MsgPostBondResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if m.BondOwner != prov.BondOwner {
		return nil, fmt.Errorf("unauthorized: bond_owner mismatch")
	}
	if types.ProviderIsTerminal(prov.Status) || prov.Status == types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
		return nil, fmt.Errorf("cannot post bond in status %s", prov.Status)
	}
	if err := s.k.requireStablecoin(ctx, prov.BondDenom); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.BondOwner); err != nil {
		return nil, err
	}
	if err := s.k.moveBond(ctx, prov.BondDenom, m.BondOwner, DataslashPoolAccount, m.Amount); err != nil {
		return nil, fmt.Errorf("bond transfer: %w", err)
	}
	nb, err := types.SafeAdd(prov.BondAmount, m.Amount)
	if err != nil {
		return nil, err
	}
	prov.BondAmount = nb
	prov.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.bond.posted",
		sdk.NewAttribute("provider_id", u64s(prov.Id)),
		sdk.NewAttribute("amount", u64s(m.Amount)),
		sdk.NewAttribute("new_bond_amount", u64s(nb)),
	)
	s.k.recordAudit(ctx, prov.Id, "bond.post", m.BondOwner, "", u64s(m.Amount))
	return &types.MsgPostBondResponse{NewBondAmount: nb}, nil
}

func (s msgServer) IncreaseBond(ctx context.Context, m *types.MsgIncreaseBond) (*types.MsgIncreaseBondResponse, error) {
	// IncreaseBond shares its full implementation with
	// PostBond (same gates, same path). Keeping the message
	// distinct documents intent for callers / wallets.
	resp, err := s.PostBond(ctx, &types.MsgPostBond{
		BondOwner: m.BondOwner, ProviderId: m.ProviderId, Amount: m.Amount,
	})
	if err != nil {
		return nil, err
	}
	return &types.MsgIncreaseBondResponse{NewBondAmount: resp.NewBondAmount}, nil
}

func (s msgServer) RequestUnbond(ctx context.Context, m *types.MsgRequestUnbond) (*types.MsgRequestUnbondResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if m.BondOwner != prov.BondOwner {
		return nil, fmt.Errorf("unauthorized: bond_owner mismatch")
	}
	if prov.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE &&
		prov.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		return nil, fmt.Errorf("cannot request unbond in status %s", prov.Status)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	prov.Status = types.ProviderStatus_PROVIDER_STATUS_UNBONDING
	prov.WithdrawAt = now + p.UnbondCooldownSeconds
	prov.UpdatedAt = now
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.bond.unbond_requested",
		sdk.NewAttribute("provider_id", u64s(prov.Id)),
		sdk.NewAttribute("withdraw_at", i64s(prov.WithdrawAt)),
	)
	s.k.recordAudit(ctx, prov.Id, "bond.unbond_request", m.BondOwner, "", i64s(prov.WithdrawAt))
	return &types.MsgRequestUnbondResponse{WithdrawAt: prov.WithdrawAt}, nil
}

func (s msgServer) WithdrawUnbonded(ctx context.Context, m *types.MsgWithdrawUnbonded) (*types.MsgWithdrawUnbondedResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if m.BondOwner != prov.BondOwner {
		return nil, fmt.Errorf("unauthorized: bond_owner mismatch")
	}
	if prov.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
		return nil, fmt.Errorf("provider %d not UNBONDING (%s)", prov.Id, prov.Status)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if now < prov.WithdrawAt {
		return nil, fmt.Errorf("cooldown not elapsed (withdraw_at=%d, now=%d)", prov.WithdrawAt, now)
	}
	amount := prov.BondAmount
	if amount > 0 {
		// Defense-in-depth: re-check sanctions on the refund
		// recipient. moveBond's IsAccountBlocked covers the
		// stablecoin-denom freeze list, but the sanctions
		// registry is a separate, OFAC-style layer. We refuse
		// to refund to a sanctioned address here; operators
		// can re-attempt once sanctions lift, or governance
		// can route the residual through a treasury sweep.
		if err := s.k.requireUnsanctioned(ctx, m.BondOwner); err != nil {
			return nil, fmt.Errorf("refund bond: %w", err)
		}
		if err := s.k.moveBond(ctx, prov.BondDenom, DataslashPoolAccount, m.BondOwner, amount); err != nil {
			return nil, fmt.Errorf("refund bond: %w", err)
		}
	}
	prov.BondAmount = 0
	prov.Status = types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN
	prov.WithdrawAt = 0
	prov.UpdatedAt = now
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.bond.withdrawn",
		sdk.NewAttribute("provider_id", u64s(prov.Id)),
		sdk.NewAttribute("amount", u64s(amount)),
	)
	s.k.recordAudit(ctx, prov.Id, "bond.withdraw", m.BondOwner, "", u64s(amount))
	return &types.MsgWithdrawUnbondedResponse{Withdrawn: amount}, nil
}

// ---- Infraction reporting ---------------------------------------------

func (s msgServer) ReportInfraction(ctx context.Context, m *types.MsgReportInfraction) (*types.MsgReportInfractionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if m.Actor != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: only authority may report infractions")
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("evidence_uri", m.EvidenceUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if prov.Status == types.ProviderStatus_PROVIDER_STATUS_BANNED {
		return nil, fmt.Errorf("provider %d already BANNED", prov.Id)
	}
	if prov.Status == types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN {
		return nil, fmt.Errorf("provider %d already WITHDRAWN", prov.Id)
	}
	// Cap infractions per provider so a malicious authority
	// cannot bloat state with infinite report rows. Hard
	// caps already limit MaxInfractionsPerProvider at param
	// level; we count up to that.
	if prov.InfractionCount >= p.MaxInfractionsPerProvider {
		return nil, fmt.Errorf("max_infractions_per_provider reached: %d", p.MaxInfractionsPerProvider)
	}
	slashBps := m.OverrideSlashBps
	if slashBps == 0 {
		slashBps = types.DefaultSlashFor(p, m.Kind)
	}
	slashed, _ := types.SlashAmount(prov.BondAmount, slashBps)
	id, err := s.k.NextInfractionID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	inf := types.Infraction{
		Id: id, ProviderId: prov.Id, Kind: m.Kind, SlashBps: slashBps,
		SlashedAmount: slashed, EvidenceUri: m.EvidenceUri, EvidenceHash: m.EvidenceHash,
		Reporter: m.Actor, DisputeId: m.DisputeId,
		OccurredAt: now, ProcessedAt: now, Reason: m.Reason,
	}
	if err := s.k.Infractions.Set(ctx, id, inf); err != nil {
		return nil, err
	}
	if err := s.k.InfractionByProvider.Set(ctx, collections.Join(prov.Id, id)); err != nil {
		return nil, err
	}
	if err := s.k.InfractionByKind.Set(ctx, collections.Join(uint32(m.Kind), id)); err != nil {
		return nil, err
	}
	// Slashed funds stay in the dispute pool — a future
	// governance treasury sweep can distribute them; the
	// keeper never burns or routes them out automatically.
	newBond, err := types.SafeSub(prov.BondAmount, slashed)
	if err != nil {
		// shouldn't happen: SlashAmount guarantees slashed <= bond
		return nil, err
	}
	prov.BondAmount = newBond
	newCount, err := safeAddU32(prov.InfractionCount, 1)
	if err != nil {
		return nil, err
	}
	prov.InfractionCount = newCount
	prov.UpdatedAt = now
	// Auto-escalation: cross threshold ⇒ jail or ban. The
	// updated provider row is written exactly once at the
	// end so the by_status / by_jail_until indexes settle
	// cleanly.
	autoBanned := false
	autoJailed := false
	if newCount >= p.AutoBanThreshold && prov.Status != types.ProviderStatus_PROVIDER_STATUS_BANNED {
		prov.Status = types.ProviderStatus_PROVIDER_STATUS_BANNED
		autoBanned = true
		prov.JailUntil = 0
	} else if newCount >= p.AutoJailThreshold &&
		prov.Status == types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		// JAILED only escalates ACTIVE; UNBONDING / JAILED
		// stay in place (you can't jail an already-jailed
		// provider, and an UNBONDING provider already
		// can't accept data).
		prov.Status = types.ProviderStatus_PROVIDER_STATUS_JAILED
		prov.JailUntil = now + p.AutoJailSeconds
		autoJailed = true
	}
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	if autoJailed {
		if err := s.recordJail(ctx, prov.Id, prov.JailUntil, m.Reason, m.Actor, true); err != nil {
			return nil, err
		}
	}
	if autoBanned {
		if err := s.recordBan(ctx, prov.Id, m.Reason, m.Actor, true); err != nil {
			return nil, err
		}
	}
	s.k.emit(ctx, "dataslash.infraction.reported",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("provider_id", u64s(prov.Id)),
		sdk.NewAttribute("kind", m.Kind.String()),
		sdk.NewAttribute("slash_bps", u32s(slashBps)),
		sdk.NewAttribute("slashed", u64s(slashed)),
		sdk.NewAttribute("new_count", u32s(newCount)),
		sdk.NewAttribute("auto_jailed", boolStr(autoJailed)),
		sdk.NewAttribute("auto_banned", boolStr(autoBanned)),
	)
	s.k.recordAudit(ctx, prov.Id, "infraction.report", m.Actor, prov.Did, m.Reason)
	return &types.MsgReportInfractionResponse{InfractionId: id, SlashedAmount: slashed}, nil
}

// ---- Discipline -------------------------------------------------------

func (s msgServer) JailProvider(ctx context.Context, m *types.MsgJailProvider) (*types.MsgJailProviderResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if types.ProviderIsTerminal(prov.Status) {
		return nil, fmt.Errorf("cannot jail terminal provider %d", prov.Id)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	prov.Status = types.ProviderStatus_PROVIDER_STATUS_JAILED
	prov.JailUntil = now + m.DurationSeconds
	prov.UpdatedAt = now
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	if err := s.recordJail(ctx, prov.Id, prov.JailUntil, m.Reason, m.Authority, false); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.provider.jailed",
		sdk.NewAttribute("id", u64s(prov.Id)),
		sdk.NewAttribute("until", i64s(prov.JailUntil)),
	)
	s.k.recordAudit(ctx, prov.Id, "provider.jail", m.Authority, prov.Did, m.Reason)
	return &types.MsgJailProviderResponse{JailedUntil: prov.JailUntil}, nil
}

func (s msgServer) UnjailProvider(ctx context.Context, m *types.MsgUnjailProvider) (*types.MsgUnjailProviderResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if prov.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		return nil, fmt.Errorf("provider %d not JAILED (%s)", prov.Id, prov.Status)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	prov.Status = types.ProviderStatus_PROVIDER_STATUS_ACTIVE
	prov.JailUntil = 0
	prov.UpdatedAt = now
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.provider.unjailed",
		sdk.NewAttribute("id", u64s(prov.Id)),
	)
	s.k.recordAudit(ctx, prov.Id, "provider.unjail", m.Authority, prov.Did, m.Reason)
	return &types.MsgUnjailProviderResponse{}, nil
}

func (s msgServer) BanProvider(ctx context.Context, m *types.MsgBanProvider) (*types.MsgBanProviderResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	prov, err := s.k.MustGetProvider(ctx, m.ProviderId)
	if err != nil {
		return nil, err
	}
	if prov.Status == types.ProviderStatus_PROVIDER_STATUS_BANNED {
		return &types.MsgBanProviderResponse{}, nil // idempotent
	}
	if prov.Status == types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN {
		return nil, fmt.Errorf("cannot ban WITHDRAWN provider %d", prov.Id)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	prov.Status = types.ProviderStatus_PROVIDER_STATUS_BANNED
	prov.JailUntil = 0
	prov.UpdatedAt = now
	if err := s.k.SetProvider(ctx, prov); err != nil {
		return nil, err
	}
	if err := s.recordBan(ctx, prov.Id, m.Reason, m.Authority, false); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.provider.banned",
		sdk.NewAttribute("id", u64s(prov.Id)),
	)
	s.k.recordAudit(ctx, prov.Id, "provider.ban", m.Authority, prov.Did, m.Reason)
	return &types.MsgBanProviderResponse{}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dataslash.params.updated", sdk.NewAttribute("authority", m.Authority))
	s.k.recordAudit(ctx, 0, "params.update", m.Authority, "", "")
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---- internal helpers -------------------------------------------------

func (s msgServer) recordJail(ctx context.Context, providerID uint64, until int64, reason, actor string, auto bool) error {
	id, err := s.k.NextJailRecordID(ctx)
	if err != nil {
		return err
	}
	jr := types.JailRecord{
		Id: id, ProviderId: providerID, Reason: reason,
		JailedAt: sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
		JailedUntil: until, Actor: actor, Auto: auto,
	}
	if err := s.k.JailRecords.Set(ctx, id, jr); err != nil {
		return err
	}
	return s.k.JailRecordByProvider.Set(ctx, collections.Join(providerID, id))
}

func (s msgServer) recordBan(ctx context.Context, providerID uint64, reason, actor string, auto bool) error {
	id, err := s.k.NextBanRecordID(ctx)
	if err != nil {
		return err
	}
	br := types.BanRecord{
		Id: id, ProviderId: providerID, Reason: reason,
		BannedAt: sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
		Actor: actor, Auto: auto,
	}
	if err := s.k.BanRecords.Set(ctx, id, br); err != nil {
		return err
	}
	return s.k.BanRecordByProvider.Set(ctx, collections.Join(providerID, id))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func safeAddU32(a, b uint32) (uint32, error) {
	if a > ^uint32(0)-b {
		return 0, fmt.Errorf("uint32 overflow: %d + %d", a, b)
	}
	return a + b, nil
}
