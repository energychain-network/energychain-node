// Package cli provides Cobra commands for the x/oracle module. Registered
// manually for the nested-payload flows (RegisterTopic, UpdateTopic,
// RegisterProvider, SubmitValue, SubmitReserveAttestation, TopUpBond);
// the simpler positional flows go through autocli.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"energychain/x/oracle/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Oracle module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterTopicCmd(),
		newUpdateTopicCmd(),
		newRegisterProviderCmd(),
		newTopUpBondCmd(),
		newSubmitValueCmd(),
		newSubmitReserveAttestationCmd(),
	)
	return cmd
}

// newRegisterTopicCmd: tx oracle register-topic <topic.json>
func newRegisterTopicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-topic [topic.json]",
		Short: "Register a new oracle topic (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			t, err := readTopicJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterTopic{
				Authority: clientCtx.GetFromAddress().String(),
				Topic:     t,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdateTopicCmd: tx oracle update-topic <topic.json>
func newUpdateTopicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-topic [topic.json]",
		Short: "Update an existing oracle topic (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			t, err := readTopicJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateTopic{
				Authority: clientCtx.GetFromAddress().String(),
				Topic:     t,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRegisterProviderCmd: tx oracle register-provider [name] [bond] [contact-uri]
func newRegisterProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-provider [name] [bond] [contact-uri]",
		Short: "Register the signer as a bonded oracle provider",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bond, err := sdk.ParseCoinNormalized(args[1])
			if err != nil {
				return fmt.Errorf("bond: %w", err)
			}
			contact := ""
			if len(args) == 3 {
				contact = args[2]
			}
			msg := &types.MsgRegisterProvider{
				Address:    clientCtx.GetFromAddress().String(),
				Name:       args[0],
				Bond:       bond,
				ContactUri: contact,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newTopUpBondCmd: tx oracle top-up-bond [amount]
func newTopUpBondCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top-up-bond [amount]",
		Short: "Increase the signer's bond by [amount]",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			amt, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return fmt.Errorf("amount: %w", err)
			}
			msg := &types.MsgTopUpBond{
				Address: clientCtx.GetFromAddress().String(),
				Amount:  amt,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newSubmitValueCmd: tx oracle submit-value [topic-id] [value] [timestamp] [metadata]
func newSubmitValueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-value [topic-id] [value] [timestamp] [metadata]",
		Short: "Submit a value to an oracle topic. Timestamp 0 → now.",
		Args:  cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			val, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("value: %w", err)
			}
			ts, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("timestamp: %w", err)
			}
			if ts == 0 {
				ts = time.Now().Unix()
			}
			meta := ""
			if len(args) == 4 {
				meta = args[3]
			}
			msg := &types.MsgSubmitValue{
				Provider:  clientCtx.GetFromAddress().String(),
				TopicId:   strings.TrimSpace(args[0]),
				Value:     val,
				Timestamp: ts,
				Metadata:  meta,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newSubmitReserveAttestationCmd: tx oracle submit-reserve <attestation.json>
func newSubmitReserveAttestationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-reserve [attestation.json]",
		Short: "Submit a reserve (PoR) attestation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read attestation: %w", err)
			}
			var raw struct {
				ID              string   `json:"id"`
				Asset           string   `json:"asset"`
				Amount          string   `json:"amount"`
				AttestationURI  string   `json:"attestation_uri"`
				AttestationHash string   `json:"attestation_hash"`
				Signers         []string `json:"signers"`
				Threshold       uint32   `json:"threshold"`
			}
			if err := json.Unmarshal(bz, &raw); err != nil {
				return fmt.Errorf("parse attestation: %w", err)
			}
			msg := &types.MsgSubmitReserveAttestation{
				Custodian:       clientCtx.GetFromAddress().String(),
				Id:              raw.ID,
				Asset:           raw.Asset,
				Amount:          raw.Amount,
				AttestationUri:  raw.AttestationURI,
				AttestationHash: raw.AttestationHash,
				Signers:         raw.Signers,
				Threshold:       raw.Threshold,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// readTopicJSON parses a Topic descriptor file. We accept the snake_case
// JSON the proto emits when marshalled by encoding/json.
func readTopicJSON(path string) (types.Topic, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return types.Topic{}, fmt.Errorf("read topic: %w", err)
	}
	var t types.Topic
	if err := json.Unmarshal(bz, &t); err != nil {
		return types.Topic{}, fmt.Errorf("parse topic: %w", err)
	}
	return t, nil
}

// silence unused-import for sdkmath when builds don't need it.
var _ = sdkmath.ZeroInt
