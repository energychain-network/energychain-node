package audit

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions exposes audit queries and txs.
//
//	energychaind query audit log [id]
//	energychaind query audit logs [--actor=...] [--event-type=...] [--from-timestamp=...] [--to-timestamp=...]
//	energychaind query audit params
//
//	energychaind tx audit record [event-type] [target] [action] [data]
//	energychaind tx audit update-params-proposal [params-json]   # gov
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.audit.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "AuditLog",
					Use:            "log [id]",
					Short:          "Query a single audit log by ID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "AuditLogs",
					Use:       "logs",
					Short:     "List paginated audit logs filtered by actor / event-type / time range",
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current audit module parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.audit.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "RecordAudit",
					Use:       "record [event-type] [target] [action] [data]",
					Short:     "Record an audit log entry",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "event_type"},
						{ProtoField: "target"},
						{ProtoField: "action"},
						{ProtoField: "data"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update audit module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
