package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/rwa/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k: k} }

var _ types.MsgServer = (*msgServer)(nil)

// ---- helpers --------------------------------------------------------------

func (s msgServer) onlyAuthority(addr string) error {
	if addr != s.k.authority {
		return fmt.Errorf("expected authority %s, got %s", s.k.authority, addr)
	}
	return nil
}

func (s msgServer) requireIssuerAdmin(ctx context.Context, issuerID, admin string) (types.Issuer, error) {
	is, err := s.k.Issuers.Get(ctx, issuerID)
	if err != nil {
		return types.Issuer{}, fmt.Errorf("issuer %q not found", issuerID)
	}
	if is.Admin != admin {
		return types.Issuer{}, fmt.Errorf("only issuer admin (%s) may perform this action", is.Admin)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return types.Issuer{}, fmt.Errorf("issuer %q not ACTIVE (status=%s)", issuerID, is.Status)
	}
	return is, nil
}

func (s msgServer) requireIssuerAuthority(ctx context.Context, issuerID, authority string) (types.Issuer, error) {
	is, err := s.k.Issuers.Get(ctx, issuerID)
	if err != nil {
		return types.Issuer{}, fmt.Errorf("issuer %q not found", issuerID)
	}
	if is.Authority != authority {
		return types.Issuer{}, fmt.Errorf("only issuer authority (%s) may perform this action", is.Authority)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return types.Issuer{}, fmt.Errorf("issuer %q not ACTIVE (status=%s)", issuerID, is.Status)
	}
	return is, nil
}

func (s msgServer) recipientSanctionsCheck(ctx context.Context, params types.Params, addr string) error {
	if !params.RequireSanctionsClear || s.k.SanctionsHook() == nil {
		return nil
	}
	if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), addr) {
		return fmt.Errorf("address %s is on the sanctions list", addr)
	}
	return nil
}

// ---- Issuer ---------------------------------------------------------------

func (s msgServer) RegisterIssuer(ctx context.Context, m *types.MsgRegisterIssuer) (*types.MsgRegisterIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if has, err := s.k.Issuers.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("issuer %q already registered", m.Id)
	}
	count, err := s.k.CountIssuers(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxIssuers {
		return nil, fmt.Errorf("max_issuers reached (%d)", params.MaxIssuers)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	is := types.Issuer{
		Id:          m.Id,
		Did:         m.Did,
		DisplayName: m.DisplayName,
		Status:      types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		Authority:   m.IssuerAuthority,
		Admin:       m.IssuerAdmin,
		CreatedBy:   m.Authority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.k.Issuers.Set(ctx, m.Id, is); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, m.Id, "register_issuer", m.Authority, m.IssuerAdmin,
		fmt.Sprintf("did=%s authority=%s", m.Did, m.IssuerAuthority))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_issuer_registered",
		[2]string{"issuer_id", m.Id},
		[2]string{"did", m.Did},
		[2]string{"authority", m.IssuerAuthority},
	)
	return &types.MsgRegisterIssuerResponse{}, nil
}

func (s msgServer) UpdateIssuer(ctx context.Context, m *types.MsgUpdateIssuer) (*types.MsgUpdateIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	is, err := s.k.Issuers.Get(ctx, m.Id)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", m.Id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return nil, fmt.Errorf("issuer %q is revoked and cannot be updated", m.Id)
	}
	if m.DisplayName != "" {
		is.DisplayName = m.DisplayName
	}
	if m.IssuerAuthority != "" {
		is.Authority = m.IssuerAuthority
	}
	if m.IssuerAdmin != "" {
		is.Admin = m.IssuerAdmin
	}
	is.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Issuers.Set(ctx, m.Id, is); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, m.Id, "update_issuer", m.Authority, "", "")
	emit(sdk.UnwrapSDKContext(ctx), "rwa_issuer_updated", [2]string{"issuer_id", m.Id})
	return &types.MsgUpdateIssuerResponse{}, nil
}

func (s msgServer) setIssuerStatus(ctx context.Context, authority, id, reason string, st types.IssuerStatus) error {
	if err := s.onlyAuthority(authority); err != nil {
		return err
	}
	is, err := s.k.Issuers.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("issuer %q not found", id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED && st != types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return fmt.Errorf("issuer %q is revoked", id)
	}
	is.Status = st
	is.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Issuers.Set(ctx, id, is); err != nil {
		return err
	}
	s.k.recordAudit(ctx, 0, id, "issuer_status", authority, "",
		fmt.Sprintf("status=%s reason=%s", st, reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_issuer_status",
		[2]string{"issuer_id", id},
		[2]string{"status", st.String()},
		[2]string{"reason", reason},
	)
	return nil
}

func (s msgServer) SuspendIssuer(ctx context.Context, m *types.MsgSuspendIssuer) (*types.MsgSuspendIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setIssuerStatus(ctx, m.Authority, m.Id, m.Reason, types.IssuerStatus_ISSUER_STATUS_SUSPENDED); err != nil {
		return nil, err
	}
	return &types.MsgSuspendIssuerResponse{}, nil
}

func (s msgServer) RevokeIssuer(ctx context.Context, m *types.MsgRevokeIssuer) (*types.MsgRevokeIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setIssuerStatus(ctx, m.Authority, m.Id, m.Reason, types.IssuerStatus_ISSUER_STATUS_REVOKED); err != nil {
		return nil, err
	}
	return &types.MsgRevokeIssuerResponse{}, nil
}

// ---- Token lifecycle ------------------------------------------------------

func (s msgServer) CreateToken(ctx context.Context, m *types.MsgCreateToken) (*types.MsgCreateTokenResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	is, err := s.requireIssuerAdmin(ctx, m.IssuerId, m.Admin)
	if err != nil {
		return nil, err
	}
	if has, err := s.k.TokenBySymbol.Has(ctx, m.Symbol); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("token symbol %q already used", m.Symbol)
	}
	if s.k.stablecoin != nil {
		if !s.k.stablecoin.HasDenom(sdk.UnwrapSDKContext(ctx), m.SettlementDenom) {
			return nil, fmt.Errorf("settlement_denom %q not registered with x/stablecoin", m.SettlementDenom)
		}
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.k.CountTokens(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxTokens {
		return nil, fmt.Errorf("max_tokens reached (%d)", params.MaxTokens)
	}
	id, err := s.k.NextTokenID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	t := types.Token{
		Id:                     id,
		IssuerId:               m.IssuerId,
		Symbol:                 m.Symbol,
		DisplayName:            m.DisplayName,
		Decimals:               m.Decimals,
		AssetClass:             m.AssetClass,
		Jurisdiction:           m.Jurisdiction,
		PolicyId:               m.PolicyId,
		SettlementDenom:        m.SettlementDenom,
		PerHolderCap:           m.PerHolderCap,
		TotalSupplyCap:         m.TotalSupplyCap,
		RedemptionRate:         m.RedemptionRate,
		RedemptionDelaySeconds: m.RedemptionDelaySeconds,
		RequireKycHolders:      m.RequireKycHolders,
		Status:                 types.TokenStatus_TOKEN_STATUS_ACTIVE,
		CreatedBy:              m.Admin,
		CreatedAt:              now,
		UpdatedAt:              now,
		TotalSupply:            0,
	}
	if err := s.k.setToken(ctx, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, id, is.Id, "create_token", m.Admin, "",
		fmt.Sprintf("symbol=%s asset_class=%s jurisdiction=%s settlement=%s",
			m.Symbol, m.AssetClass, m.Jurisdiction, m.SettlementDenom))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_token_created",
		[2]string{"token_id", strconv.FormatUint(id, 10)},
		[2]string{"issuer_id", m.IssuerId},
		[2]string{"symbol", m.Symbol},
	)
	return &types.MsgCreateTokenResponse{TokenId: id}, nil
}

func (s msgServer) UpdateToken(ctx context.Context, m *types.MsgUpdateToken) (*types.MsgUpdateTokenResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_TERMINATED {
		return nil, fmt.Errorf("token %d is terminated", m.TokenId)
	}
	if m.DisplayName != "" {
		t.DisplayName = m.DisplayName
	}
	if m.PolicyId != "" {
		t.PolicyId = m.PolicyId
	}
	if m.PerHolderCap != 0 {
		t.PerHolderCap = m.PerHolderCap
	}
	if m.RedemptionRate != 0 {
		t.RedemptionRate = m.RedemptionRate
	}
	if m.RedemptionDelaySeconds != 0 {
		t.RedemptionDelaySeconds = m.RedemptionDelaySeconds
	}
	if m.SetKycFlag {
		t.RequireKycHolders = m.RequireKycHolders
	}
	t.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, m.TokenId, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.TokenId, t.IssuerId, "update_token", m.Admin, "", "")
	emit(sdk.UnwrapSDKContext(ctx), "rwa_token_updated", [2]string{"token_id", strconv.FormatUint(m.TokenId, 10)})
	return &types.MsgUpdateTokenResponse{}, nil
}

func (s msgServer) setTokenStatus(ctx context.Context, admin string, tokenID uint64, reason string, st types.TokenStatus) error {
	t, err := s.k.Tokens.Get(ctx, tokenID)
	if err != nil {
		return fmt.Errorf("token %d not found", tokenID)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, admin); err != nil {
		return err
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_TERMINATED && st != types.TokenStatus_TOKEN_STATUS_TERMINATED {
		return fmt.Errorf("token %d is terminated", tokenID)
	}
	t.Status = st
	t.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, tokenID, t); err != nil {
		return err
	}
	s.k.recordAudit(ctx, tokenID, t.IssuerId, "token_status", admin, "",
		fmt.Sprintf("status=%s reason=%s", st, reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_token_status",
		[2]string{"token_id", strconv.FormatUint(tokenID, 10)},
		[2]string{"status", st.String()},
		[2]string{"reason", reason},
	)
	return nil
}

func (s msgServer) PauseToken(ctx context.Context, m *types.MsgPauseToken) (*types.MsgPauseTokenResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setTokenStatus(ctx, m.Admin, m.TokenId, m.Reason, types.TokenStatus_TOKEN_STATUS_PAUSED); err != nil {
		return nil, err
	}
	return &types.MsgPauseTokenResponse{}, nil
}

func (s msgServer) UnpauseToken(ctx context.Context, m *types.MsgUnpauseToken) (*types.MsgUnpauseTokenResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setTokenStatus(ctx, m.Admin, m.TokenId, m.Reason, types.TokenStatus_TOKEN_STATUS_ACTIVE); err != nil {
		return nil, err
	}
	return &types.MsgUnpauseTokenResponse{}, nil
}

func (s msgServer) TerminateToken(ctx context.Context, m *types.MsgTerminateToken) (*types.MsgTerminateTokenResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setTokenStatus(ctx, m.Admin, m.TokenId, m.Reason, types.TokenStatus_TOKEN_STATUS_TERMINATED); err != nil {
		return nil, err
	}
	return &types.MsgTerminateTokenResponse{}, nil
}

// ---- Account flags --------------------------------------------------------

func (s msgServer) SetAccountFlags(ctx context.Context, m *types.MsgSetAccountFlags) (*types.MsgSetAccountFlagsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	f := types.AccountFlags{
		TokenId:      m.TokenId,
		Account:      m.Account,
		KycCleared:   m.KycCleared,
		Accredited:   m.Accredited,
		Jurisdiction: m.Jurisdiction,
		UpdatedAt:    now,
	}
	if err := s.k.AccountFlags.Set(ctx, collections.Join(m.TokenId, m.Account), f); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.TokenId, t.IssuerId, "set_account_flags", m.Admin, m.Account,
		fmt.Sprintf("kyc=%v accredited=%v jurisdiction=%s", m.KycCleared, m.Accredited, m.Jurisdiction))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_account_flags_set",
		[2]string{"token_id", strconv.FormatUint(m.TokenId, 10)},
		[2]string{"account", m.Account},
		[2]string{"kyc_cleared", strconv.FormatBool(m.KycCleared)},
	)
	return &types.MsgSetAccountFlagsResponse{}, nil
}

// ---- Mint / burn ----------------------------------------------------------

func (s msgServer) Mint(ctx context.Context, m *types.MsgMint) (*types.MsgMintResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_TERMINATED {
		return nil, fmt.Errorf("token %d is terminated", m.TokenId)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, m.Recipient); err != nil {
		return nil, err
	}
	if t.RequireKycHolders {
		if err := s.k.requireKYC(ctx, t.Id, m.Recipient); err != nil {
			return nil, err
		}
	}
	if err := s.k.requirePerHolderCap(ctx, t, m.Recipient, m.Amount); err != nil {
		return nil, err
	}
	newSupply, err := types.SafeAdd(t.TotalSupply, m.Amount)
	if err != nil {
		return nil, err
	}
	if t.TotalSupplyCap != 0 && newSupply > t.TotalSupplyCap {
		return nil, fmt.Errorf("total_supply_cap exceeded: %d > %d", newSupply, t.TotalSupplyCap)
	}
	if err := s.k.creditBalance(ctx, t.Id, m.Recipient, m.Amount); err != nil {
		return nil, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "mint", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("amount=%d new_supply=%d memo=%s", m.Amount, newSupply, m.Memo))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_minted",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"recipient", m.Recipient},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
		[2]string{"new_supply", strconv.FormatUint(newSupply, 10)},
	)
	return &types.MsgMintResponse{NewSupply: newSupply}, nil
}

func (s msgServer) Burn(ctx context.Context, m *types.MsgBurn) (*types.MsgBurnResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if err := s.k.debitBalance(ctx, t.Id, m.From, m.Amount); err != nil {
		return nil, err
	}
	newSupply, err := types.SafeSub(t.TotalSupply, m.Amount)
	if err != nil {
		return nil, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "burn", m.IssuerAuthority, m.From,
		fmt.Sprintf("amount=%d new_supply=%d reason=%s", m.Amount, newSupply, m.Reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_burned",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"from", m.From},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
		[2]string{"new_supply", strconv.FormatUint(newSupply, 10)},
	)
	return &types.MsgBurnResponse{NewSupply: newSupply}, nil
}

// ---- Transfer / ForceTransfer --------------------------------------------

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if err := s.k.transferCompliance(ctx, t, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	_, _, _, transferable, err := s.k.transferable(ctx, t.Id, m.From, now)
	if err != nil {
		return nil, err
	}
	if transferable < m.Amount {
		return nil, fmt.Errorf("insufficient transferable balance: have %d, need %d (frozen / lockups apply)", transferable, m.Amount)
	}
	if err := s.k.requirePerHolderCap(ctx, t, m.To, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.debitBalance(ctx, t.Id, m.From, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, t.Id, m.To, m.Amount); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "rwa_transfer",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"from", m.From},
		[2]string{"to", m.To},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
	)
	return &types.MsgTransferResponse{}, nil
}

// ForceTransfer is the issuer-authority recovery path. It bypasses
// the transferable check (intentionally — the issuer can move locked
// / frozen units, e.g. to a court-ordered receiver) but still
// enforces sanctions on the receiver and the per-holder cap so a
// recovery cannot itself blow up holder limits.
func (s msgServer) ForceTransfer(ctx context.Context, m *types.MsgForceTransfer) (*types.MsgForceTransferResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_TERMINATED {
		return nil, fmt.Errorf("token %d is terminated", m.TokenId)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, m.To); err != nil {
		return nil, err
	}
	// KYC still applies on ForceTransfer when the token requires it.
	// The issuer authority can move units around for recovery, but
	// must not create non-compliant holdings; clearing the receiver's
	// flag first is the explicit one-step that should precede a
	// court-ordered transfer to a custodian.
	if t.RequireKycHolders {
		if err := s.k.requireKYC(ctx, t.Id, m.To); err != nil {
			return nil, err
		}
	}
	if err := s.k.requirePerHolderCap(ctx, t, m.To, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.debitBalance(ctx, t.Id, m.From, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, t.Id, m.To, m.Amount); err != nil {
		return nil, err
	}
	// Frozen amount may now exceed remaining balance; trim down.
	if cur, err := s.k.GetFrozen(ctx, t.Id, m.From); err != nil {
		return nil, err
	} else if cur > 0 {
		bal, err := s.k.GetBalance(ctx, t.Id, m.From)
		if err != nil {
			return nil, err
		}
		if cur > bal {
			f, _ := s.k.Frozen.Get(ctx, collections.Join(t.Id, m.From))
			f.Amount = bal
			f.FrozenAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
			if bal == 0 {
				if err := s.k.Frozen.Remove(ctx, collections.Join(t.Id, m.From)); err != nil {
					return nil, err
				}
			} else {
				if err := s.k.Frozen.Set(ctx, collections.Join(t.Id, m.From), f); err != nil {
					return nil, err
				}
			}
		}
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "force_transfer", m.IssuerAuthority, m.From,
		fmt.Sprintf("to=%s amount=%d reason=%s", m.To, m.Amount, m.Reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_force_transfer",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"from", m.From},
		[2]string{"to", m.To},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
		[2]string{"reason", m.Reason},
	)
	return &types.MsgForceTransferResponse{}, nil
}

// ---- Freeze / lockup ------------------------------------------------------

// SetFrozenBalance is total-replace on the (token, account) pair.
// Setting amount=0 clears the row. Issuer-authority only; audit-logged.
func (s msgServer) SetFrozenBalance(ctx context.Context, m *types.MsgSetFrozenBalance) (*types.MsgSetFrozenBalanceResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	bal, err := s.k.GetBalance(ctx, t.Id, m.Account)
	if err != nil {
		return nil, err
	}
	if m.Amount > bal {
		return nil, fmt.Errorf("frozen amount %d exceeds balance %d", m.Amount, bal)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	key := collections.Join(t.Id, m.Account)
	if m.Amount == 0 {
		if err := s.k.Frozen.Remove(ctx, key); err != nil {
			return nil, err
		}
	} else {
		f := types.FrozenBalance{
			TokenId:  t.Id,
			Account:  m.Account,
			Amount:   m.Amount,
			Reason:   m.Reason,
			FrozenBy: m.IssuerAuthority,
			FrozenAt: now,
		}
		if err := s.k.Frozen.Set(ctx, key, f); err != nil {
			return nil, err
		}
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "set_frozen", m.IssuerAuthority, m.Account,
		fmt.Sprintf("amount=%d reason=%s", m.Amount, m.Reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_frozen_set",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"account", m.Account},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
	)
	return &types.MsgSetFrozenBalanceResponse{}, nil
}

func (s msgServer) AddLockup(ctx context.Context, m *types.MsgAddLockup) (*types.MsgAddLockupResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if m.UnlockTime <= now {
		return nil, fmt.Errorf("unlock_time %d is not in the future (now=%d)", m.UnlockTime, now)
	}
	if m.UnlockTime-now > types.MaxLockupHorizon {
		return nil, fmt.Errorf("unlock_time exceeds max horizon (%d > now+%d)", m.UnlockTime, types.MaxLockupHorizon)
	}
	// Prune matured rows before counting / inserting so the per-holder
	// row cap reflects active lockups, not lifetime adds.
	if _, err := s.k.PruneMaturedLockups(ctx, t.Id, m.Account, now); err != nil {
		return nil, err
	}
	bal, err := s.k.GetBalance(ctx, t.Id, m.Account)
	if err != nil {
		return nil, err
	}
	frozen, err := s.k.GetFrozen(ctx, t.Id, m.Account)
	if err != nil {
		return nil, err
	}
	unmaturedLocks, _, rowCount, err := s.k.SumLocked(ctx, t.Id, m.Account, now)
	if err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if rowCount >= params.MaxLockupsPerHolder {
		return nil, fmt.Errorf("max_lockups_per_holder reached (%d)", params.MaxLockupsPerHolder)
	}
	wouldReserve, err := types.SafeAdd(frozen, unmaturedLocks)
	if err != nil {
		return nil, err
	}
	wouldReserve, err = types.SafeAdd(wouldReserve, m.Amount)
	if err != nil {
		return nil, err
	}
	if wouldReserve > bal {
		return nil, fmt.Errorf("lockup would over-reserve balance (frozen+locks+new=%d > balance=%d)", wouldReserve, bal)
	}
	id, err := s.k.NextLockupID(ctx)
	if err != nil {
		return nil, err
	}
	l := types.Lockup{
		Id:         id,
		TokenId:    t.Id,
		Account:    m.Account,
		Amount:     m.Amount,
		UnlockTime: m.UnlockTime,
		Reason:     m.Reason,
		CreatedBy:  m.IssuerAuthority,
		CreatedAt:  now,
	}
	if err := s.k.Lockups.Set(ctx, collections.Join3(t.Id, m.Account, id), l); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "add_lockup", m.IssuerAuthority, m.Account,
		fmt.Sprintf("amount=%d unlock=%d reason=%s", m.Amount, m.UnlockTime, m.Reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_lockup_added",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"account", m.Account},
		[2]string{"lockup_id", strconv.FormatUint(id, 10)},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
		[2]string{"unlock_time", strconv.FormatInt(m.UnlockTime, 10)},
	)
	return &types.MsgAddLockupResponse{LockupId: id}, nil
}

// ---- Snapshot / Distribution ---------------------------------------------

// TakeSnapshot enumerates the (token, *) balance keyset and writes
// per-holder snapshot rows. Refuses if the holder count exceeds the
// configured cap so a misuse cannot blow up state in one tx.
func (s msgServer) TakeSnapshot(ctx context.Context, m *types.MsgTakeSnapshot) (*types.MsgTakeSnapshotResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.k.NextSnapshotID(ctx)
	if err != nil {
		return nil, err
	}
	rng := collections.NewPrefixedPairRange[uint64, string](t.Id)
	var (
		count uint32
		sum   uint64
	)
	if err := s.k.Balances.Walk(ctx, rng, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		if v == 0 {
			return false, nil
		}
		if count >= params.MaxHoldersPerSnapshot {
			return true, fmt.Errorf("snapshot would exceed max_holders_per_snapshot (%d)", params.MaxHoldersPerSnapshot)
		}
		count++
		next, err := types.SafeAdd(sum, v)
		if err != nil {
			return true, err
		}
		sum = next
		if err := s.k.SnapshotBalances.Set(ctx, collections.Join(id, key.K2()), v); err != nil {
			return true, err
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	meta := types.SnapshotMeta{
		Id:          id,
		TokenId:     t.Id,
		Height:      height,
		Time:        now,
		TotalSupply: sum,
		HolderCount: uint64(count),
		TakenBy:     m.Admin,
	}
	if err := s.k.Snapshots.Set(ctx, id, meta); err != nil {
		return nil, err
	}
	if err := s.k.SnapshotByToken.Set(ctx, collections.Join(t.Id, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "take_snapshot", m.Admin, "",
		fmt.Sprintf("snapshot_id=%d holders=%d total=%d", id, count, sum))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_snapshot_taken",
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"snapshot_id", strconv.FormatUint(id, 10)},
		[2]string{"holders", strconv.FormatUint(uint64(count), 10)},
		[2]string{"total_supply", strconv.FormatUint(sum, 10)},
	)
	return &types.MsgTakeSnapshotResponse{
		SnapshotId:  id,
		HolderCount: uint64(count),
		TotalSupply: sum,
	}, nil
}

func (s msgServer) CreateDistribution(ctx context.Context, m *types.MsgCreateDistribution) (*types.MsgCreateDistributionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	snap, err := s.k.Snapshots.Get(ctx, m.SnapshotId)
	if err != nil {
		return nil, fmt.Errorf("snapshot %d not found", m.SnapshotId)
	}
	if snap.TokenId != t.Id {
		return nil, fmt.Errorf("snapshot %d belongs to token %d, not %d", m.SnapshotId, snap.TokenId, t.Id)
	}
	if snap.TotalSupply == 0 {
		return nil, fmt.Errorf("snapshot %d has zero total_supply; nothing to distribute", m.SnapshotId)
	}
	rate, err := types.MulDivFloor(m.TotalAmount, types.DistributionRateScale, snap.TotalSupply)
	if err != nil {
		return nil, fmt.Errorf("distribution rate computation: %w", err)
	}
	if rate == 0 {
		return nil, fmt.Errorf("distribution rate rounds to zero; total_amount %d too small for supply %d",
			m.TotalAmount, snap.TotalSupply)
	}
	id, err := s.k.NextDistributionID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	d := types.Distribution{
		Id:               id,
		TokenId:          t.Id,
		SnapshotId:       m.SnapshotId,
		SettlementDenom:  t.SettlementDenom,
		TotalAmount:      m.TotalAmount,
		PerUnitRate:      rate,
		FundedAmount:     0,
		ClaimedAmount:    0,
		Status:           types.DistributionStatus_DISTRIBUTION_STATUS_CREATED,
		Memo:             m.Memo,
		CreatedAt:        now,
	}
	if err := s.k.Distributions.Set(ctx, id, d); err != nil {
		return nil, err
	}
	if err := s.k.DistributionByToken.Set(ctx, collections.Join(t.Id, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "create_distribution", m.Admin, "",
		fmt.Sprintf("dist_id=%d snap=%d total=%d rate=%d denom=%s", id, m.SnapshotId, m.TotalAmount, rate, t.SettlementDenom))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_distribution_created",
		[2]string{"distribution_id", strconv.FormatUint(id, 10)},
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"snapshot_id", strconv.FormatUint(m.SnapshotId, 10)},
		[2]string{"total_amount", strconv.FormatUint(m.TotalAmount, 10)},
		[2]string{"per_unit_rate", strconv.FormatUint(rate, 10)},
	)
	return &types.MsgCreateDistributionResponse{DistributionId: id, PerUnitRate: rate}, nil
}

func (s msgServer) FundDistribution(ctx context.Context, m *types.MsgFundDistribution) (*types.MsgFundDistributionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	d, err := s.k.Distributions.Get(ctx, m.DistributionId)
	if err != nil {
		return nil, fmt.Errorf("distribution %d not found", m.DistributionId)
	}
	t, err := s.k.Tokens.Get(ctx, d.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", d.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if d.Status == types.DistributionStatus_DISTRIBUTION_STATUS_FINALIZED {
		return nil, fmt.Errorf("distribution %d already finalized", d.Id)
	}
	if d.FundedAmount > 0 {
		return nil, fmt.Errorf("distribution %d already funded", d.Id)
	}
	// Defense-in-depth: refuse to relay funds out of a sanctioned
	// issuer authority's stablecoin balance even though x/stablecoin's
	// own freeze gate is bypassed by MoveBalance.
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if s.k.stablecoin == nil {
		return nil, fmt.Errorf("stablecoin keeper not wired; cannot fund distribution")
	}
	if err := s.k.stablecoin.Move(sdk.UnwrapSDKContext(ctx), d.SettlementDenom, m.IssuerAuthority, DistributionPoolAccount, d.TotalAmount); err != nil {
		return nil, fmt.Errorf("settlement payer move failed: %w", err)
	}
	d.FundedAmount = d.TotalAmount
	d.Status = types.DistributionStatus_DISTRIBUTION_STATUS_FUNDED
	d.Funder = m.IssuerAuthority
	d.FundedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Distributions.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "fund_distribution", m.IssuerAuthority, "",
		fmt.Sprintf("dist_id=%d funded=%d", d.Id, d.FundedAmount))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_distribution_funded",
		[2]string{"distribution_id", strconv.FormatUint(d.Id, 10)},
		[2]string{"funded_amount", strconv.FormatUint(d.FundedAmount, 10)},
		[2]string{"funder", m.IssuerAuthority},
	)
	return &types.MsgFundDistributionResponse{}, nil
}

func (s msgServer) ClaimDistribution(ctx context.Context, m *types.MsgClaimDistribution) (*types.MsgClaimDistributionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	d, err := s.k.Distributions.Get(ctx, m.DistributionId)
	if err != nil {
		return nil, fmt.Errorf("distribution %d not found", m.DistributionId)
	}
	if d.Status == types.DistributionStatus_DISTRIBUTION_STATUS_CREATED {
		return nil, fmt.Errorf("distribution %d not yet funded", d.Id)
	}
	if d.Status == types.DistributionStatus_DISTRIBUTION_STATUS_FINALIZED {
		return nil, fmt.Errorf("distribution %d finalized; no further claims accepted", d.Id)
	}
	claimKey := collections.Join(d.Id, m.Claimer)
	if has, err := s.k.DistributionClaims.Has(ctx, claimKey); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("distribution %d already claimed by %s", d.Id, m.Claimer)
	}
	snapBal, err := s.k.SnapshotBalances.Get(ctx, collections.Join(d.SnapshotId, m.Claimer))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, fmt.Errorf("claimer %s held no balance at snapshot %d", m.Claimer, d.SnapshotId)
		}
		return nil, err
	}
	if snapBal == 0 {
		return nil, fmt.Errorf("claimer %s held zero at snapshot %d", m.Claimer, d.SnapshotId)
	}
	amount, err := types.MulDivFloor(snapBal, d.PerUnitRate, types.DistributionRateScale)
	if err != nil {
		return nil, err
	}
	if amount == 0 {
		return nil, fmt.Errorf("computed amount is zero for snapshot_balance=%d", snapBal)
	}
	// Cap claimed_amount at funded_amount so dust accumulation cannot
	// overdraw the pool. The remainder (if any) is swept on Finalize.
	remaining, err := types.SafeSub(d.FundedAmount, d.ClaimedAmount)
	if err != nil {
		return nil, err
	}
	if amount > remaining {
		amount = remaining
		if amount == 0 {
			return nil, fmt.Errorf("distribution pool exhausted")
		}
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, m.Claimer); err != nil {
		return nil, err
	}
	if s.k.stablecoin == nil {
		return nil, fmt.Errorf("stablecoin keeper not wired")
	}
	if err := s.k.stablecoin.Move(sdk.UnwrapSDKContext(ctx), d.SettlementDenom, DistributionPoolAccount, m.Claimer, amount); err != nil {
		return nil, fmt.Errorf("payout move failed: %w", err)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.DistributionClaims.Set(ctx, claimKey, types.DistributionClaim{
		DistributionId: d.Id,
		Account:        m.Claimer,
		Amount:         amount,
		ClaimedAt:      now,
	}); err != nil {
		return nil, err
	}
	d.ClaimedAmount, err = types.SafeAdd(d.ClaimedAmount, amount)
	if err != nil {
		return nil, err
	}
	if err := s.k.Distributions.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "rwa_distribution_claimed",
		[2]string{"distribution_id", strconv.FormatUint(d.Id, 10)},
		[2]string{"claimer", m.Claimer},
		[2]string{"amount", strconv.FormatUint(amount, 10)},
	)
	return &types.MsgClaimDistributionResponse{Amount: amount}, nil
}

func (s msgServer) FinalizeDistribution(ctx context.Context, m *types.MsgFinalizeDistribution) (*types.MsgFinalizeDistributionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	d, err := s.k.Distributions.Get(ctx, m.DistributionId)
	if err != nil {
		return nil, fmt.Errorf("distribution %d not found", m.DistributionId)
	}
	t, err := s.k.Tokens.Get(ctx, d.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", d.TokenId)
	}
	if _, err := s.requireIssuerAdmin(ctx, t.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if d.Status != types.DistributionStatus_DISTRIBUTION_STATUS_FUNDED {
		return nil, fmt.Errorf("distribution %d not in FUNDED state (current=%s)", d.Id, d.Status)
	}
	swept, err := types.SafeSub(d.FundedAmount, d.ClaimedAmount)
	if err != nil {
		return nil, err
	}
	if swept > 0 {
		if s.k.stablecoin == nil {
			return nil, fmt.Errorf("stablecoin keeper not wired")
		}
		// Sweep unclaimed funds back to the issuer authority — the
		// original funder. Capturing the funder up front during
		// FundDistribution ensures we don't accidentally credit a
		// rotated authority.
		if err := s.k.stablecoin.Move(sdk.UnwrapSDKContext(ctx), d.SettlementDenom, DistributionPoolAccount, d.Funder, swept); err != nil {
			return nil, fmt.Errorf("sweep move failed: %w", err)
		}
	}
	d.Status = types.DistributionStatus_DISTRIBUTION_STATUS_FINALIZED
	d.FinalizedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Distributions.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "finalize_distribution", m.Admin, d.Funder,
		fmt.Sprintf("dist_id=%d swept=%d", d.Id, swept))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_distribution_finalized",
		[2]string{"distribution_id", strconv.FormatUint(d.Id, 10)},
		[2]string{"swept_back", strconv.FormatUint(swept, 10)},
	)
	return &types.MsgFinalizeDistributionResponse{SweptBack: swept}, nil
}

// ---- Redemption -----------------------------------------------------------

func (s msgServer) RequestRedemption(ctx context.Context, m *types.MsgRequestRedemption) (*types.MsgRequestRedemptionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	t, err := s.k.Tokens.Get(ctx, m.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", m.TokenId)
	}
	if t.RedemptionRate == 0 {
		return nil, fmt.Errorf("token %d has redemption disabled (rate=0)", t.Id)
	}
	if t.Status == types.TokenStatus_TOKEN_STATUS_PAUSED {
		return nil, fmt.Errorf("token %d is paused", t.Id)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireSanctionsClear && s.k.SanctionsHook() != nil {
		if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), m.Holder) {
			return nil, fmt.Errorf("holder %s is sanctioned", m.Holder)
		}
	}
	pendingCount, err := s.k.CountPendingRedemptionsForHolder(ctx, m.Holder)
	if err != nil {
		return nil, err
	}
	if pendingCount >= params.MaxPendingRedemptionsPerHolder {
		return nil, fmt.Errorf("max_pending_redemptions_per_holder reached (%d)", params.MaxPendingRedemptionsPerHolder)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	_, _, _, transferable, err := s.k.transferable(ctx, t.Id, m.Holder, now)
	if err != nil {
		return nil, err
	}
	if transferable < m.Amount {
		return nil, fmt.Errorf("insufficient transferable balance to redeem: have %d, need %d", transferable, m.Amount)
	}
	settlement, err := types.MulDivFloor(m.Amount, t.RedemptionRate, types.RedemptionRateScale)
	if err != nil {
		return nil, err
	}
	if settlement == 0 {
		return nil, fmt.Errorf("settlement_amount rounds to zero (amount=%d rate=%d)", m.Amount, t.RedemptionRate)
	}
	// Move units into pending pool: debit holder's balance, then bump
	// the per-token pending counter so total_supply == sum_balances + pending.
	if err := s.k.debitBalance(ctx, t.Id, m.Holder, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.addPendingUnits(ctx, t.Id, m.Amount); err != nil {
		return nil, err
	}
	id, err := s.k.NextRedemptionID(ctx)
	if err != nil {
		return nil, err
	}
	r := types.RedemptionRequest{
		Id:               id,
		TokenId:          t.Id,
		Holder:           m.Holder,
		Amount:           m.Amount,
		SettlementDenom:  t.SettlementDenom,
		SettlementAmount: settlement,
		RequestedAt:      now,
		EligibleAt:       now + t.RedemptionDelaySeconds,
		Status:           types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
		Memo:             m.Memo,
	}
	if err := s.k.Redemptions.Set(ctx, id, r); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionByHolder.Set(ctx, collections.Join(m.Holder, id)); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByTok.Set(ctx, collections.Join(t.Id, id)); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByHolder.Set(ctx, collections.Join(m.Holder, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "request_redemption", m.Holder, "",
		fmt.Sprintf("red_id=%d amount=%d settlement=%d eligible=%d", id, m.Amount, settlement, r.EligibleAt))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_redemption_requested",
		[2]string{"redemption_id", strconv.FormatUint(id, 10)},
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"holder", m.Holder},
		[2]string{"amount", strconv.FormatUint(m.Amount, 10)},
		[2]string{"settlement_amount", strconv.FormatUint(settlement, 10)},
		[2]string{"eligible_at", strconv.FormatInt(r.EligibleAt, 10)},
	)
	return &types.MsgRequestRedemptionResponse{
		RedemptionId:     id,
		SettlementAmount: settlement,
		EligibleAt:       r.EligibleAt,
	}, nil
}

func (s msgServer) SettleRedemption(ctx context.Context, m *types.MsgSettleRedemption) (*types.MsgSettleRedemptionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	r, err := s.k.Redemptions.Get(ctx, m.RedemptionId)
	if err != nil {
		return nil, fmt.Errorf("redemption %d not found", m.RedemptionId)
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, fmt.Errorf("redemption %d not pending (status=%s)", r.Id, r.Status)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if now < r.EligibleAt {
		return nil, fmt.Errorf("redemption %d not yet eligible (now=%d, eligible=%d)", r.Id, now, r.EligibleAt)
	}
	t, err := s.k.Tokens.Get(ctx, r.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", r.TokenId)
	}
	if _, err := s.requireIssuerAuthority(ctx, t.IssuerId, m.IssuerAuthority); err != nil {
		return nil, err
	}
	// Re-run the sanctions gate at settle time: a holder who was
	// clean at request time can be added to the list during the
	// T+N window, and we must not pay them out. Same defense-in-
	// depth applies to the paying issuer authority.
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, m.IssuerAuthority); err != nil {
		return nil, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, r.Holder); err != nil {
		return nil, err
	}
	if s.k.stablecoin == nil {
		return nil, fmt.Errorf("stablecoin keeper not wired")
	}
	// Pay the holder from the issuer authority's stablecoin balance.
	if err := s.k.stablecoin.Move(sdk.UnwrapSDKContext(ctx), r.SettlementDenom, m.IssuerAuthority, r.Holder, r.SettlementAmount); err != nil {
		return nil, fmt.Errorf("settlement move failed: %w", err)
	}
	// Burn the units in pending pool.
	if err := s.k.subPendingUnits(ctx, t.Id, r.Amount); err != nil {
		return nil, err
	}
	newSupply, err := types.SafeSub(t.TotalSupply, r.Amount)
	if err != nil {
		return nil, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = now
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_SETTLED
	r.SettledAt = now
	if err := s.k.Redemptions.Set(ctx, r.Id, r); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByTok.Remove(ctx, collections.Join(t.Id, r.Id)); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByHolder.Remove(ctx, collections.Join(r.Holder, r.Id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "settle_redemption", m.IssuerAuthority, r.Holder,
		fmt.Sprintf("red_id=%d amount=%d settlement=%d new_supply=%d", r.Id, r.Amount, r.SettlementAmount, newSupply))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_redemption_settled",
		[2]string{"redemption_id", strconv.FormatUint(r.Id, 10)},
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"holder", r.Holder},
		[2]string{"settlement_amount", strconv.FormatUint(r.SettlementAmount, 10)},
		[2]string{"new_supply", strconv.FormatUint(newSupply, 10)},
	)
	return &types.MsgSettleRedemptionResponse{}, nil
}

// CancelRedemption can be invoked either by the holder (self-cancel
// before settlement) or by the issuer authority (e.g. compliance
// override). Returns units to the holder's free balance.
func (s msgServer) CancelRedemption(ctx context.Context, m *types.MsgCancelRedemption) (*types.MsgCancelRedemptionResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	r, err := s.k.Redemptions.Get(ctx, m.RedemptionId)
	if err != nil {
		return nil, fmt.Errorf("redemption %d not found", m.RedemptionId)
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, fmt.Errorf("redemption %d not pending (status=%s)", r.Id, r.Status)
	}
	t, err := s.k.Tokens.Get(ctx, r.TokenId)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", r.TokenId)
	}
	is, err := s.k.Issuers.Get(ctx, t.IssuerId)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", t.IssuerId)
	}
	if m.Actor != r.Holder && m.Actor != is.Authority {
		return nil, fmt.Errorf("only holder or issuer authority may cancel")
	}
	// Return units to holder's free balance.
	if err := s.k.subPendingUnits(ctx, t.Id, r.Amount); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, t.Id, r.Holder, r.Amount); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_CANCELLED
	r.SettledAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Redemptions.Set(ctx, r.Id, r); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByTok.Remove(ctx, collections.Join(t.Id, r.Id)); err != nil {
		return nil, err
	}
	if err := s.k.RedemptionPendingByHolder.Remove(ctx, collections.Join(r.Holder, r.Id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, t.Id, t.IssuerId, "cancel_redemption", m.Actor, r.Holder,
		fmt.Sprintf("red_id=%d amount=%d reason=%s", r.Id, r.Amount, m.Reason))
	emit(sdk.UnwrapSDKContext(ctx), "rwa_redemption_cancelled",
		[2]string{"redemption_id", strconv.FormatUint(r.Id, 10)},
		[2]string{"token_id", strconv.FormatUint(t.Id, 10)},
		[2]string{"actor", m.Actor},
		[2]string{"reason", m.Reason},
	)
	return &types.MsgCancelRedemptionResponse{}, nil
}

// ---- Params --------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "rwa_params_updated")
	return &types.MsgUpdateParamsResponse{}, nil
}
