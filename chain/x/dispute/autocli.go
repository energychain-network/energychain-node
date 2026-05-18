package dispute

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.dispute.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Module parameters"},
				{RpcMethod: "Arbitrator", Use: "arbitrator [id]", Short: "Show one arbitrator",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Arbitrators", Use: "arbitrators", Short: "List arbitrators"},
				{RpcMethod: "Dispute", Use: "dispute [id]", Short: "Show one dispute",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Disputes", Use: "disputes", Short: "List disputes"},
				{RpcMethod: "Tribunal", Use: "tribunal [dispute-id]", Short: "List tribunal members",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "dispute_id"}}},
				{RpcMethod: "Votes", Use: "votes [dispute-id]", Short: "List votes",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "dispute_id"}}},
				{RpcMethod: "Evidence", Use: "evidence [dispute-id]", Short: "List evidence",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "dispute_id"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the dispute bond pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.dispute.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateArbitratorStatus", Use: "update-arbitrator-status [id] [new-status] [reason]",
					Short: "Update an arbitrator's status (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "arbitrator_id"}, {ProtoField: "new_status"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "Respond", Use: "respond [dispute-id] [bond]",
					Short: "Respond to a dispute and post the respondent bond",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "dispute_id"}, {ProtoField: "bond"},
					}},
				{RpcMethod: "CancelDispute", Use: "cancel [dispute-id] [reason]",
					Short: "Cancel a dispute (plaintiff while OPEN, or authority any time)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "dispute_id"}, {ProtoField: "reason"},
					}},
			},
		},
	}
}
