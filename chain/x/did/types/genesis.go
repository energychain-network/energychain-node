package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/internal/cryptoid"
)

func DefaultParams() Params {
	return Params{
		MaxControllers:          DefaultMaxControllers,
		MaxVerificationMethods:  DefaultMaxVerificationMethods,
		MaxServiceEndpoints:     DefaultMaxServiceEndpoints,
		MaxCredentialHashLen:    DefaultMaxCredentialHashLen,
		RevocationGracePeriod:   DefaultRevocationGracePeriod,
	}
}

// Validate enforces the Params upper bounds and the simple structural
// rules. Bounds are deliberately loose so that operators have room to
// tune for their workload through governance.
func (p Params) Validate() error {
	if p.MaxControllers == 0 || p.MaxControllers > MaxControllersUpperBound {
		return fmt.Errorf("max_controllers %d outside (0, %d]", p.MaxControllers, MaxControllersUpperBound)
	}
	if p.MaxVerificationMethods == 0 || p.MaxVerificationMethods > MaxVerificationMethodsUpperBound {
		return fmt.Errorf("max_verification_methods %d outside (0, %d]", p.MaxVerificationMethods, MaxVerificationMethodsUpperBound)
	}
	if p.MaxServiceEndpoints > MaxServiceEndpointsUpperBound {
		return fmt.Errorf("max_service_endpoints %d exceeds %d", p.MaxServiceEndpoints, MaxServiceEndpointsUpperBound)
	}
	if p.MaxCredentialHashLen == 0 || p.MaxCredentialHashLen > MaxCredentialHashLenUpperBound {
		return fmt.Errorf("max_credential_hash_len %d outside (0, %d]", p.MaxCredentialHashLen, MaxCredentialHashLenUpperBound)
	}
	if p.RevocationGracePeriod < 0 {
		return fmt.Errorf("revocation_grace_period must be non-negative")
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		Documents:   []DIDDocument{},
		Credentials: []CredentialStatus{},
		Anchors:     []TrustAnchor{},
	}
}

// Validate ensures every document, credential and anchor in the genesis
// blob is structurally well-formed and (where applicable) cross-references
// resolve. We deliberately do NOT cross-check that every credential's
// subject has a DID document: external subjects (e.g. devices in M2) get
// their own DIDs in different module's genesis sections.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}

	seenDIDs := make(map[string]bool, len(gs.Documents))
	for i, d := range gs.Documents {
		if d.Id == "" {
			return fmt.Errorf("document %d: id empty", i)
		}
		if _, err := sdk.AccAddressFromBech32(d.Id); err != nil {
			return fmt.Errorf("document %d (%s): id not bech32: %w", i, d.Id, err)
		}
		if seenDIDs[d.Id] {
			return fmt.Errorf("document %d (%s): duplicate", i, d.Id)
		}
		seenDIDs[d.Id] = true
		if err := validateDocumentShape(&d, gs.Params); err != nil {
			return fmt.Errorf("document %d (%s): %w", i, d.Id, err)
		}
	}

	seenVCs := make(map[string]bool, len(gs.Credentials))
	for i, c := range gs.Credentials {
		if c.Id == "" {
			return fmt.Errorf("credential %d: id empty", i)
		}
		if seenVCs[c.Id] {
			return fmt.Errorf("credential %d (%s): duplicate", i, c.Id)
		}
		seenVCs[c.Id] = true
		if uint32(len(c.Hash)) > gs.Params.MaxCredentialHashLen {
			return fmt.Errorf("credential %d (%s): hash too long", i, c.Id)
		}
	}

	seenAnchors := make(map[string]bool, len(gs.Anchors))
	for i, a := range gs.Anchors {
		if a.Did == "" {
			return fmt.Errorf("anchor %d: did empty", i)
		}
		if seenAnchors[a.Did] {
			return fmt.Errorf("anchor %d (%s): duplicate", i, a.Did)
		}
		seenAnchors[a.Did] = true
		if a.Tier != AnchorTierRoot && a.Tier != AnchorTierIntermediate {
			return fmt.Errorf("anchor %d (%s): unknown tier %q", i, a.Did, a.Tier)
		}
		if a.Tier == AnchorTierIntermediate && a.ParentDid == "" {
			return fmt.Errorf("anchor %d (%s): intermediate must declare parent", i, a.Did)
		}
	}
	return nil
}

// validateDocumentShape factors out the per-document checks shared by
// genesis validation and msg_server.CreateDID / UpdateDID. Centralising
// them here means a future field addition only edits one place.
func validateDocumentShape(d *DIDDocument, p Params) error {
	if uint32(len(d.Controllers)) > p.MaxControllers {
		return fmt.Errorf("controllers %d > max %d", len(d.Controllers), p.MaxControllers)
	}
	if uint32(len(d.Verification)) > p.MaxVerificationMethods {
		return fmt.Errorf("verification %d > max %d", len(d.Verification), p.MaxVerificationMethods)
	}
	if uint32(len(d.Service)) > p.MaxServiceEndpoints {
		return fmt.Errorf("service %d > max %d", len(d.Service), p.MaxServiceEndpoints)
	}
	if d.Status != StatusActive && d.Status != StatusDeactivated {
		return fmt.Errorf("unknown status %q", d.Status)
	}
	for _, c := range d.Controllers {
		if _, err := sdk.AccAddressFromBech32(c); err != nil {
			return fmt.Errorf("controller %q not bech32: %w", c, err)
		}
	}
	seenKeys := make(map[string]bool, len(d.Verification))
	activeKeys := 0
	for i, v := range d.Verification {
		if v.Id == "" {
			return fmt.Errorf("verification %d: id empty", i)
		}
		if seenKeys[v.Id] {
			return fmt.Errorf("verification %d (%s): duplicate id", i, v.Id)
		}
		seenKeys[v.Id] = true
		if !VerificationMethodPurposesValid(v.Purposes) {
			return fmt.Errorf("verification %d (%s): unknown purpose", i, v.Id)
		}
		// SECURITY: validate the embedded key is well-formed so a
		// malformed entry can never round-trip through a CreateDID/
		// UpdateDID/RotateKey path. Without this check, a controller
		// could persist garbage bytes that downstream signature
		// verifiers would silently fail on, masquerading as "wrong
		// signature" instead of "corrupted document".
		pk := cryptoid.PublicKey{Type: cryptoid.KeyType(v.KeyType), Bytes: v.PublicKey}
		if v.RevokedAt == 0 {
			if err := pk.Validate(); err != nil {
				return fmt.Errorf("verification %d (%s): %w", i, v.Id, err)
			}
			activeKeys++
		} else {
			// Revoked rows are historical; we relax the strict key
			// check (the key may have been rotated out from a now-
			// retired KeyType), but still require KeyType to be a
			// known enum value to keep the wire format clean.
			if pk.Type == cryptoid.KeyTypeUnspecified {
				return fmt.Errorf("verification %d (%s): revoked entry has unspecified key type", i, v.Id)
			}
		}
		if v.Controller != "" {
			if _, err := sdk.AccAddressFromBech32(v.Controller); err != nil {
				return fmt.Errorf("verification %d (%s): controller not bech32: %w", i, v.Id, err)
			}
		}
	}
	// Documents lose all utility once every key is revoked. Active
	// documents must always retain at least one usable key; updates
	// that would leave zero are rejected here so the keeper can rely on
	// the invariant.
	if d.Status == StatusActive && activeKeys == 0 {
		return fmt.Errorf("active DID must retain at least one non-revoked verification method")
	}
	seenSvc := make(map[string]bool, len(d.Service))
	for i, s := range d.Service {
		if s.Id == "" {
			return fmt.Errorf("service %d: id empty", i)
		}
		if seenSvc[s.Id] {
			return fmt.Errorf("service %d (%s): duplicate id", i, s.Id)
		}
		seenSvc[s.Id] = true
	}
	return nil
}

// ValidateDocumentShape is the exported counterpart of validateDocumentShape;
// reused by the msg_server. Kept on the package surface (rather than the
// keeper) so genesis validation in module.go can call it without depending
// on the keeper.
func ValidateDocumentShape(d *DIDDocument, p Params) error {
	return validateDocumentShape(d, p)
}
