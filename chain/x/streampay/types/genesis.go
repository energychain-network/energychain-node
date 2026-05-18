package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxStreams:          DefaultMaxStreams,
		MaxStreamsPerSender: DefaultMaxStreamsPerSender,
		MinRatePerSecond:    DefaultMinRate,
		MaxRatePerSecond:    DefaultMaxRate,
		MaxDeposit:          DefaultMaxDeposit,
		MaxHorizonSeconds:   DefaultMaxHorizonSeconds,
		DefaultDenom:        "",
		MemoMaxLen:          DefaultMemoMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxStreams == 0 || p.MaxStreams > HardMaxStreams {
		return fmt.Errorf("max_streams must be in (0, %d]", HardMaxStreams)
	}
	if p.MaxStreamsPerSender == 0 || p.MaxStreamsPerSender > HardMaxStreamsPerSender {
		return fmt.Errorf("max_streams_per_sender must be in (0, %d]", HardMaxStreamsPerSender)
	}
	if p.MinRatePerSecond == 0 {
		return fmt.Errorf("min_rate_per_second must be > 0")
	}
	if p.MaxRatePerSecond < p.MinRatePerSecond {
		return fmt.Errorf("max_rate_per_second (%d) < min_rate_per_second (%d)", p.MaxRatePerSecond, p.MinRatePerSecond)
	}
	if p.MaxRatePerSecond > HardMaxRate {
		return fmt.Errorf("max_rate_per_second must be <= %d", HardMaxRate)
	}
	if p.MaxDeposit == 0 {
		return fmt.Errorf("max_deposit must be > 0")
	}
	if p.MaxHorizonSeconds <= 0 || p.MaxHorizonSeconds > HardMaxHorizonSeconds {
		return fmt.Errorf("max_horizon_seconds must be in (0, %d]", HardMaxHorizonSeconds)
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len must be <= %d", HardMemoMaxLen)
	}
	if p.DefaultDenom != "" {
		if err := ValidateDenom(p.DefaultDenom); err != nil {
			return err
		}
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:       DefaultParams(),
		NextStreamId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Streams)) > gs.Params.MaxStreams {
		return fmt.Errorf("genesis: %d streams exceeds max %d", len(gs.Streams), gs.Params.MaxStreams)
	}
	ids := map[uint64]bool{}
	perSender := map[string]uint32{}
	for _, s := range gs.Streams {
		if s.Id == 0 {
			return fmt.Errorf("genesis: stream id must be > 0")
		}
		if ids[s.Id] {
			return fmt.Errorf("genesis: duplicate stream id %d", s.Id)
		}
		ids[s.Id] = true
		if s.Id >= gs.NextStreamId {
			return fmt.Errorf("genesis: stream id %d >= next_stream_id %d", s.Id, gs.NextStreamId)
		}
		if err := ValidateAddr("sender", s.Sender); err != nil {
			return err
		}
		if err := ValidateAddr("receiver", s.Receiver); err != nil {
			return err
		}
		if s.Sender == s.Receiver {
			return fmt.Errorf("genesis: stream %d sender == receiver", s.Id)
		}
		if err := ValidateDenom(s.Denom); err != nil {
			return fmt.Errorf("genesis: stream %d: %w", s.Id, err)
		}
		if s.RatePerSecond < gs.Params.MinRatePerSecond || s.RatePerSecond > gs.Params.MaxRatePerSecond {
			return fmt.Errorf("genesis: stream %d rate %d out of range", s.Id, s.RatePerSecond)
		}
		if s.Deposit == 0 || s.Deposit > gs.Params.MaxDeposit {
			return fmt.Errorf("genesis: stream %d deposit %d out of range", s.Id, s.Deposit)
		}
		if !StatusValid(s.Status) {
			return fmt.Errorf("genesis: stream %d invalid status", s.Id)
		}
		if s.EndTime != 0 {
			if s.EndTime <= s.StartTime {
				return fmt.Errorf("genesis: stream %d end_time %d <= start_time %d", s.Id, s.EndTime, s.StartTime)
			}
			if s.EndTime-s.StartTime > gs.Params.MaxHorizonSeconds {
				return fmt.Errorf("genesis: stream %d horizon exceeds cap", s.Id)
			}
		}
		if s.Withdrawn > s.Deposit {
			return fmt.Errorf("genesis: stream %d withdrawn %d > deposit %d", s.Id, s.Withdrawn, s.Deposit)
		}
		if s.PausedAccumulatedSeconds < 0 {
			return fmt.Errorf("genesis: stream %d paused_accumulated_seconds < 0", s.Id)
		}
		if (s.PausedAt != 0) != (s.Status == Status_STATUS_PAUSED) {
			return fmt.Errorf("genesis: stream %d paused_at / status mismatch", s.Id)
		}
		perSender[s.Sender]++
		if perSender[s.Sender] > gs.Params.MaxStreamsPerSender {
			return fmt.Errorf("genesis: sender %s holds %d > max %d", s.Sender, perSender[s.Sender], gs.Params.MaxStreamsPerSender)
		}
	}
	return nil
}
