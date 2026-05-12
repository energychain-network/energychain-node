package oracle

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions exposes oracle queries and the simpler positional tx
// commands. Messages with nested payloads (RegisterTopic, UpdateTopic,
// RegisterProvider, SubmitReserveAttestation) live in client/cli/tx.go.
//
//	energychaind query oracle topic [id]
//	energychaind query oracle topics
//	energychaind query oracle provider [address]
//	energychaind query oracle providers [--status=ACTIVE]
//	energychaind query oracle submission [topic-id] [provider]
//	energychaind query oracle submissions [--topic-id=...]
//	energychaind query oracle aggregated [topic-id]
//	energychaind query oracle all-aggregated
//	energychaind query oracle reserve-attestation [id]
//	energychaind query oracle reserve-attestations [--asset=USDC]
//	energychaind query oracle params
//
//	energychaind tx oracle pause-topic [topic-id] [reason]
//	energychaind tx oracle resume-topic [topic-id]
//	energychaind tx oracle suspend-provider [address] [until-unix] [reason]
//	energychaind tx oracle request-withdraw-bond
//	energychaind tx oracle withdraw-bond
//	energychaind tx oracle submit-value [topic-id] [value] [timestamp] [metadata]
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.oracle.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "Topic",
					Use:            "topic [id]",
					Short:          "Query a topic by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Topics", Use: "topics", Short: "List paginated topics"},
				{
					RpcMethod:      "Provider",
					Use:            "provider [address]",
					Short:          "Query a provider by address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{RpcMethod: "Providers", Use: "providers", Short: "List paginated providers (filter --status=...)"},
				{
					RpcMethod: "Submission",
					Use:       "submission [topic-id] [provider]",
					Short:     "Query a single submission",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "topic_id"},
						{ProtoField: "provider"},
					},
				},
				{RpcMethod: "Submissions", Use: "submissions", Short: "List submissions (filter --topic-id=...)"},
				{
					RpcMethod:      "Aggregated",
					Use:            "aggregated [topic-id]",
					Short:          "Query the latest aggregated value for a topic",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "topic_id"}},
				},
				{RpcMethod: "AllAggregated", Use: "all-aggregated", Short: "List paginated aggregated values"},
				{
					RpcMethod:      "ReserveAttestation",
					Use:            "reserve-attestation [id]",
					Short:          "Query a single reserve attestation",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "ReserveAttestations", Use: "reserve-attestations", Short: "List reserve attestations (filter --asset=...)"},
				{RpcMethod: "Params", Use: "params", Short: "Query the current oracle module parameters"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.oracle.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "PauseTopic",
					Use:       "pause-topic [topic-id] [reason]",
					Short:     "Pause a topic (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "topic_id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod:      "ResumeTopic",
					Use:            "resume-topic [topic-id]",
					Short:          "Resume a paused topic (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "topic_id"}},
					GovProposal:    true,
				},
				{
					RpcMethod: "SuspendProvider",
					Use:       "suspend-provider [address] [until-unix] [reason]",
					Short:     "Suspend a provider until the given unix time (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
						{ProtoField: "until"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "RequestWithdrawBond",
					Use:       "request-withdraw-bond",
					Short:     "Begin the bond withdrawal cooldown",
				},
				{
					RpcMethod: "WithdrawBond",
					Use:       "withdraw-bond",
					Short:     "Complete bond withdrawal once cooldown elapses",
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update oracle module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
