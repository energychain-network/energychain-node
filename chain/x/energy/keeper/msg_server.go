package keeper

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"golang.org/x/crypto/sha3"

	"energychain/x/energy/types"
)

type msgServer struct {
	keeper Keeper
}

var _ types.MsgServer = &msgServer{}

func NewMsgServerImpl(keeper Keeper) *msgServer {
	return &msgServer{keeper: keeper}
}

func (m *msgServer) SubmitEnergyData(goCtx context.Context, msg *types.MsgSubmitEnergyData) (*types.MsgSubmitEnergyDataResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.keeper.IsAllowedSubmitter(ctx, msg.Submitter) {
		return nil, fmt.Errorf("address %s is not an allowed submitter", msg.Submitter)
	}

	params := m.keeper.GetParams(ctx)
	if params.MaxMetadataSize > 0 && uint32(len(msg.Metadata)) > params.MaxMetadataSize {
		return nil, fmt.Errorf("metadata size %d exceeds maximum %d", len(msg.Metadata), params.MaxMetadataSize)
	}

	if err := m.keeper.CheckAndIncrSubmitCount(ctx, msg.Submitter); err != nil {
		return nil, err
	}

	id := m.keeper.GenerateID(ctx)
	data := types.EnergyData{
		ID:          id,
		Category:    msg.Category,
		Submitter:   msg.Submitter,
		DataHash:    msg.DataHash,
		Metadata:    msg.Metadata,
		BlockHeight: ctx.BlockHeight(),
		Timestamp:   ctx.BlockTime().Unix(),
	}

	if err := m.keeper.SubmitEnergyData(ctx, data); err != nil {
		return nil, fmt.Errorf("storing energy data: %w", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"energy_data_submitted",
		sdk.NewAttribute("id", id),
		sdk.NewAttribute("submitter", msg.Submitter),
		sdk.NewAttribute("category", msg.Category),
		sdk.NewAttribute("data_hash", msg.DataHash),
		sdk.NewAttribute("metadata", msg.Metadata),
	))

	return &types.MsgSubmitEnergyDataResponse{ID: id}, nil
}

func (m *msgServer) BatchSubmit(goCtx context.Context, msg *types.MsgBatchSubmit) (*types.MsgBatchSubmitResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.keeper.IsAllowedSubmitter(ctx, msg.Submitter) {
		return nil, fmt.Errorf("address %s is not an allowed submitter", msg.Submitter)
	}

	params := m.keeper.GetParams(ctx)
	if uint32(len(msg.Items)) > params.MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(msg.Items), params.MaxBatchSize)
	}
	if params.MaxMetadataSize > 0 {
		for i, item := range msg.Items {
			if uint32(len(item.Metadata)) > params.MaxMetadataSize {
				return nil, fmt.Errorf("batch item %d metadata size %d exceeds maximum %d",
					i, len(item.Metadata), params.MaxMetadataSize)
			}
		}
	}

	// Counts as a single submission for rate-limit purposes regardless of
	// batch length: bulk operators are exactly the use case we want to
	// admit, while malicious spammers can't loop SubmitEnergyData.
	if err := m.keeper.CheckAndIncrSubmitCount(ctx, msg.Submitter); err != nil {
		return nil, err
	}

	if err := verifyMerkleRoot(msg.Items, msg.MerkleRoot); err != nil {
		return nil, fmt.Errorf("merkle root verification failed: %w", err)
	}

	batchID := m.keeper.GenerateID(ctx)
	now := ctx.BlockTime().Unix()
	height := ctx.BlockHeight()

	for i, item := range msg.Items {
		id := m.keeper.GenerateID(ctx)
		data := types.EnergyData{
			ID:          id,
			Category:    msg.Category,
			Submitter:   msg.Submitter,
			DataHash:    item.DataHash,
			Metadata:    item.Metadata,
			BlockHeight: height,
			Timestamp:   now,
		}
		if err := m.keeper.SubmitEnergyData(ctx, data); err != nil {
			return nil, fmt.Errorf("storing batch item %d: %w", i, err)
		}
	}

	batch := types.BatchSubmission{
		ID:         batchID,
		Submitter:  msg.Submitter,
		Category:   msg.Category,
		DataCount:  uint32(len(msg.Items)),
		MerkleRoot: msg.MerkleRoot,
		Timestamp:  now,
	}
	if err := m.keeper.SubmitBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("storing batch: %w", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"energy_batch_submitted",
		sdk.NewAttribute("batch_id", batchID),
		sdk.NewAttribute("submitter", msg.Submitter),
		sdk.NewAttribute("category", msg.Category),
		sdk.NewAttribute("data_count", fmt.Sprintf("%d", len(msg.Items))),
		sdk.NewAttribute("merkle_root", msg.MerkleRoot),
	))

	return &types.MsgBatchSubmitResponse{BatchID: batchID, DataCount: uint32(len(msg.Items))}, nil
}

// UpdateParams replaces the entire module Params. Authority is the gov
// module address (or whatever was configured in app.go); any other signer
// is rejected.
func (m *msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("invalid authority: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.keeper.SetParams(ctx, msg.Params); err != nil {
		return nil, fmt.Errorf("setting params: %w", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// verifyMerkleRoot computes a Merkle tree of item DataHashes using keccak256
// and compares it against the declared root.
//
// Each leaf is keccak256(uint64(index) || dataHash) so that two items with
// identical DataHash but different positions produce different leaves.
// Without the index prefix, a batch like [hashA, hashA] would have the same
// root as a batch with a single hashA, allowing a malicious submitter to
// claim more datapoints than they actually included. Off-chain producers
// MUST hash with the same scheme.
func verifyMerkleRoot(items []types.BatchItem, declaredRoot string) error {
	if len(items) == 0 {
		return fmt.Errorf("no items to verify")
	}

	leaves := make([][]byte, len(items))
	for i, item := range items {
		h := sha3.NewLegacyKeccak256()
		var idx [8]byte
		binary.BigEndian.PutUint64(idx[:], uint64(i))
		h.Write(idx[:])
		h.Write([]byte(item.DataHash))
		leaves[i] = h.Sum(nil)
	}

	root := computeMerkleRoot(leaves)
	computed := hex.EncodeToString(root)

	normalized := strings.TrimPrefix(strings.ToLower(declaredRoot), "0x")
	if computed != normalized {
		return fmt.Errorf("computed root %s does not match declared root %s", computed, declaredRoot)
	}
	return nil
}

func computeMerkleRoot(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		return leaves[0]
	}

	var next [][]byte
	for i := 0; i < len(leaves); i += 2 {
		if i+1 < len(leaves) {
			next = append(next, hashPair(leaves[i], leaves[i+1]))
		} else {
			next = append(next, hashPair(leaves[i], leaves[i]))
		}
	}
	return computeMerkleRoot(next)
}

func hashPair(a, b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	if strings.Compare(hex.EncodeToString(a), hex.EncodeToString(b)) <= 0 {
		h.Write(a)
		h.Write(b)
	} else {
		h.Write(b)
		h.Write(a)
	}
	return h.Sum(nil)
}
