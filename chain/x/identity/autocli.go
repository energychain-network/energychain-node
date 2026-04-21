package identity

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions exposes identity queries and txs.
//
//	energychaind query identity identity [address]
//	energychaind query identity by-role [role]
//	energychaind query identity all
//	energychaind query identity params
//
//	energychaind tx identity register [address] [name] [role] [metadata]
//	energychaind tx identity update [address] [name] [metadata]
//	energychaind tx identity revoke [address] [reason]
//	energychaind tx identity update-params-proposal [params-json]      # gov
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.identity.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "Identity",
					Use:            "identity [address]",
					Short:          "Query an identity by address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "IdentitiesByRole",
					Use:            "by-role [role]",
					Short:          "List identities holding a particular role (paginated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "role"}},
				},
				{
					RpcMethod: "AllIdentities",
					Use:       "all",
					Short:     "List every registered identity (paginated)",
				},
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the current identity module parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.identity.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "RegisterIdentity",
					Use:       "register [address] [name] [role] [metadata]",
					Short:     "Register a new identity (admin-gated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
						{ProtoField: "name"},
						{ProtoField: "role"},
						{ProtoField: "metadata"},
					},
				},
				{
					RpcMethod: "UpdateIdentity",
					Use:       "update [address] [name] [metadata]",
					Short:     "Update an existing identity's name / metadata (admin-gated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
						{ProtoField: "name"},
						{ProtoField: "metadata"},
					},
				},
				{
					RpcMethod: "RevokeIdentity",
					Use:       "revoke [address] [reason]",
					Short:     "Revoke an identity (admin-gated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update identity module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
