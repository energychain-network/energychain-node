package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (m *MsgCreateAuction) ValidateBasic() error {
	if err := ValidateAddr("seller", m.Seller); err != nil {
		return err
	}
	if !KindValid(m.Kind) {
		return fmt.Errorf("invalid kind")
	}
	if err := ValidateDenom(m.PaymentDenom); err != nil {
		return err
	}
	if err := ValidateAssetRef(m.AssetRef); err != nil {
		return err
	}
	if m.StartTime < 0 || m.EndTime < 0 || m.CommitEndTime < 0 || m.RevealEndTime < 0 {
		return fmt.Errorf("time fields cannot be negative")
	}
	switch m.Kind {
	case Kind_KIND_ENGLISH:
		if m.EndTime <= m.StartTime {
			return fmt.Errorf("end_time must be > start_time for english")
		}
	case Kind_KIND_DUTCH:
		if m.EndTime <= m.StartTime {
			return fmt.Errorf("end_time must be > start_time for dutch")
		}
		if m.DutchStartPrice == 0 {
			return fmt.Errorf("dutch_start_price must be > 0")
		}
		if m.DutchFloorPrice >= m.DutchStartPrice {
			return fmt.Errorf("dutch_floor_price must be < dutch_start_price")
		}
		if m.DutchDecaySeconds <= 0 {
			return fmt.Errorf("dutch_decay_seconds must be > 0")
		}
		if m.ReservePrice > 0 && m.ReservePrice > m.DutchStartPrice {
			return fmt.Errorf("reserve_price > dutch_start_price")
		}
	case Kind_KIND_SEALED_FIRST, Kind_KIND_SEALED_SECOND:
		if m.CommitEndTime <= m.StartTime {
			return fmt.Errorf("commit_end_time must be > start_time")
		}
		if m.RevealEndTime <= m.CommitEndTime {
			return fmt.Errorf("reveal_end_time must be > commit_end_time")
		}
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgCreateAuction) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Seller)
	return []sdk.AccAddress{a}
}

func (m *MsgCancelAuction) ValidateBasic() error {
	if err := ValidateAddr("seller", m.Seller); err != nil {
		return err
	}
	if m.AuctionId == 0 {
		return fmt.Errorf("auction_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelAuction) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Seller)
	return []sdk.AccAddress{a}
}

func (m *MsgPlaceBid) ValidateBasic() error {
	if err := ValidateAddr("bidder", m.Bidder); err != nil {
		return err
	}
	if m.AuctionId == 0 {
		return fmt.Errorf("auction_id must be > 0")
	}
	if m.Price == 0 {
		return fmt.Errorf("price must be > 0")
	}
	return nil
}
func (m *MsgPlaceBid) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Bidder)
	return []sdk.AccAddress{a}
}

func (m *MsgCommitBid) ValidateBasic() error {
	if err := ValidateAddr("bidder", m.Bidder); err != nil {
		return err
	}
	if m.AuctionId == 0 {
		return fmt.Errorf("auction_id must be > 0")
	}
	if len(m.CommitHash) != HashLen {
		return fmt.Errorf("commit_hash must be %d bytes", HashLen)
	}
	if m.Deposit == 0 {
		return fmt.Errorf("deposit must be > 0")
	}
	return nil
}
func (m *MsgCommitBid) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Bidder)
	return []sdk.AccAddress{a}
}

func (m *MsgRevealBid) ValidateBasic() error {
	if err := ValidateAddr("bidder", m.Bidder); err != nil {
		return err
	}
	if m.BidId == 0 {
		return fmt.Errorf("bid_id must be > 0")
	}
	if m.Price == 0 {
		return fmt.Errorf("price must be > 0")
	}
	if len(m.Salt) == 0 || len(m.Salt) > SaltMaxLen {
		return fmt.Errorf("salt must be 1..%d bytes", SaltMaxLen)
	}
	return nil
}
func (m *MsgRevealBid) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Bidder)
	return []sdk.AccAddress{a}
}

func (m *MsgClose) ValidateBasic() error {
	if err := ValidateAddr("caller", m.Caller); err != nil {
		return err
	}
	if m.AuctionId == 0 {
		return fmt.Errorf("auction_id must be > 0")
	}
	return nil
}
func (m *MsgClose) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Caller)
	return []sdk.AccAddress{a}
}

func (m *MsgSettle) ValidateBasic() error {
	if err := ValidateAddr("caller", m.Caller); err != nil {
		return err
	}
	if m.AuctionId == 0 {
		return fmt.Errorf("auction_id must be > 0")
	}
	return nil
}
func (m *MsgSettle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Caller)
	return []sdk.AccAddress{a}
}

func (m *MsgWithdrawRefund) ValidateBasic() error {
	if err := ValidateAddr("bidder", m.Bidder); err != nil {
		return err
	}
	if m.BidId == 0 {
		return fmt.Errorf("bid_id must be > 0")
	}
	return nil
}
func (m *MsgWithdrawRefund) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Bidder)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
