package oracle

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions exposes oracle queries and txs through the autocli command
// tree. Resulting commands:
//
//	energychaind query oracle latest [category]
//	energychaind query oracle history [category] --from-time --to-time
//	energychaind query oracle oracle [address]
//	energychaind query oracle all-oracles
//	energychaind query oracle params
//
//	energychaind tx oracle submit-data [category] [value] [metadata] [timestamp]
//	energychaind tx oracle add-oracle-proposal [oracle-address] [name] [...categories]      # gov
//	energychaind tx oracle remove-oracle-proposal [oracle-address]                          # gov
//	energychaind tx oracle update-params-proposal [params-json]                              # gov
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.oracle.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "LatestData",
					Use:            "latest [category]",
					Short:          "Query the most recent oracle datapoint for a category",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "category"}},
				},
				{
					RpcMethod:      "DataHistory",
					Use:            "history [category]",
					Short:          "Query historical oracle datapoints for a category",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "category"}},
				},
				{
					RpcMethod:      "Oracle",
					Use:            "oracle [address]",
					Short:          "Query a single registered oracle by address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod: "AllOracles",
					Use:       "all-oracles",
					Short:     "List every registered oracle (paginated)",
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current oracle module parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.oracle.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SubmitData",
					Use:       "submit-data [category] [value] [metadata] [timestamp]",
					Short:     "Submit a new oracle datapoint",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "category"},
						{ProtoField: "value"},
						{ProtoField: "metadata"},
						{ProtoField: "timestamp"},
					},
				},
				{
					RpcMethod: "AddOracle",
					Use:       "add-oracle-proposal [oracle-address] [name] [authorized-categories...]",
					Short:     "Submit a governance proposal to register a new oracle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "oracle_address"},
						{ProtoField: "name"},
						{ProtoField: "authorized_categories", Varargs: true},
					},
					GovProposal: true,
				},
				{
					RpcMethod:      "RemoveOracle",
					Use:            "remove-oracle-proposal [oracle-address]",
					Short:          "Submit a governance proposal to remove an oracle",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "oracle_address"}},
					GovProposal:    true,
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update oracle module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
