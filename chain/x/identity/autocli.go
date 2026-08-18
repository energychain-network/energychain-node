package identity

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the read-only query commands and the simple
// positional tx commands. Multi-flag policy payloads can be sent via
// the generic `tx identity <msg>` JSON path.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.identity.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Account", Use: "account [address]", Short: "Query an identity record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}}},
				{RpcMethod: "Accounts", Use: "accounts", Short: "List identity records"},
				{RpcMethod: "Registrars", Use: "registrars", Short: "List registrars"},
				{RpcMethod: "Sanctioned", Use: "sanctioned [address]", Short: "Check if an address is sanctioned",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}}},
				{RpcMethod: "Sanctions", Use: "sanctions", Short: "List sanctioned addresses"},
				{RpcMethod: "Policy", Use: "policy [id]", Short: "Query a transfer policy",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Policies", Use: "policies", Short: "List transfer policies"},
				{RpcMethod: "EvaluateTransfer", Use: "evaluate [policy-id] [from] [to] [amount]", Short: "Dry-run the compliance gate",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "policy_id"}, {ProtoField: "from"}, {ProtoField: "to"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "AuditEntries", Use: "audit", Short: "List audit entries"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.identity.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "PausePolicy", Use: "pause-policy [id] [paused]", Short: "Pause or unpause a policy",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}, {ProtoField: "paused"}}},
				{RpcMethod: "DeletePolicy", Use: "delete-policy [id]", Short: "Delete a policy",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "RemoveSanction", Use: "remove-sanction [address]", Short: "Remove a sanctions entry",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}}},
				{RpcMethod: "RemoveRegistrar", Use: "remove-registrar [registrar]", Short: "Remove a registrar",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "registrar"}}},
			},
		},
	}
}
