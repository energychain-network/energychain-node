package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MinSubmissions:  1,
		DataMaxAge:      3600,
		MaxMetadataSize: DefaultOracleMaxMetadata,
	}
}

func (p Params) Validate() error {
	if p.MinSubmissions == 0 {
		return fmt.Errorf("min_submissions must be positive")
	}
	if p.DataMaxAge <= 0 {
		return fmt.Errorf("data_max_age must be positive")
	}
	if p.DataMaxAge > OracleDataMaxAgeUpperBound {
		return fmt.Errorf("data_max_age %d exceeds hard upper bound %d (1 year)",
			p.DataMaxAge, OracleDataMaxAgeUpperBound)
	}
	if p.MaxMetadataSize < OracleMaxMetadataLowerBound {
		return fmt.Errorf("max_metadata_size %d is below hard lower bound %d",
			p.MaxMetadataSize, OracleMaxMetadataLowerBound)
	}
	if p.MaxMetadataSize > OracleMaxMetadataUpperBound {
		return fmt.Errorf("max_metadata_size %d exceeds hard upper bound %d",
			p.MaxMetadataSize, OracleMaxMetadataUpperBound)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:  DefaultParams(),
		Oracles: []OracleInfo{},
		Data:    []OracleData{},
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	oracleAddrs := make(map[string]bool)
	for i, o := range gs.Oracles {
		if o.Address == "" {
			return fmt.Errorf("oracle %d has empty address", i)
		}
		if oracleAddrs[o.Address] {
			return fmt.Errorf("duplicate oracle address: %s", o.Address)
		}
		oracleAddrs[o.Address] = true
		if o.Name == "" {
			return fmt.Errorf("oracle %d has empty name", i)
		}
	}

	// Reject obviously bogus timestamps. We don't bound by current wall clock
	// (chains may import historical genesis), but timestamps must be > 0 and
	// not exceed the year 9999 to prevent the int64-as-uint64 conversion in
	// store keys (see GetOracleDataKey) from producing collisions.
	const maxTimestamp int64 = 253402300800 // 9999-12-31T00:00:00Z
	seen := make(map[string]map[int64]bool)
	for i, d := range gs.Data {
		if d.Category == "" {
			return fmt.Errorf("data %d has empty category", i)
		}
		if d.Value == "" {
			return fmt.Errorf("data %d has empty value", i)
		}
		if d.Timestamp <= 0 {
			return fmt.Errorf("data %d has non-positive timestamp %d", i, d.Timestamp)
		}
		if d.Timestamp > maxTimestamp {
			return fmt.Errorf("data %d timestamp %d exceeds max allowed", i, d.Timestamp)
		}
		if d.Submitter != "" && !oracleAddrs[d.Submitter] {
			return fmt.Errorf("data %d submitted by unknown oracle %s", i, d.Submitter)
		}
		if uint32(len(d.Metadata)) > gs.Params.MaxMetadataSize {
			return fmt.Errorf("data %d metadata size %d exceeds max %d",
				i, len(d.Metadata), gs.Params.MaxMetadataSize)
		}
		if seen[d.Category] == nil {
			seen[d.Category] = make(map[int64]bool)
		}
		if seen[d.Category][d.Timestamp] {
			return fmt.Errorf("data %d duplicates (category=%s, timestamp=%d)", i, d.Category, d.Timestamp)
		}
		seen[d.Category][d.Timestamp] = true
	}

	return nil
}
