package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

func (m *MsgCreateMarket) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if err := ValidateMaxLen("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if err := ValidateDenom(m.SettlementDenom); err != nil {
		return err
	}
	if m.InitialPrice == 0 {
		return ErrInvalidField.Wrap("initial_price must be > 0")
	}
	if m.MintFeeBps > Bps || m.MeltFeeBps > Bps {
		return ErrInvalidField.Wrap("fees must be <= 10000 bps")
	}
	return ValidateMaxLen("policy_id", m.PolicyId, 128)
}
func (m *MsgCreateMarket) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgUpdateMarket) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if err := ValidateMaxLen("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if m.MintFeeBps > Bps || m.MeltFeeBps > Bps {
		return ErrInvalidField.Wrap("fees must be <= 10000 bps")
	}
	return ValidateMaxLen("policy_id", m.PolicyId, 128)
}
func (m *MsgUpdateMarket) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgSetMarketStatus) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if !MarketStatusValid(m.Status) {
		return ErrInvalidField.Wrap("invalid status")
	}
	return nil
}
func (m *MsgSetMarketStatus) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgMint) ValidateBasic() error {
	if err := MustBech32(m.Buyer); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.PayAmount == 0 {
		return ErrInvalidField.Wrap("pay_amount must be > 0")
	}
	return nil
}
func (m *MsgMint) GetSigners() []sdk.AccAddress { return signer(m.Buyer) }

func (m *MsgMelt) ValidateBasic() error {
	if err := MustBech32(m.Seller); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.Units == 0 {
		return ErrInvalidField.Wrap("units must be > 0")
	}
	return nil
}
func (m *MsgMelt) GetSigners() []sdk.AccAddress { return signer(m.Seller) }

func (m *MsgTransfer) ValidateBasic() error {
	if err := MustBech32(m.From); err != nil {
		return err
	}
	if err := MustBech32(m.To); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgTransfer) GetSigners() []sdk.AccAddress { return signer(m.From) }

func (m *MsgInjectTreasury) ValidateBasic() error {
	if err := MustBech32(m.Funder); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgInjectTreasury) GetSigners() []sdk.AccAddress { return signer(m.Funder) }

func (m *MsgFundReward) ValidateBasic() error {
	if err := MustBech32(m.Funder); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgFundReward) GetSigners() []sdk.AccAddress { return signer(m.Funder) }

func (m *MsgOpenInvest) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.MarketId == 0 {
		return ErrInvalidField.Wrap("market_id must be > 0")
	}
	if m.Units == 0 {
		return ErrInvalidField.Wrap("units must be > 0")
	}
	if m.TermSeconds <= 0 {
		return ErrInvalidField.Wrap("term_seconds must be > 0")
	}
	if m.ApyBps == 0 {
		return ErrInvalidField.Wrap("apy_bps must be > 0")
	}
	return nil
}
func (m *MsgOpenInvest) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgCloseInvest) ValidateBasic() error {
	if err := MustBech32(m.Caller); err != nil {
		return err
	}
	if m.InvestId == 0 {
		return ErrInvalidField.Wrap("invest_id must be > 0")
	}
	return nil
}
func (m *MsgCloseInvest) GetSigners() []sdk.AccAddress { return signer(m.Caller) }

func (m *MsgCancelInvest) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.InvestId == 0 {
		return ErrInvalidField.Wrap("invest_id must be > 0")
	}
	return nil
}
func (m *MsgCancelInvest) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
