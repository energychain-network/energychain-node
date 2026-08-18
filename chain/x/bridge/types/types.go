package types

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	DenomMaxLen    = 64
	NameMaxLen     = 128
	ChainRefMaxLen = 128
	AddrMaxLen     = 256

	DefaultMaxChains            uint32 = 256
	DefaultMaxAssets            uint32 = 256
	DefaultMaxAttestorsPerChain uint32 = 64

	HardMaxChains   uint32 = 65536
	HardMaxAssets   uint32 = 65536
	HardMaxAttestor uint32 = 1024
)

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateDenom(d string) error {
	if l := len(d); l < 1 || l > DenomMaxLen {
		return errorsmod.Wrapf(ErrInvalidField, "denom length %d out of range", l)
	}
	return nil
}

func ValidateMaxLen(field, s string, max int) error {
	if len(s) == 0 {
		return errorsmod.Wrapf(ErrInvalidField, "%s must not be empty", field)
	}
	if len(s) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s exceeds %d bytes", field, max)
	}
	return nil
}

func SafeAdd(x, y uint64) (uint64, error) {
	if y > 0 && x > ^uint64(0)-y {
		return 0, errorsmod.Wrapf(ErrOverflow, "%d + %d", x, y)
	}
	return x + y, nil
}

func SafeSub(x, y uint64) (uint64, error) {
	if y > x {
		return 0, errorsmod.Wrapf(ErrOverflow, "underflow %d - %d", x, y)
	}
	return x - y, nil
}

// ValidateAttestorSet checks every attestor is a distinct valid bech32 address,
// the set size is within `max`, and 1 <= threshold <= len(attestors).
func ValidateAttestorSet(attestors []string, threshold, max uint32) error {
	if len(attestors) == 0 {
		return errorsmod.Wrap(ErrInvalidField, "attestor set must not be empty")
	}
	if uint32(len(attestors)) > max {
		return errorsmod.Wrapf(ErrLimitExceeded, "attestors %d > max %d", len(attestors), max)
	}
	seen := make(map[string]bool, len(attestors))
	for _, a := range attestors {
		if err := MustBech32(a); err != nil {
			return err
		}
		if seen[a] {
			return errorsmod.Wrapf(ErrDuplicate, "attestor %s", a)
		}
		seen[a] = true
	}
	if threshold == 0 || threshold > uint32(len(attestors)) {
		return errorsmod.Wrapf(ErrInvalidField, "threshold %d must be in [1, %d]", threshold, len(attestors))
	}
	return nil
}

func Contains(set []string, x string) bool {
	for _, s := range set {
		if s == x {
			return true
		}
	}
	return false
}

func ChainStatusValid(s ChainStatus) bool {
	return s == ChainStatus_CHAIN_STATUS_ACTIVE || s == ChainStatus_CHAIN_STATUS_PAUSED
}

func AssetStatusValid(s AssetStatus) bool {
	return s == AssetStatus_ASSET_STATUS_ACTIVE || s == AssetStatus_ASSET_STATUS_PAUSED
}

func InboundStatusValid(s InboundStatus) bool {
	switch s {
	case InboundStatus_INBOUND_STATUS_PENDING, InboundStatus_INBOUND_STATUS_ATTESTED, InboundStatus_INBOUND_STATUS_RELEASED:
		return true
	}
	return false
}

func DefaultParams() Params {
	return Params{
		MaxChains:            DefaultMaxChains,
		MaxAssets:            DefaultMaxAssets,
		MaxAttestorsPerChain: DefaultMaxAttestorsPerChain,
		MaxLockAmount:        0,
		Paused:               false,
		// Rate limits default to off (the per-denom reserve gate is the
		// always-on total ceiling); operators tighten these via governance
		// once expected per-chain flow is known.
		MaxMintPerTx:      0,
		MintWindowSeconds: 0,
		MaxMintPerWindow:  0,
	}
}

func (p Params) Validate() error {
	if p.MaxChains == 0 || p.MaxChains > HardMaxChains {
		return fmt.Errorf("max_chains must be in (0, %d]", HardMaxChains)
	}
	if p.MaxAssets == 0 || p.MaxAssets > HardMaxAssets {
		return fmt.Errorf("max_assets must be in (0, %d]", HardMaxAssets)
	}
	if p.MaxAttestorsPerChain == 0 || p.MaxAttestorsPerChain > HardMaxAttestor {
		return fmt.Errorf("max_attestors_per_chain must be in (0, %d]", HardMaxAttestor)
	}
	// The rolling-window mint cap must be fully configured or fully off: a
	// window length with no cap (or a cap with no window) is a misconfiguration
	// that would silently leave the time-bound limiter disabled.
	if (p.MintWindowSeconds == 0) != (p.MaxMintPerWindow == 0) {
		return fmt.Errorf("mint_window_seconds and max_mint_per_window must both be set or both be zero")
	}
	return nil
}
