package automation

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.automation.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Schedule", Use: "schedule [id]", Short: "Query a schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Schedules", Use: "schedules", Short: "List schedules"},
				{RpcMethod: "Stream", Use: "stream [id]", Short: "Query a stream",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Streams", Use: "streams", Short: "List streams"},
				{RpcMethod: "StreamWithdrawable", Use: "withdrawable [id]", Short: "Query a stream's withdrawable balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.automation.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CreateSchedule", Use: "create-schedule [action] [target-id] [amount-per-run] [interval-seconds] [start-time] [end-time] [max-runs]", Short: "Create a cron schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "action"}, {ProtoField: "target_id"}, {ProtoField: "amount_per_run"},
						{ProtoField: "interval_seconds"}, {ProtoField: "start_time"}, {ProtoField: "end_time"}, {ProtoField: "max_runs"}}},
				{RpcMethod: "PauseSchedule", Use: "pause-schedule [schedule-id]", Short: "Pause a schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "schedule_id"}}},
				{RpcMethod: "ResumeSchedule", Use: "resume-schedule [schedule-id]", Short: "Resume a paused schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "schedule_id"}}},
				{RpcMethod: "CancelSchedule", Use: "cancel-schedule [schedule-id]", Short: "Cancel a schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "schedule_id"}}},
				{RpcMethod: "CreateStream", Use: "create-stream [receiver] [denom] [deposit] [rate-per-sec] [start-time]", Short: "Open a streaming payment",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "receiver"}, {ProtoField: "denom"}, {ProtoField: "deposit"}, {ProtoField: "rate_per_sec"}, {ProtoField: "start_time"}}},
				{RpcMethod: "WithdrawStream", Use: "withdraw-stream [stream-id]", Short: "Withdraw vested stream funds",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}}},
				{RpcMethod: "CancelStream", Use: "cancel-stream [stream-id]", Short: "Cancel a stream",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "stream_id"}}},
			},
		},
	}
}
