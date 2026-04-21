package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgSubmitEnergyData{}
	_ sdk.Msg = &MsgBatchSubmit{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---------------------------------------------------------------------------
// MsgSubmitEnergyData
// ---------------------------------------------------------------------------

func NewMsgSubmitEnergyData(submitter, category, dataHash, metadata string) *MsgSubmitEnergyData {
	return &MsgSubmitEnergyData{
		Submitter: submitter,
		Category:  category,
		DataHash:  dataHash,
		Metadata:  metadata,
	}
}

func (msg MsgSubmitEnergyData) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if msg.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	if msg.DataHash == "" {
		return fmt.Errorf("data hash cannot be empty")
	}
	return nil
}

func (msg MsgSubmitEnergyData) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Submitter)
	return []sdk.AccAddress{signer}
}

// ---------------------------------------------------------------------------
// MsgBatchSubmit
// ---------------------------------------------------------------------------

func NewMsgBatchSubmit(submitter, category string, items []BatchItem, merkleRoot string) *MsgBatchSubmit {
	return &MsgBatchSubmit{
		Submitter:  submitter,
		Category:   category,
		Items:      items,
		MerkleRoot: merkleRoot,
	}
}

func (msg MsgBatchSubmit) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if msg.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	if len(msg.Items) == 0 {
		return fmt.Errorf("batch must contain at least one item")
	}
	if msg.MerkleRoot == "" {
		return fmt.Errorf("merkle root cannot be empty")
	}
	for i, item := range msg.Items {
		if item.DataHash == "" {
			return fmt.Errorf("item %d: data hash cannot be empty", i)
		}
	}
	return nil
}

func (msg MsgBatchSubmit) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Submitter)
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
