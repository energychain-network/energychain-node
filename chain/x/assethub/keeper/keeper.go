package keeper

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/assethub/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	bank  types.BankKeeper
	audit types.AuditKeeper // optional, may be nil

	Schema collections.Schema

	Params    collections.Item[types.Params]
	Providers collections.Map[string, types.Provider]

	Devices          collections.Map[string, types.Device]
	DeviceByOperator collections.KeySet[collections.Pair[string, string]]

	Readings        collections.Map[uint64, types.MeteringReading]
	ReadingByDevice collections.KeySet[collections.Pair[string, uint64]]
	ReadingIDSeq    collections.Sequence

	Topics      collections.Map[string, types.OracleTopic]
	Submissions collections.Map[collections.Pair[string, string], types.OracleSubmission]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	bank types.BankKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("assethub: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		bank:         bank,
		audit:        audit,

		Params:    collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Providers: collections.NewMap(sb, types.ProviderPrefix, "providers", collections.StringKey, codec.CollValue[types.Provider](cdc)),

		Devices: collections.NewMap(sb, types.DevicePrefix, "devices", collections.StringKey, codec.CollValue[types.Device](cdc)),
		DeviceByOperator: collections.NewKeySet(sb, types.DeviceByOperatorPrefix, "device_by_operator",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		Readings: collections.NewMap(sb, types.ReadingPrefix, "readings", collections.Uint64Key, codec.CollValue[types.MeteringReading](cdc)),
		ReadingByDevice: collections.NewKeySet(sb, types.ReadingByDevicePrefix, "reading_by_device",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ReadingIDSeq: collections.NewSequence(sb, types.ReadingIDSeqPrefix, "reading_id_seq"),

		Topics: collections.NewMap(sb, types.TopicPrefix, "topics", collections.StringKey, codec.CollValue[types.OracleTopic](cdc)),
		Submissions: collections.NewMap(sb, types.SubmissionPrefix, "submissions",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey), codec.CollValue[types.OracleSubmission](cdc)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params ---------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return p, nil
}

// ---- providers -------------------------------------------------------------

func (k Keeper) GetProvider(ctx context.Context, addr string) (types.Provider, bool) {
	p, err := k.Providers.Get(ctx, addr)
	if err != nil {
		return types.Provider{}, false
	}
	return p, true
}

// requireActiveProvider loads a provider and ensures it is active and (if a
// role is provided) holds one of the permitted roles. A jailed provider whose
// jail window elapsed is auto-reactivated.
func (k Keeper) requireActiveProvider(ctx sdk.Context, addr string, roles ...types.ProviderRole) (types.Provider, error) {
	p, ok := k.GetProvider(ctx, addr)
	if !ok {
		return types.Provider{}, types.ErrNotFound.Wrapf("provider %q", addr)
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_JAILED && p.JailedUntil > 0 &&
		ctx.BlockTime().Unix() >= p.JailedUntil {
		p.Status = types.ProviderStatus_PROVIDER_STATUS_ACTIVE
		p.JailedUntil = 0
		p.UpdatedAt = ctx.BlockTime().Unix()
		if err := k.Providers.Set(ctx, addr, p); err != nil {
			return types.Provider{}, err
		}
	}
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		return types.Provider{}, types.ErrProviderState.Wrapf("provider %q is %s", addr, p.Status)
	}
	if len(roles) > 0 {
		ok := false
		for _, r := range roles {
			if p.Role == r {
				ok = true
				break
			}
		}
		if !ok {
			return types.Provider{}, types.ErrWrongRole.Wrapf("provider %q role %s", addr, p.Role)
		}
	}
	return p, nil
}

func (k Keeper) CountProviders(ctx context.Context) (uint64, error) {
	return k.count(k.Providers.Iterate(ctx, nil))
}

func (k Keeper) CountTopics(ctx context.Context) (uint64, error) {
	return k.count(k.Topics.Iterate(ctx, nil))
}

// GetTopicValue is the cross-module read used by reserve-gated modules
// (e.g. x/stableusd) to fetch an oracle topic's aggregated value. ok is
// false when the topic is unknown or has not yet reached min_sources, so
// callers fail closed. updatedAt is the unix second of the last
// aggregation for staleness checks.
func (k Keeper) GetTopicValue(ctx context.Context, topicID string) (value uint64, updatedAt int64, ok bool) {
	tpc, err := k.Topics.Get(ctx, topicID)
	if err != nil || !tpc.HasValue {
		return 0, 0, false
	}
	return tpc.Value, tpc.UpdatedAt, true
}

// DeviceOperator is the cross-module read used by asset modules
// (e.g. x/rwatoken) to anchor a tokenized asset to the physical devices
// whose metered output backs it. It returns the device's operator,
// whether the device is ACTIVE, and whether it exists at all, so callers
// can require that a token only binds devices it provably operates.
func (k Keeper) DeviceOperator(ctx context.Context, deviceID string) (operator string, active bool, found bool) {
	d, err := k.Devices.Get(ctx, deviceID)
	if err != nil {
		return "", false, false
	}
	return d.Operator, d.Status == types.DeviceStatus_DEVICE_STATUS_ACTIVE, true
}

// activeDeviceCount returns how many non-revoked devices an operator owns.
func (k Keeper) activeDeviceCount(ctx context.Context, operator string) (int, error) {
	rng := collections.NewPrefixedPairRange[string, string](operator)
	it, err := k.DeviceByOperator.Iterate(ctx, rng)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	n := 0
	for ; it.Valid(); it.Next() {
		key, err := it.Key()
		if err != nil {
			return 0, err
		}
		d, err := k.Devices.Get(ctx, key.K2())
		if err != nil {
			continue
		}
		if d.Status == types.DeviceStatus_DEVICE_STATUS_ACTIVE {
			n++
		}
	}
	return n, nil
}

// ---- bond accounting -------------------------------------------------------

func (k Keeper) bondCoins(ctx context.Context, amount uint64) (sdk.Coins, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return sdk.NewCoins(sdk.NewCoin(p.BondDenom, math.NewIntFromUint64(amount))), nil
}

func (k Keeper) collectBond(ctx context.Context, from string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	addr, err := sdk.AccAddressFromBech32(from)
	if err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}
	coins, err := k.bondCoins(ctx, amount)
	if err != nil {
		return err
	}
	return k.bank.SendCoinsFromAccountToModule(ctx, addr, types.ModuleName, coins)
}

func (k Keeper) refundBond(ctx context.Context, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	addr, err := sdk.AccAddressFromBech32(to)
	if err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}
	coins, err := k.bondCoins(ctx, amount)
	if err != nil {
		return err
	}
	return k.bank.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, coins)
}

func (k Keeper) burnBond(ctx context.Context, amount uint64) error {
	if amount == 0 {
		return nil
	}
	coins, err := k.bondCoins(ctx, amount)
	if err != nil {
		return err
	}
	return k.bank.BurnCoins(ctx, types.ModuleName, coins)
}

// slash burns slash_fraction of the provider's bond, records an infraction,
// and jails the provider once the infraction threshold is reached. It returns
// the slashed amount and whether the provider was jailed.
func (k Keeper) slash(ctx sdk.Context, p *types.Provider, params types.Params) (uint64, bool, error) {
	amount := types.MulBps(p.Bond, params.SlashFractionBps)
	if amount > p.Bond {
		amount = p.Bond
	}
	newBond, err := types.SafeSub(p.Bond, amount)
	if err != nil {
		return 0, false, err
	}
	if err := k.burnBond(ctx, amount); err != nil {
		return 0, false, err
	}
	p.Bond = newBond
	p.Infractions++
	jailed := false
	if params.JailAfterInfractions > 0 && p.Infractions >= params.JailAfterInfractions &&
		p.Status == types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		p.Status = types.ProviderStatus_PROVIDER_STATUS_JAILED
		p.JailedUntil = ctx.BlockTime().Unix() + params.JailDurationSeconds
		jailed = true
	}
	p.UpdatedAt = ctx.BlockTime().Unix()
	return amount, jailed, nil
}

// ---- oracle aggregation ----------------------------------------------------

// recomputeTopic reads every submission for a topic, recomputes the median,
// and persists the aggregate. With fewer than min_sources submitters the topic
// retains no trusted value.
func (k Keeper) recomputeTopic(ctx sdk.Context, topic *types.OracleTopic) error {
	rng := collections.NewPrefixedPairRange[string, string](topic.Id)
	it, err := k.Submissions.Iterate(ctx, rng)
	if err != nil {
		return err
	}
	defer it.Close()
	var values []uint64
	for ; it.Valid(); it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		values = append(values, v.Value)
	}
	topic.SourceCount = uint32(len(values))
	topic.UpdatedAt = ctx.BlockTime().Unix()
	if uint32(len(values)) < topic.MinSources {
		topic.HasValue = false
		return nil
	}
	topic.Value = median(values)
	topic.HasValue = true
	return nil
}

func median(v []uint64) uint64 {
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	n := len(v)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return v[n/2]
	}
	// average of the two middle elements without overflow
	a, b := v[n/2-1], v[n/2]
	return a/2 + b/2 + (a%2+b%2)/2
}

// ---- audit -----------------------------------------------------------------

func (k Keeper) recordAudit(ctx sdk.Context, action, actor, subject, detail string) {
	if k.audit != nil {
		k.audit.RecordAction(ctx, types.ModuleName, action, actor, subject, detail)
	}
}

// ---- helpers ---------------------------------------------------------------

func (k Keeper) count(it interface {
	Keys() ([]string, error)
}, err error) (uint64, error) {
	if err != nil {
		return 0, err
	}
	keys, err := it.Keys()
	if err != nil {
		return 0, err
	}
	return uint64(len(keys)), nil
}
