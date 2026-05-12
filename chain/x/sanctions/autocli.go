package sanctions

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. Messages with
// nested SanctionEntry payloads (AddEntry, ProposeDelta) live in
// client/cli/tx.go because autocli cannot synthesise the multi-flag
// combinations they need.
//
//	energychaind query sanctions params
//	energychaind query sanctions list [id]
//	energychaind query sanctions lists [--status=ACTIVE]
//	energychaind query sanctions entry [list-id] [subject]
//	energychaind query sanctions entries [list-id] [--status=ACTIVE]
//	energychaind query sanctions is-sanctioned [subject]
//	energychaind query sanctions subject-listings [subject]
//	energychaind query sanctions proposal [proposal-id]
//	energychaind query sanctions proposals [--list-id=... --status=...]
//	energychaind query sanctions hits [--subject=... --list-id=...]
//
//	energychaind tx sanctions deprecate-list [id] [reason]
//	energychaind tx sanctions archive-list [id] [reason]
//	energychaind tx sanctions remove-entry [list-id] [subject] [reason]
//	energychaind tx sanctions confirm-proposal [proposal-id]
//	energychaind tx sanctions reject-proposal [proposal-id] [reason]
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.sanctions.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{
					RpcMethod:      "List",
					Use:            "list [id]",
					Short:          "Query a single sanctions list",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Lists", Use: "lists", Short: "List sanctions registries"},
				{
					RpcMethod: "Entry",
					Use:       "entry [list-id] [subject]",
					Short:     "Query a single sanctions entry",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "list_id"},
						{ProtoField: "subject"},
					},
				},
				{
					RpcMethod:      "Entries",
					Use:            "entries [list-id]",
					Short:          "List entries on a sanctions list",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "list_id"}},
				},
				{
					RpcMethod:      "IsSanctioned",
					Use:            "is-sanctioned [subject]",
					Short:          "Hot-path: is the subject sanctioned by any active list?",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject"}},
				},
				{
					RpcMethod:      "SubjectListings",
					Use:            "subject-listings [subject]",
					Short:          "Every list (active + removed) that has ever sanctioned the subject",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject"}},
				},
				{
					RpcMethod:      "Proposal",
					Use:            "proposal [proposal-id]",
					Short:          "Query a propose+confirm proposal by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "proposal_id"}},
				},
				{RpcMethod: "Proposals", Use: "proposals", Short: "List proposals"},
				{RpcMethod: "Hits", Use: "hits", Short: "List sanctions hit records"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.sanctions.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "DeprecateList",
					Use:       "deprecate-list [id] [reason]",
					Short:     "Mark a list DEPRECATED (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "ArchiveList",
					Use:       "archive-list [id] [reason]",
					Short:     "Mark a list ARCHIVED (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "RemoveEntry",
					Use:       "remove-entry [list-id] [subject] [reason]",
					Short:     "Mark a sanctions entry REMOVED (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "list_id"},
						{ProtoField: "subject"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod:      "ConfirmProposal",
					Use:            "confirm-proposal [proposal-id]",
					Short:          "Confirm a pending sanctions proposal (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "proposal_id"}},
					GovProposal:    true,
				},
				{
					RpcMethod: "RejectProposal",
					Use:       "reject-proposal [proposal-id] [reason]",
					Short:     "Reject a pending sanctions proposal (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "proposal_id"},
						{ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update sanctions params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
