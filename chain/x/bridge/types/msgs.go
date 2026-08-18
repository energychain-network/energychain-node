package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

func (m *MsgRegisterChain) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateMaxLen("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if err := ValidateMaxLen("chain_ref", m.ChainRef, ChainRefMaxLen); err != nil {
		return err
	}
	return ValidateAttestorSet(m.Attestors, m.Threshold, HardMaxAttestor)
}
func (m *MsgRegisterChain) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgUpdateChain) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if m.ChainId == 0 {
		return ErrInvalidField.Wrap("chain_id must be > 0")
	}
	return ValidateAttestorSet(m.Attestors, m.Threshold, HardMaxAttestor)
}
func (m *MsgUpdateChain) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgSetChainStatus) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if m.ChainId == 0 {
		return ErrInvalidField.Wrap("chain_id must be > 0")
	}
	if !ChainStatusValid(m.Status) {
		return ErrInvalidField.Wrap("invalid chain status")
	}
	return nil
}
func (m *MsgSetChainStatus) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgRegisterAsset) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return ValidateDenom(m.Denom)
}
func (m *MsgRegisterAsset) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgSetAssetStatus) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return ErrInvalidField.Wrap("asset_id must be > 0")
	}
	if !AssetStatusValid(m.Status) {
		return ErrInvalidField.Wrap("invalid asset status")
	}
	return nil
}
func (m *MsgSetAssetStatus) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgLock) ValidateBasic() error {
	if err := MustBech32(m.Sender); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return ErrInvalidField.Wrap("asset_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	if m.DestChainId == 0 {
		return ErrInvalidField.Wrap("dest_chain_id must be > 0")
	}
	return ValidateMaxLen("dest_addr", m.DestAddr, AddrMaxLen)
}
func (m *MsgLock) GetSigners() []sdk.AccAddress { return signer(m.Sender) }

func (m *MsgAttest) ValidateBasic() error {
	if err := MustBech32(m.Attestor); err != nil {
		return err
	}
	if err := MustBech32(m.Recipient); err != nil {
		return err
	}
	if m.SrcChainId == 0 {
		return ErrInvalidField.Wrap("src_chain_id must be > 0")
	}
	if m.AssetId == 0 {
		return ErrInvalidField.Wrap("asset_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgAttest) GetSigners() []sdk.AccAddress { return signer(m.Attestor) }

func (m *MsgRelease) ValidateBasic() error {
	if err := MustBech32(m.Caller); err != nil {
		return err
	}
	if m.InboundId == 0 {
		return ErrInvalidField.Wrap("inbound_id must be > 0")
	}
	return nil
}
func (m *MsgRelease) GetSigners() []sdk.AccAddress { return signer(m.Caller) }

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
