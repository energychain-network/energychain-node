package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxJobs:             DefaultMaxJobs,
		MaxJobsPerOwner:     DefaultMaxJobsPerOwner,
		MinIntervalSeconds:  DefaultMinIntervalSeconds,
		MaxIntervalSeconds:  DefaultMaxIntervalSeconds,
		MaxPayloadBytes:     DefaultMaxPayloadBytes,
		MaxJobsPerBlock:     DefaultMaxJobsPerBlock,
		DefaultFeeDenom:     "",
		FeeCollector:        "",
		ErrorMaxLen:         DefaultErrorMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxJobs == 0 || p.MaxJobs > HardMaxJobs {
		return fmt.Errorf("max_jobs must be in (0, %d]", HardMaxJobs)
	}
	if p.MaxJobsPerOwner == 0 || p.MaxJobsPerOwner > HardMaxJobsPerOwner {
		return fmt.Errorf("max_jobs_per_owner must be in (0, %d]", HardMaxJobsPerOwner)
	}
	if p.MinIntervalSeconds < HardMinIntervalSeconds {
		return fmt.Errorf("min_interval_seconds must be >= %d", HardMinIntervalSeconds)
	}
	if p.MaxIntervalSeconds <= 0 || p.MaxIntervalSeconds > HardMaxIntervalSeconds {
		return fmt.Errorf("max_interval_seconds must be in (0, %d]", HardMaxIntervalSeconds)
	}
	if p.MinIntervalSeconds > p.MaxIntervalSeconds {
		return fmt.Errorf("min_interval_seconds (%d) > max_interval_seconds (%d)", p.MinIntervalSeconds, p.MaxIntervalSeconds)
	}
	if p.MaxPayloadBytes == 0 || p.MaxPayloadBytes > HardMaxPayloadBytes {
		return fmt.Errorf("max_payload_bytes must be in (0, %d]", HardMaxPayloadBytes)
	}
	if p.MaxJobsPerBlock == 0 || p.MaxJobsPerBlock > HardMaxJobsPerBlock {
		return fmt.Errorf("max_jobs_per_block must be in (0, %d]", HardMaxJobsPerBlock)
	}
	if p.ErrorMaxLen > HardErrorMaxLen {
		return fmt.Errorf("error_max_len must be <= %d", HardErrorMaxLen)
	}
	if p.DefaultFeeDenom != "" {
		if err := ValidateDenom(p.DefaultFeeDenom); err != nil {
			return err
		}
	}
	if err := ValidateOptionalAddr("fee_collector", p.FeeCollector); err != nil {
		return err
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:    DefaultParams(),
		NextJobId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Jobs)) > gs.Params.MaxJobs {
		return fmt.Errorf("genesis: %d jobs exceeds max_jobs %d", len(gs.Jobs), gs.Params.MaxJobs)
	}

	ids := map[uint64]bool{}
	perOwner := map[string]uint32{}
	for _, j := range gs.Jobs {
		if j.Id == 0 {
			return fmt.Errorf("genesis: job id must be > 0")
		}
		if ids[j.Id] {
			return fmt.Errorf("genesis: duplicate job id %d", j.Id)
		}
		ids[j.Id] = true
		if j.Id >= gs.NextJobId {
			return fmt.Errorf("genesis: job id %d >= next_job_id %d", j.Id, gs.NextJobId)
		}
		if err := ValidateAddr("owner", j.Owner); err != nil {
			return err
		}
		if !StatusValid(j.Status) {
			return fmt.Errorf("genesis: job %d invalid status", j.Id)
		}
		if j.IntervalSeconds < gs.Params.MinIntervalSeconds || j.IntervalSeconds > gs.Params.MaxIntervalSeconds {
			return fmt.Errorf("genesis: job %d interval_seconds %d out of range", j.Id, j.IntervalSeconds)
		}
		if j.MaxExecutions > 0 && j.ExecutionsDone > j.MaxExecutions {
			return fmt.Errorf("genesis: job %d executions_done %d > max %d", j.Id, j.ExecutionsDone, j.MaxExecutions)
		}
		if err := ValidateDenom(j.FeeDenom); err != nil {
			return fmt.Errorf("genesis: job %d: %w", j.Id, err)
		}
		if j.Payload == nil {
			return fmt.Errorf("genesis: job %d payload must be set", j.Id)
		}
		if uint32(j.Payload.Size()) > gs.Params.MaxPayloadBytes {
			return fmt.Errorf("genesis: job %d payload %d > max %d", j.Id, j.Payload.Size(), gs.Params.MaxPayloadBytes)
		}
		if j.FeesSpent > 0 && j.FeesSpent < j.FeesSpent { // tautology guard against silly negative-uint reasoning
			return fmt.Errorf("genesis: job %d fees_spent invariant", j.Id)
		}
		perOwner[j.Owner]++
		if perOwner[j.Owner] > gs.Params.MaxJobsPerOwner {
			return fmt.Errorf("genesis: owner %s holds %d jobs > max %d", j.Owner, perOwner[j.Owner], gs.Params.MaxJobsPerOwner)
		}
	}
	return nil
}
