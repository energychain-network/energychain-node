package types

import (
	"fmt"
	"strings"

	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// schedulerSelfTypePrefix is the proto package prefix every
// scheduler Msg's Any type_url starts with. We refuse payloads
// in this namespace at create / update time — see
// RejectSelfScheduler for the security rationale.
const schedulerSelfTypePrefix = "/energychain.scheduler.v1."

// RejectSelfScheduler bans payloads that target the scheduler
// module itself.
//
// The EndBlocker runs a payload inside a CacheContext and then
// commits the outer tick's accounting (fee debit, executions++,
// next_run_time, status) on the SAME job row. If a handler in
// the cache mutates that row (e.g. a scheduled MsgCancelJob /
// MsgPauseJob / MsgWithdrawJob targeting its own job), the outer
// SaveJob would clobber the handler's mutations, producing
// inconsistent state and silently nullifying the action.
//
// The simplest, fail-fast defense is to refuse self-scheduling
// up front — schedulers are an operational primitive, not a
// generic re-entry surface, and we don't lose meaningful product
// surface area by banning it.
func RejectSelfScheduler(any *cdctypes.Any) error {
	if any == nil {
		return nil
	}
	if strings.HasPrefix(any.TypeUrl, schedulerSelfTypePrefix) {
		return fmt.Errorf("payload type %q targets the scheduler module; self-scheduling is forbidden", any.TypeUrl)
	}
	return nil
}

func mustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("invalid bech32 %q: %w", addr, err)
	}
	return nil
}

// ValidatePayload runs the static checks that do not depend on
// runtime params: payload presence, type-url shape, and signer
// match with the job owner.
//
// Signer-match is the critical security gate: a scheduled job
// runs autonomously and never collects a fresh signature, so the
// payload's GetSigners() MUST already equal exactly the owner.
// Without this gate, any owner could schedule a tx that the
// scheduler would later execute under another account's identity.
func ValidatePayload(any *cdctypes.Any, owner string, registry cdctypes.InterfaceRegistry) error {
	if any == nil {
		return fmt.Errorf("payload must be set")
	}
	if any.TypeUrl == "" {
		return fmt.Errorf("payload type_url must be set")
	}
	if registry == nil {
		return nil
	}
	var msg sdk.Msg
	if err := registry.UnpackAny(any, &msg); err != nil {
		return fmt.Errorf("payload unpack: %w", err)
	}
	signers, _, err := signersOf(msg)
	if err != nil {
		return err
	}
	if len(signers) != 1 {
		return fmt.Errorf("payload must have exactly 1 signer (got %d)", len(signers))
	}
	if signers[0].String() != owner {
		return fmt.Errorf("payload signer %s != job owner %s", signers[0].String(), owner)
	}
	return nil
}

// signersOf reaches the legacy sdk.Msg.GetSigners() path through a
// type assertion. The optional interface keeps msgs.go free of the
// new tx-config plumbing required by the modern signing service.
func signersOf(msg sdk.Msg) ([]sdk.AccAddress, bool, error) {
	type legacy interface {
		GetSigners() []sdk.AccAddress
	}
	if l, ok := msg.(legacy); ok {
		return l.GetSigners(), true, nil
	}
	return nil, false, fmt.Errorf("payload type %T does not implement GetSigners()", msg)
}

// ---- CreateJob ----------------------------------------------------------

func (m *MsgCreateJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.Payload == nil || m.Payload.TypeUrl == "" {
		return fmt.Errorf("payload must be set")
	}
	if m.IntervalSeconds <= 0 {
		return fmt.Errorf("interval_seconds must be > 0")
	}
	if m.EndTime < 0 {
		return fmt.Errorf("end_time cannot be negative")
	}
	if m.StartTime < 0 {
		return fmt.Errorf("start_time cannot be negative")
	}
	if m.FeePerRun == 0 {
		return fmt.Errorf("fee_per_run must be > 0")
	}
	if m.InitialBudget < m.FeePerRun {
		return fmt.Errorf("initial_budget (%d) must be >= fee_per_run (%d)", m.InitialBudget, m.FeePerRun)
	}
	if m.FeeDenom != "" {
		if err := ValidateDenom(m.FeeDenom); err != nil {
			return err
		}
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgCreateJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgTopUpJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgTopUpJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgWithdrawJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	return nil
}
func (m *MsgWithdrawJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgPauseJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgPauseJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgResumeJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	return nil
}
func (m *MsgResumeJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgCancelJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateJob) ValidateBasic() error {
	if err := ValidateAddr("owner", m.Owner); err != nil {
		return err
	}
	if m.JobId == 0 {
		return fmt.Errorf("job_id must be > 0")
	}
	if m.IntervalSeconds < 0 {
		return fmt.Errorf("interval_seconds cannot be negative")
	}
	if m.EndTime < 0 {
		return fmt.Errorf("end_time cannot be negative")
	}
	return nil
}
func (m *MsgUpdateJob) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return fmt.Errorf("authority: %w", err)
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}
