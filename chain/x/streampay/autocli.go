package streampay

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires positional / simple commands. MsgCreateStream
// and MsgUpdateParams remain available via the standard
// auto-generated path; they are uncomplicated proto bodies so no
// custom Cobra command is needed (callers can also pass a JSON
// payload via `tx ... --generate-only` if desired).
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.streampay.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Stream", Use: "stream [id]", Short: "Query a stream by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Streams", Use: "streams", Short: "List all streams"},
				{RpcMethod: "StreamsBySender", Use: "by-sender [sender]", Short: "List streams sent by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "sender"}}},
				{RpcMethod: "StreamsByReceiver", Use: "by-receiver [receiver]", Short: "List streams received by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "receiver"}}},
				{RpcMethod: "Withdrawable", Use: "withdrawable [stream-id] [at-time]", Short: "Live accrued amount (off-chain helper)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "at_time"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the streampay pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.streampay.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CreateStream", Use: "create [receiver] [denom] [rate-per-second] [deposit] [start-time] [end-time] [memo]", Short: "Open a new stream",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "receiver"}, {ProtoField: "denom"}, {ProtoField: "rate_per_second"},
						{ProtoField: "deposit"}, {ProtoField: "start_time"}, {ProtoField: "end_time"}, {ProtoField: "memo"},
					}},
				{RpcMethod: "DepositToStream", Use: "deposit [stream-id] [amount]", Short: "Top up a stream's deposit",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "Withdraw", Use: "withdraw [stream-id] [amount]", Short: "Withdraw accrued (amount=0 for all)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "Pause", Use: "pause [stream-id] [reason]", Short: "Pause a stream (sender only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "Resume", Use: "resume [stream-id]", Short: "Resume a paused stream (sender only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}}},
				{RpcMethod: "Cancel", Use: "cancel [stream-id] [reason]", Short: "Cancel a stream (sender or receiver)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "Transfer", Use: "transfer [stream-id] [new-receiver]", Short: "Transfer receivership (current receiver only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "new_receiver"}}},
				{RpcMethod: "ChangeRate", Use: "change-rate [stream-id] [new-rate]", Short: "Settle then change the per-second rate (sender only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}, {ProtoField: "new_rate_per_second"}}},
			},
		},
	}
}
