package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func bech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("invalid bech32 %q: %w", addr, err)
	}
	return nil
}

func (m *MsgCreateStream) ValidateBasic() error {
	if err := ValidateAddr("sender", m.Sender); err != nil {
		return err
	}
	if err := ValidateAddr("receiver", m.Receiver); err != nil {
		return err
	}
	if m.Sender == m.Receiver {
		return fmt.Errorf("sender and receiver must differ")
	}
	if m.Denom != "" {
		if err := ValidateDenom(m.Denom); err != nil {
			return err
		}
	}
	if m.RatePerSecond == 0 {
		return fmt.Errorf("rate_per_second must be > 0")
	}
	if m.Deposit == 0 {
		return fmt.Errorf("deposit must be > 0")
	}
	if m.StartTime < 0 {
		return fmt.Errorf("start_time cannot be negative")
	}
	if m.EndTime < 0 {
		return fmt.Errorf("end_time cannot be negative")
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgCreateStream) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}

func (m *MsgDepositToStream) ValidateBasic() error {
	if err := ValidateAddr("sender", m.Sender); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgDepositToStream) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}

func (m *MsgWithdraw) ValidateBasic() error {
	if err := ValidateAddr("receiver", m.Receiver); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	return nil
}
func (m *MsgWithdraw) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Receiver)
	return []sdk.AccAddress{addr}
}

func (m *MsgPause) ValidateBasic() error {
	if err := ValidateAddr("sender", m.Sender); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgPause) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}

func (m *MsgResume) ValidateBasic() error {
	if err := ValidateAddr("sender", m.Sender); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	return nil
}
func (m *MsgResume) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}

func (m *MsgCancel) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancel) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{addr}
}

func (m *MsgTransfer) ValidateBasic() error {
	if err := ValidateAddr("receiver", m.Receiver); err != nil {
		return err
	}
	if err := ValidateAddr("new_receiver", m.NewReceiver); err != nil {
		return err
	}
	if m.Receiver == m.NewReceiver {
		return fmt.Errorf("new_receiver must differ from current receiver")
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	return nil
}
func (m *MsgTransfer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Receiver)
	return []sdk.AccAddress{addr}
}

func (m *MsgChangeRate) ValidateBasic() error {
	if err := ValidateAddr("sender", m.Sender); err != nil {
		return err
	}
	if m.StreamId == 0 {
		return fmt.Errorf("stream_id must be > 0")
	}
	if m.NewRatePerSecond == 0 {
		return fmt.Errorf("new_rate_per_second must be > 0")
	}
	return nil
}
func (m *MsgChangeRate) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := bech32(m.Authority); err != nil {
		return fmt.Errorf("authority: %w", err)
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}
