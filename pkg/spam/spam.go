// Package spam implements the anti-spam tier evaluation system for NOSTR Mail.
//
// Incoming messages are classified into tiers 0-2 based on the sender's
// relationship to the recipient and ecash payment. Tiers are evaluated in
// priority order; the first matching tier wins.
//
// Tier 0: Sender in recipient's contact list (kind 3) — FREE
// Tier 1: Valid Cashu P2PK token >= threshold sats — PAID
// Tier 2: None of the above — quarantine or reject
package spam

import (
	"fmt"

	"github.com/nostr-mail/nostr-mail-go/pkg/mail"
)

// SpamTier holds the classification result for an incoming message.
type SpamTier struct {
	Tier   int    // 0, 1, or 2
	Name   string // human-readable tier name
	Reason string // explanation of why this tier was selected
	Action string // "inbox", "quarantine", or "reject"
}

// Policy describes the recipient's anti-spam requirements, typically derived
// from a kind 10097 event.
type Policy struct {
	ContactsFree  bool     // whether contacts get free delivery (tier 0)
	CashuMinSats  int64    // minimum Cashu payment in sats for tier 1
	AcceptedMints []string // list of trusted Cashu mint URLs
	UnknownAction string   // "quarantine" or "reject" for tier 2
}

// DefaultPolicy returns a sensible default policy where contacts are free,
// Cashu requires 10 sats, and unknown senders are quarantined.
func DefaultPolicy() Policy {
	return Policy{
		ContactsFree:  true,
		CashuMinSats:  10,
		AcceptedMints: nil,
		UnknownAction: "quarantine",
	}
}

// EvaluateTier determines the anti-spam tier for an incoming message.
//
// Parameters:
//   - senderPubKey: the hex public key of the sender (from the seal)
//   - contacts: set of hex public keys in the recipient's kind 3 follow list
//   - postage: Cashu postage token from the rumor, or nil
//   - policy: the recipient's anti-spam policy
//
// Tiers are evaluated in order from 0 to 2. The first matching tier is returned.
func EvaluateTier(
	senderPubKey string,
	contacts map[string]bool,
	postage *mail.CashuPostage,
	policy Policy,
) SpamTier {
	// Tier 0: Sender in contact list.
	if policy.ContactsFree && contacts[senderPubKey] {
		return SpamTier{
			Tier:   0,
			Name:   "Contact",
			Reason: fmt.Sprintf("Sender pubkey %.4s...%.4s found in recipient's kind 3 contact list.", senderPubKey, senderPubKey[len(senderPubKey)-4:]),
			Action: "inbox",
		}
	}

	// Tier 1: Cashu payment (must be P2PK locked).
	if postage != nil && postage.P2PK && postage.Amount >= policy.CashuMinSats && policy.CashuMinSats > 0 {
		// If accepted mints are specified, verify the token's mint is trusted.
		mintAccepted := len(policy.AcceptedMints) == 0 // no restrictions means all accepted
		if !mintAccepted && postage.Mint != "" {
			for _, accepted := range policy.AcceptedMints {
				if accepted == postage.Mint {
					mintAccepted = true
					break
				}
			}
		}
		if mintAccepted {
			return SpamTier{
				Tier:   1,
				Name:   "Cashu Payment",
				Reason: fmt.Sprintf("Cashu P2PK token attached with %d sats (>= %d sat minimum).", postage.Amount, policy.CashuMinSats),
				Action: "inbox",
			}
		}
	}

	// Tier 2: Unknown — no qualifying signal.
	action := policy.UnknownAction
	if action == "" {
		action = "quarantine"
	}

	return SpamTier{
		Tier:   2,
		Name:   "Unknown",
		Reason: "Sender not in contacts and no valid Cashu postage.",
		Action: action,
	}
}
