// Package commitment provides Pedersen-style commitment helpers shared by the
// energy-chain modules that store privacy-preserving readings on-chain
// (notably x/meter and x/eac).
//
// The on-chain representation is intentionally minimal: a 32-byte commitment
// value plus an opaque scheme identifier. Schemes are versioned so the chain
// can roll forward from "raw sha256(value || nonce)" prototypes to a real
// Pedersen / Bulletproof setup without breaking historical reads.
//
// This package does NOT verify zero-knowledge range proofs. Range-proof
// verification lives next to the verifier registry that owns it (see
// docs/native-modules.md §4.5 "Privacy可选"). Only the structural / format
// invariants are checked here.
package commitment

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// Scheme identifies the commitment family used for a particular value.
//
// Wire format MUST be stable: the integer is persisted as part of the
// on-chain Reading and rotating it would orphan all historical commitments.
type Scheme uint32

const (
	// SchemeUnspecified is the zero value and is rejected by Validate.
	// Modules MUST pick an explicit scheme so legacy/empty Reading rows
	// surface as malformed rather than silently rendering as "default".
	SchemeUnspecified Scheme = 0

	// SchemePlaintextSHA256 is the bootstrapping scheme: the commitment is
	// sha256(scheme || domain || value || nonce) where value is a plain
	// uint64 and nonce is 32 random bytes. There is no homomorphic
	// addition. Intended for the M1 milestone where the priority is
	// shipping the data path rather than full ZK.
	SchemePlaintextSHA256 Scheme = 1

	// SchemePedersenSecp256k1 is the production target: a curve-point
	// commitment under secp256k1 generators (g, h). Reserved here so the
	// type table is stable across milestones; verification will land
	// alongside the M2 ZK toolchain.
	SchemePedersenSecp256k1 Scheme = 2
)

// commitmentSize is the byte length every supported scheme MUST conform to.
// Mismatched lengths are rejected up front so the on-chain decoder never
// has to branch on scheme to learn the expected size.
const commitmentSize = 32

// ErrInvalidLength is returned when a Commitment.Value byte slice does not
// have the canonical 32-byte length.
var ErrInvalidLength = fmt.Errorf("commitment: value must be %d bytes", commitmentSize)

// Commitment is the on-chain wire shape consumed by x/meter, x/eac, and
// anywhere else a privacy-preserving numeric value is stored.
//
// Fields are intentionally exported (not opaque) so the proto-generated
// types in module packages can copy them into / out of pb structs without
// reflection.
type Commitment struct {
	Scheme Scheme
	// Domain is a short ASCII tag that namespaces the commitment so two
	// independently-generated commitments to the same value over the same
	// scheme do not collide. Examples: "meter.read.v1", "eac.attr.v1".
	Domain string
	// Value is the 32-byte commitment digest / curve point compressed
	// representation, depending on Scheme.
	Value []byte
}

// Validate ensures the commitment is structurally well-formed for its
// scheme. It does NOT verify the binding (that requires a nonce or a proof
// that callers hold separately).
func (c Commitment) Validate() error {
	if c.Scheme == SchemeUnspecified {
		return errors.New("commitment: scheme must be set")
	}
	if c.Scheme != SchemePlaintextSHA256 && c.Scheme != SchemePedersenSecp256k1 {
		return fmt.Errorf("commitment: unknown scheme %d", c.Scheme)
	}
	if c.Domain == "" {
		return errors.New("commitment: domain must be set")
	}
	if len(c.Domain) > 64 {
		return fmt.Errorf("commitment: domain too long (%d > 64)", len(c.Domain))
	}
	if len(c.Value) != commitmentSize {
		return ErrInvalidLength
	}
	return nil
}

// IsZero returns true when no commitment has been set; useful in genesis
// validators that distinguish "field omitted" from "field set to invalid".
func (c Commitment) IsZero() bool {
	return c.Scheme == SchemeUnspecified && c.Domain == "" && len(c.Value) == 0
}

// HashPlaintext deterministically computes the SchemePlaintextSHA256
// commitment for a uint64 value with a 32-byte nonce. Provided so tests and
// off-chain producers share the on-chain encoding rules.
//
// Layout: sha256( LE(uint32 scheme) || LE(uint16 len(domain)) || domain ||
//                 BE(uint64 value)  || nonce )
//
// Big-endian on the value mirrors the convention used elsewhere in the SDK
// for monotonically-comparable bytes; the rest is little-endian for cheap
// off-chain decoding.
func HashPlaintext(domain string, value uint64, nonce [32]byte) Commitment {
	h := sha256.New()

	var scheme [4]byte
	binary.LittleEndian.PutUint32(scheme[:], uint32(SchemePlaintextSHA256))
	h.Write(scheme[:])

	var dlen [2]byte
	binary.LittleEndian.PutUint16(dlen[:], uint16(len(domain)))
	h.Write(dlen[:])
	h.Write([]byte(domain))

	var v [8]byte
	binary.BigEndian.PutUint64(v[:], value)
	h.Write(v[:])

	h.Write(nonce[:])

	return Commitment{
		Scheme: SchemePlaintextSHA256,
		Domain: domain,
		Value:  h.Sum(nil),
	}
}

// VerifyPlaintext returns nil when the candidate commitment binds to
// (value, nonce) under the SchemePlaintextSHA256 scheme. Used by view-key
// holders (regulators) to confirm a disclosed reading matches its on-chain
// commitment without trusting the disclosing party.
func VerifyPlaintext(c Commitment, value uint64, nonce [32]byte) error {
	if c.Scheme != SchemePlaintextSHA256 {
		return fmt.Errorf("commitment: verify expects plaintext scheme, got %d", c.Scheme)
	}
	expected := HashPlaintext(c.Domain, value, nonce)
	if len(expected.Value) != len(c.Value) {
		return ErrInvalidLength
	}
	for i := range expected.Value {
		if expected.Value[i] != c.Value[i] {
			return errors.New("commitment: verification failed")
		}
	}
	return nil
}
