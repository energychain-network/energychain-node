package assethub

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires read-only query commands plus the simple positional
// tx commands. Richer payloads can be sent via the generic
// `tx assethub <msg>` JSON path.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.assethub.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Provider", Use: "provider [address]", Short: "Query a provider",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}}},
				{RpcMethod: "Providers", Use: "providers", Short: "List providers"},
				{RpcMethod: "Device", Use: "device [id]", Short: "Query a device",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Devices", Use: "devices", Short: "List devices"},
				{RpcMethod: "DevicesByOperator", Use: "devices-by-operator [operator]", Short: "List an operator's devices",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "operator"}}},
				{RpcMethod: "Reading", Use: "reading [id]", Short: "Query a metering reading",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "ReadingsByDevice", Use: "readings-by-device [device-id]", Short: "List a device's readings",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "device_id"}}},
				{RpcMethod: "Topic", Use: "topic [id]", Short: "Query an oracle topic",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Topics", Use: "topics", Short: "List oracle topics"},
				{RpcMethod: "Submission", Use: "submission [topic-id] [provider]", Short: "Query an oracle submission",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "topic_id"}, {ProtoField: "provider"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.assethub.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "IncreaseBond", Use: "increase-bond [amount]", Short: "Increase provider bond",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}}},
				{RpcMethod: "WithdrawBond", Use: "withdraw-bond [amount]", Short: "Withdraw provider bond",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}}},
				{RpcMethod: "DeregisterProvider", Use: "deregister-provider", Short: "Deregister and refund bond"},
				{RpcMethod: "RevokeDevice", Use: "revoke-device [id] [reason]", Short: "Revoke a device",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}, {ProtoField: "reason"}}},
				{RpcMethod: "SubmitValue", Use: "submit-value [topic-id] [value]", Short: "Submit an oracle value",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "topic_id"}, {ProtoField: "value"}}},
			},
		},
	}
}
