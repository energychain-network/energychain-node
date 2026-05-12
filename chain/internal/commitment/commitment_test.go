package commitment

import "testing"

func TestPlaintextRoundTrip(t *testing.T) {
	var nonce [32]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}

	c := HashPlaintext("meter.read.v1", 12345, nonce)
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := VerifyPlaintext(c, 12345, nonce); err != nil {
		t.Fatalf("VerifyPlaintext: %v", err)
	}
	if err := VerifyPlaintext(c, 12346, nonce); err == nil {
		t.Fatalf("VerifyPlaintext should reject wrong value")
	}
	var bad [32]byte
	if err := VerifyPlaintext(c, 12345, bad); err == nil {
		t.Fatalf("VerifyPlaintext should reject wrong nonce")
	}
}

func TestValidateRejectsZeroValue(t *testing.T) {
	cases := []Commitment{
		{},
		{Scheme: SchemePlaintextSHA256},
		{Scheme: SchemePlaintextSHA256, Domain: "x"},
		{Scheme: SchemePlaintextSHA256, Domain: "x", Value: make([]byte, 31)},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}
