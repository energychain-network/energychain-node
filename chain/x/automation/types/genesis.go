package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	scheduleIDs := map[uint64]bool{}
	var maxSchedule uint64
	for _, s := range gs.Schedules {
		if s.Id == 0 {
			return fmt.Errorf("genesis: schedule id must be > 0")
		}
		if scheduleIDs[s.Id] {
			return fmt.Errorf("genesis: duplicate schedule id %d", s.Id)
		}
		scheduleIDs[s.Id] = true
		if _, err := sdk.AccAddressFromBech32(s.Creator); err != nil {
			return fmt.Errorf("genesis: schedule %d creator: %w", s.Id, err)
		}
		if !ActionKindValid(s.Action) {
			return fmt.Errorf("genesis: schedule %d invalid action", s.Id)
		}
		if s.TargetId == 0 {
			return fmt.Errorf("genesis: schedule %d target_id must be > 0", s.Id)
		}
		if s.IntervalSeconds <= 0 {
			return fmt.Errorf("genesis: schedule %d interval must be > 0", s.Id)
		}
		if ActionNeedsAmount(s.Action) {
			if s.AmountPerRun == 0 {
				return fmt.Errorf("genesis: schedule %d requires amount_per_run", s.Id)
			}
		} else if s.AmountPerRun != 0 {
			return fmt.Errorf("genesis: schedule %d must not set amount_per_run", s.Id)
		}
		if !ScheduleStatusValid(s.Status) {
			return fmt.Errorf("genesis: schedule %d invalid status", s.Id)
		}
		if s.MaxRuns != 0 && s.RunsDone > s.MaxRuns {
			return fmt.Errorf("genesis: schedule %d runs_done %d > max_runs %d", s.Id, s.RunsDone, s.MaxRuns)
		}
		// An ACTIVE schedule that has already hit its run cap would otherwise
		// fire one extra funded action on the first tick; such a schedule must
		// have been completed before export.
		if s.Status == ScheduleStatus_SCHEDULE_STATUS_ACTIVE && s.MaxRuns != 0 && s.RunsDone >= s.MaxRuns {
			return fmt.Errorf("genesis: active schedule %d already at max_runs", s.Id)
		}
		if s.EndTime != 0 && s.NextRun > s.EndTime && s.Status == ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
			return fmt.Errorf("genesis: active schedule %d next_run past end_time", s.Id)
		}
		if s.Id > maxSchedule {
			maxSchedule = s.Id
		}
	}
	if uint32(len(gs.Schedules)) > gs.Params.MaxSchedules {
		return fmt.Errorf("genesis: schedules exceed max")
	}

	streamIDs := map[uint64]bool{}
	var maxStream uint64
	for _, st := range gs.Streams {
		if st.Id == 0 {
			return fmt.Errorf("genesis: stream id must be > 0")
		}
		if streamIDs[st.Id] {
			return fmt.Errorf("genesis: duplicate stream id %d", st.Id)
		}
		streamIDs[st.Id] = true
		if _, err := sdk.AccAddressFromBech32(st.Sender); err != nil {
			return fmt.Errorf("genesis: stream %d sender: %w", st.Id, err)
		}
		if _, err := sdk.AccAddressFromBech32(st.Receiver); err != nil {
			return fmt.Errorf("genesis: stream %d receiver: %w", st.Id, err)
		}
		if err := ValidateDenom(st.Denom); err != nil {
			return fmt.Errorf("genesis: stream %d denom: %w", st.Id, err)
		}
		if st.Deposit == 0 || st.RatePerSec == 0 {
			return fmt.Errorf("genesis: stream %d deposit and rate must be > 0", st.Id)
		}
		if st.Withdrawn > st.Deposit {
			return fmt.Errorf("genesis: stream %d withdrawn > deposit", st.Id)
		}
		wantStop, err := StopTime(st.StartTime, st.Deposit, st.RatePerSec)
		if err != nil {
			return fmt.Errorf("genesis: stream %d stop_time: %w", st.Id, err)
		}
		if st.StopTime != wantStop {
			return fmt.Errorf("genesis: stream %d stop_time %d != %d", st.Id, st.StopTime, wantStop)
		}
		if !StreamStatusValid(st.Status) {
			return fmt.Errorf("genesis: stream %d invalid status", st.Id)
		}
		switch st.Status {
		case StreamStatus_STREAM_STATUS_ACTIVE:
			if st.Withdrawn >= st.Deposit {
				return fmt.Errorf("genesis: active stream %d fully withdrawn", st.Id)
			}
		case StreamStatus_STREAM_STATUS_COMPLETED:
			if st.Withdrawn != st.Deposit {
				return fmt.Errorf("genesis: completed stream %d not fully withdrawn", st.Id)
			}
		}
		if st.Id > maxStream {
			maxStream = st.Id
		}
	}
	if uint32(len(gs.Streams)) > gs.Params.MaxStreams {
		return fmt.Errorf("genesis: streams exceed max")
	}

	if gs.ScheduleIdSeq < maxSchedule {
		return fmt.Errorf("genesis: schedule_id_seq %d < max id %d", gs.ScheduleIdSeq, maxSchedule)
	}
	if gs.StreamIdSeq < maxStream {
		return fmt.Errorf("genesis: stream_id_seq %d < max id %d", gs.StreamIdSeq, maxStream)
	}
	return nil
}

// EscrowByDenom returns the settlement each denom's escrow must hold to back
// the outstanding (unwithdrawn) balance of ACTIVE streams. Used by
// InitGenesis to reconcile declared streams against the x/stableusd ledger.
func (gs GenesisState) EscrowByDenom() (map[string]uint64, error) {
	out := map[string]uint64{}
	for _, st := range gs.Streams {
		if st.Status != StreamStatus_STREAM_STATUS_ACTIVE {
			continue
		}
		rem, err := SafeSub(st.Deposit, st.Withdrawn)
		if err != nil {
			return nil, err
		}
		s, err := SafeAdd(out[st.Denom], rem)
		if err != nil {
			return nil, err
		}
		out[st.Denom] = s
	}
	return out, nil
}
