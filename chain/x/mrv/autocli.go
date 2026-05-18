package mrv

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.mrv.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Module parameters"},
				{RpcMethod: "Schema", Use: "schema [id]", Short: "Show one schema",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Schemas", Use: "schemas", Short: "List schemas"},
				{RpcMethod: "Verifier", Use: "verifier [id]", Short: "Show one verifier",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Verifiers", Use: "verifiers", Short: "List verifiers"},
				{RpcMethod: "Report", Use: "report [id]", Short: "Show one report",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Reports", Use: "reports", Short: "List reports"},
				{RpcMethod: "ViewKeyGrant", Use: "grant [id]", Short: "Show one grant",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "ViewKeyGrants", Use: "grants", Short: "List grants"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.mrv.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateSchemaStatus", Use: "update-schema-status [id] [new-status] [reason]",
					Short: "Update a schema's status (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "schema_id"}, {ProtoField: "new_status"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "UpdateVerifierStatus", Use: "update-verifier-status [id] [new-status] [reason]",
					Short: "Update a verifier's status (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "verifier_id"}, {ProtoField: "new_status"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "RetractReport", Use: "retract-report [id] [reason]",
					Short: "Retract a report (subject or authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "report_id"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "RevokeViewKey", Use: "revoke-grant [id] [reason]",
					Short: "Revoke a view-key grant",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "grant_id"}, {ProtoField: "reason"},
					}},
			},
		},
	}
}
