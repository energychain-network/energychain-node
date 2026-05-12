// Package cli wires the x/did module into Cobra. Used for the message
// types whose payloads contain repeated VerificationMethod / ServiceEndpoint
// nested messages — autocli's positional-arg synthesizer cannot encode
// those, so we serve them through bespoke commands that take a JSON file
// argument.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/did/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "DID module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreateCmd(),
		newRotateKeyCmd(),
		newIssueCredentialCmd(),
		newRegisterAnchorCmd(),
	)
	return cmd
}

// docPayload is the JSON shape consumed by the create / rotate commands.
// Mirrors the proto fields one-to-one to keep round-tripping trivial.
type docPayload struct {
	Subject      string                       `json:"subject"`
	Controllers  []string                     `json:"controllers"`
	Verification []types.VerificationMethod   `json:"verification"`
	Service      []types.ServiceEndpoint      `json:"service"`
}

func loadJSON(path string, dst interface{}) error {
	bz, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(bz, dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// newCreateCmd: tx did create <doc.json>
//
// Doc JSON shape:
//   {
//     "subject":      "energy1abc...",
//     "controllers":  ["energy1abc..."],
//     "verification": [{"id":"key-1","key_type":1,"public_key":"<base64>","purposes":["authentication"]}],
//     "service":      []
//   }
func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [path/to/doc.json]",
		Short: "Create a DID Document for --from (must equal subject)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var p docPayload
			if err := loadJSON(args[0], &p); err != nil {
				return err
			}
			subject := strings.TrimSpace(p.Subject)
			if subject == "" {
				subject = ctx.GetFromAddress().String()
			}
			msg := &types.MsgCreateDID{
				Creator:      ctx.GetFromAddress().String(),
				Subject:      subject,
				Controllers:  p.Controllers,
				Verification: p.Verification,
				Service:      p.Service,
			}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRotateKeyCmd: tx did rotate-key <subject> <old_key_id> <new_key.json>
func newRotateKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-key [subject] [old_key_id] [new_key.json]",
		Short: "Rotate a verification method (controller-gated)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var nk types.VerificationMethod
			if err := loadJSON(args[2], &nk); err != nil {
				return err
			}
			msg := &types.MsgRotateKey{
				Controller: ctx.GetFromAddress().String(),
				Subject:    args[0],
				OldKeyId:   args[1],
				NewKey:     nk,
			}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newIssueCredentialCmd:
//   tx did issue-credential <subject> <id> <type> <hash> [issued_at] [expires_at]
func newIssueCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-credential [subject] [id] [type] [hash] [issued_at] [expires_at]",
		Short: "Issue a credential (--from must be a registered trust anchor)",
		Args:  cobra.RangeArgs(4, 6),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var issued, expires int64
			if len(args) >= 5 {
				if _, err := fmt.Sscan(args[4], &issued); err != nil {
					return fmt.Errorf("issued_at: %w", err)
				}
			}
			if len(args) >= 6 {
				if _, err := fmt.Sscan(args[5], &expires); err != nil {
					return fmt.Errorf("expires_at: %w", err)
				}
			}
			msg := &types.MsgIssueCredential{
				Issuer:    ctx.GetFromAddress().String(),
				Subject:   args[0],
				Id:        args[1],
				Type:      args[2],
				Hash:      args[3],
				IssuedAt:  issued,
				ExpiresAt: expires,
			}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRegisterAnchorCmd:
//   tx did register-anchor <did> <tier> [parent_did] [credential_types_csv]
func newRegisterAnchorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-anchor [did] [tier] [parent_did] [credential_types_csv]",
		Short: "Register a trust anchor (root requires governance, intermediate requires parent root)",
		Args:  cobra.RangeArgs(2, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			parent := ""
			if len(args) >= 3 {
				parent = args[2]
			}
			var creds []string
			if len(args) >= 4 && args[3] != "" {
				creds = strings.Split(args[3], ",")
			}
			msg := &types.MsgRegisterAnchor{
				Authority:       ctx.GetFromAddress().String(),
				Did:             args[0],
				Tier:            args[1],
				ParentDid:       parent,
				CredentialTypes: creds,
			}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
