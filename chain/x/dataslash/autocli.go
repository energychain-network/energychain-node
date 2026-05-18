package dataslash

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.dataslash.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Module parameters"},
				{RpcMethod: "Provider", Use: "provider [id]", Short: "Show one provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Providers", Use: "providers", Short: "List providers (optional status / role filter)"},
				{RpcMethod: "ProviderBySigner", Use: "provider-by-signer [signer]", Short: "Lookup provider by signer address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "signer_address"}}},
				{RpcMethod: "Infraction", Use: "infraction [id]", Short: "Show one infraction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Infractions", Use: "infractions", Short: "List infractions (optional provider_id / kind filter)"},
				{RpcMethod: "JailRecords", Use: "jail-records [provider-id]", Short: "List jail records for a provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "provider_id"}}},
				{RpcMethod: "BanRecords", Use: "ban-records [provider-id]", Short: "List ban records for a provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "provider_id"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the dataslash bond pool address"},
				{RpcMethod: "IsActive", Use: "is-active [signer]", Short: "Probe whether a signer can supply data",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "signer_address"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.dataslash.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "PostBond", Use: "post-bond [provider-id] [amount]",
					Short: "Post additional bond against a provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "IncreaseBond", Use: "increase-bond [provider-id] [amount]",
					Short: "Top up a provider's bond",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"}, {ProtoField: "amount"},
					}},
				{RpcMethod: "RequestUnbond", Use: "request-unbond [provider-id]",
					Short: "Start the unbond cool-down",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"},
					}},
				{RpcMethod: "WithdrawUnbonded", Use: "withdraw [provider-id]",
					Short: "Withdraw bond after cool-down",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"},
					}},
				{RpcMethod: "UnjailProvider", Use: "unjail [provider-id] [reason]",
					Short: "Authority: clear an active jail",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "BanProvider", Use: "ban [provider-id] [reason]",
					Short: "Authority: permanently ban a provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "provider_id"}, {ProtoField: "reason"},
					}},
			},
		},
	}
}
