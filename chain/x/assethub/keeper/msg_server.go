package keeper

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/assethub/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func (s msgServer) requireAuthority(addr string) error {
	if addr != s.k.authority {
		return types.ErrUnauthorized.Wrapf("expected authority %q", s.k.authority)
	}
	return nil
}

// ---- providers -------------------------------------------------------------

func (s msgServer) RegisterProvider(ctx context.Context, m *types.MsgRegisterProvider) (*types.MsgRegisterProviderResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.Bond < params.MinBond {
		return nil, types.ErrInsufficientBond.Wrapf("bond %d < min %d", m.Bond, params.MinBond)
	}
	if _, ok := s.k.GetProvider(ctx, m.Provider); ok {
		return nil, types.ErrAlreadyExists.Wrapf("provider %q", m.Provider)
	}
	n, err := s.k.CountProviders(ctx)
	if err != nil {
		return nil, err
	}
	if n >= uint64(params.MaxProviders) {
		return nil, types.ErrLimitExceeded.Wrap("max_providers")
	}
	if err := s.k.collectBond(ctx, m.Provider, m.Bond); err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	p := types.Provider{
		Address:     m.Provider,
		Role:        m.Role,
		DisplayName: m.DisplayName,
		Bond:        m.Bond,
		Status:      types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.k.Providers.Set(ctx, m.Provider, p); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "register_provider", m.Provider, m.Provider, m.Role.String())
	emitEvent(sdkCtx, types.EventTypeProvider, "register", m.Provider)
	return &types.MsgRegisterProviderResponse{}, nil
}

func (s msgServer) IncreaseBond(ctx context.Context, m *types.MsgIncreaseBond) (*types.MsgIncreaseBondResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	p, ok := s.k.GetProvider(ctx, m.Provider)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %q", m.Provider)
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_BANNED {
		return nil, types.ErrProviderState.Wrap("banned")
	}
	newBond, err := types.SafeAdd(p.Bond, m.Amount)
	if err != nil {
		return nil, err
	}
	if err := s.k.collectBond(ctx, m.Provider, m.Amount); err != nil {
		return nil, err
	}
	p.Bond = newBond
	p.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Providers.Set(ctx, m.Provider, p); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeProvider, "increase_bond", m.Provider)
	return &types.MsgIncreaseBondResponse{NewBond: newBond}, nil
}

func (s msgServer) WithdrawBond(ctx context.Context, m *types.MsgWithdrawBond) (*types.MsgWithdrawBondResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	p, ok := s.k.GetProvider(ctx, m.Provider)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %q", m.Provider)
	}
	newBond, err := types.SafeSub(p.Bond, m.Amount)
	if err != nil {
		return nil, types.ErrInsufficientBond.Wrap(err.Error())
	}
	if newBond < params.MinBond {
		return nil, types.ErrInsufficientBond.Wrapf("remaining %d < min %d", newBond, params.MinBond)
	}
	if err := s.k.refundBond(ctx, m.Provider, m.Amount); err != nil {
		return nil, err
	}
	p.Bond = newBond
	p.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Providers.Set(ctx, m.Provider, p); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeProvider, "withdraw_bond", m.Provider)
	return &types.MsgWithdrawBondResponse{NewBond: newBond}, nil
}

func (s msgServer) DeregisterProvider(ctx context.Context, m *types.MsgDeregisterProvider) (*types.MsgDeregisterProviderResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	p, ok := s.k.GetProvider(ctx, m.Provider)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %q", m.Provider)
	}
	active, err := s.k.activeDeviceCount(ctx, m.Provider)
	if err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, types.ErrHasDevices.Wrapf("%d active devices", active)
	}
	if err := s.k.refundBond(ctx, m.Provider, p.Bond); err != nil {
		return nil, err
	}
	if err := s.k.Providers.Remove(ctx, m.Provider); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "deregister_provider", m.Provider, m.Provider, "")
	emitEvent(sdkCtx, types.EventTypeProvider, "deregister", m.Provider)
	return &types.MsgDeregisterProviderResponse{}, nil
}

func (s msgServer) SlashProvider(ctx context.Context, m *types.MsgSlashProvider) (*types.MsgSlashProviderResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	p, ok := s.k.GetProvider(ctx, m.Provider)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %q", m.Provider)
	}
	slashed, jailed, err := s.k.slash(sdkCtx, &p, params)
	if err != nil {
		return nil, err
	}
	if err := s.k.Providers.Set(ctx, m.Provider, p); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "slash_provider", m.Authority, m.Provider, m.Reason)
	emitEvent(sdkCtx, types.EventTypeProvider, "slash", m.Provider,
		"slashed", strconv.FormatUint(slashed, 10), "jailed", strconv.FormatBool(jailed))
	return &types.MsgSlashProviderResponse{Slashed: slashed, Jailed: jailed}, nil
}

func (s msgServer) UnjailProvider(ctx context.Context, m *types.MsgUnjailProvider) (*types.MsgUnjailProviderResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	p, ok := s.k.GetProvider(ctx, m.Provider)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %q", m.Provider)
	}
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		return nil, types.ErrProviderState.Wrap("not jailed")
	}
	if p.Bond < params.MinBond {
		return nil, types.ErrInsufficientBond.Wrapf("bond %d < min %d; top up first", p.Bond, params.MinBond)
	}
	p.Status = types.ProviderStatus_PROVIDER_STATUS_ACTIVE
	p.JailedUntil = 0
	p.Infractions = 0
	p.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Providers.Set(ctx, m.Provider, p); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeProvider, "unjail", m.Provider)
	return &types.MsgUnjailProviderResponse{}, nil
}

// ---- devices ---------------------------------------------------------------

func (s msgServer) RegisterDevice(ctx context.Context, m *types.MsgRegisterDevice) (*types.MsgRegisterDeviceResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := s.k.requireActiveProvider(sdkCtx, m.Operator,
		types.ProviderRole_PROVIDER_ROLE_DEVICE, types.ProviderRole_PROVIDER_ROLE_METER); err != nil {
		return nil, err
	}
	if _, err := s.k.Devices.Get(ctx, m.Id); err == nil {
		return nil, types.ErrAlreadyExists.Wrapf("device %q", m.Id)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.k.count(s.k.Devices.Iterate(ctx, nil))
	if err != nil {
		return nil, err
	}
	if n >= uint64(params.MaxDevices) {
		return nil, types.ErrLimitExceeded.Wrap("max_devices")
	}
	now := sdkCtx.BlockTime().Unix()
	d := types.Device{
		Id:           m.Id,
		Operator:     m.Operator,
		DeviceType:   m.DeviceType,
		Pubkey:       m.Pubkey,
		Jurisdiction: m.Jurisdiction,
		Status:       types.DeviceStatus_DEVICE_STATUS_ACTIVE,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.k.Devices.Set(ctx, m.Id, d); err != nil {
		return nil, err
	}
	if err := s.k.DeviceByOperator.Set(ctx, collections.Join(m.Operator, m.Id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "register_device", m.Operator, m.Id, m.DeviceType)
	emitEvent(sdkCtx, types.EventTypeDevice, "register", m.Id)
	return &types.MsgRegisterDeviceResponse{}, nil
}

func (s msgServer) UpdateDevice(ctx context.Context, m *types.MsgUpdateDevice) (*types.MsgUpdateDeviceResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.k.Devices.Get(ctx, m.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("device %q", m.Id)
	}
	if d.Operator != m.Operator {
		return nil, types.ErrUnauthorized.Wrap("not device operator")
	}
	if d.Status == types.DeviceStatus_DEVICE_STATUS_REVOKED {
		return nil, types.ErrDeviceRevoked
	}
	if m.DeviceType != "" {
		d.DeviceType = m.DeviceType
	}
	if m.Jurisdiction != "" {
		d.Jurisdiction = m.Jurisdiction
	}
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Devices.Set(ctx, m.Id, d); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeDevice, "update", m.Id)
	return &types.MsgUpdateDeviceResponse{}, nil
}

func (s msgServer) AttestDevice(ctx context.Context, m *types.MsgAttestDevice) (*types.MsgAttestDeviceResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.k.Devices.Get(ctx, m.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("device %q", m.Id)
	}
	if d.Operator != m.Operator {
		return nil, types.ErrUnauthorized.Wrap("not device operator")
	}
	if d.Status == types.DeviceStatus_DEVICE_STATUS_REVOKED {
		return nil, types.ErrDeviceRevoked
	}
	d.AttestationHash = m.AttestationHash
	d.Firmware = m.Firmware
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Devices.Set(ctx, m.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "attest_device", m.Operator, m.Id, m.Firmware)
	emitEvent(sdkCtx, types.EventTypeDevice, "attest", m.Id)
	return &types.MsgAttestDeviceResponse{}, nil
}

func (s msgServer) RevokeDevice(ctx context.Context, m *types.MsgRevokeDevice) (*types.MsgRevokeDeviceResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.k.Devices.Get(ctx, m.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("device %q", m.Id)
	}
	if d.Operator != m.Actor && m.Actor != s.k.authority {
		return nil, types.ErrUnauthorized.Wrap("only operator or authority may revoke")
	}
	if d.Status == types.DeviceStatus_DEVICE_STATUS_REVOKED {
		return nil, types.ErrDeviceRevoked.Wrap("already revoked")
	}
	d.Status = types.DeviceStatus_DEVICE_STATUS_REVOKED
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Devices.Set(ctx, m.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "revoke_device", m.Actor, m.Id, m.Reason)
	emitEvent(sdkCtx, types.EventTypeDevice, "revoke", m.Id)
	return &types.MsgRevokeDeviceResponse{}, nil
}

// ---- readings --------------------------------------------------------------

func (s msgServer) SubmitReading(ctx context.Context, m *types.MsgSubmitReading) (*types.MsgSubmitReadingResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := s.k.requireActiveProvider(sdkCtx, m.Provider,
		types.ProviderRole_PROVIDER_ROLE_METER, types.ProviderRole_PROVIDER_ROLE_DEVICE); err != nil {
		return nil, err
	}
	d, err := s.k.Devices.Get(ctx, m.DeviceId)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("device %q", m.DeviceId)
	}
	if d.Status != types.DeviceStatus_DEVICE_STATUS_ACTIVE {
		return nil, types.ErrDeviceRevoked
	}
	if d.Operator != m.Provider {
		return nil, types.ErrUnauthorized.Wrap("provider does not operate this device")
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	// Optional production gate: only trust readings from attested devices.
	if params.RequireDeviceAttestation && d.AttestationHash == "" {
		return nil, types.ErrInvalidReading.Wrapf("device %q has no attestation", m.DeviceId)
	}
	verified := types.CrossVerified(m.IotValue, m.OperationalValue, params.ReadingToleranceBps)
	// Optional production gate: reject (rather than merely flag) a reading
	// whose IoT and operational values disagree beyond tolerance, so an
	// unverifiable reading never enters trusted state.
	if params.RejectUnverifiedReadings && !verified {
		return nil, types.ErrInvalidReading.Wrapf(
			"reading failed cross-verification (iot=%d op=%d tolerance=%dbps)",
			m.IotValue, m.OperationalValue, params.ReadingToleranceBps)
	}

	id, err := s.k.ReadingIDSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	id++ // 1-based ids
	r := types.MeteringReading{
		Id:               id,
		DeviceId:         m.DeviceId,
		PeriodStart:      m.PeriodStart,
		PeriodEnd:        m.PeriodEnd,
		Unit:             m.Unit,
		IotValue:         m.IotValue,
		OperationalValue: m.OperationalValue,
		Verified:         verified,
		SubmittedBy:      m.Provider,
		SubmittedAt:      sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.Readings.Set(ctx, id, r); err != nil {
		return nil, err
	}
	if err := s.k.ReadingByDevice.Set(ctx, collections.Join(m.DeviceId, id)); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeReading, "submit", m.DeviceId,
		types.AttrVerified, strconv.FormatBool(verified))
	return &types.MsgSubmitReadingResponse{ReadingId: id, Verified: verified}, nil
}

// ---- oracle ----------------------------------------------------------------

func (s msgServer) CreateTopic(ctx context.Context, m *types.MsgCreateTopic) (*types.MsgCreateTopicResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if _, err := s.k.Topics.Get(ctx, m.Id); err == nil {
		return nil, types.ErrAlreadyExists.Wrapf("topic %q", m.Id)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.MinSources > params.MaxSourcesPerTopic {
		return nil, types.ErrInvalidField.Wrapf("min_sources %d > max_sources_per_topic %d", m.MinSources, params.MaxSourcesPerTopic)
	}
	n, err := s.k.CountTopics(ctx)
	if err != nil {
		return nil, err
	}
	if n >= uint64(params.MaxTopics) {
		return nil, types.ErrLimitExceeded.Wrap("max_topics")
	}
	tpc := types.OracleTopic{
		Id:          m.Id,
		Description: m.Description,
		MinSources:  m.MinSources,
		UpdatedAt:   sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.Topics.Set(ctx, m.Id, tpc); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTopic, "create", m.Id)
	return &types.MsgCreateTopicResponse{}, nil
}

func (s msgServer) SubmitValue(ctx context.Context, m *types.MsgSubmitValue) (*types.MsgSubmitValueResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := s.k.requireActiveProvider(sdkCtx, m.Provider, types.ProviderRole_PROVIDER_ROLE_ORACLE); err != nil {
		return nil, err
	}
	tpc, err := s.k.Topics.Get(ctx, m.TopicId)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("topic %q", m.TopicId)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	key := collections.Join(m.TopicId, m.Provider)
	if _, err := s.k.Submissions.Get(ctx, key); err != nil {
		// new submitter: enforce the per-topic source cap
		if tpc.SourceCount >= params.MaxSourcesPerTopic {
			return nil, types.ErrLimitExceeded.Wrap("max_sources_per_topic")
		}
	}
	now := sdkCtx.BlockTime().Unix()
	if err := s.k.Submissions.Set(ctx, key, types.OracleSubmission{
		TopicId:     m.TopicId,
		Provider:    m.Provider,
		Value:       m.Value,
		SubmittedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := s.k.recomputeTopic(sdkCtx, &tpc); err != nil {
		return nil, err
	}
	if err := s.k.Topics.Set(ctx, m.TopicId, tpc); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTopic, "submit", m.TopicId)
	return &types.MsgSubmitValueResponse{AggregatedValue: tpc.Value, HasValue: tpc.HasValue}, nil
}

// ---- params ----------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
