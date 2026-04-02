// Package wrap implements NIP-59 seal and gift-wrap encryption for NOSTR Mail.
//
// The three-layer encryption model is:
//  1. Rumor (kind 1400) — unsigned mail content
//  2. Seal (kind 13) — NIP-44 encrypt rumor with ECDH(sender, recipient),
//     signed by sender, randomized timestamp, empty tags
//  3. Gift Wrap (kind 1059) — NIP-44 encrypt seal with ECDH(ephemeral, recipient),
//     signed by ephemeral key, randomized timestamp, p-tag for recipient
package wrap

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"

	"github.com/nostr-mail/nostr-mail-go/pkg/mail"
)

// maxTimestampJitter is the maximum random offset applied to seal and wrap
// timestamps, specified as ±2 days in seconds.
const maxTimestampJitter = 2 * 24 * 60 * 60 // 172800 seconds

// randomTimestampOffset returns a cryptographically random offset in the range
// [-maxTimestampJitter, +maxTimestampJitter].
func randomTimestampOffset() (int64, error) {
	// Generate a random value in [0, 2*max] then subtract max to center at 0.
	rangeSize := big.NewInt(2 * int64(maxTimestampJitter))
	n, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return 0, fmt.Errorf("generating random timestamp offset: %w", err)
	}
	return n.Int64() - int64(maxTimestampJitter), nil
}

// generateEphemeralKey creates a fresh random private key for gift wrapping.
func generateEphemeralKey() string {
	return nostr.GeneratePrivateKey()
}

// nip44Encrypt encrypts plaintext using NIP-44 with the shared secret derived
// from the sender's private key and recipient's public key.
func nip44Encrypt(plaintext string, senderPrivKey string, recipientPubKey string) (string, error) {
	sharedKey, err := nip44.GenerateConversationKey(senderPrivKey, recipientPubKey)
	if err != nil {
		return "", fmt.Errorf("generating conversation key: %w", err)
	}
	ciphertext, err := nip44.Encrypt(plaintext, sharedKey)
	if err != nil {
		return "", fmt.Errorf("nip44 encrypt: %w", err)
	}
	return ciphertext, nil
}

// nip44Decrypt decrypts NIP-44 ciphertext using the shared secret derived from
// the recipient's private key and sender's public key.
func nip44Decrypt(ciphertext string, recipientPrivKey string, senderPubKey string) (string, error) {
	sharedKey, err := nip44.GenerateConversationKey(recipientPrivKey, senderPubKey)
	if err != nil {
		return "", fmt.Errorf("generating conversation key: %w", err)
	}
	plaintext, err := nip44.Decrypt(ciphertext, sharedKey)
	if err != nil {
		return "", fmt.Errorf("nip44 decrypt: %w", err)
	}
	return plaintext, nil
}

// WrapMail seals and gift-wraps a rumor for a specific recipient.
// It returns a kind 1059 gift-wrap event ready for relay publication.
//
// The process:
//  1. Serialize the rumor to JSON.
//  2. NIP-44 encrypt with ECDH(senderPrivKey, recipientPubKey) to create the seal content.
//  3. Build kind 13 seal event (sender pubkey, empty tags, randomized timestamp), sign it.
//  4. Serialize the seal to JSON.
//  5. Generate an ephemeral keypair.
//  6. NIP-44 encrypt with ECDH(ephemeralPrivKey, recipientPubKey) to create the wrap content.
//  7. Build kind 1059 gift-wrap event (ephemeral pubkey, p-tag for recipient, randomized timestamp), sign it.
func WrapMail(rumor mail.Rumor, senderPrivKey string, recipientPubKey string) (*nostr.Event, error) {
	// Step 1: Serialize the rumor.
	rumorJSON, err := json.Marshal(rumor)
	if err != nil {
		return nil, fmt.Errorf("marshaling rumor: %w", err)
	}

	// Step 2: Encrypt rumor for the seal.
	sealContent, err := nip44Encrypt(string(rumorJSON), senderPrivKey, recipientPubKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting rumor for seal: %w", err)
	}

	// Step 3: Build and sign the seal.
	senderPubKey, err := nostr.GetPublicKey(senderPrivKey)
	if err != nil {
		return nil, fmt.Errorf("deriving sender public key: %w", err)
	}

	sealOffset, err := randomTimestampOffset()
	if err != nil {
		return nil, err
	}
	sealTimestamp := time.Now().Unix() + sealOffset

	seal := nostr.Event{
		Kind:      13,
		PubKey:    senderPubKey,
		CreatedAt: nostr.Timestamp(sealTimestamp),
		Tags:      nostr.Tags{}, // empty tags — no metadata leakage
		Content:   sealContent,
	}
	if err := seal.Sign(senderPrivKey); err != nil {
		return nil, fmt.Errorf("signing seal: %w", err)
	}

	// Step 4: Serialize the seal.
	sealJSON, err := json.Marshal(seal)
	if err != nil {
		return nil, fmt.Errorf("marshaling seal: %w", err)
	}

	// Step 5: Generate ephemeral keypair.
	ephPrivKey := generateEphemeralKey()
	ephPubKey, err := nostr.GetPublicKey(ephPrivKey)
	if err != nil {
		return nil, fmt.Errorf("deriving ephemeral public key: %w", err)
	}

	// Step 6: Encrypt seal for the gift wrap.
	wrapContent, err := nip44Encrypt(string(sealJSON), ephPrivKey, recipientPubKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting seal for wrap: %w", err)
	}

	// Step 7: Build and sign the gift wrap.
	wrapOffset, err := randomTimestampOffset()
	if err != nil {
		return nil, err
	}
	wrapTimestamp := time.Now().Unix() + wrapOffset

	wrap := nostr.Event{
		Kind:      1059,
		PubKey:    ephPubKey,
		CreatedAt: nostr.Timestamp(wrapTimestamp),
		Tags: nostr.Tags{
			nostr.Tag{"p", recipientPubKey},
		},
		Content: wrapContent,
	}
	if err := wrap.Sign(ephPrivKey); err != nil {
		return nil, fmt.Errorf("signing gift wrap: %w", err)
	}

	return &wrap, nil
}

// UnwrapMail decrypts a kind 1059 gift-wrap event and returns the contained
// mail rumor, the sender's public key (from the seal), whether the seal
// signature was valid, and any error encountered.
//
// The process:
//  1. NIP-44 decrypt gift-wrap content with ECDH(recipientPrivKey, wrap.pubkey) to get the seal.
//  2. Parse the seal JSON and verify its Schnorr signature.
//  3. NIP-44 decrypt seal content with ECDH(recipientPrivKey, seal.pubkey) to get the rumor.
//  4. Parse the rumor JSON.
func UnwrapMail(wrapEvent *nostr.Event, recipientPrivKey string) (*mail.Rumor, string, bool, error) {
	if wrapEvent.Kind != 1059 {
		return nil, "", false, fmt.Errorf("expected kind 1059 gift wrap, got kind %d", wrapEvent.Kind)
	}

	// Step 1: Decrypt gift wrap to get the seal.
	sealJSON, err := nip44Decrypt(wrapEvent.Content, recipientPrivKey, wrapEvent.PubKey)
	if err != nil {
		return nil, "", false, fmt.Errorf("decrypting gift wrap: %w", err)
	}

	// Step 2: Parse and verify the seal.
	var seal nostr.Event
	if err := json.Unmarshal([]byte(sealJSON), &seal); err != nil {
		return nil, "", false, fmt.Errorf("parsing seal JSON: %w", err)
	}

	if seal.Kind != 13 {
		return nil, "", false, fmt.Errorf("expected kind 13 seal, got kind %d", seal.Kind)
	}

	sigValid, err := seal.CheckSignature()
	if err != nil {
		sigValid = false
	}

	senderPubKey := seal.PubKey

	// Step 3: Decrypt seal to get the rumor.
	rumorJSON, err := nip44Decrypt(seal.Content, recipientPrivKey, seal.PubKey)
	if err != nil {
		return nil, senderPubKey, sigValid, fmt.Errorf("decrypting seal: %w", err)
	}

	// Step 4: Parse the rumor.
	var rumor mail.Rumor
	if err := json.Unmarshal([]byte(rumorJSON), &rumor); err != nil {
		return nil, senderPubKey, sigValid, fmt.Errorf("parsing rumor JSON: %w", err)
	}

	return &rumor, senderPubKey, sigValid, nil
}

// WrapForMultipleRecipients wraps the same rumor for each recipient, producing
// one distinct gift-wrap event per recipient. Each wrap uses a unique ephemeral
// key and fresh random nonces.
func WrapForMultipleRecipients(rumor mail.Rumor, senderPrivKey string, recipientPubKeys []string) ([]*nostr.Event, error) {
	wraps := make([]*nostr.Event, 0, len(recipientPubKeys))
	for _, pubKey := range recipientPubKeys {
		wrap, err := WrapMail(rumor, senderPrivKey, pubKey)
		if err != nil {
			return nil, fmt.Errorf("wrapping for recipient %s: %w", pubKey, err)
		}
		wraps = append(wraps, wrap)
	}
	return wraps, nil
}
