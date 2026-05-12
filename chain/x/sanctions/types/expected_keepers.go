package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// AuditKeeper is the optional cross-module hook x/sanctions uses to
// emit a structured audit event when a list mutation lands or a hit
// fires. Wired as nil during early bring-up; the keeper is nil-safe.
type AuditKeeper interface {
	RecordSanctionsAction(ctx sdk.Context, listID, subject, action, detail string)
}
