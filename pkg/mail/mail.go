// Package mail implements kind 1111 NOSTR Mail event creation and parsing.
//
// A kind 1111 event is a "rumor" — an unsigned event that carries structured
// email-like messages over the NOSTR protocol. Rumors are never published
// directly; they are sealed (kind 13) and gift-wrapped (kind 1059) before
// transmission.
package mail

import (
	"fmt"
	"strconv"
	"time"
)

// Rumor represents an unsigned kind 1111 mail event.
// It intentionally omits the id and sig fields because rumors are unsigned.
type Rumor struct {
	Kind      int        `json:"kind"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
}

// Recipient describes a mail recipient with their public key, optional relay
// hint, and role (to, cc, or bcc).
type Recipient struct {
	PubKey string
	Relay  string
	Role   string // "to", "cc", "bcc"
}

// Attachment describes a Blossom-hosted encrypted file attachment.
type Attachment struct {
	Hash          string
	Filename      string
	MimeType      string
	Size          int64
	EncryptionKey string
}

// InlineImage describes an inline image referenced by content-id in the body.
type InlineImage struct {
	Hash          string
	ContentID     string
	EncryptionKey string
}

// CashuPostage holds a Cashu ecash token used as anti-spam postage.
type CashuPostage struct {
	Token  string
	Mint   string
	Amount int64
	P2PK   bool
}

// CreateParams holds all parameters needed to construct a kind 1111 mail rumor.
type CreateParams struct {
	SenderPubKey string
	Recipients   []Recipient
	Subject      string
	Body         string
	ContentType  string // default "text/plain"
	Attachments  []Attachment
	InlineImages []InlineImage
	BlossomURLs  []string
	Postage      *CashuPostage
	ReplyTo      string
	ReplyRelay   string
	ThreadID     string
	ThreadRelay  string
	CreatedAt    int64 // 0 = use current time
}

// ParsedMail is the structured representation extracted from a kind 1111 rumor.
type ParsedMail struct {
	From         string
	Recipients   []Recipient
	Subject      string
	Body         string
	ContentType  string
	Attachments  []Attachment
	InlineImages []InlineImage
	BlossomURLs  []string
	Postage      *CashuPostage
	ReplyTo      string
	ReplyRelay   string
	ThreadID     string
	ThreadRelay  string
}

// CreateRumor builds a kind 1111 mail rumor from the given parameters.
// It assembles the tag array in the canonical order specified by the protocol:
// recipients (p tags), subject, content-type, reply, thread, attachments,
// attachment-keys, inline images, blossom servers, and cashu tokens.
func CreateRumor(p CreateParams) Rumor {
	createdAt := p.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	var tags [][]string

	// Recipient p tags: ["p", pubkey, relay, role]
	for _, r := range p.Recipients {
		relay := r.Relay
		role := r.Role
		if role == "" {
			role = "to"
		}
		tags = append(tags, []string{"p", r.PubKey, relay, role})
	}

	// Subject tag
	if p.Subject != "" {
		tags = append(tags, []string{"subject", p.Subject})
	}

	// Content-type tag (omit if text/plain or empty, which defaults to text/plain)
	if p.ContentType != "" && p.ContentType != "text/plain" {
		tags = append(tags, []string{"content-type", p.ContentType})
	}

	// Reply tag: ["reply", parentEventId, relayHint]
	if p.ReplyTo != "" {
		tags = append(tags, []string{"reply", p.ReplyTo, p.ReplyRelay})
	}

	// Thread tag: ["thread", rootEventId, relayHint]
	if p.ThreadID != "" {
		tags = append(tags, []string{"thread", p.ThreadID, p.ThreadRelay})
	}

	// Attachment tags: ["attachment", hash, filename, mime, size]
	for _, a := range p.Attachments {
		sizeStr := fmt.Sprintf("%d", a.Size)
		tags = append(tags, []string{"attachment", a.Hash, a.Filename, a.MimeType, sizeStr})
	}

	// Attachment-key tags for file attachments: ["attachment-key", hash, encKey]
	for _, a := range p.Attachments {
		if a.EncryptionKey != "" {
			tags = append(tags, []string{"attachment-key", a.Hash, a.EncryptionKey})
		}
	}

	// Inline image tags: ["inline", hash, contentId]
	for _, img := range p.InlineImages {
		tags = append(tags, []string{"inline", img.Hash, img.ContentID})
	}

	// Attachment-key tags for inline images
	for _, img := range p.InlineImages {
		if img.EncryptionKey != "" {
			tags = append(tags, []string{"attachment-key", img.Hash, img.EncryptionKey})
		}
	}

	// Blossom server tags: ["blossom", url1, url2, ...]
	if len(p.BlossomURLs) > 0 {
		tag := []string{"blossom"}
		tag = append(tag, p.BlossomURLs...)
		tags = append(tags, tag)
	}

	// Cashu postage token: ["cashu", serializedToken]
	if p.Postage != nil && p.Postage.Token != "" {
		tags = append(tags, []string{"cashu", p.Postage.Token})
	}

	return Rumor{
		Kind:      1111,
		PubKey:    p.SenderPubKey,
		CreatedAt: createdAt,
		Tags:      tags,
		Content:   p.Body,
	}
}

// ParseRumor extracts structured data from a kind 1111 rumor.
// It reads the tag array and populates all fields of ParsedMail.
// Unknown tags are silently ignored per the conformance spec.
func ParseRumor(r Rumor) ParsedMail {
	m := ParsedMail{
		From:        r.PubKey,
		Body:        r.Content,
		ContentType: "text/plain", // default
	}

	// Index attachment-key tags by hash for linking with attachments and inline images.
	encKeys := make(map[string]string)
	for _, tag := range r.Tags {
		if len(tag) >= 3 && tag[0] == "attachment-key" {
			encKeys[tag[1]] = tag[2]
		}
	}

	for _, tag := range r.Tags {
		if len(tag) == 0 {
			continue
		}
		switch tag[0] {
		case "p":
			if len(tag) >= 2 {
				rec := Recipient{PubKey: tag[1]}
				if len(tag) >= 3 {
					rec.Relay = tag[2]
				}
				if len(tag) >= 4 {
					rec.Role = tag[3]
				}
				m.Recipients = append(m.Recipients, rec)
			}
		case "subject":
			if len(tag) >= 2 {
				m.Subject = tag[1]
			}
		case "content-type":
			if len(tag) >= 2 {
				m.ContentType = tag[1]
			}
		case "reply":
			if len(tag) >= 2 {
				m.ReplyTo = tag[1]
			}
			if len(tag) >= 3 {
				m.ReplyRelay = tag[2]
			}
		case "thread":
			if len(tag) >= 2 {
				m.ThreadID = tag[1]
			}
			if len(tag) >= 3 {
				m.ThreadRelay = tag[2]
			}
		case "attachment":
			if len(tag) >= 5 {
				size, _ := strconv.ParseInt(tag[4], 10, 64)
				a := Attachment{
					Hash:     tag[1],
					Filename: tag[2],
					MimeType: tag[3],
					Size:     size,
				}
				if key, ok := encKeys[a.Hash]; ok {
					a.EncryptionKey = key
				}
				m.Attachments = append(m.Attachments, a)
			}
		case "inline":
			if len(tag) >= 3 {
				img := InlineImage{
					Hash:      tag[1],
					ContentID: tag[2],
				}
				if key, ok := encKeys[img.Hash]; ok {
					img.EncryptionKey = key
				}
				m.InlineImages = append(m.InlineImages, img)
			}
		case "blossom":
			if len(tag) >= 2 {
				m.BlossomURLs = append(m.BlossomURLs, tag[1:]...)
			}
		case "cashu":
			if len(tag) >= 2 {
				if m.Postage == nil {
					m.Postage = &CashuPostage{}
				}
				m.Postage.Token = tag[1]
			}
		case "cashu-mint":
			if len(tag) >= 2 {
				if m.Postage == nil {
					m.Postage = &CashuPostage{}
				}
				m.Postage.Mint = tag[1]
			}
		case "cashu-amount":
			if len(tag) >= 2 {
				if m.Postage == nil {
					m.Postage = &CashuPostage{}
				}
				m.Postage.Amount, _ = strconv.ParseInt(tag[1], 10, 64)
			}
		}
	}

	return m
}
