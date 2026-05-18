// Package cli wires the multi-arg / sealed-bid Cobra commands.
// Simple positional messages (PlaceBid, Cancel, Close, Settle,
// WithdrawRefund) live in autocli.
package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/auction/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Auction module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreateAuctionCmd(),
		newCommitBidCmd(),
		newRevealBidCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

type createJSON struct {
	Kind              string `json:"kind"` // ENGLISH | DUTCH | SEALED_FIRST | SEALED_SECOND
	AssetRef          string `json:"asset_ref"`
	PaymentDenom      string `json:"payment_denom"`
	ReservePrice      uint64 `json:"reserve_price"`
	MinBidIncrement   uint64 `json:"min_bid_increment"`
	DutchStartPrice   uint64 `json:"dutch_start_price"`
	DutchFloorPrice   uint64 `json:"dutch_floor_price"`
	DutchDecaySeconds int64  `json:"dutch_decay_seconds"`
	StartTime         int64  `json:"start_time"`
	CommitEndTime     int64  `json:"commit_end_time"`
	RevealEndTime     int64  `json:"reveal_end_time"`
	EndTime           int64  `json:"end_time"`
	Memo              string `json:"memo"`
}

func parseKind(s string) (types.Kind, error) {
	switch strings.ToUpper(s) {
	case "ENGLISH":
		return types.Kind_KIND_ENGLISH, nil
	case "DUTCH":
		return types.Kind_KIND_DUTCH, nil
	case "SEALED_FIRST", "SEALED-FIRST":
		return types.Kind_KIND_SEALED_FIRST, nil
	case "SEALED_SECOND", "SEALED-SECOND", "VICKREY":
		return types.Kind_KIND_SEALED_SECOND, nil
	}
	return types.Kind_KIND_UNSPECIFIED, fmt.Errorf("invalid kind %q (ENGLISH|DUTCH|SEALED_FIRST|SEALED_SECOND)", s)
}

func newCreateAuctionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [auction.json]",
		Short: "Create a new auction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}
			var v createJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			kind, err := parseKind(v.Kind)
			if err != nil {
				return err
			}
			msg := &types.MsgCreateAuction{
				Seller:            cliCtx.GetFromAddress().String(),
				Kind:              kind,
				AssetRef:          v.AssetRef,
				PaymentDenom:      v.PaymentDenom,
				ReservePrice:      v.ReservePrice,
				MinBidIncrement:   v.MinBidIncrement,
				DutchStartPrice:   v.DutchStartPrice,
				DutchFloorPrice:   v.DutchFloorPrice,
				DutchDecaySeconds: v.DutchDecaySeconds,
				StartTime:         v.StartTime,
				CommitEndTime:     v.CommitEndTime,
				RevealEndTime:     v.RevealEndTime,
				EndTime:           v.EndTime,
				Memo:              v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newCommitBidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit [auction-id] [commit-hash-hex] [deposit]",
		Short: "Commit a sealed bid",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			aid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("auction_id: %w", err)
			}
			hash, err := hex.DecodeString(strings.TrimPrefix(args[1], "0x"))
			if err != nil {
				return fmt.Errorf("commit_hash hex: %w", err)
			}
			deposit, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("deposit: %w", err)
			}
			msg := &types.MsgCommitBid{
				Bidder:     cliCtx.GetFromAddress().String(),
				AuctionId:  aid,
				CommitHash: hash,
				Deposit:    deposit,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newRevealBidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reveal [bid-id] [price] [salt-hex]",
		Short: "Reveal a previously-committed sealed bid",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("bid_id: %w", err)
			}
			price, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("price: %w", err)
			}
			salt, err := hex.DecodeString(strings.TrimPrefix(args[2], "0x"))
			if err != nil {
				return fmt.Errorf("salt hex: %w", err)
			}
			msg := &types.MsgRevealBid{
				Bidder: cliCtx.GetFromAddress().String(),
				BidId:  bid,
				Price:  price,
				Salt:   salt,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxAuctions               uint32 `json:"max_auctions"`
	MaxAuctionsPerSeller      uint32 `json:"max_auctions_per_seller"`
	MaxBidsPerAuction         uint32 `json:"max_bids_per_auction"`
	MinAuctionDurationSeconds int64  `json:"min_auction_duration_seconds"`
	MaxAuctionDurationSeconds int64  `json:"max_auction_duration_seconds"`
	MemoMaxLen                uint32 `json:"memo_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update auction module parameters (governance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}
			var v paramsJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params: types.Params{
					MaxAuctions:               v.MaxAuctions,
					MaxAuctionsPerSeller:      v.MaxAuctionsPerSeller,
					MaxBidsPerAuction:         v.MaxBidsPerAuction,
					MinAuctionDurationSeconds: v.MinAuctionDurationSeconds,
					MaxAuctionDurationSeconds: v.MaxAuctionDurationSeconds,
					MemoMaxLen:                v.MemoMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
