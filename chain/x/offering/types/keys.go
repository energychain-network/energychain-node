package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "offering"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

const (
	EventTypeOffering     = "offering"
	EventTypeSubscription = "offering_subscription"
	EventTypeTranche      = "offering_tranche"
	EventTypeReturn       = "offering_return"
	EventTypeRefund       = "offering_refund"

	AttrAction     = "action"
	AttrOfferingID = "offering_id"
	AttrInvestor   = "investor"
	AttrIssuer     = "issuer"
	AttrAmount     = "amount"
	AttrUnits      = "units"
	AttrStatus     = "status"
	AttrReason     = "reason"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)
	OfferingPrefix         = collections.NewPrefix(0x01) // id -> Offering
	OfferingIDSeqPrefix    = collections.NewPrefix(0x02)
	SubscriptionPrefix     = collections.NewPrefix(0x03) // (offeringID, investor) -> Subscription
)
