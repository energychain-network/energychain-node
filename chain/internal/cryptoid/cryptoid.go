// Package cryptoid centralises the multi-curve signature verification used
// by x/did, x/device, and x/oracle to authenticate off-chain signers
// (subjects, devices, oracle nodes) without leaking curve-specific code
// into every keeper.
//
// Supported curves (matched to docs/native-modules.md §4.1):
//
//   - secp256k1 (default; matches Cosmos and Ethereum keys)
//   - ed25519   (Tendermint-validator-style keys)
//
// SM2 / BLS / P-256 are reserved enum values: the verifiers will be added
// alongside the corresponding keepers in M2 / M3. Listing them here keeps
// the on-chain enum stable across milestones.
package cryptoid

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
)

// KeyType is the canonical wire identifier for the public-key family.
// Values are persisted in DID Documents — DO NOT renumber.
type KeyType uint32

const (
	KeyTypeUnspecified KeyType = 0
	KeyTypeSecp256k1   KeyType = 1
	KeyTypeEd25519     KeyType = 2

	// Reserved for future verifier plug-ins. Adding the verifier MUST
	// land before any DID Document persists a key with these values.
	KeyTypeP256 KeyType = 10
	KeyTypeBLS  KeyType = 11
	KeyTypeSM2  KeyType = 20
)

// PublicKey is the verifier's view of a DID-controlled key. Bytes are the
// canonical compressed encoding for the curve:
//
//   - secp256k1: 33-byte 0x02/0x03-prefixed x coordinate
//   - ed25519:   32-byte raw public key
//
// We store the canonical form rather than a parsed struct so the value can
// round-trip through proto bytes without curve-specific (de)serialization.
type PublicKey struct {
	Type  KeyType
	Bytes []byte
}

// Validate enforces the per-curve byte length and rejects unknown / not-yet
// implemented types so corrupted DID Documents fail fast.
func (k PublicKey) Validate() error {
	switch k.Type {
	case KeyTypeSecp256k1:
		if len(k.Bytes) != 33 {
			return fmt.Errorf("secp256k1 public key must be 33 bytes, got %d", len(k.Bytes))
		}
	case KeyTypeEd25519:
		if len(k.Bytes) != ed25519.PublicKeySize {
			return fmt.Errorf("ed25519 public key must be %d bytes, got %d", ed25519.PublicKeySize, len(k.Bytes))
		}
	case KeyTypeUnspecified:
		return errors.New("public key type must be set")
	case KeyTypeP256, KeyTypeBLS, KeyTypeSM2:
		return fmt.Errorf("public key type %d not yet supported", k.Type)
	default:
		return fmt.Errorf("unknown public key type %d", k.Type)
	}
	return nil
}

// Verify checks sig against msg. The hash convention matches Cosmos:
// secp256k1 expects sha256(msg) and a 64- or 65-byte (R||S [||V]) sig;
// ed25519 verifies the raw msg. This mirrors how off-chain SDKs sign.
func (k PublicKey) Verify(msg, sig []byte) error {
	if err := k.Validate(); err != nil {
		return err
	}
	switch k.Type {
	case KeyTypeSecp256k1:
		// secp256k1.PubKey.VerifySignature applies sha256(msg) itself
		// and accepts the canonical 64-byte R||S signature (a leading
		// recovery byte is tolerated by the SDK implementation).
		// Wrapping the raw key bytes lets us reuse the audited Cosmos
		// helper rather than pulling in btcec for plain ECDSA.
		pk := &secp256k1.PubKey{Key: k.Bytes}
		if !pk.VerifySignature(msg, sig) {
			return errors.New("secp256k1: signature verification failed")
		}
		return nil
	case KeyTypeEd25519:
		if len(sig) != ed25519.SignatureSize {
			return fmt.Errorf("ed25519: signature must be %d bytes, got %d", ed25519.SignatureSize, len(sig))
		}
		if !ed25519.Verify(ed25519.PublicKey(k.Bytes), msg, sig) {
			return errors.New("ed25519: signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("verify: unsupported key type %d", k.Type)
	}
}
