// Package cli provides Cobra commands for the x/sanctions module. The
// nested-payload flows live here because autocli can't synthesise the
// SanctionEntry / ProposalAction shapes via positional flags.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/sanctions/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Sanctions module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterListCmd(),
		newUpdateListCmd(),
		newAddEntryCmd(),
		newProposeDeltaCmd(),
	)
	return cmd
}

// listJSON is the file format for register/update operations.
type listJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SourceURI     string `json:"source_uri"`
	Jurisdiction  string `json:"jurisdiction"`
	ListAuthority string `json:"list_authority"`
}

func readListJSON(path string) (listJSON, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return listJSON{}, fmt.Errorf("read %s: %w", path, err)
	}
	var l listJSON
	if err := json.Unmarshal(bz, &l); err != nil {
		return listJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return l, nil
}

// newRegisterListCmd: tx sanctions register-list [list.json]
func newRegisterListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-list [list.json]",
		Short: "Register a new sanctions list from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			l, err := readListJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterList{
				Authority:     cliCtx.GetFromAddress().String(),
				Id:            l.ID,
				Name:          l.Name,
				Description:   l.Description,
				SourceUri:     l.SourceURI,
				Jurisdiction:  l.Jurisdiction,
				ListAuthority: l.ListAuthority,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdateListCmd: tx sanctions update-list [list.json]
func newUpdateListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-list [list.json]",
		Short: "Update an existing sanctions list from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			l, err := readListJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateList{
				Authority:     cliCtx.GetFromAddress().String(),
				Id:            l.ID,
				Name:          l.Name,
				Description:   l.Description,
				SourceUri:     l.SourceURI,
				ListAuthority: l.ListAuthority,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// entryJSON mirrors SanctionEntry on the wire but uses a string for
// the SubjectKind enum so file authors don't have to memorise integers.
type entryJSON struct {
	ListID      string `json:"list_id"`
	Subject     string `json:"subject"`
	SubjectKind string `json:"subject_kind"` // "ADDRESS" | "DID"
	Program     string `json:"program"`
	Reason      string `json:"reason"`
	SourceRef   string `json:"source_ref"`
}

func parseSubjectKind(s string) (types.SubjectKind, error) {
	switch s {
	case "ADDRESS", "address", "":
		return types.SubjectKind_SUBJECT_KIND_ADDRESS, nil
	case "DID", "did":
		return types.SubjectKind_SUBJECT_KIND_DID, nil
	default:
		return 0, fmt.Errorf("unknown subject_kind %q (use ADDRESS or DID)", s)
	}
}

func entryFromJSON(j entryJSON) (types.SanctionEntry, error) {
	kind, err := parseSubjectKind(j.SubjectKind)
	if err != nil {
		return types.SanctionEntry{}, err
	}
	return types.SanctionEntry{
		ListId:      j.ListID,
		Subject:     j.Subject,
		SubjectKind: kind,
		Program:     j.Program,
		Reason:      j.Reason,
		SourceRef:   j.SourceRef,
	}, nil
}

// newAddEntryCmd: tx sanctions add-entry [entry.json]
func newAddEntryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-entry [entry.json]",
		Short: "Add a sanctions entry from a JSON file (governance OR list authority)",
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
			var j entryJSON
			if err := json.Unmarshal(bz, &j); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			entry, err := entryFromJSON(j)
			if err != nil {
				return err
			}
			msg := &types.MsgAddEntry{
				Authority: cliCtx.GetFromAddress().String(),
				Entry:     entry,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// proposalJSON encodes the propose+confirm payload. action.kind:
// 1=ADD, 2=REMOVE.
type proposalActionJSON struct {
	Kind  uint32    `json:"kind"`
	Entry entryJSON `json:"entry"`
}

type proposalJSON struct {
	ListID       string               `json:"list_id"`
	SourceURI    string               `json:"source_uri"`
	SourceDigest string               `json:"source_digest"`
	Actions      []proposalActionJSON `json:"actions"`
}

// newProposeDeltaCmd: tx sanctions propose-delta [proposal.json]
func newProposeDeltaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose-delta [proposal.json]",
		Short: "Submit an oracle-proposed sanctions delta from a JSON file",
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
			var j proposalJSON
			if err := json.Unmarshal(bz, &j); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			actions := make([]types.ProposalAction, 0, len(j.Actions))
			for i, a := range j.Actions {
				entry, err := entryFromJSON(a.Entry)
				if err != nil {
					return fmt.Errorf("action %d: %w", i, err)
				}
				if entry.ListId == "" {
					entry.ListId = j.ListID
				}
				kind := types.ProposalAction_Kind(a.Kind)
				if !types.ProposalActionKindValid(kind) {
					return fmt.Errorf("action %d: invalid kind %d", i, a.Kind)
				}
				actions = append(actions, types.ProposalAction{Kind: kind, Entry: entry})
			}
			msg := &types.MsgProposeDelta{
				Proposer:     cliCtx.GetFromAddress().String(),
				ListId:       j.ListID,
				Actions:      actions,
				SourceUri:    j.SourceURI,
				SourceDigest: j.SourceDigest,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// strconvU64 is a tiny helper kept here for any future positional uint
// arguments in custom commands. Unused today but cheap to retain.
var _ = strconv.ParseUint
