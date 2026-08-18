package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "bridge"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

const (
	EventTypeChain   = "bridge_chain"
	EventTypeAsset   = "bridge_asset"
	EventTypeLock    = "bridge_lock"
	EventTypeAttest  = "bridge_attest"
	EventTypeRelease = "bridge_release"

	AttrAction    = "action"
	AttrChainID   = "chain_id"
	AttrAssetID   = "asset_id"
	AttrNonce     = "nonce"
	AttrInboundID = "inbound_id"
	AttrSender    = "sender"
	AttrRecipient = "recipient"
	AttrAmount    = "amount"
	AttrAttestor  = "attestor"
	AttrStatus    = "status"
	AttrDenom     = "denom"
)

var (
	ParamsPrefix       = collections.NewPrefix(0x00)
	ChainPrefix        = collections.NewPrefix(0x01) // id -> ExternalChain
	ChainIDSeqPrefix   = collections.NewPrefix(0x02)
	AssetPrefix        = collections.NewPrefix(0x03) // id -> Asset
	AssetByDenomPrefix = collections.NewPrefix(0x04) // denom -> id
	AssetIDSeqPrefix   = collections.NewPrefix(0x05)
	OutboundPrefix     = collections.NewPrefix(0x06) // nonce -> Outbound
	OutboundSeqPrefix  = collections.NewPrefix(0x07)
	InboundPrefix      = collections.NewPrefix(0x08) // id -> Inbound
	InboundBySrcPrefix = collections.NewPrefix(0x09) // (src_chain, src_nonce) -> inbound id
	InboundIDSeqPrefix = collections.NewPrefix(0x0A)

	// ReleasedByChainPrefix indexes RELEASED inbounds as
	// (src_chain_id, released_at, inbound_id) -> amount so the rolling
	// mint-window check only reads entries inside the window instead of
	// walking the full (ever-growing) inbound history.
	ReleasedByChainPrefix = collections.NewPrefix(0x0B)
	// MintedTotalPrefix / BurnedTotalPrefix are per-denom cumulative
	// counters (released mints / outbound burns) backing the O(1)
	// net-outstanding (EscrowBalance) query. Rebuilt from transfer history
	// at InitGenesis.
	MintedTotalPrefix = collections.NewPrefix(0x0C)
	BurnedTotalPrefix = collections.NewPrefix(0x0D)
)
