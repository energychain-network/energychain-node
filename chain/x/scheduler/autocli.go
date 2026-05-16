package scheduler

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. Commands
// requiring a packed-Any payload (CreateJob / UpdateJob /
// UpdateParams) live in client/cli/tx.go.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.scheduler.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},

				{RpcMethod: "Job", Use: "job [id]", Short: "Query a job by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Jobs", Use: "jobs", Short: "List jobs"},
				{RpcMethod: "JobsByOwner", Use: "by-owner [owner]", Short: "List jobs owned by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}}},
				{RpcMethod: "DueJobs", Use: "due [cutoff-time] [limit]", Short: "List jobs due to run at or before the cutoff",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cutoff_time"}, {ProtoField: "limit"}}},

				{RpcMethod: "FeePoolBalance", Use: "fee-pool [fee-denom]", Short: "Show the scheduler fee-pool address (query stablecoin for balance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "fee_denom"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.scheduler.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "TopUpJob", Use: "topup [job-id] [amount] [resume-if-exhausted]", Short: "Top up a job's fee budget",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "job_id"}, {ProtoField: "amount"}, {ProtoField: "resume_if_exhausted"}}},
				{RpcMethod: "WithdrawJob", Use: "withdraw [job-id] [amount]", Short: "Withdraw budget from a paused / exhausted / cancelled job",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "job_id"}, {ProtoField: "amount"}}},

				{RpcMethod: "PauseJob", Use: "pause [job-id] [reason]", Short: "Pause an active job",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "job_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ResumeJob", Use: "resume [job-id]", Short: "Resume a paused / exhausted job",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "job_id"}}},
				{RpcMethod: "CancelJob", Use: "cancel [job-id] [refund] [reason]", Short: "Cancel a job",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "job_id"}, {ProtoField: "refund"}, {ProtoField: "reason"}}},
			},
		},
	}
}
