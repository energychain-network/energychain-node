package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/device/types"
)

type msgServer struct{ Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

// emitDeviceEvent collapses the boilerplate of constructing structured
// events. Centralising it keeps the audit module's subscriber regex
// simple — every device-bearing event has the same attribute keys.
func emitDeviceEvent(ctx sdk.Context, eventType, deviceDID, actor, info string) {
	attrs := []sdk.Attribute{
		sdk.NewAttribute("device_did", deviceDID),
		sdk.NewAttribute("actor", actor),
	}
	if info != "" {
		attrs = append(attrs, sdk.NewAttribute("info", info))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(eventType, attrs...))
}

func (m msgServer) RegisterDevice(goCtx context.Context, msg *types.MsgRegisterDevice) (*types.MsgRegisterDeviceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.CheckOwnerHasDID(ctx, msg.Owner) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("owner %s has no active DID", msg.Owner)
	}
	if _, exists := m.GetDevice(ctx, msg.Device.DeviceDid); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("device %s already registered", msg.Device.DeviceDid)
	}
	params := m.GetParams(ctx)
	if c := m.CountByOwner(ctx, msg.Owner, params.MaxDevicesPerOwner); c >= params.MaxDevicesPerOwner {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("owner reached cap %d devices", params.MaxDevicesPerOwner)
	}

	d := msg.Device
	d.OwnerDid = msg.Owner
	now := ctx.BlockTime().Unix()
	d.RegisteredAt = now
	d.UpdatedAt = now
	d.Status = types.AttestationStatus_ATTESTATION_STATUS_UNVERIFIED
	d.AttestedAt = 0
	d.AttestationExpiresAt = 0
	if err := m.SetDevice(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}

	emitDeviceEvent(ctx, "device_registered", d.DeviceDid, msg.Owner, d.Class.String())
	return &types.MsgRegisterDeviceResponse{}, nil
}

func (m msgServer) UpdateDevice(goCtx context.Context, msg *types.MsgUpdateDevice) (*types.MsgUpdateDeviceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, ok := m.GetDevice(ctx, msg.DeviceDid)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("device %s", msg.DeviceDid)
	}
	if d.OwnerDid != msg.Owner {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("not owner of %s", msg.DeviceDid)
	}
	if d.Status == types.AttestationStatus_ATTESTATION_STATUS_REVOKED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("revoked device cannot be updated")
	}

	prev := d
	if msg.FirmwareHash != "" {
		d.FirmwareHash = msg.FirmwareHash
	}
	if msg.GridZone != "" {
		d.GridZone = msg.GridZone
	}
	// 0,0 is technically valid coordinates (Atlantic) but rare enough
	// for energy infra that we treat it as "unset". Override only when
	// caller passes non-zero on either axis.
	if msg.LatE7 != 0 || msg.LonE7 != 0 {
		d.LatE7 = msg.LatE7
		d.LonE7 = msg.LonE7
	}

	if msg.FirmwareChanged {
		// Per docs §4.2: any firmware mutation forces a fresh attestation.
		d.Status = types.AttestationStatus_ATTESTATION_STATUS_UNVERIFIED
		d.AttestedAt = 0
		d.AttestationExpiresAt = 0
	}
	d.UpdatedAt = ctx.BlockTime().Unix()

	if err := m.ClearDeviceIndexes(ctx, prev); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("clear indexes: %s", err)
	}
	if err := m.SetDevice(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDeviceEvent(ctx, "device_updated", d.DeviceDid, msg.Owner, "")
	return &types.MsgUpdateDeviceResponse{}, nil
}

func (m msgServer) TransferDevice(goCtx context.Context, msg *types.MsgTransferDevice) (*types.MsgTransferDeviceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, ok := m.GetDevice(ctx, msg.DeviceDid)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("device %s", msg.DeviceDid)
	}
	if d.OwnerDid != msg.CurrentOwner {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only current owner may transfer")
	}
	if !m.CheckOwnerHasDID(ctx, msg.NewOwner) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("new owner %s has no active DID", msg.NewOwner)
	}
	params := m.GetParams(ctx)
	if c := m.CountByOwner(ctx, msg.NewOwner, params.MaxDevicesPerOwner); c >= params.MaxDevicesPerOwner {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("new owner already at device cap")
	}

	prev := d
	d.OwnerDid = msg.NewOwner
	d.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.ClearDeviceIndexes(ctx, prev); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("clear indexes: %s", err)
	}
	if err := m.SetDevice(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDeviceEvent(ctx, "device_transferred", d.DeviceDid, msg.CurrentOwner, msg.NewOwner)
	return &types.MsgTransferDeviceResponse{}, nil
}

func (m msgServer) RevokeDevice(goCtx context.Context, msg *types.MsgRevokeDevice) (*types.MsgRevokeDeviceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, ok := m.GetDevice(ctx, msg.DeviceDid)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("device %s", msg.DeviceDid)
	}
	if d.OwnerDid != msg.Actor && msg.Actor != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner or governance may revoke")
	}
	if d.Status == types.AttestationStatus_ATTESTATION_STATUS_REVOKED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("already revoked")
	}

	prev := d
	d.Status = types.AttestationStatus_ATTESTATION_STATUS_REVOKED
	d.UpdatedAt = ctx.BlockTime().Unix()
	d.AttestedAt = 0
	d.AttestationExpiresAt = 0
	if err := m.ClearDeviceIndexes(ctx, prev); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("clear indexes: %s", err)
	}
	if err := m.SetDevice(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDeviceEvent(ctx, "device_revoked", d.DeviceDid, msg.Actor, msg.Reason)
	return &types.MsgRevokeDeviceResponse{}, nil
}

// SubmitAttestation lodges evidence and stamps it as pending. Status
// updates wait for ConfirmAttestation.
func (m msgServer) SubmitAttestation(goCtx context.Context, msg *types.MsgSubmitAttestation) (*types.MsgSubmitAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, ok := m.GetDevice(ctx, msg.Evidence.DeviceDid)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("device %s", msg.Evidence.DeviceDid)
	}
	if d.OwnerDid != msg.Owner {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner may submit attestation")
	}
	if d.Status == types.AttestationStatus_ATTESTATION_STATUS_REVOKED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("revoked device cannot attest")
	}

	id := m.NextAttestationID(ctx)
	ev := msg.Evidence
	ev.SubmittedAt = ctx.BlockTime().Unix()
	ev.Verdict = types.VerdictPending
	if err := m.StoreAttestation(ctx, id, ev); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDeviceEvent(ctx, "device_attestation_submitted", d.DeviceDid, msg.Owner, ev.Format)
	return &types.MsgSubmitAttestationResponse{AttestationId: id}, nil
}

func (m msgServer) ConfirmAttestation(goCtx context.Context, msg *types.MsgConfirmAttestation) (*types.MsgConfirmAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.IsAttestationVerifier(ctx, msg.Verifier) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("%s not in attestation_verifiers list", msg.Verifier)
	}

	verdict := types.VerdictRejected
	if msg.Accept {
		verdict = types.VerdictAccepted
	}
	ev, err := m.ResolveAttestation(ctx, msg.AttestationId, verdict, msg.Reason)
	if err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}

	if msg.Accept {
		// Flip the device to ATTESTED + reset expiry.
		d, ok := m.GetDevice(ctx, ev.DeviceDid)
		if !ok {
			return nil, sdkerrors.ErrNotFound.Wrap("device vanished mid-confirm")
		}
		// SECURITY: a REVOKED device must NEVER be resurrected by a
		// late-arriving verifier confirmation. The attestation row itself
		// is still recorded as accepted (audit trail / verifier
		// accountability), but we leave the device terminal status
		// untouched. Same guard for SUSPECT-via-flag is intentionally
		// NOT applied: a SUSPECT device can be rehabilitated by a fresh
		// successful attestation, that's the whole point of the flow.
		if d.Status == types.AttestationStatus_ATTESTATION_STATUS_REVOKED {
			emitDeviceEvent(ctx, "device_attestation_confirmed_skipped_revoked",
				d.DeviceDid, msg.Verifier, "")
		} else {
			prev := d
			now := ctx.BlockTime().Unix()
			d.Status = types.AttestationStatus_ATTESTATION_STATUS_ATTESTED
			d.AttestedAt = now
			d.AttestationExpiresAt = now + m.GetParams(ctx).AttestationValidity
			d.UpdatedAt = now
			if err := m.ClearDeviceIndexes(ctx, prev); err != nil {
				return nil, sdkerrors.ErrLogic.Wrapf("clear indexes: %s", err)
			}
			if err := m.SetDevice(ctx, d); err != nil {
				return nil, sdkerrors.ErrLogic.Wrapf("store device: %s", err)
			}
		}
	}

	emitDeviceEvent(ctx, "device_attestation_confirmed", ev.DeviceDid, msg.Verifier,
		map[bool]string{true: "accept", false: "reject"}[msg.Accept])
	return &types.MsgConfirmAttestationResponse{}, nil
}

func (m msgServer) FlagFirmwareMismatch(goCtx context.Context, msg *types.MsgFlagFirmwareMismatch) (*types.MsgFlagFirmwareMismatchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, ok := m.GetDevice(ctx, msg.DeviceDid)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("device %s", msg.DeviceDid)
	}
	// Either the owner or governance can flag.
	if d.OwnerDid != msg.Actor && msg.Actor != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only owner or governance may flag")
	}
	prev := d
	d.Status = types.AttestationStatus_ATTESTATION_STATUS_SUSPECT
	d.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.ClearDeviceIndexes(ctx, prev); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("clear indexes: %s", err)
	}
	if err := m.SetDevice(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("store: %s", err)
	}
	emitDeviceEvent(ctx, "device_flagged_suspect", d.DeviceDid, msg.Actor, msg.Reason)
	return &types.MsgFlagFirmwareMismatchResponse{}, nil
}

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("expected %s", m.GetAuthority())
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrap(err.Error())
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
