package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (m *MsgCreateContract) ValidateBasic() error {
	if err := ValidateAddr("drafter", m.Drafter); err != nil {
		return err
	}
	if err := ValidateAddr("buyer", m.Buyer); err != nil {
		return err
	}
	if err := ValidateAddr("seller", m.Seller); err != nil {
		return err
	}
	if m.Buyer == m.Seller {
		return fmt.Errorf("buyer and seller must differ")
	}
	if m.Drafter != m.Buyer && m.Drafter != m.Seller {
		return fmt.Errorf("drafter must be buyer or seller")
	}
	if !KindValid(m.Kind) {
		return fmt.Errorf("invalid kind")
	}
	if err := ValidateDenom(m.AssetDenom); err != nil {
		return err
	}
	if m.StrikePrice == 0 {
		return fmt.Errorf("strike_price must be > 0")
	}
	if m.NotionalQuantity == 0 {
		return fmt.Errorf("notional_quantity must be > 0")
	}
	if err := ValidateTopic(m.PriceOracleTopic); err != nil {
		return err
	}
	if err := ValidateTopic(m.QuantityOracleTopic); err != nil {
		return err
	}
	if m.MaxOracleStalenessSeconds < 0 {
		return fmt.Errorf("max_oracle_staleness_seconds cannot be negative")
	}
	if err := ValidateURI(m.SchemaUri); err != nil {
		return err
	}
	if err := ValidateHash(m.SchemaHash); err != nil {
		return err
	}
	if m.SettlementPeriodSeconds <= 0 {
		return fmt.Errorf("settlement_period_seconds must be > 0")
	}
	if m.MarginRequirement == 0 {
		return fmt.Errorf("margin_requirement must be > 0")
	}
	if m.StartTime < 0 || m.EndTime < 0 {
		return fmt.Errorf("time fields cannot be negative")
	}
	if m.GracePeriodSeconds < 0 {
		return fmt.Errorf("grace_period_seconds cannot be negative")
	}
	// Kind-specific shape checks: VPPA / CFD require a price
	// oracle topic; PPA may use a fixed notional_quantity (no
	// quantity_oracle_topic) but still benefits from one.
	switch m.Kind {
	case Kind_KIND_VPPA, Kind_KIND_CFD:
		if m.PriceOracleTopic == "" {
			return fmt.Errorf("price_oracle_topic required for VPPA / CFD")
		}
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgCreateContract) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Drafter)
	return []sdk.AccAddress{a}
}

func (m *MsgDepositMargin) ValidateBasic() error {
	if err := ValidateAddr("party", m.Party); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgDepositMargin) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Party)
	return []sdk.AccAddress{a}
}

func (m *MsgWithdrawMargin) ValidateBasic() error {
	if err := ValidateAddr("party", m.Party); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return nil
}
func (m *MsgWithdrawMargin) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Party)
	return []sdk.AccAddress{a}
}

func (m *MsgSign) ValidateBasic() error {
	if err := ValidateAddr("party", m.Party); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return nil
}
func (m *MsgSign) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Party)
	return []sdk.AccAddress{a}
}

func (m *MsgRevoke) ValidateBasic() error {
	if err := ValidateAddr("party", m.Party); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgRevoke) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Party)
	return []sdk.AccAddress{a}
}

func (m *MsgSettle) ValidateBasic() error {
	if err := ValidateAddr("caller", m.Caller); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return nil
}
func (m *MsgSettle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Caller)
	return []sdk.AccAddress{a}
}

func (m *MsgDefault) ValidateBasic() error {
	if err := ValidateAddr("caller", m.Caller); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgDefault) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Caller)
	return []sdk.AccAddress{a}
}

func (m *MsgTerminate) ValidateBasic() error {
	if err := ValidateAddr("party", m.Party); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgTerminate) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Party)
	return []sdk.AccAddress{a}
}

func (m *MsgMarkDisputed) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgMarkDisputed) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgResolveDispute) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ContractId == 0 {
		return fmt.Errorf("contract_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgResolveDispute) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
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
