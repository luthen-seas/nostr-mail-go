// Package spam implements the anti-spam tier evaluation system for NOSTR Mail.
//
// Incoming messages are classified into tiers 0-5 based on the sender's
// relationship to the recipient, identity verification, proof-of-work, and
// ecash payment. Tiers are evaluated in priority order; the first matching
// tier wins.
//
// Tier 0: Sender in recipient's contact list (kind 3) — FREE
// Tier 1: Sender has a verified NIP-05 identifier — FREE
// Tier 2: Event has PoW >= policy threshold (NIP-13) — FREE (compute)
// Tier 3: Valid Cashu P2PK token >= threshold sats — PAID
// Tier 5: None of the above — quarantine or reject
package spam

import (
	"fmt"

	"github.com/nostr-mail/second-go/pkg/mail"
)

// SpamTier holds the classification result for an incoming message.
type SpamTier struct {
	Tier   int    // 0, 1, 2, 3, or 5
	Name   string // human-readable tier name
	Reason string // explanation of why this tier was selected
	Action string // "inbox", "quarantine", or "reject"
}

// Policy describes the recipient's anti-spam requirements, typically derived
// from a kind 10097 event.
type Policy struct {
	ContactsFree  bool     // whether contacts get free delivery (tier 0)
	NIP05Free     bool     // whether NIP-05 verified senders get free delivery (tier 1)
	POWMinBits    int      // minimum proof-of-work bits for tier 2
	CashuMinSats  int64    // minimum Cashu payment in sats for tier 3
	AcceptedMints []string // list of trusted Cashu mint URLs
	UnknownAction string   // "quarantine" or "reject" for tier 5
}

// DefaultPolicy returns a sensible default policy where contacts and NIP-05
// are free, PoW requires 20 bits, Cashu requires 10 sats, and unknown senders
// are quarantined.
func DefaultPolicy() Policy {
	return Policy{
		ContactsFree:  true,
		NIP05Free:     true,
		POWMinBits:    20,
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
//   - nip05Verified: whether the sender's NIP-05 has been verified
//   - powBits: number of leading zero bits in the seal or gift-wrap event ID
//   - postage: Cashu postage token from the rumor, or nil
//   - policy: the recipient's anti-spam policy
//
// Tiers are evaluated in order from 0 to 5. The first matching tier is returned.
func EvaluateTier(
	senderPubKey string,
	contacts map[string]bool,
	nip05Verified bool,
	powBits int,
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

	// Tier 1: NIP-05 verified.
	if policy.NIP05Free && nip05Verified {
		return SpamTier{
			Tier:   1,
			Name:   "NIP-05 Verified",
			Reason: "Sender has verified NIP-05 identifier.",
			Action: "inbox",
		}
	}

	// Tier 2: Proof of Work.
	if policy.POWMinBits > 0 && powBits >= policy.POWMinBits {
		return SpamTier{
			Tier:   2,
			Name:   "Proof of Work",
			Reason: fmt.Sprintf("Event has %d leading zero bits, meeting the minimum threshold of %d bits.", powBits, policy.POWMinBits),
			Action: "inbox",
		}
	}

	// Tier 3: Cashu payment.
	if postage != nil && postage.Amount >= policy.CashuMinSats && policy.CashuMinSats > 0 {
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
				Tier:   3,
				Name:   "Cashu Payment",
				Reason: fmt.Sprintf("Cashu P2PK token attached with %d sats (>= %d sat minimum).", postage.Amount, policy.CashuMinSats),
				Action: "inbox",
			}
		}
	}

	// Tier 5: Unknown — no qualifying signal.
	action := policy.UnknownAction
	if action == "" {
		action = "quarantine"
	}

	return SpamTier{
		Tier:   5,
		Name:   "Unknown",
		Reason: "Sender not in contacts, no NIP-05 verification, no proof-of-work, no payment.",
		Action: action,
	}
}
