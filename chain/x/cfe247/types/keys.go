package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "cfe247"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	HourSeconds int64 = 3600
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	DataProviderCollectionPrefix = collections.NewPrefix(0x01) // id -> DataProvider
	GridZoneCollectionPrefix     = collections.NewPrefix(0x02) // id -> GridZone
	SubjectCollectionPrefix      = collections.NewPrefix(0x03) // id -> Subject

	ConsumptionCollectionPrefix = collections.NewPrefix(0x04) // (subject_id, hour_start) -> HourlyConsumption
	AggregateCollectionPrefix   = collections.NewPrefix(0x05) // (subject_id, hour_start) -> HourlyAggregate

	MatchCollectionPrefix       = collections.NewPrefix(0x06) // id -> MatchEntry
	MatchByHourPrefix           = collections.NewPrefix(0x07) // ((subject_id, hour_start), id)
	MatchIDSeqPrefix            = collections.NewPrefix(0x08)

	AnnualScoreCollectionPrefix = collections.NewPrefix(0x09) // (subject_id, year) -> AnnualScore

	ReportCollectionPrefix      = collections.NewPrefix(0x0a) // id -> ReportPackage
	ReportBySubjectPrefix       = collections.NewPrefix(0x0b) // (subject_id, id)
	ReportIDSeqPrefix           = collections.NewPrefix(0x0c)

	// RetirementAllocationPrefix tracks how many Wh of an x/eac
	// retirement have already been spent on CFE matches. Bounded by
	// the retirement's amount.
	RetirementAllocationPrefix = collections.NewPrefix(0x0d) // retirement_id -> uint64
)
