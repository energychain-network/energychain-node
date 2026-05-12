package audit

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions exposes audit queries and txs.
//
//	energychaind query audit log [id]
//	energychaind query audit logs [--actor=...] [--event-type=...]
//	                              [--from-timestamp=...] [--to-timestamp=...]
//	                              [--min-severity=...] [--schema-id=...]
//	energychaind query audit params
//	energychaind query audit schema [event-type]
//	energychaind query audit schemas
//	energychaind query audit archive-segment [id]
//	energychaind query audit archive-segments
//	energychaind query audit view-key-grant [id]
//	energychaind query audit view-key-grants [--grantee=...]
//
//	energychaind tx audit record [event-type] [target] [action] [data]
//	                             [--severity=...] [--schema-id=...]
//	                             [--payload-encrypted] [--payload-digest=...]
//	energychaind tx audit deprecate-schema [event-type]
//	energychaind tx audit revoke-view-key [grant-id] [reason]
//	# Governance / operator-only commands (RegisterSchema, Archive,
//	# GrantViewKey, UpdateParams) are exposed as JSON-payload commands
//	# under client/cli/tx.go because their bodies are nested messages.
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
					Short:     "List paginated audit logs filtered by actor / event-type / time range / severity / schema",
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current audit module parameters",
				},
				{
					RpcMethod:      "Schema",
					Use:            "schema [event-type]",
					Short:          "Query a single registered schema descriptor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "event_type"}},
				},
				{
					RpcMethod: "Schemas",
					Use:       "schemas",
					Short:     "List paginated schema descriptors",
				},
				{
					RpcMethod:      "ArchiveSegment",
					Use:            "archive-segment [id]",
					Short:          "Query a single archive segment by ID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "ArchiveSegments",
					Use:       "archive-segments",
					Short:     "List paginated archive segments",
				},
				{
					RpcMethod:      "ViewKeyGrant",
					Use:            "view-key-grant [id]",
					Short:          "Query a single view-key grant by ID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "ViewKeyGrants",
					Use:       "view-key-grants",
					Short:     "List paginated view-key grants (optionally filtered by grantee)",
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
					RpcMethod:      "DeprecateSchema",
					Use:            "deprecate-schema [event-type]",
					Short:          "Mark a registered schema as deprecated (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "event_type"}},
					GovProposal:    true,
				},
				{
					RpcMethod: "RevokeViewKey",
					Use:       "revoke-view-key [grant-id] [reason]",
					Short:     "Revoke a previously issued view-key grant",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "grant_id"},
						{ProtoField: "reason"},
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
