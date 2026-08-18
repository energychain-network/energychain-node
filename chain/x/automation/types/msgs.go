package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

func (m *MsgCreateSchedule) ValidateBasic() error {
	if err := MustBech32(m.Creator); err != nil {
		return err
	}
	if !ActionKindValid(m.Action) {
		return ErrInvalidField.Wrap("invalid action")
	}
	if m.TargetId == 0 {
		return ErrInvalidField.Wrap("target_id must be > 0")
	}
	if m.IntervalSeconds <= 0 {
		return ErrInvalidField.Wrap("interval_seconds must be > 0")
	}
	if m.EndTime != 0 && m.StartTime != 0 && m.EndTime <= m.StartTime {
		return ErrInvalidField.Wrap("end_time must be after start_time")
	}
	if ActionNeedsAmount(m.Action) {
		if m.AmountPerRun == 0 {
			return ErrInvalidField.Wrap("amount_per_run must be > 0 for this action")
		}
	} else if m.AmountPerRun != 0 {
		return ErrInvalidField.Wrap("amount_per_run must be 0 for this action")
	}
	return nil
}
func (m *MsgCreateSchedule) GetSigners() []sdk.AccAddress { return signer(m.Creator) }

func (m *MsgPauseSchedule) ValidateBasic() error {
	if err := MustBech32(m.Creator); err != nil {
		return err
	}
	if m.ScheduleId == 0 {
		return ErrInvalidField.Wrap("schedule_id must be > 0")
	}
	return nil
}
func (m *MsgPauseSchedule) GetSigners() []sdk.AccAddress { return signer(m.Creator) }

func (m *MsgResumeSchedule) ValidateBasic() error {
	if err := MustBech32(m.Creator); err != nil {
		return err
	}
	if m.ScheduleId == 0 {
		return ErrInvalidField.Wrap("schedule_id must be > 0")
	}
	return nil
}
func (m *MsgResumeSchedule) GetSigners() []sdk.AccAddress { return signer(m.Creator) }

func (m *MsgCancelSchedule) ValidateBasic() error {
	if err := MustBech32(m.Creator); err != nil {
		return err
	}
	if m.ScheduleId == 0 {
		return ErrInvalidField.Wrap("schedule_id must be > 0")
	}
	return nil
}
func (m *MsgCancelSchedule) GetSigners() []sdk.AccAddress { return signer(m.Creator) }

func (m *MsgCreateStream) ValidateBasic() error {
	if err := MustBech32(m.Sender); err != nil {
		return err
	}
	if err := MustBech32(m.Receiver); err != nil {
		return err
	}
	if m.Sender == m.Receiver {
		return ErrInvalidField.Wrap("sender and receiver must differ")
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if m.Deposit == 0 {
		return ErrInvalidField.Wrap("deposit must be > 0")
	}
	if m.RatePerSec == 0 {
		return ErrInvalidField.Wrap("rate_per_sec must be > 0")
	}
	return nil
}
func (m *MsgCreateStream) GetSigners() []sdk.AccAddress { return signer(m.Sender) }

func (m *MsgWithdrawStream) ValidateBasic() error {
	if err := MustBech32(m.Caller); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return ErrInvalidField.Wrap("stream_id must be > 0")
	}
	return nil
}
func (m *MsgWithdrawStream) GetSigners() []sdk.AccAddress { return signer(m.Caller) }

func (m *MsgCancelStream) ValidateBasic() error {
	if err := MustBech32(m.Sender); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return ErrInvalidField.Wrap("stream_id must be > 0")
	}
	return nil
}
func (m *MsgCancelStream) GetSigners() []sdk.AccAddress { return signer(m.Sender) }

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
