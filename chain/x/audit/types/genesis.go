package types

import "fmt"

// DefaultParams returns the default audit module parameters.
//
// Permissionless mirrors the energy module: when true (default for dev),
// any address can record audit logs; when false, only addresses listed in
// AllowedAuditors are accepted.
func DefaultParams() Params {
	return Params{
		LargeTransferThreshold:       1_000_000,
		RetentionBlocks:              0,
		AllowedAuditors:              []string{},
		MaxDataSize:                  DefaultAuditMaxData,
		Permissionless:               true,
		MaxAuditsPerBlockPerCreator:  DefaultAuditMaxPerBlock,
	}
}

func (p Params) Validate() error {
	if p.RetentionBlocks < 0 {
		return fmt.Errorf("retention_blocks must be non-negative, got %d", p.RetentionBlocks)
	}
	if p.RetentionBlocks > AuditRetentionUpperBound {
		return fmt.Errorf("retention_blocks %d exceeds hard upper bound %d",
			p.RetentionBlocks, AuditRetentionUpperBound)
	}
	if p.MaxDataSize < AuditMaxDataLowerBound {
		return fmt.Errorf("max_data_size %d is below hard lower bound %d",
			p.MaxDataSize, AuditMaxDataLowerBound)
	}
	if p.MaxDataSize > AuditMaxDataUpperBound {
		return fmt.Errorf("max_data_size %d exceeds hard upper bound %d (1 MiB)",
			p.MaxDataSize, AuditMaxDataUpperBound)
	}
	if p.MaxAuditsPerBlockPerCreator > AuditMaxAuditsPerBlockUpper {
		return fmt.Errorf("max_audits_per_block_per_creator %d exceeds hard upper bound %d",
			p.MaxAuditsPerBlockPerCreator, AuditMaxAuditsPerBlockUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:  DefaultParams(),
		Logs:    []AuditLog{},
		Counter: 0,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	seen := make(map[uint64]bool)
	for i, log := range gs.Logs {
		if seen[log.ID] {
			return fmt.Errorf("duplicate audit log ID %d at index %d", log.ID, i)
		}
		seen[log.ID] = true

		if log.Actor == "" {
			return fmt.Errorf("audit log %d has empty actor", log.ID)
		}
		if log.Action == "" {
			return fmt.Errorf("audit log %d has empty action", log.ID)
		}
		if log.EventType == "" {
			return fmt.Errorf("audit log %d has empty event_type", log.ID)
		}
		if uint32(len(log.Data)) > gs.Params.MaxDataSize {
			return fmt.Errorf("audit log %d data size %d exceeds max %d",
				log.ID, len(log.Data), gs.Params.MaxDataSize)
		}
	}
	return nil
}
