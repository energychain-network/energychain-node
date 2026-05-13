package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxDataProviders:        DefaultMaxDataProviders,
		MaxGridZones:            DefaultMaxGridZones,
		MaxSubjects:             DefaultMaxSubjects,
		MaxAttestationsPerBatch: DefaultMaxAttestationsPerBatch,
		MaxMatchesPerCall:       DefaultMaxMatchesPerCall,
		MaxReportPeriodDays:     DefaultMaxReportPeriodDays,
		RequireActiveProvider:   true,
		HourSeconds:             uint32(HourSeconds),
	}
}

func (p Params) Validate() error {
	if p.MaxDataProviders == 0 {
		return fmt.Errorf("max_data_providers must be > 0")
	}
	if p.MaxGridZones == 0 {
		return fmt.Errorf("max_grid_zones must be > 0")
	}
	if p.MaxSubjects == 0 {
		return fmt.Errorf("max_subjects must be > 0")
	}
	if p.MaxAttestationsPerBatch == 0 || p.MaxAttestationsPerBatch > MaxAttestationsPerBatchUpper {
		return fmt.Errorf("max_attestations_per_batch %d out of range (1..=%d)",
			p.MaxAttestationsPerBatch, MaxAttestationsPerBatchUpper)
	}
	if p.MaxReportPeriodDays == 0 || p.MaxReportPeriodDays > MaxReportPeriodDaysUpper {
		return fmt.Errorf("max_report_period_days %d out of range (1..=%d)",
			p.MaxReportPeriodDays, MaxReportPeriodDaysUpper)
	}
	if p.HourSeconds != uint32(HourSeconds) {
		return fmt.Errorf("hour_seconds must be %d (24/7 CFE Compact requires 1-hour granularity)", HourSeconds)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	providers := map[string]bool{}
	for _, p := range gs.DataProviders {
		if err := ValidateID(p.Id, "provider"); err != nil {
			return err
		}
		if providers[p.Id] {
			return fmt.Errorf("duplicate data provider %q", p.Id)
		}
		providers[p.Id] = true
		if !DataProviderStatusValid(p.Status) {
			return fmt.Errorf("provider %q invalid status", p.Id)
		}
	}
	zones := map[string]bool{}
	for _, z := range gs.GridZones {
		if err := ValidateID(z.Id, "zone"); err != nil {
			return err
		}
		if zones[z.Id] {
			return fmt.Errorf("duplicate grid zone %q", z.Id)
		}
		zones[z.Id] = true
	}
	subjects := map[string]bool{}
	for _, s := range gs.Subjects {
		if err := ValidateID(s.Id, "subject"); err != nil {
			return err
		}
		if subjects[s.Id] {
			return fmt.Errorf("duplicate subject %q", s.Id)
		}
		subjects[s.Id] = true
		if !SubjectStatusValid(s.Status) {
			return fmt.Errorf("subject %q invalid status", s.Id)
		}
		if s.DefaultGridZone != "" && !zones[s.DefaultGridZone] {
			return fmt.Errorf("subject %q references unknown grid zone %q", s.Id, s.DefaultGridZone)
		}
	}
	type hkey struct {
		s string
		h int64
	}
	consumptions := map[hkey]uint64{}
	for _, c := range gs.Consumptions {
		if !subjects[c.SubjectId] {
			return fmt.Errorf("consumption references unknown subject %q", c.SubjectId)
		}
		if err := ValidateHourStart(c.HourStart); err != nil {
			return err
		}
		if c.HourEnd != c.HourStart+HourSeconds {
			return fmt.Errorf("consumption hour_end mismatch for subject %q hour %d", c.SubjectId, c.HourStart)
		}
		k := hkey{c.SubjectId, c.HourStart}
		if _, dup := consumptions[k]; dup {
			return fmt.Errorf("duplicate consumption for subject %q hour %d", c.SubjectId, c.HourStart)
		}
		consumptions[k] = c.WhConsumed
	}
	for _, m := range gs.Matches {
		if m.Id == 0 || m.Id > gs.MatchIdSeq {
			return fmt.Errorf("match id %d out of range", m.Id)
		}
		if !subjects[m.SubjectId] {
			return fmt.Errorf("match references unknown subject %q", m.SubjectId)
		}
		if err := ValidateHourStart(m.HourStart); err != nil {
			return err
		}
	}
	for _, agg := range gs.Aggregates {
		if !subjects[agg.SubjectId] {
			return fmt.Errorf("aggregate references unknown subject %q", agg.SubjectId)
		}
		if agg.WhMatchedTotal < agg.WhMatchedSameZone {
			return fmt.Errorf("aggregate invariant: matched_total < matched_same_zone for subject %q hour %d",
				agg.SubjectId, agg.HourStart)
		}
		if agg.WhMatchedTotal < agg.WhMatchedStorage {
			return fmt.Errorf("aggregate invariant: matched_total < matched_storage for subject %q hour %d",
				agg.SubjectId, agg.HourStart)
		}
	}
	for _, r := range gs.Reports {
		if r.Id == 0 || r.Id > gs.ReportIdSeq {
			return fmt.Errorf("report id %d out of range", r.Id)
		}
		if !subjects[r.SubjectId] {
			return fmt.Errorf("report references unknown subject %q", r.SubjectId)
		}
	}
	return nil
}
