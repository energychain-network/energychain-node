package energy

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions makes the energy module discoverable by the autocli
// framework. The CLI command tree is generated from the Query and Msg
// service descriptors registered through gogo proto, so we only need to
// declare display strings, positional arg ordering, and which RPCs
// require a gov proposal.
//
// Resulting commands once wired into root:
//
//	energychaind query energy energy-data <id>
//	energychaind query energy by-category <category> [--page-key=...]
//	energychaind query energy by-submitter <submitter> [--page-key=...]
//	energychaind query energy batch <batch-id>
//	energychaind query energy params
//
//	energychaind tx energy submit <category> <data-hash> [metadata]
//	energychaind tx energy batch-submit <category> <merkle-root> --items='[...]'
//	energychaind tx energy update-params-proposal <params-json>   # gov proposal
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.energy.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "EnergyData",
					Use:            "energy-data [id]",
					Short:          "Query an energy data record by ID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "EnergyDataByCategory",
					Use:            "by-category [category]",
					Short:          "List paginated energy data records for a category",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "category"}},
				},
				{
					RpcMethod:      "EnergyDataBySubmitter",
					Use:            "by-submitter [submitter]",
					Short:          "List paginated energy data records by submitter",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "submitter"}},
				},
				{
					RpcMethod:      "Batch",
					Use:            "batch [id]",
					Short:          "Query a batch submission by ID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current energy module parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.energy.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SubmitEnergyData",
					Use:       "submit [category] [data-hash] [metadata]",
					Short:     "Submit an energy data record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "category"},
						{ProtoField: "data_hash"},
						{ProtoField: "metadata"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update energy module params (entire Params object required)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
				// BatchSubmit takes a repeated message; CLI usage requires
				// crafting JSON, so we keep it autocli-skipped and rely on
				// the dedicated CLI builder. Mark it explicitly so the
				// auto-tree does not produce a misleading partial command.
				{RpcMethod: "BatchSubmit", Skip: true},
			},
		},
	}
}
