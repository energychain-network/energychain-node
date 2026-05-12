package types

import (
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const TypeMsgRecordAudit = "record_audit"

var (
	_ sdk.Msg = &MsgRecordAudit{}
	_ sdk.Msg = &MsgUpdateParams{}
	_ sdk.Msg = &MsgRegisterSchema{}
	_ sdk.Msg = &MsgDeprecateSchema{}
	_ sdk.Msg = &MsgArchive{}
	_ sdk.Msg = &MsgGrantViewKey{}
	_ sdk.Msg = &MsgRevokeViewKey{}
)

// ---------------------------------------------------------------------------
// MsgRecordAudit
// ---------------------------------------------------------------------------

func NewMsgRecordAudit(creator, eventType, target, action, data string) *MsgRecordAudit {
	return &MsgRecordAudit{
		Creator:   creator,
		EventType: eventType,
		Target:    target,
		Action:    action,
		Data:      data,
	}
}

func (msg *MsgRecordAudit) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address: %s", err)
	}
	if msg.EventType == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("event_type cannot be empty")
	}
	if msg.Action == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("action cannot be empty")
	}
	if !SeverityValid(msg.Severity) {
		return sdkerrors.ErrInvalidRequest.Wrapf("unknown severity %d", msg.Severity)
	}
	if msg.PayloadEncrypted && msg.PayloadDigest == "" {
		// Without a digest there is no way for an off-chain decoder to
		// detect ciphertext tampering. Require the writer to commit.
		return sdkerrors.ErrInvalidRequest.Wrap("payload_encrypted requires payload_digest")
	}
	if msg.PayloadDigest != "" {
		if len(msg.PayloadDigest) != SchemaHashHexLen {
			return sdkerrors.ErrInvalidRequest.Wrapf("payload_digest must be %d hex chars", SchemaHashHexLen)
		}
		if _, err := hex.DecodeString(msg.PayloadDigest); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrap("payload_digest must be hex")
		}
	}
	return nil
}

func (msg *MsgRecordAudit) GetSigners() []sdk.AccAddress {
	creator, _ := sdk.AccAddressFromBech32(msg.Creator)
	return []sdk.AccAddress{creator}
}

// ---------------------------------------------------------------------------
// MsgUpdateParams (gov-only)
// ---------------------------------------------------------------------------

func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}
	if err := msg.Params.Validate(); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("invalid params: %s", err)
	}
	return nil
}

func (msg *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{signer}
}

// ---------------------------------------------------------------------------
// MsgRegisterSchema (gov-only)
// ---------------------------------------------------------------------------

func (msg *MsgRegisterSchema) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}
	d := msg.Descriptor_
	if d.EventType == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("descriptor.event_type required")
	}
	if d.Uri == "" || len(d.Uri) > SchemaURIMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("descriptor.uri length %d outside (0, %d]",
			len(d.Uri), SchemaURIMaxLen)
	}
	if len(d.Hash) != SchemaHashHexLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("descriptor.hash must be %d hex chars", SchemaHashHexLen)
	}
	if _, err := hex.DecodeString(d.Hash); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap("descriptor.hash must be hex")
	}
	return nil
}

func (msg *MsgRegisterSchema) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// MsgDeprecateSchema
// ---------------------------------------------------------------------------

func (msg *MsgDeprecateSchema) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}
	if msg.EventType == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("event_type required")
	}
	return nil
}

func (msg *MsgDeprecateSchema) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// MsgArchive
// ---------------------------------------------------------------------------

func (msg *MsgArchive) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}
	if msg.FromLogId == 0 || msg.ToLogId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("from_log_id and to_log_id must be > 0")
	}
	if msg.FromLogId > msg.ToLogId {
		return sdkerrors.ErrInvalidRequest.Wrap("from_log_id must be <= to_log_id")
	}
	if msg.MerkleRoot == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("merkle_root required")
	}
	if msg.Uri == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("uri required")
	}
	return nil
}

func (msg *MsgArchive) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// MsgGrantViewKey
// ---------------------------------------------------------------------------

func (msg *MsgGrantViewKey) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Grantor); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid grantor address: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Grantee); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid grantee address: %s", err)
	}
	if msg.Grantor == msg.Grantee {
		return sdkerrors.ErrInvalidRequest.Wrap("grantor and grantee must differ")
	}
	if len(msg.EncryptedKey) == 0 || len(msg.EncryptedKey) > GrantEncryptedKeyMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("encrypted_key length %d outside (0, %d]",
			len(msg.EncryptedKey), GrantEncryptedKeyMaxLen)
	}
	if len(msg.Scope.EventTypePrefix) > GrantScopePrefixMaxLen ||
		len(msg.Scope.TargetPrefix) > GrantScopePrefixMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("scope prefix exceeds %d", GrantScopePrefixMaxLen)
	}
	if msg.Scope.FromTimestamp < 0 || msg.Scope.ToTimestamp < 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("scope timestamps must be non-negative")
	}
	if msg.Scope.ToTimestamp != 0 && msg.Scope.FromTimestamp > msg.Scope.ToTimestamp {
		return sdkerrors.ErrInvalidRequest.Wrap("scope from > to")
	}
	if msg.ExpiresAt < 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("expires_at must be non-negative")
	}
	return nil
}

func (msg *MsgGrantViewKey) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(msg.Grantor)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// MsgRevokeViewKey
// ---------------------------------------------------------------------------

func (msg *MsgRevokeViewKey) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Actor); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid actor address: %s", err)
	}
	if msg.GrantId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("grant_id required")
	}
	return nil
}

func (msg *MsgRevokeViewKey) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(msg.Actor)
	return []sdk.AccAddress{a}
}
