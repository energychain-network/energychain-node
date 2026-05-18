package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (m *MsgCreatePair) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if err := ValidateDenom(m.BaseDenom); err != nil {
		return err
	}
	if err := ValidateDenom(m.QuoteDenom); err != nil {
		return err
	}
	if m.BaseDenom == m.QuoteDenom {
		return fmt.Errorf("base_denom == quote_denom")
	}
	if !ModeValid(m.Mode) {
		return fmt.Errorf("invalid mode")
	}
	if m.Mode == MatchMode_MATCH_MODE_FBA && m.BatchIntervalSeconds <= 0 {
		return fmt.Errorf("batch_interval_seconds must be > 0 for FBA")
	}
	if m.PriceBandLo > 0 && m.PriceBandHi > 0 && m.PriceBandLo > m.PriceBandHi {
		return fmt.Errorf("price_band_lo > price_band_hi")
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgCreatePair) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgPauseUnpausePair) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.PairId == 0 {
		return fmt.Errorf("pair_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgPauseUnpausePair) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdatePairRisk) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.PairId == 0 {
		return fmt.Errorf("pair_id must be > 0")
	}
	if m.PriceBandLo > 0 && m.PriceBandHi > 0 && m.PriceBandLo > m.PriceBandHi {
		return fmt.Errorf("price_band_lo > price_band_hi")
	}
	return nil
}
func (m *MsgUpdatePairRisk) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgPlaceLimitOrder) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.PairId == 0 {
		return fmt.Errorf("pair_id must be > 0")
	}
	if !SideValid(m.Side) {
		return fmt.Errorf("invalid side")
	}
	if m.Price == 0 {
		return fmt.Errorf("price must be > 0")
	}
	if m.Quantity == 0 {
		return fmt.Errorf("quantity must be > 0")
	}
	if m.Price > MaxPrice {
		return fmt.Errorf("price %d exceeds max %d (buy-book sort sentinel)", m.Price, MaxPrice)
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgPlaceLimitOrder) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgCancelOrder) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.OrderId == 0 {
		return fmt.Errorf("order_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelOrder) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgClearBatch) ValidateBasic() error {
	if err := ValidateAddr("caller", m.Caller); err != nil {
		return err
	}
	if m.PairId == 0 {
		return fmt.Errorf("pair_id must be > 0")
	}
	return nil
}
func (m *MsgClearBatch) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Caller)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
