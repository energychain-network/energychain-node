package clearing

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.clearing.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Module parameters"},
				{RpcMethod: "Member", Use: "member [id]", Short: "Show one member",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Members", Use: "members", Short: "List members"},
				{RpcMethod: "Cycle", Use: "cycle [id]", Short: "Show one cycle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Cycles", Use: "cycles", Short: "List cycles"},
				{RpcMethod: "Obligation", Use: "obligation [id]", Short: "Show one obligation",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Obligations", Use: "obligations [cycle-id]", Short: "List obligations in a cycle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}}},
				{RpcMethod: "NetPositions", Use: "net [cycle-id]", Short: "Show net positions",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}}},
				{RpcMethod: "DefaultEvents", Use: "defaults [cycle-id]", Short: "Show default events",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}}},
				{RpcMethod: "MarginBalance", Use: "margin [member-id] [denom]", Short: "Show margin balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "member_id"}, {ProtoField: "denom"}}},
				{RpcMethod: "DefaultFund", Use: "fund [denom]", Short: "Show default fund total",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.clearing.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "PostMargin", Use: "post-margin [member-id] [denom] [amount]",
					Short: "Post margin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "member_id"}, {ProtoField: "denom"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "WithdrawMargin", Use: "withdraw-margin [member-id] [denom] [amount]",
					Short: "Withdraw margin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "member_id"}, {ProtoField: "denom"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "FundDefaultFund", Use: "fund-default [member-id] [denom] [amount]",
					Short: "Contribute to default fund",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "member_id"}, {ProtoField: "denom"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "SubmitObligation",
					Use: "submit [cycle-id] [from-id] [to-id] [denom] [amount] [source-ref]",
					Short: "Submit an obligation",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "cycle_id"}, {ProtoField: "from_member_id"},
						{ProtoField: "to_member_id"}, {ProtoField: "denom"},
						{ProtoField: "amount"}, {ProtoField: "source_ref"},
					}},
				{RpcMethod: "CloseCycle", Use: "close [cycle-id]", Short: "Close a cycle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}}},
				{RpcMethod: "SettleCycle", Use: "settle [cycle-id]", Short: "Settle a cycle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}}},
				{RpcMethod: "CancelCycle", Use: "cancel [cycle-id] [reason]", Short: "Cancel a cycle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cycle_id"}, {ProtoField: "reason"}}},
			},
		},
	}
}
