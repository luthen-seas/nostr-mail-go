package spam

import (
	"testing"

	"github.com/nostr-mail/nostr-mail-go/pkg/mail"
)

const (
	alicePub   = "2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748"
	bobPub     = "98b30d5bfd1e2e751d7a57e7a58e67e15b3f2e0a90f9f7e8e40f7f6e5d4c3b2a"
	unknownPub = "f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3"
)

// testPolicy mirrors the test vectors' recipient_policy (Bob's policy).
func testPolicy() Policy {
	return Policy{
		ContactsFree:  true,
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
	result := EvaluateTier(alicePub, bobContacts(), nil, testPolicy())

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

func TestTier1_CashuP2PK(t *testing.T) {
	// Unknown sender with valid Cashu P2PK token >= 10 sats.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   true,
	}
	cashuPub := "f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2"
	result := EvaluateTier(cashuPub, bobContacts(), postage, testPolicy())

	if result.Tier != 1 {
		t.Errorf("expected tier 1, got %d", result.Tier)
	}
	if result.Action != "inbox" {
		t.Errorf("expected action inbox, got %s", result.Action)
	}
}

func TestTier2_Unknown(t *testing.T) {
	// Unknown sender: not in contacts, no Cashu.
	result := EvaluateTier(unknownPub, bobContacts(), nil, testPolicy())

	if result.Tier != 2 {
		t.Errorf("expected tier 2, got %d", result.Tier)
	}
	if result.Action != "quarantine" {
		t.Errorf("expected quarantine, got %s", result.Action)
	}
}

func TestTierPriority_ContactWinsOverCashu(t *testing.T) {
	// Alice is a contact AND has Cashu. Tier 0 should win.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   true,
	}
	result := EvaluateTier(alicePub, bobContacts(), postage, testPolicy())

	if result.Tier != 0 {
		t.Errorf("expected tier 0 (Contact wins over Cashu), got %d", result.Tier)
	}
}

func TestCashu_NonP2PKRejected(t *testing.T) {
	// Cashu token that is NOT P2PK locked should not qualify for tier 1.
	postage := &mail.CashuPostage{
		Token:  "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i",
		Mint:   "https://mint.example.com",
		Amount: 21,
		P2PK:   false,
	}
	result := EvaluateTier(unknownPub, bobContacts(), postage, testPolicy())

	if result.Tier == 1 {
		t.Errorf("non-P2PK Cashu token should not qualify for tier 1")
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
	result := EvaluateTier(unknownPub, bobContacts(), postage, testPolicy())

	if result.Tier == 1 {
		t.Errorf("5 sats should not qualify for tier 1 (min is 10)")
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
	result := EvaluateTier(unknownPub, bobContacts(), postage, testPolicy())

	if result.Tier == 1 {
		t.Errorf("untrusted mint should not qualify for tier 1")
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if !p.ContactsFree {
		t.Errorf("default ContactsFree should be true")
	}
	if p.CashuMinSats != 10 {
		t.Errorf("default CashuMinSats should be 10, got %d", p.CashuMinSats)
	}
	if p.UnknownAction != "quarantine" {
		t.Errorf("default UnknownAction should be quarantine, got %s", p.UnknownAction)
	}
}
