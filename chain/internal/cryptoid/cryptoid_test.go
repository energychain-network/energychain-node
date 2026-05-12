package cryptoid

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
)

func TestPublicKeyValidate(t *testing.T) {
	cases := []struct {
		name string
		k    PublicKey
		ok   bool
	}{
		{"unspecified rejected", PublicKey{Type: KeyTypeUnspecified, Bytes: make([]byte, 33)}, false},
		{"secp256k1 33 bytes ok", PublicKey{Type: KeyTypeSecp256k1, Bytes: make([]byte, 33)}, true},
		{"secp256k1 short rejected", PublicKey{Type: KeyTypeSecp256k1, Bytes: make([]byte, 32)}, false},
		{"secp256k1 long rejected", PublicKey{Type: KeyTypeSecp256k1, Bytes: make([]byte, 65)}, false},
		{"ed25519 32 bytes ok", PublicKey{Type: KeyTypeEd25519, Bytes: make([]byte, ed25519.PublicKeySize)}, true},
		{"ed25519 short rejected", PublicKey{Type: KeyTypeEd25519, Bytes: make([]byte, 31)}, false},
		{"reserved type rejected", PublicKey{Type: KeyTypeP256, Bytes: make([]byte, 33)}, false},
		{"unknown type rejected", PublicKey{Type: KeyType(99), Bytes: make([]byte, 33)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.k.Validate()
			if c.ok && err != nil {
				t.Errorf("expected ok, got %v", err)
			}
			if !c.ok && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestVerifyEd25519RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("attestation-payload")
	sig := ed25519.Sign(priv, msg)

	k := PublicKey{Type: KeyTypeEd25519, Bytes: pub}
	if err := k.Verify(msg, sig); err != nil {
		t.Errorf("expected verify ok, got %v", err)
	}
	// Tamper with the message: signature must fail.
	if err := k.Verify(append(msg, '!'), sig); err == nil {
		t.Errorf("expected tampered msg to fail")
	}
	// Wrong-length signature: must fail with a clean error, not panic.
	if err := k.Verify(msg, sig[:32]); err == nil {
		t.Errorf("expected short-sig rejection")
	}
}

func TestVerifySecp256k1RoundTrip(t *testing.T) {
	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey().Bytes()
	msg := []byte("device-attestation")
	sig, err := priv.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	k := PublicKey{Type: KeyTypeSecp256k1, Bytes: pub}
	if err := k.Verify(msg, sig); err != nil {
		t.Errorf("expected verify ok, got %v", err)
	}
	if err := k.Verify(append(msg, 0xff), sig); err == nil {
		t.Errorf("expected tampered msg to fail")
	}
}
