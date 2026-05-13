package keeper

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/cfe247/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k: k} }

var _ types.MsgServer = (*msgServer)(nil)

// ---- helpers --------------------------------------------------------------

func (s msgServer) onlyAuthority(addr string) error {
	if addr != s.k.authority {
		return fmt.Errorf("expected authority %s, got %s", s.k.authority, addr)
	}
	return nil
}

func (s msgServer) requireSubjectAdmin(ctx context.Context, subjectID, admin string) (types.Subject, error) {
	sb, err := s.k.Subjects.Get(ctx, subjectID)
	if err != nil {
		return types.Subject{}, fmt.Errorf("subject %q not found", subjectID)
	}
	if sb.Admin != admin {
		return types.Subject{}, fmt.Errorf("only subject admin (%s) may perform this action", sb.Admin)
	}
	return sb, nil
}

func (s msgServer) sanctionsCheck(ctx context.Context, addr string) error {
	if s.k.SanctionsHook() == nil {
		return nil
	}
	if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), addr) {
		return fmt.Errorf("address %s is on the sanctions list", addr)
	}
	return nil
}

// ---- Data provider --------------------------------------------------------

func (s msgServer) RegisterDataProvider(ctx context.Context, m *types.MsgRegisterDataProvider) (*types.MsgRegisterDataProviderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if has, err := s.k.DataProviders.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("data provider %q already registered", m.Id)
	}
	count, err := s.k.CountDataProviders(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxDataProviders {
		return nil, fmt.Errorf("max_data_providers reached (%d)", params.MaxDataProviders)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	p := types.DataProvider{
		Id: m.Id, Did: m.Did, DisplayName: m.DisplayName,
		Status:    types.DataProviderStatus_DATA_PROVIDER_STATUS_ACTIVE,
		Attestor:  m.Attestor, Admin: m.Admin,
		CreatedBy: m.Authority, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.k.DataProviders.Set(ctx, m.Id, p); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "", "register_provider", m.Authority,
		fmt.Sprintf("provider=%s did=%s attestor=%s", m.Id, m.Did, m.Attestor))
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_provider_registered",
		[2]string{"provider_id", m.Id}, [2]string{"did", m.Did})
	return &types.MsgRegisterDataProviderResponse{}, nil
}

func (s msgServer) UpdateDataProvider(ctx context.Context, m *types.MsgUpdateDataProvider) (*types.MsgUpdateDataProviderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	p, err := s.k.DataProviders.Get(ctx, m.Id)
	if err != nil {
		return nil, fmt.Errorf("data provider %q not found", m.Id)
	}
	if p.Status == types.DataProviderStatus_DATA_PROVIDER_STATUS_REVOKED {
		return nil, fmt.Errorf("provider %q revoked", m.Id)
	}
	if m.DisplayName != "" {
		p.DisplayName = m.DisplayName
	}
	if m.Attestor != "" {
		p.Attestor = m.Attestor
	}
	if m.Admin != "" {
		p.Admin = m.Admin
	}
	p.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.DataProviders.Set(ctx, m.Id, p); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "", "update_provider", m.Authority, "provider="+m.Id)
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_provider_updated", [2]string{"provider_id", m.Id})
	return &types.MsgUpdateDataProviderResponse{}, nil
}

func (s msgServer) setProviderStatus(ctx context.Context, authority, id, reason string, st types.DataProviderStatus) error {
	if err := s.onlyAuthority(authority); err != nil {
		return err
	}
	p, err := s.k.DataProviders.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("provider %q not found", id)
	}
	if p.Status == types.DataProviderStatus_DATA_PROVIDER_STATUS_REVOKED && st != types.DataProviderStatus_DATA_PROVIDER_STATUS_REVOKED {
		return fmt.Errorf("provider %q is revoked", id)
	}
	p.Status = st
	p.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.DataProviders.Set(ctx, id, p); err != nil {
		return err
	}
	s.k.recordAudit(ctx, "", "provider_status", authority,
		fmt.Sprintf("provider=%s status=%s reason=%s", id, st, reason))
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_provider_status",
		[2]string{"provider_id", id}, [2]string{"status", st.String()}, [2]string{"reason", reason})
	return nil
}

func (s msgServer) SuspendDataProvider(ctx context.Context, m *types.MsgSuspendDataProvider) (*types.MsgSuspendDataProviderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setProviderStatus(ctx, m.Authority, m.Id, m.Reason,
		types.DataProviderStatus_DATA_PROVIDER_STATUS_SUSPENDED); err != nil {
		return nil, err
	}
	return &types.MsgSuspendDataProviderResponse{}, nil
}

func (s msgServer) RevokeDataProvider(ctx context.Context, m *types.MsgRevokeDataProvider) (*types.MsgRevokeDataProviderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setProviderStatus(ctx, m.Authority, m.Id, m.Reason,
		types.DataProviderStatus_DATA_PROVIDER_STATUS_REVOKED); err != nil {
		return nil, err
	}
	return &types.MsgRevokeDataProviderResponse{}, nil
}

// ---- Grid zone ------------------------------------------------------------

func (s msgServer) RegisterGridZone(ctx context.Context, m *types.MsgRegisterGridZone) (*types.MsgRegisterGridZoneResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if has, err := s.k.GridZones.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("grid zone %q already registered", m.Id)
	}
	count, err := s.k.CountGridZones(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxGridZones {
		return nil, fmt.Errorf("max_grid_zones reached (%d)", params.MaxGridZones)
	}
	z := types.GridZone{
		Id: m.Id, DisplayName: m.DisplayName, Country: m.Country,
		Description: m.Description,
		CreatedAt:   sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	if err := s.k.GridZones.Set(ctx, m.Id, z); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_zone_registered",
		[2]string{"zone_id", m.Id}, [2]string{"country", m.Country})
	return &types.MsgRegisterGridZoneResponse{}, nil
}

func (s msgServer) UpdateGridZone(ctx context.Context, m *types.MsgUpdateGridZone) (*types.MsgUpdateGridZoneResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	z, err := s.k.GridZones.Get(ctx, m.Id)
	if err != nil {
		return nil, fmt.Errorf("zone %q not found", m.Id)
	}
	if m.DisplayName != "" {
		z.DisplayName = m.DisplayName
	}
	if m.Country != "" {
		z.Country = m.Country
	}
	if m.Description != "" {
		z.Description = m.Description
	}
	if err := s.k.GridZones.Set(ctx, m.Id, z); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_zone_updated", [2]string{"zone_id", m.Id})
	return &types.MsgUpdateGridZoneResponse{}, nil
}

func (s msgServer) RemoveGridZone(ctx context.Context, m *types.MsgRemoveGridZone) (*types.MsgRemoveGridZoneResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.GridZones.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, fmt.Errorf("zone %q not found", m.Id)
	}
	// Refuse removal if any subject defaults to it; refuse-or-rename
	// is left to governance.
	usingSubject := ""
	if err := s.k.Subjects.Walk(ctx, nil, func(_ string, v types.Subject) (bool, error) {
		if v.DefaultGridZone == m.Id {
			usingSubject = v.Id
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if usingSubject != "" {
		return nil, fmt.Errorf("zone %q is the default for subject %q; clear it first", m.Id, usingSubject)
	}
	if err := s.k.GridZones.Remove(ctx, m.Id); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_zone_removed", [2]string{"zone_id", m.Id})
	return &types.MsgRemoveGridZoneResponse{}, nil
}

// ---- Subject --------------------------------------------------------------

func (s msgServer) RegisterSubject(ctx context.Context, m *types.MsgRegisterSubject) (*types.MsgRegisterSubjectResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if has, err := s.k.Subjects.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("subject %q already registered", m.Id)
	}
	count, err := s.k.CountSubjects(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxSubjects {
		return nil, fmt.Errorf("max_subjects reached (%d)", params.MaxSubjects)
	}
	if m.DefaultGridZone != "" {
		if has, err := s.k.GridZones.Has(ctx, m.DefaultGridZone); err != nil {
			return nil, err
		} else if !has {
			return nil, fmt.Errorf("default_grid_zone %q not registered", m.DefaultGridZone)
		}
	}
	if err := s.sanctionsCheck(ctx, m.Admin); err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	sb := types.Subject{
		Id: m.Id, DisplayName: m.DisplayName,
		Status: types.SubjectStatus_SUBJECT_STATUS_ACTIVE,
		Admin:  m.Admin, DefaultGridZone: m.DefaultGridZone,
		CreatedBy: m.Creator, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.k.Subjects.Set(ctx, m.Id, sb); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.Id, "register_subject", m.Creator,
		fmt.Sprintf("admin=%s zone=%s", m.Admin, m.DefaultGridZone))
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_subject_registered",
		[2]string{"subject_id", m.Id}, [2]string{"admin", m.Admin})
	return &types.MsgRegisterSubjectResponse{}, nil
}

func (s msgServer) UpdateSubject(ctx context.Context, m *types.MsgUpdateSubject) (*types.MsgUpdateSubjectResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	sb, err := s.requireSubjectAdmin(ctx, m.Id, m.Admin)
	if err != nil {
		return nil, err
	}
	if m.DisplayName != "" {
		sb.DisplayName = m.DisplayName
	}
	if m.DefaultGridZone != "" {
		if has, err := s.k.GridZones.Has(ctx, m.DefaultGridZone); err != nil {
			return nil, err
		} else if !has {
			return nil, fmt.Errorf("default_grid_zone %q not registered", m.DefaultGridZone)
		}
		sb.DefaultGridZone = m.DefaultGridZone
	}
	if m.NewAdmin != "" {
		if err := s.sanctionsCheck(ctx, m.NewAdmin); err != nil {
			return nil, err
		}
		sb.Admin = m.NewAdmin
	}
	sb.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Subjects.Set(ctx, m.Id, sb); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.Id, "update_subject", m.Admin, "")
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_subject_updated", [2]string{"subject_id", m.Id})
	return &types.MsgUpdateSubjectResponse{}, nil
}

func (s msgServer) DeactivateSubject(ctx context.Context, m *types.MsgDeactivateSubject) (*types.MsgDeactivateSubjectResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	sb, err := s.requireSubjectAdmin(ctx, m.Id, m.Admin)
	if err != nil {
		return nil, err
	}
	sb.Status = types.SubjectStatus_SUBJECT_STATUS_INACTIVE
	sb.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Subjects.Set(ctx, m.Id, sb); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.Id, "deactivate_subject", m.Admin, m.Reason)
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_subject_deactivated",
		[2]string{"subject_id", m.Id}, [2]string{"reason", m.Reason})
	return &types.MsgDeactivateSubjectResponse{}, nil
}

// ---- Consumption ----------------------------------------------------------

func (s msgServer) attestOne(ctx context.Context, p types.DataProvider, attestor string, e types.ConsumptionEntry) error {
	sb, err := s.k.Subjects.Get(ctx, e.SubjectId)
	if err != nil {
		return fmt.Errorf("subject %q not found", e.SubjectId)
	}
	if sb.Status != types.SubjectStatus_SUBJECT_STATUS_ACTIVE {
		return fmt.Errorf("subject %q is not ACTIVE", e.SubjectId)
	}
	if has, err := s.k.GridZones.Has(ctx, e.GridZone); err != nil {
		return err
	} else if !has {
		return fmt.Errorf("grid zone %q not registered", e.GridZone)
	}
	c := types.HourlyConsumption{
		SubjectId: e.SubjectId, GridZone: e.GridZone,
		HourStart: e.HourStart, HourEnd: e.HourStart + types.HourSeconds,
		WhConsumed: e.WhConsumed,
		DataProviderId: p.Id, Attestor: attestor,
		MeterBatchId: e.MeterBatchId,
		AttestedAt:   sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	return s.k.SetConsumption(ctx, c)
}

func (s msgServer) requireActiveAttestor(ctx context.Context, providerID, attestor string) (types.DataProvider, error) {
	p, err := s.k.DataProviders.Get(ctx, providerID)
	if err != nil {
		return types.DataProvider{}, fmt.Errorf("data provider %q not found", providerID)
	}
	if p.Attestor != attestor {
		return types.DataProvider{}, fmt.Errorf("attestor %s is not registered for provider %s", attestor, providerID)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return types.DataProvider{}, err
	}
	if params.RequireActiveProvider && p.Status != types.DataProviderStatus_DATA_PROVIDER_STATUS_ACTIVE {
		return types.DataProvider{}, fmt.Errorf("provider %s is not ACTIVE (status=%s)", p.Id, p.Status)
	}
	return p, nil
}

func (s msgServer) AttestHourlyConsumption(ctx context.Context, m *types.MsgAttestHourlyConsumption) (*types.MsgAttestHourlyConsumptionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.requireActiveAttestor(ctx, m.DataProviderId, m.Attestor)
	if err != nil {
		return nil, err
	}
	if err := s.attestOne(ctx, p, m.Attestor, types.ConsumptionEntry{
		SubjectId: m.SubjectId, GridZone: m.GridZone,
		HourStart: m.HourStart, WhConsumed: m.WhConsumed,
		MeterBatchId: m.MeterBatchId,
	}); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_consumption_attested",
		[2]string{"subject_id", m.SubjectId},
		[2]string{"hour_start", strconv.FormatInt(m.HourStart, 10)},
		[2]string{"wh", strconv.FormatUint(m.WhConsumed, 10)},
		[2]string{"provider_id", m.DataProviderId},
	)
	return &types.MsgAttestHourlyConsumptionResponse{}, nil
}

func (s msgServer) AttestHourlyConsumptionBatch(ctx context.Context, m *types.MsgAttestHourlyConsumptionBatch) (*types.MsgAttestHourlyConsumptionBatchResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint32(len(m.Entries)) > params.MaxAttestationsPerBatch {
		return nil, fmt.Errorf("entries %d exceeds max_attestations_per_batch %d",
			len(m.Entries), params.MaxAttestationsPerBatch)
	}
	p, err := s.requireActiveAttestor(ctx, m.DataProviderId, m.Attestor)
	if err != nil {
		return nil, err
	}
	for i, e := range m.Entries {
		if err := s.attestOne(ctx, p, m.Attestor, e); err != nil {
			return nil, fmt.Errorf("entry[%d]: %w", i, err)
		}
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_consumption_batch",
		[2]string{"provider_id", m.DataProviderId},
		[2]string{"count", strconv.Itoa(len(m.Entries))},
	)
	return &types.MsgAttestHourlyConsumptionBatchResponse{Attested: uint32(len(m.Entries))}, nil
}

// ---- Match ----------------------------------------------------------------

func (s msgServer) AllocateMatch(ctx context.Context, m *types.MsgAllocateMatch) (*types.MsgAllocateMatchResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	sb, err := s.k.Subjects.Get(ctx, m.SubjectId)
	if err != nil {
		return nil, fmt.Errorf("subject %q not found", m.SubjectId)
	}
	if sb.Status != types.SubjectStatus_SUBJECT_STATUS_ACTIVE {
		return nil, fmt.Errorf("subject %q is not ACTIVE", m.SubjectId)
	}
	if err := s.sanctionsCheck(ctx, m.Allocator); err != nil {
		return nil, err
	}

	// Consumption row must already exist; matches against thin air
	// would silently inflate scores and are explicitly disallowed.
	cons, hadCons, err := s.k.GetConsumption(ctx, m.SubjectId, m.HourStart)
	if err != nil {
		return nil, err
	}
	if !hadCons {
		return nil, fmt.Errorf("no consumption row for subject %q hour %d", m.SubjectId, m.HourStart)
	}

	view, ok := s.k.EACHook().LookupRetirement(sdk.UnwrapSDKContext(ctx), m.EacRetirementId)
	if !ok {
		return nil, fmt.Errorf("EAC retirement %d not found", m.EacRetirementId)
	}
	if view.Retirer != m.Allocator {
		return nil, fmt.Errorf("only the original retirer (%s) may allocate this retirement", view.Retirer)
	}
	// Hour interval containment (strict).
	hourEnd := m.HourStart + types.HourSeconds
	if !(m.HourStart >= view.CertHourStart && hourEnd <= view.CertHourEnd) {
		return nil, fmt.Errorf("certificate %d hour interval [%d,%d) does not contain consumption hour [%d,%d)",
			view.CertificateID, view.CertHourStart, view.CertHourEnd, m.HourStart, hourEnd)
	}

	// Reserve units against the retirement's running tally.
	if err := s.k.reserveRetirement(ctx, m.EacRetirementId, view.Amount, m.WhMatched); err != nil {
		return nil, err
	}

	sameZone := view.GridZone == cons.GridZone

	agg, hadAgg, err := s.k.getAggregate(ctx, m.SubjectId, m.HourStart)
	if err != nil {
		return nil, err
	}
	if !hadAgg {
		// shouldn't happen because consumption insert created it.
		agg = types.HourlyAggregate{SubjectId: m.SubjectId, HourStart: m.HourStart, GridZone: cons.GridZone, WhConsumed: cons.WhConsumed}
	}
	prevMatched := agg.WhMatchedTotal

	newMatched, err := types.SafeAdd(agg.WhMatchedTotal, m.WhMatched)
	if err != nil {
		return nil, err
	}
	agg.WhMatchedTotal = newMatched
	if sameZone {
		v, err := types.SafeAdd(agg.WhMatchedSameZone, m.WhMatched)
		if err != nil {
			return nil, err
		}
		agg.WhMatchedSameZone = v
	}
	if view.IsStorage {
		v, err := types.SafeAdd(agg.WhMatchedStorage, m.WhMatched)
		if err != nil {
			return nil, err
		}
		agg.WhMatchedStorage = v
	}
	agg.LastUpdated = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Aggregates.Set(ctx, collections.Join(m.SubjectId, m.HourStart), agg); err != nil {
		return nil, err
	}

	if _, err := s.k.adjustAnnualForMatchAdded(ctx, m.SubjectId, m.HourStart,
		prevMatched, cons.WhConsumed, m.WhMatched, sameZone, view.IsStorage); err != nil {
		return nil, err
	}

	id, err := s.k.NextMatchID(ctx)
	if err != nil {
		return nil, err
	}
	entry := types.MatchEntry{
		Id: id, SubjectId: m.SubjectId, HourStart: m.HourStart,
		EacRetirementId: m.EacRetirementId, EacCertificateId: view.CertificateID,
		WhMatched: m.WhMatched, GridZone: view.GridZone, Technology: view.Technology,
		SameZone: sameZone, Allocator: m.Allocator,
		CreatedAt: sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	if err := s.k.Matches.Set(ctx, id, entry); err != nil {
		return nil, err
	}
	if err := s.k.MatchByHour.Set(ctx, collections.Join3(m.SubjectId, m.HourStart, id)); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, m.SubjectId, "allocate_match", m.Allocator,
		fmt.Sprintf("hour=%d retirement=%d cert=%d wh=%d same_zone=%t storage=%t",
			m.HourStart, m.EacRetirementId, view.CertificateID, m.WhMatched, sameZone, view.IsStorage))
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_match_allocated",
		[2]string{"match_id", strconv.FormatUint(id, 10)},
		[2]string{"subject_id", m.SubjectId},
		[2]string{"hour_start", strconv.FormatInt(m.HourStart, 10)},
		[2]string{"eac_retirement_id", strconv.FormatUint(m.EacRetirementId, 10)},
		[2]string{"wh", strconv.FormatUint(m.WhMatched, 10)},
		[2]string{"same_zone", strconv.FormatBool(sameZone)},
	)
	return &types.MsgAllocateMatchResponse{MatchId: id}, nil
}

// ---- Report ---------------------------------------------------------------

func (s msgServer) GenerateReport(ctx context.Context, m *types.MsgGenerateReport) (*types.MsgGenerateReportResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	sb, err := s.requireSubjectAdmin(ctx, m.SubjectId, m.Admin)
	if err != nil {
		return nil, err
	}
	if sb.Status != types.SubjectStatus_SUBJECT_STATUS_ACTIVE {
		return nil, fmt.Errorf("subject %q not ACTIVE", m.SubjectId)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	periodSecs := m.PeriodEnd - m.PeriodStart
	maxSecs := int64(params.MaxReportPeriodDays) * 86400
	if periodSecs > maxSecs {
		return nil, fmt.Errorf("period %ds exceeds max_report_period_days*86400 = %ds", periodSecs, maxSecs)
	}

	// Walk aggregates for the period and build the snapshot. This is
	// proportional to the number of distinct hours with data inside
	// the window, which is bounded above by max_report_period_days*24.
	var (
		totalConsumed, totalMatched, totalSameZone uint64
		hoursWithData, hoursFully, sumMin          uint64
	)
	rng := collections.NewPrefixedPairRange[string, int64](m.SubjectId).
		StartInclusive(m.PeriodStart).
		EndExclusive(m.PeriodEnd)
	if err := s.k.Aggregates.Walk(ctx, rng, func(_ collections.Pair[string, int64], v types.HourlyAggregate) (bool, error) {
		totalConsumed += v.WhConsumed
		totalMatched += v.WhMatchedTotal
		totalSameZone += v.WhMatchedSameZone
		hoursWithData++
		hourMin := types.Min64(v.WhMatchedTotal, v.WhConsumed)
		sumMin += hourMin
		if v.WhConsumed > 0 && v.WhMatchedTotal >= v.WhConsumed {
			hoursFully++
		}
		return false, nil
	}); err != nil {
		return nil, err
	}

	hourlyBps := types.BasisPoints(sumMin, totalConsumed)
	annualBps := types.BasisPoints(totalMatched, totalConsumed)
	locBps := types.BasisPoints(totalSameZone, totalConsumed)

	id, err := s.k.NextReportID(ctx)
	if err != nil {
		return nil, err
	}
	rep := types.ReportPackage{
		Id: id, SubjectId: m.SubjectId, Format: m.Format,
		PeriodStart: m.PeriodStart, PeriodEnd: m.PeriodEnd,
		HourlyScoreBps: hourlyBps, AnnualScoreBps: annualBps,
		LocationMatchedScoreBps: locBps,
		TotalWhConsumed:         totalConsumed,
		TotalWhMatched:          totalMatched,
		HoursWithData:           hoursWithData,
		HoursFullyMatched:       hoursFully,
		ReportUri:               m.ReportUri,
		ReportHash:              m.ReportHash,
		GeneratedBy:             m.Admin,
		GeneratedAt:             sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	if err := s.k.Reports.Set(ctx, id, rep); err != nil {
		return nil, err
	}
	if err := s.k.ReportBySubject.Set(ctx, collections.Join(m.SubjectId, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.SubjectId, "generate_report", m.Admin,
		fmt.Sprintf("format=%s period=[%d,%d) hourly=%d annual=%d loc=%d",
			m.Format, m.PeriodStart, m.PeriodEnd, hourlyBps, annualBps, locBps))
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_report_generated",
		[2]string{"report_id", strconv.FormatUint(id, 10)},
		[2]string{"subject_id", m.SubjectId},
		[2]string{"format", m.Format.String()},
		[2]string{"hourly_bps", strconv.FormatUint(uint64(hourlyBps), 10)},
		[2]string{"annual_bps", strconv.FormatUint(uint64(annualBps), 10)},
		[2]string{"location_bps", strconv.FormatUint(uint64(locBps), 10)},
	)
	return &types.MsgGenerateReportResponse{ReportId: id}, nil
}

// ---- Params ---------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "cfe247_params_updated")
	return &types.MsgUpdateParamsResponse{}, nil
}
