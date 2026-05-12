package meter

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions exposes meter queries and the simpler positional tx
// commands. Messages with nested payloads (RegisterMeteringPoint,
// SubmitReading, SubmitBatch, AuthorizeStream) live in client/cli/tx.go
// because autocli cannot synthesise the bytes / enum / nested-struct
// flag combinations required.
//
//	energychaind query meter params
//	energychaind query meter metering-point [id]
//	energychaind query meter metering-points [--owner-address=... --grid-zone=... --active-only]
//	energychaind query meter reading [mp-id] [start-time]
//	energychaind query meter readings [mp-id]
//	energychaind query meter batch [id]
//	energychaind query meter batches [--metering-point-id=...]
//	energychaind query meter stream-authorization [id]
//	energychaind query meter stream-authorizations [--metering-point-id=... --active-only]
//
//	energychaind tx meter deactivate-metering-point [id] [reason]
//	energychaind tx meter revoke-stream [id] [reason]
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.meter.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query the current meter module parameters"},
				{
					RpcMethod:      "MeteringPoint",
					Use:            "metering-point [id]",
					Short:          "Query a single metering point by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "MeteringPoints", Use: "metering-points", Short: "List paginated metering points"},
				{
					RpcMethod: "Reading",
					Use:       "reading [metering-point-id] [start-time]",
					Short:     "Query a single reading by (mp, start_time)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "metering_point_id"},
						{ProtoField: "start_time"},
					},
				},
				{
					RpcMethod:      "Readings",
					Use:            "readings [metering-point-id]",
					Short:          "List readings for a metering point",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "metering_point_id"}},
				},
				{
					RpcMethod:      "Batch",
					Use:            "batch [id]",
					Short:          "Query a single batch by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Batches", Use: "batches", Short: "List batches (filter --metering-point-id=...)"},
				{
					RpcMethod:      "StreamAuthorization",
					Use:            "stream-authorization [id]",
					Short:          "Query a single stream authorization",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "StreamAuthorizations", Use: "stream-authorizations", Short: "List stream authorizations"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.meter.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "DeactivateMeteringPoint",
					Use:       "deactivate-metering-point [id] [reason]",
					Short:     "Deactivate a metering point (owner or governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "RevokeStream",
					Use:       "revoke-stream [id] [reason]",
					Short:     "Revoke a stream authorization",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update meter module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
