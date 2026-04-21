package types

import "fmt"

// Hard upper bounds enforced by ValidateGenesis. These prevent obviously
// malicious genesis files from booting a chain that immediately OOMs the
// validator processes (e.g. MaxBatchSize: math.MaxUint32).
const (
	MaxBatchSizeUpperBound       = 10_000
	MaxMetadataSizeLowerBound    = 128
	MaxMetadataSizeUpperBound    = 65_536
	MaxSubmissionsPerBlockUpper  = 10_000
	DefaultMaxMetadataSize       = 4_096
	DefaultMaxSubmissionsPerAddr = 100
)

// DefaultParams returns the default energy module parameters.
//
// Permissionless controls how AllowedSubmitters is interpreted:
//   - true  (default, dev/test): anyone can submit; AllowedSubmitters is ignored.
//   - false (production):        only addresses in AllowedSubmitters can submit.
//     An empty AllowedSubmitters with Permissionless=false means no one can
//     submit, which is intentional - it forces operators to make an explicit
//     access-control decision before opening the chain.
func DefaultParams() Params {
	return Params{
		MaxBatchSize:                        100,
		AllowedSubmitters:                   []string{},
		Permissionless:                      true,
		MaxMetadataSize:                     DefaultMaxMetadataSize,
		MaxSubmissionsPerBlockPerSubmitter:  DefaultMaxSubmissionsPerAddr,
	}
}

func (p Params) Validate() error {
	if p.MaxBatchSize == 0 {
		return fmt.Errorf("max_batch_size must be positive")
	}
	if p.MaxBatchSize > MaxBatchSizeUpperBound {
		return fmt.Errorf("max_batch_size %d exceeds hard upper bound %d", p.MaxBatchSize, MaxBatchSizeUpperBound)
	}
	if p.MaxMetadataSize < MaxMetadataSizeLowerBound {
		return fmt.Errorf("max_metadata_size %d is below hard lower bound %d", p.MaxMetadataSize, MaxMetadataSizeLowerBound)
	}
	if p.MaxMetadataSize > MaxMetadataSizeUpperBound {
		return fmt.Errorf("max_metadata_size %d exceeds hard upper bound %d", p.MaxMetadataSize, MaxMetadataSizeUpperBound)
	}
	if p.MaxSubmissionsPerBlockPerSubmitter > MaxSubmissionsPerBlockUpper {
		return fmt.Errorf("max_submissions_per_block_per_submitter %d exceeds hard upper bound %d",
			p.MaxSubmissionsPerBlockPerSubmitter, MaxSubmissionsPerBlockUpper)
	}
	return nil
}

// DefaultGenesis returns the default genesis state for the energy module.
// NextID persists the auto-increment counter so export/import preserves
// monotonically increasing IDs and avoids overwriting existing records.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		DataRecords: []EnergyData{},
		Batches:     []BatchSubmission{},
		NextID:      0,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	ids := make(map[string]bool)
	for i, d := range gs.DataRecords {
		if d.ID == "" {
			return fmt.Errorf("data record %d has empty id", i)
		}
		if ids[d.ID] {
			return fmt.Errorf("duplicate data record id: %s", d.ID)
		}
		ids[d.ID] = true
		if d.Category == "" {
			return fmt.Errorf("data record %s has empty category", d.ID)
		}
		if d.DataHash == "" {
			return fmt.Errorf("data record %s has empty data hash", d.ID)
		}
		if d.Submitter == "" {
			return fmt.Errorf("data record %s has empty submitter", d.ID)
		}
		if uint32(len(d.Metadata)) > gs.Params.MaxMetadataSize {
			return fmt.Errorf("data record %s metadata size %d exceeds max %d",
				d.ID, len(d.Metadata), gs.Params.MaxMetadataSize)
		}
	}

	batchIDs := make(map[string]bool)
	for i, b := range gs.Batches {
		if b.ID == "" {
			return fmt.Errorf("batch %d has empty id", i)
		}
		if batchIDs[b.ID] {
			return fmt.Errorf("duplicate batch id: %s", b.ID)
		}
		batchIDs[b.ID] = true
	}

	return nil
}
