package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

func (m *MsgCreateMarket) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateDenom(m.BaseDenom); err != nil {
		return err
	}
	if err := ValidateDenom(m.QuoteDenom); err != nil {
		return err
	}
	if m.BaseDenom == m.QuoteDenom {
		return ErrInvalidField.Wrap("base and quote denom must differ")
	}
	if m.FeeBps > HardMaxFeeBps {
		return ErrInvalidField.Wrap("fee_bps too high")
	}
	if m.MinBaseQty == 0 {
		return ErrInvalidField.Wrap("min_base_qty must be > 0")
	}
	if m.BatchInterval < 1 {
		return ErrInvalidField.Wrap("batch_interval must be >= 1")
	}
	if m.Operator != "" {
		if err := MustBech32(m.Operator); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgCreateMarket) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgPostBond) ValidateBasic() error {
	if err := MustBech32(m.Operator); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	return nil
}
func (m *MsgPostBond) GetSigners() []sdk.AccAddress { return signer(m.Operator) }

func (m *MsgDelistMarket) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	return nil
}
func (m *MsgDelistMarket) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgSetMarketStatus) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if !MarketStatusValid(m.Status) {
		return ErrInvalidField.Wrap("invalid market status")
	}
	return nil
}
func (m *MsgSetMarketStatus) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgPlaceOrder) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if !OrderSideValid(m.Side) {
		return ErrInvalidField.Wrap("invalid order side")
	}
	if m.Price == 0 {
		return ErrInvalidField.Wrap("price must be > 0")
	}
	if m.Quantity == 0 {
		return ErrInvalidField.Wrap("quantity must be > 0")
	}
	return nil
}
func (m *MsgPlaceOrder) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgCancelOrder) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	if m.OrderId == 0 {
		return ErrInvalidField.Wrap("order_id must be > 0")
	}
	return nil
}
func (m *MsgCancelOrder) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
