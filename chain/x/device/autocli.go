package device

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.device.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Device", Use: "device [device-did]", Short: "Query a device by DID",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "device_did"}}},
				{RpcMethod: "DevicesByOwner", Use: "by-owner [owner]", Short: "List devices by owner (paginated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}}},
				{RpcMethod: "DevicesByGrid", Use: "by-grid [grid-zone]", Short: "List devices by grid zone (paginated)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "grid_zone"}}},
				{RpcMethod: "AllDevices", Use: "all", Short: "List every device (paginated)"},
				{RpcMethod: "Attestation", Use: "attestation [id]", Short: "Query an attestation by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "PendingAttestations", Use: "pending", Short: "List pending attestations (paginated)"},
				{RpcMethod: "Params", Use: "params", Short: "Query device module parameters"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.device.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "TransferDevice", Use: "transfer [device-did] [new-owner]",
					Short:          "Transfer ownership (signed by current owner)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "device_did"}, {ProtoField: "new_owner"}}},
				{RpcMethod: "RevokeDevice", Use: "revoke [device-did] [reason]",
					Short:          "Revoke a device (owner or governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "device_did"}, {ProtoField: "reason"}}},
				{RpcMethod: "ConfirmAttestation", Use: "confirm-attestation [id] [accept] [reason]",
					Short:          "Confirm or reject an attestation (verifier whitelist)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "attestation_id"}, {ProtoField: "accept"}, {ProtoField: "reason"}}},
				{RpcMethod: "FlagFirmwareMismatch", Use: "flag [device-did] [reason]",
					Short:          "Flag a device as firmware-suspect",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "device_did"}, {ProtoField: "reason"}}},
				{RpcMethod: "UpdateParams", Use: "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update device module params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true},
			},
		},
	}
}
