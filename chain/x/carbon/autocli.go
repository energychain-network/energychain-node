package carbon

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. The
// IssueAllowance / IssueOffset / RegisterIssuer / SetArticle6Status
// commands take wide multi-flag payloads and live in
// client/cli/tx.go.
//
//	energychaind query carbon params
//	energychaind query carbon issuer [id]
//	energychaind query carbon issuers
//	energychaind query carbon asset [id]
//	energychaind query carbon assets
//	energychaind query carbon assets-by-issuer [issuer-id]
//	energychaind query carbon balance [asset-id] [account]
//	energychaind query carbon balances-by-owner [owner]
//	energychaind query carbon retirement [id]
//	energychaind query carbon retirements-by-beneficiary [beneficiary]
//	energychaind query carbon retirements-by-asset [asset-id]
//	energychaind query carbon article6-authorization [asset-id]
//	energychaind query carbon bridge-attestation [asset-id]
//	energychaind query carbon eac-offset-claimed [eac-cert-id]
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.carbon.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{
					RpcMethod:      "Issuer",
					Use:            "issuer [id]",
					Short:          "Query a registered issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Issuers", Use: "issuers", Short: "List registered issuers"},
				{
					RpcMethod:      "Asset",
					Use:            "asset [id]",
					Short:          "Query a carbon asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Assets", Use: "assets", Short: "List carbon assets"},
				{
					RpcMethod:      "AssetsByIssuer",
					Use:            "assets-by-issuer [issuer-id]",
					Short:          "List assets issued by an issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer_id"}},
				},
				{
					RpcMethod: "Balance",
					Use:       "balance [asset-id] [account]",
					Short:     "Query a per-asset balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "account"},
					},
				},
				{
					RpcMethod:      "BalancesByOwner",
					Use:            "balances-by-owner [owner]",
					Short:          "List all (asset, balance) pairs owned by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}},
				},
				{
					RpcMethod:      "Retirement",
					Use:            "retirement [id]",
					Short:          "Query a retirement record (= NFT) by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "RetirementsByBeneficiary",
					Use:            "retirements-by-beneficiary [beneficiary]",
					Short:          "List retirements credited to a beneficiary",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "beneficiary"}},
				},
				{
					RpcMethod:      "RetirementsByAsset",
					Use:            "retirements-by-asset [asset-id]",
					Short:          "List retirements applied to an asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod:      "Article6Authorization",
					Use:            "article6-authorization [asset-id]",
					Short:          "Query the Article 6 authorisation row for an asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod:      "BridgeAttestation",
					Use:            "bridge-attestation [asset-id]",
					Short:          "Query the bridge attestation row for an asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod:      "EACOffsetClaimed",
					Use:            "eac-offset-claimed [eac-cert-id]",
					Short:          "Tally of OFFSET units that referenced this x/eac certificate",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "eac_certificate_id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.carbon.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SuspendIssuer",
					Use:       "suspend-issuer [id] [reason]",
					Short:     "Suspend an issuer (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "RevokeIssuer",
					Use:       "revoke-issuer [id] [reason]",
					Short:     "Revoke an issuer (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "SealAsset",
					Use:       "seal-asset [asset-id] [reason]",
					Short:     "Seal an asset (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "UnsealAsset",
					Use:       "unseal-asset [asset-id] [reason]",
					Short:     "Unseal an asset (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Transfer",
					Use:       "transfer [asset-id] [to] [units] [memo]",
					Short:     "Transfer carbon units (full compliance gate)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "to"},
						{ProtoField: "units"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Retire",
					Use:       "retire [asset-id] [units] [beneficiary] [purpose] [claim] [memo] [beneficiary-jurisdiction]",
					Short:     "Retire carbon units; mints a Retirement NFT-style record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "units"},
						{ProtoField: "beneficiary"}, {ProtoField: "purpose"},
						{ProtoField: "claim"}, {ProtoField: "memo"},
						{ProtoField: "beneficiary_jurisdiction"},
					},
				},
				{
					RpcMethod: "BridgeMint",
					Use:       "bridge-mint [asset-id] [units] [recipient]",
					Short:     "Bridge-mint carbon units (issuer authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "units"}, {ProtoField: "recipient"},
					},
				},
				{
					RpcMethod: "BridgeBurn",
					Use:       "bridge-burn [asset-id] [units] [external-recipient] [memo]",
					Short:     "Burn carbon units to release them on the upstream registry",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"}, {ProtoField: "units"},
						{ProtoField: "external_recipient"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update carbon params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
