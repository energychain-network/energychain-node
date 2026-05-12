package dsl

import "testing"

func newCtx() Context {
	return Context{
		Sender: Subject{
			DID:          "energy1sender",
			Jurisdiction: "DE",
			Credentials:  map[string]bool{"urn:vc:kyc:passed": true},
		},
		Receiver: Subject{
			DID:          "energy1receiver",
			Jurisdiction: "DE",
			Credentials:  map[string]bool{"urn:vc:kyc:passed": true},
		},
		Amount:       100,
		Now:          1_700_000_000,
		Sanctioned:   map[string]bool{},
		Frozen:       map[string]bool{},
		LockupExpiry: map[string]int64{},
	}
}

func TestRequireKYCAllow(t *testing.T) {
	p := Policy{Name: "kyc", Rules: []Rule{{Kind: RuleRequireKYC, Side: SideBoth}}}
	if r := p.Evaluate(newCtx()); !r.Allowed {
		t.Errorf("expected allow, got %+v", r)
	}
}

func TestRequireKYCDeniesMissingSide(t *testing.T) {
	ctx := newCtx()
	delete(ctx.Receiver.Credentials, "urn:vc:kyc:passed")
	p := Policy{Name: "kyc", Rules: []Rule{{Kind: RuleRequireKYC, Side: SideBoth}}}
	r := p.Evaluate(ctx)
	if r.Allowed {
		t.Fatalf("expected deny, got allow")
	}
	if r.Code != CodeRequireKYC {
		t.Errorf("code = %d, want %d", r.Code, CodeRequireKYC)
	}
}

func TestJurisdictionAllow(t *testing.T) {
	p := Policy{Name: "eu", Rules: []Rule{{
		Kind:   RuleJurisdictionAllow,
		Side:   SideBoth,
		Params: map[string]string{"countries": "DE,FR,NL"},
	}}}
	if r := p.Evaluate(newCtx()); !r.Allowed {
		t.Errorf("expected allow for DE, got %+v", r)
	}

	ctx := newCtx()
	ctx.Receiver.Jurisdiction = "US"
	if r := p.Evaluate(ctx); r.Allowed {
		t.Errorf("expected deny for US, got allow")
	}
}

func TestSanctionsBlocks(t *testing.T) {
	ctx := newCtx()
	ctx.Sanctioned["energy1sender"] = true
	p := Policy{Name: "ofac", Rules: []Rule{{Kind: RuleNotSanctioned, Side: SideBoth}}}
	r := p.Evaluate(ctx)
	if r.Allowed {
		t.Fatalf("expected deny")
	}
	if r.Code != CodeSanctionsHit {
		t.Errorf("code = %d, want %d", r.Code, CodeSanctionsHit)
	}
}

func TestMaxPerHolderCap(t *testing.T) {
	ctx := newCtx()
	ctx.Receiver.Holdings = 950
	ctx.Amount = 100
	p := Policy{Name: "cap1k", Rules: []Rule{{
		Kind: RuleMaxPerHolder, Side: SideReceiver,
		Params: map[string]string{"max": "1000"},
	}}}
	if r := p.Evaluate(ctx); r.Allowed {
		t.Fatalf("expected deny (950+100 > 1000)")
	}
	ctx.Amount = 50
	if r := p.Evaluate(ctx); !r.Allowed {
		t.Fatalf("expected allow (950+50 = 1000)")
	}
}

func TestPolicyValidate(t *testing.T) {
	if err := (Policy{}).Validate(); err == nil {
		t.Errorf("expected empty name to fail")
	}
	if err := (Policy{Name: "x", Rules: []Rule{{Kind: RuleJurisdictionAllow}}}).Validate(); err == nil {
		t.Errorf("expected missing 'countries' param to fail")
	}
}

// TestMaxPerHolderForcedSide locks in the security fix: regardless of
// the rule-declared Side, RuleMaxPerHolder must only ever be evaluated
// against the receiver. A misconfigured Side=SideSender or Side=SideBoth
// must NOT incorrectly block on the sender's pre-transfer holdings (the
// sender's balance is shrinking).
func TestMaxPerHolderForcedSide(t *testing.T) {
	ctx := newCtx()
	ctx.Sender.Holdings = 5_000   // would blow the cap if checked
	ctx.Receiver.Holdings = 100   // well under
	ctx.Amount = 50
	for _, side := range []Side{SideSender, SideReceiver, SideBoth} {
		p := Policy{Name: "cap", Rules: []Rule{{
			Kind: RuleMaxPerHolder, Side: side,
			Params: map[string]string{"max": "1000"},
		}}}
		if r := p.Evaluate(ctx); !r.Allowed {
			t.Errorf("Side=%v: expected allow (cap is on receiver only), got %+v", side, r)
		}
	}
}

// TestMaxPerHolderOverflow exercises the safe-add guard so an attacker-
// supplied Amount near uint64 max cannot wrap to a small post-transfer
// value that slips past the cap.
func TestMaxPerHolderOverflow(t *testing.T) {
	ctx := newCtx()
	ctx.Receiver.Holdings = 1<<63 + 1
	ctx.Amount = 1<<63 + 1 // sum overflows uint64
	p := Policy{Name: "cap", Rules: []Rule{{
		Kind: RuleMaxPerHolder, Side: SideReceiver,
		Params: map[string]string{"max": "100"},
	}}}
	if r := p.Evaluate(ctx); r.Allowed {
		t.Fatalf("overflow should deny, got allow")
	}
}

// TestParseUintOverflow ensures the policy author cannot supply
// "999...999" to wrap to a permissive cap.
func TestParseUintOverflow(t *testing.T) {
	if _, err := parseUint("99999999999999999999"); err == nil {
		t.Errorf("expected overflow error, got nil")
	}
}
