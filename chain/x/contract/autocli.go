package contract

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple, single-row positional
// commands. MsgCreateContract has too many fields for a sane
// positional CLI and is implemented as a custom Cobra command
// in client/cli/tx.go. MsgUpdateParams takes a params JSON
// blob and is also implemented as a custom Cobra command.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.contract.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Contract", Use: "contract [id]", Short: "Show one contract",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Contracts", Use: "list", Short: "List all contracts"},
				{RpcMethod: "ContractsByParty", Use: "by-party [party]", Short: "List contracts where party is buyer or seller",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "party"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the contract margin pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.contract.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "DepositMargin", Use: "deposit-margin [contract-id] [amount]", Short: "Deposit margin into a contract",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "WithdrawMargin", Use: "withdraw-margin [contract-id] [amount]", Short: "Withdraw margin (amount=0 for all withdrawable)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "Sign", Use: "sign [contract-id]", Short: "Sign a draft contract",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}}},
				{RpcMethod: "Revoke", Use: "revoke [contract-id] [reason]", Short: "Revoke your signature on a draft",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "Settle", Use: "settle [contract-id]", Short: "Trigger a settlement (anyone)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}}},
				{RpcMethod: "Default", Use: "default [contract-id] [reason]", Short: "Mark an overdue contract as DEFAULTED",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "Terminate", Use: "terminate [contract-id] [reason]", Short: "Signal mutual termination",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "MarkDisputed", Use: "mark-disputed [contract-id] [reason]", Short: "Mark a contract as disputed (authority only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ResolveDispute", Use: "resolve-dispute [contract-id] [reason]", Short: "Resolve dispute (authority only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "contract_id"}, {ProtoField: "reason"}}},
			},
		},
	}
}
