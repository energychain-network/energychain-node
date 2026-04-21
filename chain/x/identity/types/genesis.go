package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultParams() Params {
	return Params{
		AdminAddress:    "",
		MaxMetadataSize: DefaultIdentityMaxMetadata,
	}
}

// Validate ensures Params are well-formed. An empty AdminAddress is allowed
// (so DefaultParams() and dev chains pass validation), but if set it must
// be a valid bech32 address.
func (p Params) Validate() error {
	if p.AdminAddress != "" {
		if _, err := sdk.AccAddressFromBech32(p.AdminAddress); err != nil {
			return fmt.Errorf("invalid admin_address: %w", err)
		}
	}
	if p.MaxMetadataSize < IdentityMaxMetadataLowerBound {
		return fmt.Errorf("max_metadata_size %d is below hard lower bound %d",
			p.MaxMetadataSize, IdentityMaxMetadataLowerBound)
	}
	if p.MaxMetadataSize > IdentityMaxMetadataUpperBound {
		return fmt.Errorf("max_metadata_size %d exceeds hard upper bound %d",
			p.MaxMetadataSize, IdentityMaxMetadataUpperBound)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:     DefaultParams(),
		Identities: []Identity{},
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid identity params: %w", err)
	}
	seen := make(map[string]bool)
	for i, id := range gs.Identities {
		if id.Address == "" {
			return fmt.Errorf("identity at index %d has empty address", i)
		}
		if seen[id.Address] {
			return fmt.Errorf("duplicate identity address: %s", id.Address)
		}
		seen[id.Address] = true
		if id.Name == "" {
			return fmt.Errorf("identity %s has empty name", id.Address)
		}
		if id.Role == "" {
			return fmt.Errorf("identity %s has empty role", id.Address)
		}
		if id.Status == "" {
			return fmt.Errorf("identity %s has empty status", id.Address)
		}
		if uint32(len(id.Metadata)) > gs.Params.MaxMetadataSize {
			return fmt.Errorf("identity %s metadata size %d exceeds max %d",
				id.Address, len(id.Metadata), gs.Params.MaxMetadataSize)
		}
	}
	return nil
}
