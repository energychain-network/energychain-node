package rwa

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

// AutoCLIOptions wires the simple positional commands. Commands
// requiring multi-flag JSON payloads (CreateToken / UpdateToken /
// RegisterIssuer / SetAccountFlags / UpdateParams) live in
// client/cli/tx.go.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.rwa.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},

				{RpcMethod: "Issuer", Use: "issuer [id]", Short: "Query a registered issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Issuers", Use: "issuers", Short: "List registered issuers"},

				{RpcMethod: "Token", Use: "token [id]", Short: "Query a token by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "TokenBySymbol", Use: "token-by-symbol [symbol]", Short: "Query a token by symbol",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "symbol"}}},
				{RpcMethod: "Tokens", Use: "tokens", Short: "List tokens"},
				{RpcMethod: "TokensByIssuer", Use: "tokens-by-issuer [issuer-id]", Short: "List tokens issued by issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer_id"}}},

				{RpcMethod: "Balance", Use: "balance [token-id] [account]", Short: "Query a per-token balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "account"}}},
				{RpcMethod: "BalancesByOwner", Use: "balances-by-owner [owner]", Short: "List all (token, balance) pairs owned by an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}}},
				{RpcMethod: "Transferable", Use: "transferable [token-id] [account]", Short: "Show balance, frozen, locked and transferable units",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "account"}}},
				{RpcMethod: "FrozenBalance", Use: "frozen [token-id] [account]", Short: "Query frozen amount for an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "account"}}},
				{RpcMethod: "LockupsByHolder", Use: "lockups-by-holder [token-id] [account]", Short: "List lockups for an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "account"}}},
				{RpcMethod: "AccountFlags", Use: "account-flags [token-id] [account]", Short: "Query the per-account compliance flags",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "account"}}},

				{RpcMethod: "Snapshot", Use: "snapshot [id]", Short: "Query a snapshot",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "SnapshotsByToken", Use: "snapshots-by-token [token-id]", Short: "List snapshots for a token",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}}},
				{RpcMethod: "SnapshotBalance", Use: "snapshot-balance [snapshot-id] [account]", Short: "Per-holder balance recorded at a snapshot",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "snapshot_id"}, {ProtoField: "account"}}},

				{RpcMethod: "Distribution", Use: "distribution [id]", Short: "Query a distribution by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "DistributionsByToken", Use: "distributions-by-token [token-id]", Short: "List distributions for a token",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}}},
				{RpcMethod: "DistributionClaim", Use: "distribution-claim [distribution-id] [account]", Short: "Query a per-account claim row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}, {ProtoField: "account"}}},

				{RpcMethod: "Redemption", Use: "redemption [id]", Short: "Query a redemption request",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "RedemptionsByHolder", Use: "redemptions-by-holder [holder]", Short: "List redemptions filed by a holder",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "holder"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.rwa.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "SuspendIssuer", Use: "suspend-issuer [id] [reason]",
					Short:          "Suspend an issuer (governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}, {ProtoField: "reason"}},
					GovProposal:    true},
				{RpcMethod: "RevokeIssuer", Use: "revoke-issuer [id] [reason]",
					Short:          "Revoke an issuer (governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}, {ProtoField: "reason"}},
					GovProposal:    true},

				{RpcMethod: "PauseToken", Use: "pause-token [token-id] [reason]", Short: "Pause a token (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "UnpauseToken", Use: "unpause-token [token-id] [reason]", Short: "Unpause a token (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "TerminateToken", Use: "terminate-token [token-id] [reason]", Short: "Terminate a token (issuer admin)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "reason"}}},

				{RpcMethod: "Mint", Use: "mint [token-id] [recipient] [amount] [memo]", Short: "Mint units to a recipient (issuer authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "recipient"}, {ProtoField: "amount"}, {ProtoField: "memo"},
					}},
				{RpcMethod: "Burn", Use: "burn [token-id] [from] [amount] [reason]", Short: "Burn units from a holder (issuer authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "from"}, {ProtoField: "amount"}, {ProtoField: "reason"},
					}},

				{RpcMethod: "Transfer", Use: "transfer [to] [token-id] [amount] [memo]", Short: "Transfer units (compliance gates apply)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "to"}, {ProtoField: "token_id"}, {ProtoField: "amount"}, {ProtoField: "memo"},
					}},
				{RpcMethod: "ForceTransfer", Use: "force-transfer [token-id] [from] [to] [amount] [reason]", Short: "Issuer-driven recovery transfer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "from"}, {ProtoField: "to"}, {ProtoField: "amount"}, {ProtoField: "reason"},
					}},

				{RpcMethod: "SetFrozenBalance", Use: "set-frozen [token-id] [account] [amount] [reason]", Short: "Set the frozen amount for an account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "account"}, {ProtoField: "amount"}, {ProtoField: "reason"},
					}},
				{RpcMethod: "AddLockup", Use: "add-lockup [token-id] [account] [amount] [unlock-time] [reason]", Short: "Add a vesting lockup row",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "account"}, {ProtoField: "amount"}, {ProtoField: "unlock_time"}, {ProtoField: "reason"},
					}},

				{RpcMethod: "TakeSnapshot", Use: "take-snapshot [token-id]", Short: "Take a snapshot of token holders",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}}},
				{RpcMethod: "FundDistribution", Use: "fund-distribution [distribution-id]", Short: "Fund a created distribution from issuer authority's stablecoin balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}}},
				{RpcMethod: "ClaimDistribution", Use: "claim-distribution [distribution-id]", Short: "Claim your share of a funded distribution",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}}},
				{RpcMethod: "FinalizeDistribution", Use: "finalize-distribution [distribution-id]", Short: "Finalize a distribution and sweep unclaimed back to issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}}},

				{RpcMethod: "RequestRedemption", Use: "request-redemption [token-id] [amount] [memo]", Short: "Request a T+N redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "token_id"}, {ProtoField: "amount"}, {ProtoField: "memo"},
					}},
				{RpcMethod: "SettleRedemption", Use: "settle-redemption [redemption-id]", Short: "Settle an eligible redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "redemption_id"}}},
				{RpcMethod: "CancelRedemption", Use: "cancel-redemption [redemption-id] [reason]", Short: "Cancel a pending redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "redemption_id"}, {ProtoField: "reason"}}},
			},
		},
	}
}
