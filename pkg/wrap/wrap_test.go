package wrap

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/nostr-mail/nostr-mail-go/pkg/mail"
)

// Test keys: Alice and Bob are from the NIP-59 test vectors (known valid keypairs).
// Charlie uses a generated key to guarantee a valid secp256k1 keypair.
var (
	alicePriv = "7f7ff03d123792d6ac594bfa67bf6d0c0ab55b6b1fdb6249303fe861f1ccba9a"
	alicePub  string

	bobPriv = "c15d2a640a7bd00f291e074e5e40419e08593833a5b9bd1b4e89100ef750fa35"
	bobPub  string

	charliePriv string
	charliePub  string
)

func init() {
	// Derive public keys from private keys to guarantee valid keypairs.
	var err error
	alicePub, err = nostr.GetPublicKey(alicePriv)
	if err != nil {
		panic("invalid alice privkey: " + err.Error())
	}
	bobPub, err = nostr.GetPublicKey(bobPriv)
	if err != nil {
		panic("invalid bob privkey: " + err.Error())
	}
	// Generate Charlie's keypair fresh to avoid synthetic key issues.
	charliePriv = nostr.GeneratePrivateKey()
	charliePub, err = nostr.GetPublicKey(charliePriv)
	if err != nil {
		panic("invalid charlie privkey: " + err.Error())
	}
}

func testRumor() mail.Rumor {
	return mail.CreateRumor(mail.CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []mail.Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Round-trip test",
		Body:         "This message should survive seal+wrap+unwrap+unseal intact.",
		CreatedAt:    1711843200,
	})
}

func TestWrapUnwrap_RoundTrip(t *testing.T) {
	rumor := testRumor()

	// Alice wraps for Bob
	wrap, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail failed: %v", err)
	}

	// Verify wrap structure
	if wrap.Kind != 1059 {
		t.Errorf("wrap kind should be 1059, got %d", wrap.Kind)
	}
	if wrap.PubKey == alicePub {
		t.Errorf("wrap pubkey should be ephemeral, not Alice's")
	}
	if wrap.PubKey == bobPub {
		t.Errorf("wrap pubkey should be ephemeral, not Bob's")
	}

	// Verify p-tag points to recipient
	if len(wrap.Tags) != 1 || wrap.Tags[0][0] != "p" || wrap.Tags[0][1] != bobPub {
		t.Errorf("wrap should have exactly one p-tag for Bob, got %v", wrap.Tags)
	}

	// Bob unwraps
	recovered, senderPubKey, sigValid, err := UnwrapMail(wrap, bobPriv)
	if err != nil {
		t.Fatalf("UnwrapMail failed: %v", err)
	}

	// Verify sender identity
	if senderPubKey != alicePub {
		t.Errorf("sender pubkey should be Alice's %s, got %s", alicePub, senderPubKey)
	}
	if !sigValid {
		t.Errorf("seal signature should be valid")
	}

	// Verify rumor content survived the round-trip
	if recovered.Kind != 1400 {
		t.Errorf("recovered kind should be 1400, got %d", recovered.Kind)
	}
	if recovered.PubKey != alicePub {
		t.Errorf("recovered pubkey should be Alice's")
	}
	if recovered.Content != "This message should survive seal+wrap+unwrap+unseal intact." {
		t.Errorf("recovered content mismatch: got %q", recovered.Content)
	}
	if recovered.CreatedAt != 1711843200 {
		t.Errorf("recovered created_at mismatch: got %d", recovered.CreatedAt)
	}

	// Verify tags survived
	subjectTag := findTag(recovered.Tags, "subject")
	if subjectTag == nil || subjectTag[1] != "Round-trip test" {
		t.Errorf("recovered subject tag mismatch: got %v", subjectTag)
	}
}

func TestWrapUnwrap_DifferentEphemeralKeys(t *testing.T) {
	rumor := testRumor()

	wrap1, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail 1 failed: %v", err)
	}

	wrap2, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail 2 failed: %v", err)
	}

	// Each wrap should have a different ephemeral pubkey
	if wrap1.PubKey == wrap2.PubKey {
		t.Errorf("two wraps should use different ephemeral keys, both got %s", wrap1.PubKey)
	}

	// Each wrap should have a different event ID
	if wrap1.ID == wrap2.ID {
		t.Errorf("two wraps should have different event IDs")
	}

	// Different ciphertexts (random nonces)
	if wrap1.Content == wrap2.Content {
		t.Errorf("two wraps should have different ciphertexts")
	}

	// Both should be unwrappable by Bob
	r1, _, _, err := UnwrapMail(wrap1, bobPriv)
	if err != nil {
		t.Fatalf("unwrap 1 failed: %v", err)
	}
	r2, _, _, err := UnwrapMail(wrap2, bobPriv)
	if err != nil {
		t.Fatalf("unwrap 2 failed: %v", err)
	}

	// Both should produce the same rumor content
	if r1.Content != r2.Content {
		t.Errorf("both unwrapped rumors should have same content")
	}
}

func TestWrapUnwrap_TimestampRandomization(t *testing.T) {
	rumor := testRumor()

	// Create multiple wraps and check that timestamps vary
	timestamps := make(map[int64]bool)
	for i := 0; i < 5; i++ {
		wrap, err := WrapMail(rumor, alicePriv, bobPub)
		if err != nil {
			t.Fatalf("WrapMail failed: %v", err)
		}
		timestamps[int64(wrap.CreatedAt)] = true
	}

	// With random offsets, it's extremely unlikely all 5 have the same timestamp
	if len(timestamps) == 1 {
		t.Errorf("wrap timestamps should be randomized, all 5 were identical")
	}
}

func TestUnwrap_NonRecipientCannotDecrypt(t *testing.T) {
	rumor := testRumor()

	// Alice wraps for Bob
	wrap, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail failed: %v", err)
	}

	// Charlie tries to unwrap -- should fail
	_, _, _, err = UnwrapMail(wrap, charliePriv)
	if err == nil {
		t.Errorf("Charlie should NOT be able to decrypt Bob's gift wrap")
	}
}

func TestUnwrap_SenderIdentityVerified(t *testing.T) {
	rumor := testRumor()

	wrap, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail failed: %v", err)
	}

	_, senderPubKey, sigValid, err := UnwrapMail(wrap, bobPriv)
	if err != nil {
		t.Fatalf("UnwrapMail failed: %v", err)
	}

	// The seal is signed by Alice's key, so sender identity is verified
	if senderPubKey != alicePub {
		t.Errorf("sender should be Alice %s, got %s", alicePub, senderPubKey)
	}
	if !sigValid {
		t.Errorf("seal signature should be valid, confirming Alice as sender")
	}
}

func TestWrapForMultipleRecipients(t *testing.T) {
	rumor := mail.CreateRumor(mail.CreateParams{
		SenderPubKey: alicePub,
		Recipients: []mail.Recipient{
			{PubKey: bobPub, Role: "to"},
			{PubKey: charliePub, Role: "cc"},
		},
		Subject:   "Group message",
		Body:      "Hi Bob and Charlie, this goes to both of you.",
		CreatedAt: 1711843200,
	})

	wraps, err := WrapForMultipleRecipients(rumor, alicePriv, []string{bobPub, charliePub})
	if err != nil {
		t.Fatalf("WrapForMultipleRecipients failed: %v", err)
	}

	if len(wraps) != 2 {
		t.Fatalf("expected 2 wraps, got %d", len(wraps))
	}

	// Different ephemeral keys
	if wraps[0].PubKey == wraps[1].PubKey {
		t.Errorf("multi-recipient wraps should use different ephemeral keys")
	}

	// Different event IDs
	if wraps[0].ID == wraps[1].ID {
		t.Errorf("multi-recipient wraps should have different event IDs")
	}

	// Different ciphertexts
	if wraps[0].Content == wraps[1].Content {
		t.Errorf("multi-recipient wraps should have different ciphertexts")
	}

	// Bob can decrypt his wrap
	bobRumor, bobSender, bobSigValid, err := UnwrapMail(wraps[0], bobPriv)
	if err != nil {
		t.Fatalf("Bob unwrap failed: %v", err)
	}
	if bobSender != alicePub {
		t.Errorf("Bob's wrap: sender should be Alice")
	}
	if !bobSigValid {
		t.Errorf("Bob's wrap: seal signature should be valid")
	}

	// Charlie can decrypt his wrap
	charlieRumor, charlieSender, charlieSigValid, err := UnwrapMail(wraps[1], charliePriv)
	if err != nil {
		t.Fatalf("Charlie unwrap failed: %v", err)
	}
	if charlieSender != alicePub {
		t.Errorf("Charlie's wrap: sender should be Alice")
	}
	if !charlieSigValid {
		t.Errorf("Charlie's wrap: seal signature should be valid")
	}

	// Both recovered rumors should be identical
	if bobRumor.Content != charlieRumor.Content {
		t.Errorf("Bob and Charlie should recover the same rumor content")
	}
	if bobRumor.Kind != charlieRumor.Kind {
		t.Errorf("Bob and Charlie should recover the same kind")
	}

	// Bob CANNOT decrypt Charlie's wrap
	_, _, _, err = UnwrapMail(wraps[1], bobPriv)
	if err == nil {
		t.Errorf("Bob should NOT be able to decrypt Charlie's gift wrap")
	}

	// Charlie CANNOT decrypt Bob's wrap
	_, _, _, err = UnwrapMail(wraps[0], charliePriv)
	if err == nil {
		t.Errorf("Charlie should NOT be able to decrypt Bob's gift wrap")
	}
}

func TestSelfCopyWrapping(t *testing.T) {
	rumor := testRumor()

	// Wrap for Bob AND for self (Alice)
	recipients := []string{bobPub, alicePub}
	wraps, err := WrapForMultipleRecipients(rumor, alicePriv, recipients)
	if err != nil {
		t.Fatalf("WrapForMultipleRecipients failed: %v", err)
	}

	if len(wraps) != 2 {
		t.Fatalf("expected 2 wraps (Bob + self), got %d", len(wraps))
	}

	// Wrap for Bob: p-tag should be Bob's pubkey
	if wraps[0].Tags[0][1] != bobPub {
		t.Errorf("first wrap p-tag should be Bob, got %s", wraps[0].Tags[0][1])
	}

	// Wrap for Alice (self): p-tag should be Alice's pubkey
	if wraps[1].Tags[0][1] != alicePub {
		t.Errorf("second wrap p-tag should be Alice (self-copy), got %s", wraps[1].Tags[0][1])
	}

	// Alice can decrypt her self-copy
	selfRumor, selfSender, selfSigValid, err := UnwrapMail(wraps[1], alicePriv)
	if err != nil {
		t.Fatalf("Alice self-copy unwrap failed: %v", err)
	}
	if selfSender != alicePub {
		t.Errorf("self-copy sender should be Alice")
	}
	if !selfSigValid {
		t.Errorf("self-copy seal signature should be valid")
	}
	if selfRumor.Content != rumor.Content {
		t.Errorf("self-copy content mismatch")
	}

	// Bob CANNOT decrypt Alice's self-copy
	_, _, _, err = UnwrapMail(wraps[1], bobPriv)
	if err == nil {
		t.Errorf("Bob should NOT be able to decrypt Alice's self-copy wrap")
	}
}

func TestWrapEvent_StructuralChecks(t *testing.T) {
	rumor := testRumor()

	wrap, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail failed: %v", err)
	}

	// Kind must be 1059
	if wrap.Kind != 1059 {
		t.Errorf("wrap kind must be 1059, got %d", wrap.Kind)
	}

	// Pubkey must be a valid 64-char hex (ephemeral key)
	if len(wrap.PubKey) != 64 {
		t.Errorf("wrap pubkey should be 64 hex chars, got %d", len(wrap.PubKey))
	}

	// ID must be a valid 64-char hex
	if len(wrap.ID) != 64 {
		t.Errorf("wrap ID should be 64 hex chars, got %d", len(wrap.ID))
	}

	// Sig must be a valid 128-char hex
	if len(wrap.Sig) != 128 {
		t.Errorf("wrap sig should be 128 hex chars, got %d", len(wrap.Sig))
	}

	// Verify the wrap signature against the ephemeral pubkey
	valid, err := wrap.CheckSignature()
	if err != nil {
		t.Fatalf("signature check error: %v", err)
	}
	if !valid {
		t.Errorf("wrap signature should be valid for ephemeral pubkey")
	}

	// Content should not be empty
	if wrap.Content == "" {
		t.Errorf("wrap content (encrypted seal) should not be empty")
	}
}

func TestSealLayer_Structure(t *testing.T) {
	rumor := testRumor()

	wrap, err := WrapMail(rumor, alicePriv, bobPub)
	if err != nil {
		t.Fatalf("WrapMail failed: %v", err)
	}

	// Unwrap the outer layer to inspect the seal
	sealJSON, err := nip44Decrypt(wrap.Content, bobPriv, wrap.PubKey)
	if err != nil {
		t.Fatalf("decrypting wrap content failed: %v", err)
	}

	var seal nostr.Event
	if err := json.Unmarshal([]byte(sealJSON), &seal); err != nil {
		t.Fatalf("parsing seal JSON failed: %v", err)
	}

	// Seal kind must be 13
	if seal.Kind != 13 {
		t.Errorf("seal kind must be 13, got %d", seal.Kind)
	}

	// Seal pubkey must be the sender (Alice)
	if seal.PubKey != alicePub {
		t.Errorf("seal pubkey should be Alice's %s, got %s", alicePub, seal.PubKey)
	}

	// Seal tags must be empty (no metadata leakage)
	if len(seal.Tags) != 0 {
		t.Errorf("seal tags must be empty, got %v", seal.Tags)
	}

	// Seal signature must be valid
	valid, err := seal.CheckSignature()
	if err != nil {
		t.Fatalf("seal signature check error: %v", err)
	}
	if !valid {
		t.Errorf("seal signature should be valid for Alice's pubkey")
	}
}

func TestUnwrap_WrongKindRejected(t *testing.T) {
	// Create a fake event with wrong kind
	fake := &nostr.Event{
		Kind:   1, // not 1059
		PubKey: "0000000000000000000000000000000000000000000000000000000000000000",
	}

	_, _, _, err := UnwrapMail(fake, bobPriv)
	if err == nil {
		t.Errorf("should reject non-1059 events")
	}
}

// --- helper ---

func findTag(tags [][]string, name string) []string {
	for _, t := range tags {
		if len(t) > 0 && t[0] == name {
			return t
		}
	}
	return nil
}
