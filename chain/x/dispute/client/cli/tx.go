// Package cli wires the Cobra commands that are too rich for
// autocli — typically those that take JSON payloads or have
// enum arguments needing string ↔ enum coercion.
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

	"energychain/x/dispute/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Dispute module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterArbitratorCmd(),
		newOpenDisputeCmd(),
		newSubmitEvidenceCmd(),
		newAssignTribunalCmd(),
		newCastVoteCmd(),
		newFinalizeRulingCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

// ---- helpers -----------------------------------------------------------

func parseSubjectKind(s string) (types.SubjectKind, error) {
	switch strings.ToUpper(s) {
	case "ORACLE_TOPIC", "ORACLE":
		return types.SubjectKind_SUBJECT_KIND_ORACLE_TOPIC, nil
	case "METER_READING", "METER":
		return types.SubjectKind_SUBJECT_KIND_METER_READING, nil
	case "CONTRACT_SETTLEMENT", "CONTRACT":
		return types.SubjectKind_SUBJECT_KIND_CONTRACT_SETTLEMENT, nil
	case "MARKET_FILL", "MARKET":
		return types.SubjectKind_SUBJECT_KIND_MARKET_FILL, nil
	case "EAC_ISSUANCE", "EAC":
		return types.SubjectKind_SUBJECT_KIND_EAC_ISSUANCE, nil
	case "CARBON_RETIRE", "CARBON":
		return types.SubjectKind_SUBJECT_KIND_CARBON_RETIRE, nil
	case "OTHER":
		return types.SubjectKind_SUBJECT_KIND_OTHER, nil
	}
	return 0, fmt.Errorf("invalid subject_kind %q", s)
}

func parseVoteChoice(s string) (types.VoteChoice, error) {
	switch strings.ToUpper(s) {
	case "PLAINTIFF":
		return types.VoteChoice_VOTE_CHOICE_PLAINTIFF, nil
	case "RESPONDENT":
		return types.VoteChoice_VOTE_CHOICE_RESPONDENT, nil
	case "SPLIT":
		return types.VoteChoice_VOTE_CHOICE_SPLIT, nil
	case "NO_FAULT", "NOFAULT":
		return types.VoteChoice_VOTE_CHOICE_NO_FAULT, nil
	case "ABSTAIN":
		return types.VoteChoice_VOTE_CHOICE_ABSTAIN, nil
	}
	return 0, fmt.Errorf("invalid vote choice %q", s)
}

// ---- commands ----------------------------------------------------------

type arbitratorJSON struct {
	DID                 string   `json:"did"`
	Name                string   `json:"name"`
	SignerAddress       string   `json:"signer_address"`
	AccreditedStandards []string `json:"accredited_standards"`
	Jurisdictions       []string `json:"jurisdictions"`
}

func newRegisterArbitratorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-arbitrator [arbitrator.json]",
		Short: "Register an accredited arbitrator (authority)",
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
			var v arbitratorJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgRegisterArbitrator{
				Authority:           cliCtx.GetFromAddress().String(),
				Did:                 v.DID,
				Name:                v.Name,
				SignerAddress:       v.SignerAddress,
				AccreditedStandards: v.AccreditedStandards,
				Jurisdictions:       v.Jurisdictions,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type openDisputeJSON struct {
	Respondent             string `json:"respondent"`
	SubjectKind            string `json:"subject_kind"`
	SubjectRef             string `json:"subject_ref"`
	ClaimURI               string `json:"claim_uri"`
	ClaimHash              string `json:"claim_hash"`
	Memo                   string `json:"memo"`
	BondDenom              string `json:"bond_denom"`
	PlaintiffBond          uint64 `json:"plaintiff_bond"`
	RespondentBondRequired uint64 `json:"respondent_bond_required"`
}

func newOpenDisputeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [dispute.json]",
		Short: "Open a new dispute (plaintiff)",
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
			var v openDisputeJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			sk, err := parseSubjectKind(v.SubjectKind)
			if err != nil {
				return err
			}
			msg := &types.MsgOpenDispute{
				Plaintiff:              cliCtx.GetFromAddress().String(),
				Respondent:             v.Respondent,
				SubjectKind:            sk,
				SubjectRef:             v.SubjectRef,
				ClaimUri:               v.ClaimURI,
				ClaimHash:              v.ClaimHash,
				Memo:                   v.Memo,
				BondDenom:              v.BondDenom,
				PlaintiffBond:          v.PlaintiffBond,
				RespondentBondRequired: v.RespondentBondRequired,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newSubmitEvidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-evidence [dispute-id] [uri] [hash]",
		Short: "Submit evidence (party or tribunal arbitrator)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			did, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("dispute_id: %w", err)
			}
			msg := &types.MsgSubmitEvidence{
				Submitter: cliCtx.GetFromAddress().String(),
				DisputeId: did,
				Uri:       args[1],
				Hash:      args[2],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newAssignTribunalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assign-tribunal [dispute-id] [arb-id-1,arb-id-2,...]",
		Short: "Assign arbitrators to a dispute (authority)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			did, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("dispute_id: %w", err)
			}
			var ids []uint64
			for _, s := range strings.Split(args[1], ",") {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return fmt.Errorf("arbitrator id %q: %w", s, err)
				}
				ids = append(ids, id)
			}
			if len(ids) == 0 {
				return fmt.Errorf("no arbitrator ids")
			}
			msg := &types.MsgAssignTribunal{
				Authority:     cliCtx.GetFromAddress().String(),
				DisputeId:     did,
				ArbitratorIds: ids,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newCastVoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cast-vote [arbitrator-id] [dispute-id] [choice] [reason]",
		Short: "Cast a vote on a dispute (arbitrator signer)",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			aID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("arbitrator_id: %w", err)
			}
			dID, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("dispute_id: %w", err)
			}
			choice, err := parseVoteChoice(args[2])
			if err != nil {
				return err
			}
			msg := &types.MsgCastVote{
				VoterAddress: cliCtx.GetFromAddress().String(),
				ArbitratorId: aID,
				DisputeId:    dID,
				Choice:       choice,
				Reason:       args[3],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newFinalizeRulingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finalize [dispute-id] [override-slash-bps] [ruling-uri] [ruling-hash]",
		Short: "Finalize a dispute ruling (authority or anyone past deadline/quorum)",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			did, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("dispute_id: %w", err)
			}
			ob, err := strconv.ParseUint(args[1], 10, 32)
			if err != nil {
				return fmt.Errorf("override_slash_bps: %w", err)
			}
			msg := &types.MsgFinalizeRuling{
				Actor:            cliCtx.GetFromAddress().String(),
				DisputeId:        did,
				OverrideSlashBps: uint32(ob),
				RulingUri:        args[2],
				RulingHash:       args[3],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxArbitrators                uint32 `json:"max_arbitrators"`
	MaxOpenDisputes               uint32 `json:"max_open_disputes"`
	MaxEvidencePerDispute         uint32 `json:"max_evidence_per_dispute"`
	PanelSize                     uint32 `json:"panel_size"`
	QuorumBps                     uint32 `json:"quorum_bps"`
	RespondPeriodSeconds          int64  `json:"respond_period_seconds"`
	DeliberationPeriodSeconds     int64  `json:"deliberation_period_seconds"`
	MinPlaintiffBond              uint64 `json:"min_plaintiff_bond"`
	MaxPlaintiffBond              uint64 `json:"max_plaintiff_bond"`
	DefaultSlashBps               uint32 `json:"default_slash_bps"`
	MemoMaxLen                    uint32 `json:"memo_max_len"`
	ReasonMaxLen                  uint32 `json:"reason_max_len"`
	URIMaxLen                     uint32 `json:"uri_max_len"`
	MaxJurisdictionsPerArbitrator uint32 `json:"max_jurisdictions_per_arbitrator"`
	MaxStandardsPerArbitrator     uint32 `json:"max_standards_per_arbitrator"`
	MaxFinalizationsPerBlock      uint32 `json:"max_finalizations_per_block"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update module parameters (authority)",
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
					MaxArbitrators:                v.MaxArbitrators,
					MaxOpenDisputes:               v.MaxOpenDisputes,
					MaxEvidencePerDispute:         v.MaxEvidencePerDispute,
					PanelSize:                     v.PanelSize,
					QuorumBps:                     v.QuorumBps,
					RespondPeriodSeconds:          v.RespondPeriodSeconds,
					DeliberationPeriodSeconds:     v.DeliberationPeriodSeconds,
					MinPlaintiffBond:              v.MinPlaintiffBond,
					MaxPlaintiffBond:              v.MaxPlaintiffBond,
					DefaultSlashBps:               v.DefaultSlashBps,
					MemoMaxLen:                    v.MemoMaxLen,
					ReasonMaxLen:                  v.ReasonMaxLen,
					UriMaxLen:                     v.URIMaxLen,
					MaxJurisdictionsPerArbitrator: v.MaxJurisdictionsPerArbitrator,
					MaxStandardsPerArbitrator:     v.MaxStandardsPerArbitrator,
					MaxFinalizationsPerBlock:      v.MaxFinalizationsPerBlock,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
