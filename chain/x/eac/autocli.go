package eac

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. The
// IssueBatch / RegisterIssuer / UpdateIssuer / SetBridgeAttestation
// flows take wide multi-flag payloads and are exposed via custom
// Cobra commands in client/cli/tx.go.
//
//	energychaind query eac params
//	energychaind query eac issuer [id]
//	energychaind query eac issuers
//	energychaind query eac certificate [id]
//	energychaind query eac certificates
//	energychaind query eac certificates-by-issuer [issuer-id]
//	energychaind query eac balance [cert-id] [account]
//	energychaind query eac balances-by-owner [owner]
//	energychaind query eac retirement [id]
//	energychaind query eac retirements-by-beneficiary [beneficiary]
//	energychaind query eac retirements-by-certificate [cert-id]
//	energychaind query eac bridge-attestation [cert-id]
//
//	energychaind tx eac suspend-issuer / revoke-issuer       (gov)
//	energychaind tx eac seal-certificate / unseal-certificate
//	energychaind tx eac transfer / retire / bridge-burn
//	energychaind tx eac bridge-mint
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.eac.v1.Query",
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
					RpcMethod:      "Certificate",
					Use:            "certificate [id]",
					Short:          "Query a certificate by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Certificates", Use: "certificates", Short: "List certificates"},
				{
					RpcMethod:      "CertificatesByIssuer",
					Use:            "certificates-by-issuer [issuer-id]",
					Short:          "List certificates issued by an issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer_id"}},
				},
				{
					RpcMethod: "Balance",
					Use:       "balance [cert-id] [account]",
					Short:     "Query a per-certificate balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "account"},
					},
				},
				{
					RpcMethod:      "BalancesByOwner",
					Use:            "balances-by-owner [owner]",
					Short:          "List all (cert, balance) pairs owned by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}},
				},
				{
					RpcMethod:      "Retirement",
					Use:            "retirement [id]",
					Short:          "Query a retirement record by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "RetirementsByBeneficiary",
					Use:            "retirements-by-beneficiary [beneficiary]",
					Short:          "List retirements credited to a beneficiary",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "beneficiary"}},
				},
				{
					RpcMethod:      "RetirementsByCertificate",
					Use:            "retirements-by-certificate [cert-id]",
					Short:          "List retirements applied to a certificate",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "certificate_id"}},
				},
				{
					RpcMethod:      "BridgeAttestation",
					Use:            "bridge-attestation [cert-id]",
					Short:          "Query the bridge attestation row for a certificate",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "certificate_id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.eac.v1.Msg",
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
					RpcMethod: "SealCertificate",
					Use:       "seal-certificate [cert-id] [reason]",
					Short:     "Seal a certificate so transfers are blocked (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "UnsealCertificate",
					Use:       "unseal-certificate [cert-id] [reason]",
					Short:     "Unseal a previously sealed certificate (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Transfer",
					Use:       "transfer [cert-id] [to] [units] [memo]",
					Short:     "Transfer EAC units to a recipient (full compliance gate)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "to"},
						{ProtoField: "units"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Retire",
					Use:       "retire [cert-id] [units] [beneficiary] [purpose] [memo]",
					Short:     "Retire EAC units (beneficiary may equal retirer)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "units"},
						{ProtoField: "beneficiary"}, {ProtoField: "purpose"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "BridgeMint",
					Use:       "bridge-mint [cert-id] [units] [recipient]",
					Short:     "Mint additional units after upstream registry lock (issuer authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "units"}, {ProtoField: "recipient"},
					},
				},
				{
					RpcMethod: "BridgeBurn",
					Use:       "bridge-burn [cert-id] [units] [external-recipient] [memo]",
					Short:     "Burn units to release them on the upstream registry",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "certificate_id"}, {ProtoField: "units"},
						{ProtoField: "external_recipient"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update eac params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
