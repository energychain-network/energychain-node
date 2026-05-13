package cfe247

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.cfe247.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Module params"},
				{RpcMethod: "DataProvider", Use: "data-provider [id]", Short: "Get a data provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "DataProviders", Use: "data-providers", Short: "List data providers"},
				{RpcMethod: "GridZone", Use: "grid-zone [id]", Short: "Get a grid zone",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "GridZones", Use: "grid-zones", Short: "List grid zones"},
				{RpcMethod: "Subject", Use: "subject [id]", Short: "Get a subject",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Subjects", Use: "subjects", Short: "List subjects"},
				{RpcMethod: "HourlyConsumption", Use: "consumption [subject-id] [hour-start]", Short: "Hourly consumption row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "subject_id"}, {ProtoField: "hour_start"},
					}},
				{RpcMethod: "HourlyAggregate", Use: "aggregate [subject-id] [hour-start]", Short: "Per-hour aggregate",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "subject_id"}, {ProtoField: "hour_start"},
					}},
				{RpcMethod: "MatchEntry", Use: "match [id]", Short: "Match entry by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "MatchesByHour", Use: "matches-by-hour [subject-id] [hour-start]", Short: "Matches for one hour",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "subject_id"}, {ProtoField: "hour_start"},
					}},
				{RpcMethod: "AnnualScore", Use: "annual-score [subject-id] [year]", Short: "Per-year roll-up",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "subject_id"}, {ProtoField: "year"},
					}},
				{RpcMethod: "Report", Use: "report [id]", Short: "Report by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "ReportsBySubject", Use: "reports-by-subject [subject-id]", Short: "Reports for one subject",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject_id"}}},
				{RpcMethod: "RetirementAllocated", Use: "retirement-allocated [eac-retirement-id]",
					Short:          "Wh of an x/eac retirement already spent on CFE matches",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "eac_retirement_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.cfe247.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "SuspendDataProvider", Use: "suspend-data-provider [id] [reason]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					}, GovProposal: true},
				{RpcMethod: "RevokeDataProvider", Use: "revoke-data-provider [id] [reason]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					}, GovProposal: true},
				{RpcMethod: "RemoveGridZone", Use: "remove-grid-zone [id]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
					GovProposal:    true},
				{RpcMethod: "RegisterSubject",
					Use: "register-subject [id] [display-name] [admin] [default-grid-zone]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "display_name"},
						{ProtoField: "admin"}, {ProtoField: "default_grid_zone"},
					}},
				{RpcMethod: "DeactivateSubject", Use: "deactivate-subject [id] [reason]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "AttestHourlyConsumption",
					Use: "attest-consumption [data-provider-id] [subject-id] [grid-zone] [hour-start] [wh-consumed] [meter-batch-id]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "data_provider_id"}, {ProtoField: "subject_id"},
						{ProtoField: "grid_zone"}, {ProtoField: "hour_start"},
						{ProtoField: "wh_consumed"}, {ProtoField: "meter_batch_id"},
					}},
				{RpcMethod: "AllocateMatch",
					Use: "allocate-match [subject-id] [hour-start] [eac-retirement-id] [wh-matched]",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "subject_id"}, {ProtoField: "hour_start"},
						{ProtoField: "eac_retirement_id"}, {ProtoField: "wh_matched"},
					}},
			},
		},
	}
}
