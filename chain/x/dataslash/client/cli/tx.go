// Package cli wires the Cobra commands that need richer
// argument parsing than autocli can produce (e.g. enum string
// coercion or JSON inputs).
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/dataslash/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Dataslash module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterProviderCmd(),
		newUpdateProviderInfoCmd(),
		newReportInfractionCmd(),
		newJailProviderCmd(),
	)
	return cmd
}

func parseProviderRole(s string) (types.ProviderRole, error) {
	switch strings.ToUpper(s) {
	case "ORACLE":
		return types.ProviderRole_PROVIDER_ROLE_ORACLE, nil
	case "METER":
		return types.ProviderRole_PROVIDER_ROLE_METER, nil
	case "BRIDGE":
		return types.ProviderRole_PROVIDER_ROLE_BRIDGE, nil
	case "OTHER":
		return types.ProviderRole_PROVIDER_ROLE_OTHER, nil
	}
	return 0, fmt.Errorf("invalid role %q (want oracle|meter|bridge|other)", s)
}

func parseInfractionKind(s string) (types.InfractionKind, error) {
	switch strings.ToUpper(s) {
	case "MISREPORT":
		return types.InfractionKind_INFRACTION_KIND_MISREPORT, nil
	case "STALE":
		return types.InfractionKind_INFRACTION_KIND_STALE, nil
	case "MISSING_SIG", "MISSINGSIG":
		return types.InfractionKind_INFRACTION_KIND_MISSING_SIG, nil
	case "EQUIVOCATION":
		return types.InfractionKind_INFRACTION_KIND_EQUIVOCATION, nil
	case "UNREACHABLE":
		return types.InfractionKind_INFRACTION_KIND_UNREACHABLE, nil
	case "DISPUTE_RULING", "DISPUTERULING":
		return types.InfractionKind_INFRACTION_KIND_DISPUTE_RULING, nil
	case "OTHER":
		return types.InfractionKind_INFRACTION_KIND_OTHER, nil
	}
	return 0, fmt.Errorf("invalid kind %q", s)
}

type providerJSON struct {
	DID           string   `json:"did"`
	SignerAddress string   `json:"signer_address"`
	BondOwner     string   `json:"bond_owner"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	BondDenom     string   `json:"bond_denom"`
	Jurisdictions []string `json:"jurisdictions"`
}

func newRegisterProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-provider [provider.json]",
		Short: "Register a bonded data provider (authority)",
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
			var v providerJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			role, err := parseProviderRole(v.Role)
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterProvider{
				Authority:     cliCtx.GetFromAddress().String(),
				Did:           v.DID,
				SignerAddress: v.SignerAddress,
				BondOwner:     v.BondOwner,
				Name:          v.Name,
				Role:          role,
				BondDenom:     v.BondDenom,
				Jurisdictions: v.Jurisdictions,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type providerInfoUpdateJSON struct {
	NewName          string   `json:"new_name"`
	NewBondOwner     string   `json:"new_bond_owner"`
	NewJurisdictions []string `json:"new_jurisdictions"`
}

func newUpdateProviderInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-provider [provider-id] [update.json]",
		Short: "Update provider info (authority)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("provider_id: %w", err)
			}
			bz, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[1], err)
			}
			var v providerInfoUpdateJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[1], err)
			}
			msg := &types.MsgUpdateProviderInfo{
				Authority:        cliCtx.GetFromAddress().String(),
				ProviderId:       pid,
				NewName:          v.NewName,
				NewBondOwner:     v.NewBondOwner,
				NewJurisdictions: v.NewJurisdictions,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newReportInfractionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report [provider-id] [kind] [override-slash-bps] [evidence-uri] [evidence-hash] [reason] [dispute-id]",
		Short: "Authority reports a provider infraction",
		Args:  cobra.ExactArgs(7),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("provider_id: %w", err)
			}
			kind, err := parseInfractionKind(args[1])
			if err != nil {
				return err
			}
			ov, err := strconv.ParseUint(args[2], 10, 32)
			if err != nil {
				return fmt.Errorf("override_slash_bps: %w", err)
			}
			did, err := strconv.ParseUint(args[6], 10, 64)
			if err != nil {
				return fmt.Errorf("dispute_id: %w", err)
			}
			msg := &types.MsgReportInfraction{
				Actor:            cliCtx.GetFromAddress().String(),
				ProviderId:       pid,
				Kind:             kind,
				OverrideSlashBps: uint32(ov),
				EvidenceUri:      args[3],
				EvidenceHash:     args[4],
				Reason:           args[5],
				DisputeId:        did,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newJailProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jail [provider-id] [duration-seconds] [reason]",
		Short: "Authority jails a provider for duration seconds",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("provider_id: %w", err)
			}
			dur, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("duration_seconds: %w", err)
			}
			msg := &types.MsgJailProvider{
				Authority:       cliCtx.GetFromAddress().String(),
				ProviderId:      pid,
				DurationSeconds: dur,
				Reason:          args[2],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
