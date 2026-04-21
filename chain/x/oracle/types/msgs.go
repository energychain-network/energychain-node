package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgSubmitData{}
	_ sdk.Msg = &MsgAddOracle{}
	_ sdk.Msg = &MsgRemoveOracle{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---------------------------------------------------------------------------
// MsgSubmitData
// ---------------------------------------------------------------------------

func NewMsgSubmitData(submitter, category, value, metadata string, timestamp int64) *MsgSubmitData {
	return &MsgSubmitData{
		Submitter: submitter,
		Category:  category,
		Value:     value,
		Metadata:  metadata,
		Timestamp: timestamp,
	}
}

func (msg MsgSubmitData) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if msg.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	if msg.Value == "" {
		return fmt.Errorf("value cannot be empty")
	}
	if msg.Timestamp <= 0 {
		return fmt.Errorf("timestamp must be positive")
	}
	return nil
}

func (msg MsgSubmitData) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Submitter)
	return []sdk.AccAddress{signer}
}

// ---------------------------------------------------------------------------
// MsgAddOracle
// ---------------------------------------------------------------------------

func NewMsgAddOracle(authority, oracleAddress, name string, categories []string) *MsgAddOracle {
	return &MsgAddOracle{
		Authority:            authority,
		OracleAddress:        oracleAddress,
		Name:                 name,
		AuthorizedCategories: categories,
	}
}

func (msg MsgAddOracle) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.OracleAddress); err != nil {
		return fmt.Errorf("invalid oracle address: %w", err)
	}
	if msg.Name == "" {
		return fmt.Errorf("oracle name cannot be empty")
	}
	return nil
}

func (msg MsgAddOracle) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{signer}
}

// ---------------------------------------------------------------------------
// MsgRemoveOracle
// ---------------------------------------------------------------------------

func NewMsgRemoveOracle(authority, oracleAddress string) *MsgRemoveOracle {
	return &MsgRemoveOracle{
		Authority:     authority,
		OracleAddress: oracleAddress,
	}
}

func (msg MsgRemoveOracle) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.OracleAddress); err != nil {
		return fmt.Errorf("invalid oracle address: %w", err)
	}
	return nil
}

func (msg MsgRemoveOracle) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{signer}
}

// ---------------------------------------------------------------------------
// MsgUpdateParams (gov-only)
// ---------------------------------------------------------------------------

func (msg MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if err := msg.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func (msg MsgUpdateParams) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{signer}
}
