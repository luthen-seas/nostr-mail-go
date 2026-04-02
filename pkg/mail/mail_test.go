package mail

import (
	"testing"
)

const (
	alicePub   = "2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748"
	bobPub     = "98b30d5bfd1e2e751d7a57e7a58e67e15b3f2e0a90f9f7e8e40f7f6e5d4c3b2a"
	charliePub = "d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4"
)

func TestCreateRumor_SimpleMessage(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Hello",
		Body:         "Hi Bob, how are you?",
		CreatedAt:    1711843200,
	})

	if r.Kind != 1400 {
		t.Errorf("expected kind 1400, got %d", r.Kind)
	}
	if r.PubKey != alicePub {
		t.Errorf("pubkey mismatch: got %s", r.PubKey)
	}
	if r.CreatedAt != 1711843200 {
		t.Errorf("created_at mismatch: got %d", r.CreatedAt)
	}
	if r.Content != "Hi Bob, how are you?" {
		t.Errorf("content mismatch: got %q", r.Content)
	}

	// Verify p tag
	pTag := findTag(r.Tags, "p")
	if pTag == nil {
		t.Fatal("missing p tag")
	}
	if pTag[1] != bobPub {
		t.Errorf("p tag pubkey mismatch: got %s", pTag[1])
	}
	if pTag[3] != "to" {
		t.Errorf("p tag role mismatch: got %s", pTag[3])
	}

	// Verify subject tag
	subTag := findTag(r.Tags, "subject")
	if subTag == nil {
		t.Fatal("missing subject tag")
	}
	if subTag[1] != "Hello" {
		t.Errorf("subject mismatch: got %s", subTag[1])
	}

	// Verify no content-type tag for default text/plain
	ctTag := findTag(r.Tags, "content-type")
	if ctTag != nil {
		t.Errorf("should not have content-type tag for default text/plain, got %v", ctTag)
	}
}

func TestCreateRumor_CCRecipients(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients: []Recipient{
			{PubKey: bobPub, Relay: "wss://relay.bob.com", Role: "to"},
			{PubKey: charliePub, Relay: "wss://relay.charlie.com", Role: "cc"},
		},
		Subject:   "Meeting Notes",
		Body:      "Hi Bob, CC'ing Charlie for visibility on the meeting notes.",
		CreatedAt: 1711843200,
	})

	if r.Kind != 1400 {
		t.Errorf("expected kind 1400, got %d", r.Kind)
	}

	// Verify two p tags
	pTags := findAllTags(r.Tags, "p")
	if len(pTags) != 2 {
		t.Fatalf("expected 2 p tags, got %d", len(pTags))
	}

	// First p tag: Bob (to)
	if pTags[0][1] != bobPub {
		t.Errorf("first p tag pubkey mismatch: %s", pTags[0][1])
	}
	if pTags[0][2] != "wss://relay.bob.com" {
		t.Errorf("first p tag relay mismatch: %s", pTags[0][2])
	}
	if pTags[0][3] != "to" {
		t.Errorf("first p tag role mismatch: %s", pTags[0][3])
	}

	// Second p tag: Charlie (cc)
	if pTags[1][1] != charliePub {
		t.Errorf("second p tag pubkey mismatch: %s", pTags[1][1])
	}
	if pTags[1][2] != "wss://relay.charlie.com" {
		t.Errorf("second p tag relay mismatch: %s", pTags[1][2])
	}
	if pTags[1][3] != "cc" {
		t.Errorf("second p tag role mismatch: %s", pTags[1][3])
	}
}

func TestCreateRumor_Reply(t *testing.T) {
	parentID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	r := CreateRumor(CreateParams{
		SenderPubKey: bobPub,
		Recipients:   []Recipient{{PubKey: alicePub, Role: "to"}},
		Subject:      "Re: Hello",
		Body:         "Hi Alice, I'm doing well! Thanks for asking.",
		CreatedAt:    1711846800,
		ReplyTo:      parentID,
		ReplyRelay:   "wss://relay.alice.com",
		ThreadID:     parentID,
		ThreadRelay:  "wss://relay.alice.com",
	})

	// Verify reply tag
	replyTag := findTag(r.Tags, "reply")
	if replyTag == nil {
		t.Fatal("missing reply tag")
	}
	if replyTag[1] != parentID {
		t.Errorf("reply event ID mismatch: got %s", replyTag[1])
	}
	if replyTag[2] != "wss://relay.alice.com" {
		t.Errorf("reply relay hint mismatch: got %s", replyTag[2])
	}

	// Verify thread tag
	threadTag := findTag(r.Tags, "thread")
	if threadTag == nil {
		t.Fatal("missing thread tag")
	}
	if threadTag[1] != parentID {
		t.Errorf("thread event ID mismatch: got %s", threadTag[1])
	}
	if threadTag[2] != "wss://relay.alice.com" {
		t.Errorf("thread relay hint mismatch: got %s", threadTag[2])
	}
}

func TestCreateRumor_ThreadWithDifferentRootAndParent(t *testing.T) {
	rootID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	parentID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Re: Project Update",
		Body:         "Good question -- we're targeting end of Q2.",
		CreatedAt:    1711850400,
		ReplyTo:      parentID,
		ReplyRelay:   "wss://relay.bob.com",
		ThreadID:     rootID,
		ThreadRelay:  "wss://relay.alice.com",
	})

	replyTag := findTag(r.Tags, "reply")
	threadTag := findTag(r.Tags, "thread")

	if replyTag[1] != parentID {
		t.Errorf("reply should point to parent %s, got %s", parentID, replyTag[1])
	}
	if threadTag[1] != rootID {
		t.Errorf("thread should point to root %s, got %s", rootID, threadTag[1])
	}
}

func TestCreateRumor_MarkdownContentType(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Q3 Revenue Report",
		Body:         "## Q3 Report Summary\n\n**Revenue**: $2.4M (+23%)",
		ContentType:  "text/markdown",
		CreatedAt:    1711843200,
	})

	ctTag := findTag(r.Tags, "content-type")
	if ctTag == nil {
		t.Fatal("missing content-type tag for text/markdown")
	}
	if ctTag[1] != "text/markdown" {
		t.Errorf("content-type mismatch: got %s", ctTag[1])
	}
}

func TestCreateRumor_Attachment(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Report attached",
		Body:         "Hi Bob, please find the Q3 report attached.",
		CreatedAt:    1711843200,
		Attachments: []Attachment{{
			Hash:          "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Filename:      "Q3-Report.pdf",
			MimeType:      "application/pdf",
			Size:          2048576,
			EncryptionKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		}},
		BlossomURLs: []string{"https://blossom.example.com"},
	})

	attTag := findTag(r.Tags, "attachment")
	if attTag == nil {
		t.Fatal("missing attachment tag")
	}
	if attTag[1] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("attachment hash mismatch: got %s", attTag[1])
	}
	if attTag[2] != "Q3-Report.pdf" {
		t.Errorf("attachment filename mismatch: got %s", attTag[2])
	}
	if attTag[3] != "application/pdf" {
		t.Errorf("attachment mime mismatch: got %s", attTag[3])
	}
	if attTag[4] != "2048576" {
		t.Errorf("attachment size mismatch: got %s", attTag[4])
	}

	keyTag := findTag(r.Tags, "attachment-key")
	if keyTag == nil {
		t.Fatal("missing attachment-key tag")
	}
	if keyTag[2] != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("attachment-key mismatch: got %s", keyTag[2])
	}

	blossomTag := findTag(r.Tags, "blossom")
	if blossomTag == nil {
		t.Fatal("missing blossom tag")
	}
	if blossomTag[1] != "https://blossom.example.com" {
		t.Errorf("blossom URL mismatch: got %s", blossomTag[1])
	}
}

func TestCreateRumor_CashuPostage(t *testing.T) {
	token := "cashuBo2FteCJodHRwczovL21pbnQuZXhhbXBsZS5jb20i"
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Introduction",
		Body:         "Hi Bob, we met at the conference.",
		CreatedAt:    1711843200,
		Postage:      &CashuPostage{Token: token, Mint: "https://mint.example.com", Amount: 21, P2PK: true},
	})

	cashuTag := findTag(r.Tags, "cashu")
	if cashuTag == nil {
		t.Fatal("missing cashu tag")
	}
	if cashuTag[1] != token {
		t.Errorf("cashu token mismatch")
	}
}

func TestCreateRumor_MultipleAttachmentsAndInline(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Q3 Report with Charts",
		Body:         "## Q3 Revenue Report\n\n![Revenue Chart](cid:chart001)",
		ContentType:  "text/markdown",
		CreatedAt:    1711843200,
		Attachments: []Attachment{
			{
				Hash:          "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
				Filename:      "Q3-Report.pdf",
				MimeType:      "application/pdf",
				Size:          2048576,
				EncryptionKey: "1111111111111111111111111111111111111111111111111111111111111111",
			},
			{
				Hash:          "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2",
				Filename:      "Q3-Spreadsheet.xlsx",
				MimeType:      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				Size:          524288,
				EncryptionKey: "2222222222222222222222222222222222222222222222222222222222222222",
			},
		},
		InlineImages: []InlineImage{{
			Hash:          "c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3",
			ContentID:     "chart001",
			EncryptionKey: "3333333333333333333333333333333333333333333333333333333333333333",
		}},
		BlossomURLs: []string{"https://blossom.example.com", "https://blossom.backup.com"},
	})

	// Verify 2 attachment tags
	attTags := findAllTags(r.Tags, "attachment")
	if len(attTags) != 2 {
		t.Fatalf("expected 2 attachment tags, got %d", len(attTags))
	}

	// Verify 3 attachment-key tags (2 files + 1 inline)
	keyTags := findAllTags(r.Tags, "attachment-key")
	if len(keyTags) != 3 {
		t.Fatalf("expected 3 attachment-key tags, got %d", len(keyTags))
	}

	// Verify 1 inline tag
	inlineTags := findAllTags(r.Tags, "inline")
	if len(inlineTags) != 1 {
		t.Fatalf("expected 1 inline tag, got %d", len(inlineTags))
	}
	if inlineTags[0][2] != "chart001" {
		t.Errorf("inline content-id mismatch: got %s", inlineTags[0][2])
	}

	// Verify blossom tag with 2 URLs
	blossomTag := findTag(r.Tags, "blossom")
	if blossomTag == nil {
		t.Fatal("missing blossom tag")
	}
	if len(blossomTag) != 3 {
		t.Errorf("expected 3 elements in blossom tag (blossom + 2 urls), got %d", len(blossomTag))
	}
}

func TestParseRumor_RoundTrip(t *testing.T) {
	original := CreateParams{
		SenderPubKey: alicePub,
		Recipients: []Recipient{
			{PubKey: bobPub, Relay: "wss://relay.bob.com", Role: "to"},
			{PubKey: charliePub, Relay: "wss://relay.charlie.com", Role: "cc"},
		},
		Subject:     "Round-trip test",
		Body:        "This should survive create then parse.",
		ContentType: "text/markdown",
		CreatedAt:   1711843200,
		ReplyTo:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ReplyRelay:  "wss://relay.alice.com",
		ThreadID:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ThreadRelay: "wss://relay.alice.com",
		Attachments: []Attachment{{
			Hash:          "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			Filename:      "test.pdf",
			MimeType:      "application/pdf",
			Size:          1024,
			EncryptionKey: "eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111",
		}},
		BlossomURLs: []string{"https://blossom.example.com"},
	}

	rumor := CreateRumor(original)
	parsed := ParseRumor(rumor)

	if parsed.From != alicePub {
		t.Errorf("from mismatch: got %s", parsed.From)
	}
	if parsed.Subject != "Round-trip test" {
		t.Errorf("subject mismatch: got %s", parsed.Subject)
	}
	if parsed.Body != "This should survive create then parse." {
		t.Errorf("body mismatch: got %s", parsed.Body)
	}
	if parsed.ContentType != "text/markdown" {
		t.Errorf("content type mismatch: got %s", parsed.ContentType)
	}
	if len(parsed.Recipients) != 2 {
		t.Fatalf("expected 2 recipients, got %d", len(parsed.Recipients))
	}
	if parsed.Recipients[0].Role != "to" || parsed.Recipients[1].Role != "cc" {
		t.Errorf("recipient roles mismatch: got %s, %s", parsed.Recipients[0].Role, parsed.Recipients[1].Role)
	}
	if parsed.ReplyTo != original.ReplyTo {
		t.Errorf("replyTo mismatch: got %s", parsed.ReplyTo)
	}
	if parsed.ThreadID != original.ThreadID {
		t.Errorf("threadID mismatch: got %s", parsed.ThreadID)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(parsed.Attachments))
	}
	if parsed.Attachments[0].EncryptionKey != original.Attachments[0].EncryptionKey {
		t.Errorf("attachment encryption key mismatch")
	}
}

func TestParseRumor_DefaultContentType(t *testing.T) {
	r := Rumor{
		Kind:      1400,
		PubKey:    alicePub,
		CreatedAt: 1711843200,
		Tags: [][]string{
			{"p", bobPub, "", "to"},
			{"subject", "Plain text"},
		},
		Content: "No content-type tag means text/plain.",
	}

	parsed := ParseRumor(r)
	if parsed.ContentType != "text/plain" {
		t.Errorf("expected default content type text/plain, got %s", parsed.ContentType)
	}
}

func TestCreateRumor_NoContentTypeTagForPlainText(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Plain text",
		Body:         "No special content type.",
		ContentType:  "text/plain",
		CreatedAt:    1711843200,
	})

	if findTag(r.Tags, "content-type") != nil {
		t.Errorf("should not include content-type tag for text/plain")
	}
}

func TestCreateRumor_DefaultCreatedAt(t *testing.T) {
	r := CreateRumor(CreateParams{
		SenderPubKey: alicePub,
		Recipients:   []Recipient{{PubKey: bobPub, Role: "to"}},
		Subject:      "Time test",
		Body:         "Should use current time.",
	})

	if r.CreatedAt == 0 {
		t.Errorf("created_at should be set to current time, got 0")
	}
}

// --- helpers ---

func findTag(tags [][]string, name string) []string {
	for _, t := range tags {
		if len(t) > 0 && t[0] == name {
			return t
		}
	}
	return nil
}

func findAllTags(tags [][]string, name string) [][]string {
	var result [][]string
	for _, t := range tags {
		if len(t) > 0 && t[0] == name {
			result = append(result, t)
		}
	}
	return result
}
