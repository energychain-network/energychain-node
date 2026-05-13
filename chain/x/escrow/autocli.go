package escrow

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions exposes the simple positional commands. Commands
// requiring multi-flag JSON payloads (CreateEscrow / UpdateParams)
// live in client/cli/tx.go.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.escrow.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},

				{RpcMethod: "Escrow", Use: "escrow [id]", Short: "Query an escrow by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Escrows", Use: "escrows", Short: "List escrows"},
				{RpcMethod: "EscrowsByDepositor", Use: "by-depositor [depositor]", Short: "List escrows opened by a depositor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "depositor"}}},
				{RpcMethod: "EscrowsByBeneficiary", Use: "by-beneficiary [beneficiary]", Short: "List escrows targeting a beneficiary",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "beneficiary"}}},

				{RpcMethod: "ApprovalsForEscrow", Use: "approvals [escrow-id]", Short: "List per-signer approvals for an escrow",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}}},
				{RpcMethod: "Approval", Use: "approval [escrow-id] [signer]", Short: "Query a single signer's approval row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "signer"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.escrow.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Fund", Use: "fund [escrow-id]", Short: "Fund a draft escrow (depositor signer)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}}},
				{RpcMethod: "Cancel", Use: "cancel [escrow-id] [reason]", Short: "Cancel a draft escrow (depositor signer)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "reason"}}},

				{RpcMethod: "Release", Use: "release [escrow-id]", Short: "Release a funded escrow once thresholds met",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}}},
				{RpcMethod: "Refund", Use: "refund [escrow-id]", Short: "Refund a funded escrow once thresholds met",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}}},
				{RpcMethod: "RevokeApproval", Use: "revoke-approval [escrow-id]", Short: "Revoke your prior approval",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}}},

				{RpcMethod: "ArbiterRelease", Use: "arbiter-release [escrow-id] [reason]", Short: "Force release as arbiter",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ArbiterRefund", Use: "arbiter-refund [escrow-id] [reason]", Short: "Force refund as arbiter",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "reason"}}},

				{RpcMethod: "MarkDisputed", Use: "mark-disputed [escrow-id] [reason]", Short: "Mark an escrow disputed (party signer)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ResolveDispute", Use: "resolve-dispute [escrow-id] [release] [reason]", Short: "Resolve a dispute (arbiter)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "escrow_id"}, {ProtoField: "release"}, {ProtoField: "reason"}}},
			},
		},
	}
}
