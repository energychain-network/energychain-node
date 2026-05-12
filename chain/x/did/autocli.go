package did

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions exposes the did module commands. Manual entries because the
// repo doesn't generate the pulsar descriptors required for full automatic
// command synthesis.
//
// Queries:
//   energychaind query did did [subject]
//   energychaind query did all
//   energychaind query did credential [id]
//   energychaind query did creds-by-subject [subject]
//   energychaind query did creds-by-issuer [issuer]
//   energychaind query did anchor [did]
//   energychaind query did anchors
//   energychaind query did params
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.did.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "DID", Use: "did [subject]", Short: "Query a DID Document",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject"}}},
				{RpcMethod: "AllDIDs", Use: "all", Short: "List every DID Document (paginated)"},
				{RpcMethod: "Credential", Use: "credential [id]", Short: "Query a credential by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "CredentialsBySubject", Use: "creds-by-subject [subject]",
					Short: "List credentials by subject DID (paginated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject"}}},
				{RpcMethod: "CredentialsByIssuer", Use: "creds-by-issuer [issuer]",
					Short: "List credentials by issuer DID (paginated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer"}}},
				{RpcMethod: "Anchor", Use: "anchor [did]", Short: "Query a trust anchor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "did"}}},
				{RpcMethod: "AllAnchors", Use: "anchors", Short: "List trust anchors (paginated)"},
				{RpcMethod: "Params", Use: "params", Short: "Query DID module parameters"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.did.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				// Document operations are exposed via Cobra (see
				// client/cli/tx.go) because their payloads include
				// repeated message fields that don't survive autocli
				// positional encoding.
				{
					RpcMethod:      "DeactivateDID",
					Use:            "deactivate [subject]",
					Short:          "Deactivate a DID (controller-gated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "subject"}},
				},
				{
					RpcMethod: "RevokeCredential",
					Use:       "revoke-credential [id] [reason]",
					Short:     "Revoke a credential issued by --from (issuer)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod:      "DeactivateAnchor",
					Use:            "deactivate-anchor [did]",
					Short:          "Deactivate a trust anchor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "did"}},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update did module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
