// Package cli wires the x/audit module into Cobra (manual registration;
// autocli covers the simpler positional commands while the JSON-payload
// flows for nested messages live here).
package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/audit/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Audit module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRecordCmd(),
		newRegisterSchemaCmd(),
		newArchiveCmd(),
		newGrantViewKeyCmd(),
		newRevokeViewKeyCmd(),
	)
	return cmd
}

// newRecordCmd: tx audit record <event-type> <target> <action> [data]
func newRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record [event-type] [target] [action] [data]",
		Short: "Record an audit event (supports --severity, --schema-id, --payload-encrypted, --payload-digest)",
		Long: `Append an audit event signed by --from. Use this for compliance
trails (login, settings change, governance proposal, etc).

Example:
  energychaind tx audit record settings energy1abc... update \
    '{"field":"capacity_kw","old":400,"new":500}' \
    --severity warn \
    --schema-id "param_update.x/eac" \
    --from dev0 ...`,
		Args: cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			data := ""
			if len(args) == 4 {
				data = args[3]
			}
			sev, _ := cmd.Flags().GetString("severity")
			severity, err := parseSeverity(sev)
			if err != nil {
				return err
			}
			schemaID, _ := cmd.Flags().GetString("schema-id")
			encrypted, _ := cmd.Flags().GetBool("payload-encrypted")
			digest, _ := cmd.Flags().GetString("payload-digest")

			msg := &types.MsgRecordAudit{
				Creator:          clientCtx.GetFromAddress().String(),
				EventType:        strings.TrimSpace(args[0]),
				Target:           strings.TrimSpace(args[1]),
				Action:           strings.TrimSpace(args[2]),
				Data:             data,
				Severity:         severity,
				SchemaId:         schemaID,
				PayloadEncrypted: encrypted,
				PayloadDigest:    digest,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("severity", "info", "Severity tier: info|notice|warn|critical|fatal")
	cmd.Flags().String("schema-id", "", "Optional registered schema id (event_type)")
	cmd.Flags().Bool("payload-encrypted", false, "When true, data is an opaque encrypted blob (requires --payload-digest)")
	cmd.Flags().String("payload-digest", "", "sha256 hex of the cleartext payload (required when --payload-encrypted)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRegisterSchemaCmd: tx audit register-schema <descriptor.json>
//
// JSON shape:
//
//	{
//	  "event_type": "param_update.x/eac",
//	  "uri":        "ipfs://QmHash...",
//	  "hash":       "<sha256-hex>",
//	  "version":    0
//	}
//
// Sender (typically governance proposer) is read from --from.
func newRegisterSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-schema [descriptor.json]",
		Short: "Pin or bump a self-describing event schema (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read descriptor: %w", err)
			}
			var d types.SchemaDescriptor
			if err := json.Unmarshal(bz, &d); err != nil {
				return fmt.Errorf("parse descriptor: %w", err)
			}
			msg := &types.MsgRegisterSchema{
				Authority:   clientCtx.GetFromAddress().String(),
				Descriptor_: d,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newArchiveCmd: tx audit archive <from> <to> <merkle-root> <uri> [--algo=sha256]
func newArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive [from-id] [to-id] [merkle-root] [uri]",
		Short: "Seal a contiguous span of audit logs into an off-chain batch",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			from, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("from-id: %w", err)
			}
			to, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("to-id: %w", err)
			}
			algo, _ := cmd.Flags().GetString("algo")
			msg := &types.MsgArchive{
				Authority:       clientCtx.GetFromAddress().String(),
				FromLogId:       from,
				ToLogId:         to,
				MerkleRoot:      args[2],
				MerkleAlgorithm: algo,
				Uri:             args[3],
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("algo", types.MerkleAlgorithmSHA256, "Merkle algorithm tag stored on-chain")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newGrantViewKeyCmd: tx audit grant-view-key <grant.json>
//
// JSON shape:
//
//	{
//	  "grantee":         "energy1regulator...",
//	  "scope":           {"event_type_prefix":"freeze.", "from_timestamp":0, "to_timestamp":0},
//	  "encrypted_key_b64":"<base64>",
//	  "encrypted_key_hex":"<hex>",   // pick one
//	  "expires_at":      0
//	}
func newGrantViewKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-view-key [grant.json]",
		Short: "Issue a view-key grant authorising a regulator to decrypt scoped audit payloads",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read grant: %w", err)
			}
			var raw struct {
				Grantee         string         `json:"grantee"`
				Scope           types.ViewScope `json:"scope"`
				EncryptedKeyB64 string         `json:"encrypted_key_b64"`
				EncryptedKeyHex string         `json:"encrypted_key_hex"`
				ExpiresAt       int64          `json:"expires_at"`
			}
			if err := json.Unmarshal(bz, &raw); err != nil {
				return fmt.Errorf("parse grant: %w", err)
			}
			var key []byte
			switch {
			case raw.EncryptedKeyB64 != "":
				key, err = base64.StdEncoding.DecodeString(raw.EncryptedKeyB64)
			case raw.EncryptedKeyHex != "":
				key, err = hex.DecodeString(raw.EncryptedKeyHex)
			default:
				return fmt.Errorf("grant must supply encrypted_key_b64 or encrypted_key_hex")
			}
			if err != nil {
				return fmt.Errorf("decode encrypted_key: %w", err)
			}
			msg := &types.MsgGrantViewKey{
				Grantor:      clientCtx.GetFromAddress().String(),
				Grantee:      raw.Grantee,
				Scope:        raw.Scope,
				EncryptedKey: key,
				ExpiresAt:    raw.ExpiresAt,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRevokeViewKeyCmd: tx audit revoke-view-key <grant-id> [reason]
func newRevokeViewKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-view-key [grant-id] [reason]",
		Short: "Revoke a previously issued view-key grant",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			reason := ""
			if len(args) == 2 {
				reason = args[1]
			}
			msg := &types.MsgRevokeViewKey{
				Actor:   clientCtx.GetFromAddress().String(),
				GrantId: args[0],
				Reason:  reason,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// parseSeverity maps human-friendly tier names to the proto enum.
func parseSeverity(s string) (types.Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return types.Severity_SEVERITY_INFO, nil
	case "notice":
		return types.Severity_SEVERITY_NOTICE, nil
	case "warn", "warning":
		return types.Severity_SEVERITY_WARN, nil
	case "critical":
		return types.Severity_SEVERITY_CRITICAL, nil
	case "fatal":
		return types.Severity_SEVERITY_FATAL, nil
	default:
		return 0, fmt.Errorf("unknown severity %q", s)
	}
}
