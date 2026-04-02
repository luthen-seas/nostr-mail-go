package spam

import (
	"testing"

	"github.com/nostr-mail/second-go/pkg/mail"
)

const (
	alicePub   = "2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748"
	bobPub     = "98b30d5bfd1e2e751d7a57e7a58e67e15b3f2e0a90f9f7e8e40f7f6e5d4c3b2a"
	charliePub = "d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4"
	unknownPub = "f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3"
)

// testPolicy mirrors the test vectors' recipient_policy (Bob's policy).
func testPolicy() Policy {
	return Policy{
		ContactsFree:  true,
		NIP05Free:     true,
		POWMinBits:    20,
		CashuMinSats:  10,
		AcceptedMints: []string{"https://mint.example.com"},
		UnknownAction: "quarantine",
	}
}

// bobContacts returns Bob's contact list containing Alice.
func bobContacts() map[string]bool {
	return map[string]bool{alicePub: true}
}

func TestTier0_ContactInList(t *testing.T) {
	result := EvaluateTier(alicePub, bobContacts(), false, 0, nil, testPolicy())

	if result.Tier != 0 {
		t.Errorf("expected tier 0, got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected action inbox, got %s", result.Action)
	}
	if result.Name != "Contact" {
		t.Errorf("expected name 'Contact', got %s", result.Name)
	}
}

func TestTier1_NIP05Verified(t *testing.T) {
	// Charlie is NOT in contacts but has verified NIP-05.
	result := EvaluateTier(charliePub, bobContacts(), true, 0, nil, testPolicy())

	if result.Tier != 1 {
		t.Errorf("expected tier 1, got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected action inbox, got %s", result.Action)
	}
	if result.Name != "NIP-05 Verified" {
		t.Errorf("expected name 'NIP-05 Verified', got %s", result.Name)
	}
}

func TestTier2_ProofOfWork(t *testing.T) {
	// Unknown sender with 21 PoW bits (meets >= 20 threshold).
	powPub := "f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1"
	result := EvaluateTier(powPub, bobContacts(), false, 21, nil, testPolicy())

	if result.Tier != 2 {
		t.Errorf("expected tier 2, got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected action inbox, got %s", result.Action)
	}
}

func TestTier2_PowBelowThreshold(t *testing.T) {
	// 15 bits is below the 20-bit threshold.
	result := EvaluateTier(unknownPub, bobContacts(), false, 15, nil, testPolicy())

	if result.Tier != 5 {
		t.Errorf("expected tier 5 (pow below threshold), got %d", result.Tier)
	}
	if result.Action != "quarantine" {
		t.Errorf("expected quarantine, got %s", result.Action)
	}
}

func TestTier3_CashuP2PK(t *testing.T) {
	// Unknown sender with valid Cashu P2PK token >= 10 sats.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   true,
	}
	cashuPub := "f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2"
	result := EvaluateTier(cashuPub, bobContacts(), false, 0, postage, testPolicy())

	if result.Tier != 3 {
		t.Errorf("expected tier 3, got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected action inbox, got %s", result.Action)
	}
}

func TestTier5_Unknown(t *testing.T) {
	// Unknown sender: not in contacts, no NIP-05, no PoW, no Cashu.
	result := EvaluateTier(unknownPub, bobContacts(), false, 0, nil, testPolicy())

	if result.Tier != 5 {
		t.Errorf("expected tier 5, got %d", result.Tier)
	}
	if result.Action != "quarantine" {
		t.Errorf("expected quarantine, got %s", result.Action)
	}
}

func TestTierPriority_NIP05WinsOverCashu(t *testing.T) {
	// Charlie has NIP-05 AND attaches a Cashu token. Tier 1 should win
	// because tiers are evaluated in order and NIP-05 is checked before Cashu.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   true,
	}
	result := EvaluateTier(charliePub, bobContacts(), true, 0, postage, testPolicy())

	if result.Tier != 1 {
		t.Errorf("expected tier 1 (NIP-05 wins over Cashu), got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected inbox, got %s", result.Action)
	}
}

func TestTierPriority_ContactWinsOverAll(t *testing.T) {
	// Alice is a contact AND has NIP-05 AND has PoW AND has Cashu.
	// Tier 0 should win.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   true,
	}
	result := EvaluateTier(alicePub, bobContacts(), true, 25, postage, testPolicy())

	if result.Tier != 0 {
		t.Errorf("expected tier 0 (Contact wins over all), got %d", result.Tier)
	}
}

func TestCashu_NonP2PKRejected(t *testing.T) {
	// Cashu token that is NOT P2PK locked should not qualify for tier 3.
	// The reference implementation checks for p2pk: true on the postage.
	// A non-P2PK token means amount might be valid but lock is wrong.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   false, // NOT P2PK locked
	}
	result := EvaluateTier(unknownPub, bobContacts(), false, 0, postage, testPolicy())

	// Should NOT get tier 3 -- non-P2PK tokens are treated as if
	// there is no valid postage. The implementation may still evaluate
	// to tier 5 because the postage is invalid.
	if result.Tier == 3 {
		t.Errorf("non-P2PK Cashu token should not qualify for tier 3")
	}
	if result.Action == "inbox" {
		t.Errorf("non-P2PK token should not deliver to inbox")
	}
}

func TestCashu_InsufficientAmount(t *testing.T) {
	// Cashu P2PK token with only 5 sats (below 10-sat minimum).
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 5,
		P2PK:   true,
	}
	result := EvaluateTier(unknownPub, bobContacts(), false, 0, postage, testPolicy())

	if result.Tier == 3 {
		t.Errorf("5 sats should not qualify for tier 3 (min is 10)")
	}
	if result.Action == "inbox" {
		t.Errorf("insufficient Cashu amount should not deliver to inbox")
	}
}

func TestCashu_UntrustedMint(t *testing.T) {
	// Cashu token from a mint not in the accepted list.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://evil-mint.example.com",
		Amount: 100,
		P2PK:   true,
	}
	result := EvaluateTier(unknownPub, bobContacts(), false, 0, postage, testPolicy())

	if result.Tier == 3 {
		t.Errorf("untrusted mint should not qualify for tier 3")
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if !p.ContactsFree {
		t.Errorf("default ContactsFree should be true")
	}
	if !p.NIP05Free {
		t.Errorf("default NIP05Free should be true")
	}
	if p.POWMinBits != 20 {
		t.Errorf("default POWMinBits should be 20, got %d", p.POWMinBits)
	}
	if p.UnknownAction != "quarantine" {
		t.Errorf("default UnknownAction should be quarantine, got %s", p.UnknownAction)
	}
}
