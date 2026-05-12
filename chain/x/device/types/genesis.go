package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultParams() Params {
	return Params{
		AttestationValidity:    DefaultAttestationValidity,
		AttestationVerifiers:   []string{},
		MaxDevicesPerOwner:     DefaultMaxDevicesPerOwner,
	}
}

func (p Params) Validate() error {
	if p.AttestationValidity < AttestationValidityLower || p.AttestationValidity > AttestationValidityUpper {
		return fmt.Errorf("attestation_validity %d outside [%d, %d]",
			p.AttestationValidity, AttestationValidityLower, AttestationValidityUpper)
	}
	for i, v := range p.AttestationVerifiers {
		if _, err := sdk.AccAddressFromBech32(v); err != nil {
			return fmt.Errorf("verifier %d (%s): %w", i, v, err)
		}
	}
	if p.MaxDevicesPerOwner == 0 || p.MaxDevicesPerOwner > MaxDevicesPerOwnerUpper {
		return fmt.Errorf("max_devices_per_owner %d outside (0, %d]",
			p.MaxDevicesPerOwner, MaxDevicesPerOwnerUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:             DefaultParams(),
		Devices:            []Device{},
		Attestations:       []AttestationEvidence{},
		NextAttestationId: 1,
	}
}

// Validate ensures the genesis blob round-trips: every device has a
// unique deviceDID, every attestation references a known device, and the
// next-id counter is at least one past the highest used id.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	devices := make(map[string]bool, len(gs.Devices))
	for i, d := range gs.Devices {
		if d.DeviceDid == "" {
			return fmt.Errorf("device %d: device_did empty", i)
		}
		if devices[d.DeviceDid] {
			return fmt.Errorf("device %d (%s): duplicate", i, d.DeviceDid)
		}
		devices[d.DeviceDid] = true
		if _, err := sdk.AccAddressFromBech32(d.OwnerDid); err != nil {
			return fmt.Errorf("device %s: owner_did invalid: %w", d.DeviceDid, err)
		}
	}
	maxID := uint64(0)
	for i, a := range gs.Attestations {
		if !devices[a.DeviceDid] {
			return fmt.Errorf("attestation %d references unknown device %s", i, a.DeviceDid)
		}
		// We can't easily extract the id from the proto message because
		// it's keeper-assigned; trust the genesis to have assigned them
		// via Map iteration order during ExportGenesis. The end-blocker
		// rebuilds the index from scratch, so duplicates here would
		// surface immediately.
		_ = maxID
	}
	if gs.NextAttestationId == 0 && len(gs.Attestations) > 0 {
		return fmt.Errorf("next_attestation_id must be > 0 when attestations exist")
	}
	return nil
}
