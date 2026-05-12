package policy

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions exposes policy queries and the simpler positional tx
// commands. Messages with nested rule payloads (RegisterPolicy,
// UpdatePolicy) live in client/cli/tx.go because autocli cannot
// synthesise the map<string,string>-of-rules flag combinations they
// need.
//
//	energychaind query policy params
//	energychaind query policy policy [id]
//	energychaind query policy policies [--status=ACTIVE]
//	energychaind query policy binding [asset-class] [asset-id]
//	energychaind query policy bindings [--policy-id=... --asset-class=...]
//	energychaind query policy evaluation-logs [--policy-id=... --asset-class=...]
//	energychaind query policy evaluate [asset-class] [asset-id] [sender] [receiver] [amount]
//
//	energychaind tx policy bind-policy [asset-class] [asset-id] [policy-id]
//	energychaind tx policy unbind-policy [asset-class] [asset-id] [reason]
//	energychaind tx policy deprecate-policy [id] [reason]
//	energychaind tx policy disable-policy [id] [reason]
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.policy.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query the current policy module parameters"},
				{
					RpcMethod:      "Policy",
					Use:            "policy [id]",
					Short:          "Query a single policy by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Policies", Use: "policies", Short: "List paginated policies (filter --status=...)"},
				{
					RpcMethod: "Binding",
					Use:       "binding [asset-class] [asset-id]",
					Short:     "Query a single policy binding",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_class"},
						{ProtoField: "asset_id"},
					},
				},
				{RpcMethod: "Bindings", Use: "bindings", Short: "List bindings (filter --policy-id=... --asset-class=...)"},
				{RpcMethod: "EvaluationLogs", Use: "evaluation-logs", Short: "List denial logs (filter --policy-id=... --asset-class=...)"},
				{
					RpcMethod: "Evaluate",
					Use:       "evaluate [asset-class] [asset-id] [sender] [receiver] [amount]",
					Short:     "Dry-run evaluate a transfer against the bound policy",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_class"},
						{ProtoField: "asset_id"},
						{ProtoField: "sender"},
						{ProtoField: "receiver"},
						{ProtoField: "amount"},
					},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.policy.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "BindPolicy",
					Use:       "bind-policy [asset-class] [asset-id] [policy-id]",
					Short:     "Bind a policy to an asset (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_class"},
						{ProtoField: "asset_id"},
						{ProtoField: "policy_id"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "UnbindPolicy",
					Use:       "unbind-policy [asset-class] [asset-id] [reason]",
					Short:     "Unbind a policy from an asset (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_class"},
						{ProtoField: "asset_id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "DeprecatePolicy",
					Use:       "deprecate-policy [id] [reason]",
					Short:     "Mark a policy as DEPRECATED (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "DisablePolicy",
					Use:       "disable-policy [id] [reason]",
					Short:     "Mark a policy as DISABLED — bound assets fail closed (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update policy module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
