package stablecoin

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. Messages with
// nested payloads (RegisterIssuer with mint authorities, RegisterDenom
// with the full set of fields) live in client/cli/tx.go because
// autocli cannot synthesise the multi-flag list types they need.
//
//	energychaind query stablecoin params
//	energychaind query stablecoin denom [id]
//	energychaind query stablecoin denoms
//	energychaind query stablecoin issuer [id]
//	energychaind query stablecoin issuers
//	energychaind query stablecoin quota [issuer-id] [denom-id]
//	energychaind query stablecoin reserve [denom-id]
//	energychaind query stablecoin balance [denom-id] [account]
//	energychaind query stablecoin allowance [denom-id] [owner] [spender]
//	energychaind query stablecoin supply [denom-id]
//	energychaind query stablecoin account-flags [denom-id] [account]
//	energychaind query stablecoin redemption [id]
//
//	energychaind tx stablecoin pause-denom [id] [reason]   (gov)
//	energychaind tx stablecoin resume-denom [id] [reason]  (gov)
//	energychaind tx stablecoin retire-denom [id] [reason]  (gov)
//	energychaind tx stablecoin mint [issuer-id] [denom-id] [recipient] [amount] [memo]
//	energychaind tx stablecoin burn [denom-id] [amount] [memo]
//	energychaind tx stablecoin transfer [denom-id] [to] [amount] [memo]
//	energychaind tx stablecoin approve [denom-id] [spender] [amount]
//	energychaind tx stablecoin transfer-from [denom-id] [from] [to] [amount] [memo]
//	energychaind tx stablecoin freeze / unfreeze / blacklist / unblacklist
//	energychaind tx stablecoin force-transfer ...
//	energychaind tx stablecoin set-mint-paused ...
//	energychaind tx stablecoin request-redemption / fulfill-redemption / cancel-redemption
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.stablecoin.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{
					RpcMethod:      "Denom",
					Use:            "denom [id]",
					Short:          "Query a registered denom",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Denoms", Use: "denoms", Short: "List registered denoms"},
				{
					RpcMethod:      "Issuer",
					Use:            "issuer [id]",
					Short:          "Query a registered issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{RpcMethod: "Issuers", Use: "issuers", Short: "List registered issuers"},
				{
					RpcMethod: "Quota",
					Use:       "quota [issuer-id] [denom-id]",
					Short:     "Query the (issuer, denom) mint quota row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
					},
				},
				{
					RpcMethod:      "Reserve",
					Use:            "reserve [denom-id]",
					Short:          "Query the reserve attestation requirement",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}},
				},
				{
					RpcMethod: "Balance",
					Use:       "balance [denom-id] [account]",
					Short:     "Query an account balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "account"},
					},
				},
				{
					RpcMethod: "Allowance",
					Use:       "allowance [denom-id] [owner] [spender]",
					Short:     "Query an ERC-20-style allowance row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "owner"}, {ProtoField: "spender"},
					},
				},
				{
					RpcMethod:      "Supply",
					Use:            "supply [denom-id]",
					Short:          "Query the total supply for a denom",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}},
				},
				{
					RpcMethod: "AccountFlags",
					Use:       "account-flags [denom-id] [account]",
					Short:     "Query freeze/blacklist flags for an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "account"},
					},
				},
				{
					RpcMethod:      "Redemption",
					Use:            "redemption [id]",
					Short:          "Query a redemption request by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.stablecoin.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "PauseDenom",
					Use:       "pause-denom [id] [reason]",
					Short:     "Pause a denom (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "ResumeDenom",
					Use:       "resume-denom [id] [reason]",
					Short:     "Resume a paused denom (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "RetireDenom",
					Use:       "retire-denom [id] [reason]",
					Short:     "Retire an idle denom permanently (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"}, {ProtoField: "reason"},
					},
					GovProposal: true,
				},
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
					RpcMethod: "SetMintQuota",
					Use:       "set-mint-quota [issuer-id] [denom-id] [ceiling]",
					Short:     "Set or update the per-(issuer, denom) mint ceiling (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"}, {ProtoField: "ceiling"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "SetReserveRequirement",
					Use:       "set-reserve-requirement [denom-id] [oracle-topic-id] [ratio-bps] [max-staleness-seconds]",
					Short:     "Set the reserve coverage requirement for a denom (governance only)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"},
						{ProtoField: "oracle_topic_id"},
						{ProtoField: "required_ratio_bps"},
						{ProtoField: "max_staleness_seconds"},
					},
					GovProposal: true,
				},
				{
					RpcMethod: "Mint",
					Use:       "mint [issuer-id] [denom-id] [recipient] [amount] [memo]",
					Short:     "Mint stablecoin to a recipient (issuer mint authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "recipient"}, {ProtoField: "amount"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Burn",
					Use:       "burn [denom-id] [amount] [memo]",
					Short:     "Burn stablecoin from the caller's balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "amount"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Transfer",
					Use:       "transfer [denom-id] [to] [amount] [memo]",
					Short:     "Transfer stablecoin (runs full compliance gate)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "to"},
						{ProtoField: "amount"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Approve",
					Use:       "approve [denom-id] [spender] [amount]",
					Short:     "Set an ERC-20-style allowance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "spender"}, {ProtoField: "amount"},
					},
				},
				{
					RpcMethod: "TransferFrom",
					Use:       "transfer-from [denom-id] [from] [to] [amount] [memo]",
					Short:     "Spend an allowance to transfer on behalf of an owner",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom_id"}, {ProtoField: "from"}, {ProtoField: "to"},
						{ProtoField: "amount"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "Freeze",
					Use:       "freeze [issuer-id] [denom-id] [account] [reason]",
					Short:     "Freeze an account on a denom (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "account"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Unfreeze",
					Use:       "unfreeze [issuer-id] [denom-id] [account] [reason]",
					Short:     "Unfreeze an account on a denom (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "account"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Blacklist",
					Use:       "blacklist [issuer-id] [denom-id] [account] [reason]",
					Short:     "Blacklist an account on a denom (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "account"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Unblacklist",
					Use:       "unblacklist [issuer-id] [denom-id] [account] [reason]",
					Short:     "Unblacklist an account on a denom (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "account"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "ForceTransfer",
					Use:       "force-transfer [issuer-id] [denom-id] [from] [to] [amount] [reason]",
					Short:     "Force-transfer between accounts (issuer admin compliance op)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "from"}, {ProtoField: "to"},
						{ProtoField: "amount"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "SetMintPaused",
					Use:       "set-mint-paused [issuer-id] [denom-id] [paused] [reason]",
					Short:     "Pause/unpause issuer-side minting on a quota row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "paused"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "RequestRedemption",
					Use:       "request-redemption [issuer-id] [denom-id] [amount] [memo]",
					Short:     "Burn tokens and queue a fiat-side redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "issuer_id"}, {ProtoField: "denom_id"},
						{ProtoField: "amount"}, {ProtoField: "memo"},
					},
				},
				{
					RpcMethod: "FulfillRedemption",
					Use:       "fulfill-redemption [id] [payout-ref]",
					Short:     "Mark a redemption fulfilled with off-chain payout reference",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "redemption_id"}, {ProtoField: "payout_ref"},
					},
				},
				{
					RpcMethod: "CancelRedemption",
					Use:       "cancel-redemption [id] [reason]",
					Short:     "Cancel a pending redemption (re-mints tokens to holder)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "redemption_id"}, {ProtoField: "reason"},
					},
				},
				{
					RpcMethod:      "UpdateParams",
					Use:            "update-params-proposal [params]",
					Short:          "Submit a governance proposal to update stablecoin params",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}},
					GovProposal:    true,
				},
			},
		},
	}
}
