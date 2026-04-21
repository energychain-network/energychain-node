package types_test

import (
	"strings"
	"testing"

	"energychain/x/energy/types"
)

// TestParams_DefaultParams_AreValid is a "smoke test" pinning the invariant
// that DefaultParams() always returns a Validate-clean Params. If a future
// refactor of DefaultParams drifts the cap below the lower bound this fires.
func TestParams_DefaultParams_AreValid(t *testing.T) {
	if err := types.DefaultParams().Validate(); err != nil {
		t.Fatalf("default params must validate: %v", err)
	}
}

func TestParams_Validate_Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*types.Params)
		wantErr string
	}{
		{
			name:    "max_batch_size zero",
			mutate:  func(p *types.Params) { p.MaxBatchSize = 0 },
			wantErr: "max_batch_size",
		},
		{
			name:    "max_batch_size above ceiling",
			mutate:  func(p *types.Params) { p.MaxBatchSize = types.MaxBatchSizeUpperBound + 1 },
			wantErr: "max_batch_size",
		},
		{
			name:    "metadata cap below floor",
			mutate:  func(p *types.Params) { p.MaxMetadataSize = types.MaxMetadataSizeLowerBound - 1 },
			wantErr: "max_metadata_size",
		},
		{
			name:    "metadata cap above ceiling",
			mutate:  func(p *types.Params) { p.MaxMetadataSize = types.MaxMetadataSizeUpperBound + 1 },
			wantErr: "max_metadata_size",
		},
		{
			name:    "rate-limit above ceiling",
			mutate:  func(p *types.Params) { p.MaxSubmissionsPerBlockPerSubmitter = types.MaxSubmissionsPerBlockUpper + 1 },
			wantErr: "max_submissions_per_block",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := types.DefaultParams()
			tc.mutate(&p)
			err := p.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q must mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestParams_Validate_AtBoundariesAccepted asserts the inclusive bounds
// (lower floor for metadata, upper ceiling for batch / metadata / rate)
// are NOT rejected. This guards against off-by-one regressions.
func TestParams_Validate_AtBoundariesAccepted(t *testing.T) {
	p := types.Params{
		MaxBatchSize:                       types.MaxBatchSizeUpperBound,
		MaxMetadataSize:                    types.MaxMetadataSizeUpperBound,
		MaxSubmissionsPerBlockPerSubmitter: types.MaxSubmissionsPerBlockUpper,
		Permissionless:                     true,
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("upper-bound params must be accepted: %v", err)
	}
	p.MaxMetadataSize = types.MaxMetadataSizeLowerBound
	if err := p.Validate(); err != nil {
		t.Fatalf("lower-bound metadata cap must be accepted: %v", err)
	}
}

func TestGenesis_DefaultGenesis_IsValid(t *testing.T) {
	if err := types.DefaultGenesis().Validate(); err != nil {
		t.Fatalf("default genesis must validate: %v", err)
	}
}

func TestGenesis_RejectsDuplicateRecordIDs(t *testing.T) {
	gs := types.GenesisState{
		Params: types.DefaultParams(),
		DataRecords: []types.EnergyData{
			{ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h"},
			{ID: "energy-1", Category: "meter", Submitter: "b", DataHash: "h2"},
		},
	}
	err := gs.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestGenesis_RejectsEmptyRequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		rec     types.EnergyData
		wantErr string
	}{
		{"empty id", types.EnergyData{Category: "meter", Submitter: "a", DataHash: "h"}, "empty id"},
		{"empty category", types.EnergyData{ID: "energy-1", Submitter: "a", DataHash: "h"}, "empty category"},
		{"empty hash", types.EnergyData{ID: "energy-1", Category: "meter", Submitter: "a"}, "empty data hash"},
		{"empty submitter", types.EnergyData{ID: "energy-1", Category: "meter", DataHash: "h"}, "empty submitter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gs := types.GenesisState{Params: types.DefaultParams(), DataRecords: []types.EnergyData{tc.rec}}
			err := gs.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestGenesis_RejectsMetadataOverCap mirrors the keeper's runtime check at
// genesis-load time. Without this, a hostile genesis could seed records that
// immediately violate the MetadataWithinCap invariant.
func TestGenesis_RejectsMetadataOverCap(t *testing.T) {
	gs := types.GenesisState{
		Params: types.Params{
			MaxBatchSize:                       100,
			MaxMetadataSize:                    256,
			MaxSubmissionsPerBlockPerSubmitter: 10,
			Permissionless:                     true,
		},
		DataRecords: []types.EnergyData{{
			ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h",
			Metadata: strings.Repeat("X", 257),
		}},
	}
	err := gs.Validate()
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected metadata-over-cap rejection, got %v", err)
	}
}

func TestGenesis_RejectsDuplicateBatchIDs(t *testing.T) {
	gs := types.GenesisState{
		Params: types.DefaultParams(),
		Batches: []types.BatchSubmission{
			{ID: "b-1", Submitter: "a", Category: "meter", DataCount: 1, MerkleRoot: "r"},
			{ID: "b-1", Submitter: "b", Category: "meter", DataCount: 1, MerkleRoot: "r2"},
		},
	}
	err := gs.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate batch error, got %v", err)
	}
}

// FuzzParams_Validate exercises the validator with random byte inputs to
// confirm it never panics. Run with: go test -fuzz=FuzzParams ./x/energy/types
func FuzzParams_Validate(f *testing.F) {
	seeds := [][]uint32{
		{100, 4096, 100, 0, 1},
		{0, 0, 0, 0, 0},
		{1, 128, 0, 1, 0},
		{10000, 65536, 10000, 1, 0},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2], byte(s[3]), byte(s[4]))
	}
	f.Fuzz(func(t *testing.T, batch, meta, rate uint32, perm, _ byte) {
		p := types.Params{
			MaxBatchSize:                       batch,
			MaxMetadataSize:                    meta,
			MaxSubmissionsPerBlockPerSubmitter: rate,
			Permissionless:                     perm&1 == 1,
		}
		// Validation must never panic.
		_ = p.Validate()
	})
}
