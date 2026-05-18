// Package cli wires the Cobra commands that are too rich for
// autocli (RegisterMember, OpenCycle, UpdateMemberStatus,
// UpdateParams). Simple commands flow via autocli.
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

	"energychain/x/clearing/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Clearing module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterMemberCmd(),
		newUpdateMemberStatusCmd(),
		newOpenCycleCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

type memberJSON struct {
	Address        string            `json:"address"`
	RequiredMargin map[string]uint64 `json:"required_margin"`
	Memo           string            `json:"memo"`
}

func newRegisterMemberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-member [member.json]",
		Short: "Register a clearing member (governance)",
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
			var v memberJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgRegisterMember{
				Authority:      cliCtx.GetFromAddress().String(),
				MemberAddress:  v.Address,
				RequiredMargin: v.RequiredMargin,
				Memo:           v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func parseMemberStatus(s string) (types.MemberStatus, error) {
	switch strings.ToUpper(s) {
	case "ACTIVE":
		return types.MemberStatus_MEMBER_STATUS_ACTIVE, nil
	case "SUSPENDED":
		return types.MemberStatus_MEMBER_STATUS_SUSPENDED, nil
	case "EXPELLED":
		return types.MemberStatus_MEMBER_STATUS_EXPELLED, nil
	}
	return types.MemberStatus_MEMBER_STATUS_UNSPECIFIED,
		fmt.Errorf("invalid status %q (ACTIVE|SUSPENDED|EXPELLED)", s)
}

func newUpdateMemberStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-member-status [member-id] [status] [reason]",
		Short: "Update a member's status (governance)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("member_id: %w", err)
			}
			st, err := parseMemberStatus(args[1])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateMemberStatus{
				Authority: cliCtx.GetFromAddress().String(),
				MemberId:  id,
				NewStatus: st,
				Reason:    args[2],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newOpenCycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open-cycle [memo]",
		Short: "Open a new settlement cycle (governance)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			memo := ""
			if len(args) == 1 {
				memo = args[0]
			}
			msg := &types.MsgOpenCycle{
				Authority: cliCtx.GetFromAddress().String(),
				Memo:      memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxOpenCycles          uint32 `json:"max_open_cycles"`
	MaxObligationsPerCycle uint32 `json:"max_obligations_per_cycle"`
	MaxMembers             uint32 `json:"max_members"`
	MemoMaxLen             uint32 `json:"memo_max_len"`
	SourceRefMaxLen        uint32 `json:"source_ref_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update module parameters (governance)",
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
					MaxOpenCycles:          v.MaxOpenCycles,
					MaxObligationsPerCycle: v.MaxObligationsPerCycle,
					MaxMembers:             v.MaxMembers,
					MemoMaxLen:             v.MemoMaxLen,
					SourceRefMaxLen:        v.SourceRefMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
