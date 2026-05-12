// Package cli provides Cobra commands for the x/policy module. The
// nested-payload flows live here because autocli can't synthesise the
// repeated-rule + map<string,string> flag combinations they need
// (RegisterPolicy, UpdatePolicy).
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/policy/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Policy module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterPolicyCmd(),
		newUpdatePolicyCmd(),
	)
	return cmd
}

// policyJSON is the file format read by register/update. We keep it
// distinct from the on-chain Policy proto so a future schema bump
// (renaming a field, adding a top-level key) does not break existing
// CLI scripts as long as the JSON loader stays back-compatible.
type policyJSON struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Rules       []policyRuleJSON `json:"rules"`
}

type policyRuleJSON struct {
	Kind   uint32            `json:"kind"`
	Side   uint32            `json:"side"`
	Params map[string]string `json:"params"`
}

func readPolicyJSON(path string) (policyJSON, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return policyJSON{}, fmt.Errorf("read %s: %w", path, err)
	}
	var p policyJSON
	if err := json.Unmarshal(bz, &p); err != nil {
		return policyJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return p, nil
}

func policyRulesFromJSON(jsRules []policyRuleJSON) []types.PolicyRule {
	out := make([]types.PolicyRule, 0, len(jsRules))
	for _, r := range jsRules {
		out = append(out, types.PolicyRule{
			Kind:   r.Kind,
			Side:   r.Side,
			Params: r.Params,
		})
	}
	return out
}

// newRegisterPolicyCmd: tx policy register-policy [policy.json]
func newRegisterPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-policy [policy.json]",
		Short: "Register a new policy from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			p, err := readPolicyJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterPolicy{
				Authority:   clientCtx.GetFromAddress().String(),
				Id:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Version:     p.Version,
				Rules:       policyRulesFromJSON(p.Rules),
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdatePolicyCmd: tx policy update-policy [policy.json]
func newUpdatePolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-policy [policy.json]",
		Short: "Update an existing policy from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			p, err := readPolicyJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdatePolicy{
				Authority:   clientCtx.GetFromAddress().String(),
				Id:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Version:     p.Version,
				Rules:       policyRulesFromJSON(p.Rules),
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
