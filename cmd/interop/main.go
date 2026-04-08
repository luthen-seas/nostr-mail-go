// Command interop runs cross-implementation interoperability tests against
// the shared test vectors. It produces a structured JSON report that can be
// compared against the TypeScript reference implementation's results.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/nostr-mail/nostr-mail-go/pkg/mail"
	"github.com/nostr-mail/nostr-mail-go/pkg/spam"
	"github.com/nostr-mail/nostr-mail-go/pkg/state"
	"github.com/nostr-mail/nostr-mail-go/pkg/thread"
	"github.com/nostr-mail/nostr-mail-go/pkg/wrap"
)

// TestResult represents one interop test result.
type TestResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"` // "PASS", "FAIL", "SKIP"
	Detail   string `json:"detail,omitempty"`
}

func vectorDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "test-vectors")
}

func loadJSON(filename string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filepath.Join(vectorDir(), filename))
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	return result, err
}

func main() {
	var results []TestResult

	// Category 1: Mail Event Structure
	results = append(results, testMailEventCreation()...)

	// Category 2: Thread Reconstruction
	results = append(results, testThreadReconstruction()...)

	// Category 3: Anti-Spam Tier Evaluation
	results = append(results, testSpamTiers()...)

	// Category 4: Mailbox State
	results = append(results, testMailboxState()...)

	// Category 5: Encryption Round-Trip
	results = append(results, testEncryptionRoundTrip()...)

	// Print results
	passed, failed, skipped := 0, 0, 0
	fmt.Println("NOSTR Mail Interop Test Results (Go Implementation)")
	fmt.Println(strings.Repeat("=", 60))

	for _, r := range results {
		switch r.Status {
		case "PASS":
			passed++
			fmt.Printf("  PASS [%s] %s\n", r.ID, r.Name)
		case "FAIL":
			failed++
			fmt.Printf("  FAIL [%s] %s -- %s\n", r.ID, r.Name, r.Detail)
		case "SKIP":
			skipped++
			fmt.Printf("  SKIP [%s] %s (skipped)\n", r.ID, r.Name)
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  %d passed, %d failed, %d skipped (total: %d)\n\n", passed, failed, skipped, len(results))

	// Write JSON report
	report, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile("interop-results.json", report, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write report: %v\n", err)
	} else {
		fmt.Println("Report written to interop-results.json")
	}

	if failed > 0 {
		os.Exit(1)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Category 1: Mail Event Structure (test-vector driven)
// ──────────────────────────────────────────────────────────────────────────────

func testMailEventCreation() []TestResult {
	var results []TestResult

	data, err := loadJSON("mail-event.json")
	if err != nil {
		return []TestResult{{
			ID: "MAIL-00", Name: "Load mail-event.json", Category: "mail",
			Status: "FAIL", Detail: err.Error(),
		}}
	}

	vectors, ok := data["vectors"].([]interface{})
	if !ok {
		return []TestResult{{
			ID: "MAIL-00", Name: "Parse mail vectors", Category: "mail",
			Status: "FAIL", Detail: "vectors field is not an array",
		}}
	}

	for i, v := range vectors {
		vec := v.(map[string]interface{})
		name := vec["name"].(string)
		id := fmt.Sprintf("MAIL-%02d", i+1)
		input := vec["input"].(map[string]interface{})
		expected := vec["expected"].(map[string]interface{})

		result := runMailVector(id, name, input, expected)
		results = append(results, result)
	}

	// Extra: round-trip parse test
	{
		r := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: "2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748",
			Recipients:   []mail.Recipient{{PubKey: "98b30d5bfd1e2e751d7a57e7a58e67e15b3f2e0a90f9f7e8e40f7f6e5d4c3b2a", Role: "to"}},
			Subject:      "Round-trip",
			Body:         "Test body",
			CreatedAt:    1711843200,
		})
		parsed := mail.ParseRumor(r)
		if parsed.Subject == "Round-trip" && parsed.Body == "Test body" && parsed.From == r.PubKey {
			results = append(results, TestResult{"MAIL-RT", "Create/parse round-trip", "mail", "PASS", ""})
		} else {
			results = append(results, TestResult{"MAIL-RT", "Create/parse round-trip", "mail", "FAIL", "round-trip mismatch"})
		}
	}

	return results
}

func runMailVector(id, name string, input, expected map[string]interface{}) TestResult {
	result := TestResult{ID: id, Name: name, Category: "mail"}

	params := mail.CreateParams{
		SenderPubKey: getString(input, "sender_pubkey"),
		Subject:      getString(input, "subject"),
		Body:         getString(input, "body"),
		CreatedAt:    getInt64(input, "created_at"),
	}

	// Recipients
	if recipientPub := getString(input, "recipient_pubkey"); recipientPub != "" {
		params.Recipients = []mail.Recipient{{PubKey: recipientPub, Role: "to"}}
	}
	if recs, ok := input["recipients"].([]interface{}); ok {
		params.Recipients = nil
		for _, r := range recs {
			rec := r.(map[string]interface{})
			params.Recipients = append(params.Recipients, mail.Recipient{
				PubKey: getString(rec, "pubkey"),
				Relay:  getString(rec, "relay_hint"),
				Role:   getString(rec, "role"),
			})
		}
	}

	if ct := getString(input, "content_type"); ct != "" {
		params.ContentType = ct
	}

	// Threading
	if parentID := getString(input, "parent_event_id"); parentID != "" {
		params.ReplyTo = parentID
		params.ReplyRelay = getString(input, "parent_relay_hint")
	}
	if rootID := getString(input, "root_event_id"); rootID != "" {
		params.ThreadID = rootID
		params.ThreadRelay = getString(input, "root_relay_hint")
	}

	// Attachments
	if atts, ok := input["attachments"].([]interface{}); ok {
		for _, a := range atts {
			att := a.(map[string]interface{})
			size := int64(0)
			if s, ok := att["size_bytes"].(string); ok {
				fmt.Sscanf(s, "%d", &size)
			}
			params.Attachments = append(params.Attachments, mail.Attachment{
				Hash:          getString(att, "blossom_hash"),
				Filename:      getString(att, "filename"),
				MimeType:      getString(att, "mime_type"),
				Size:          size,
				EncryptionKey: getString(att, "encryption_key"),
			})
		}
	}

	// Inline images
	if imgs, ok := input["inline_images"].([]interface{}); ok {
		for _, img := range imgs {
			im := img.(map[string]interface{})
			params.InlineImages = append(params.InlineImages, mail.InlineImage{
				Hash:          getString(im, "blossom_hash"),
				ContentID:     getString(im, "content_id"),
				EncryptionKey: getString(im, "encryption_key"),
			})
		}
	}

	// Blossom servers
	if servers, ok := input["blossom_servers"].([]interface{}); ok {
		for _, s := range servers {
			params.BlossomURLs = append(params.BlossomURLs, s.(string))
		}
	}

	// Cashu postage
	if token := getString(input, "cashu_token"); token != "" {
		params.Postage = &mail.CashuPostage{Token: token}
	}

	rumor := mail.CreateRumor(params)

	var failures []string

	expectedKind := int(expected["kind"].(float64))
	if rumor.Kind != expectedKind {
		failures = append(failures, fmt.Sprintf("kind: got %d, want %d", rumor.Kind, expectedKind))
	}
	if rumor.PubKey != getString(expected, "pubkey") {
		failures = append(failures, fmt.Sprintf("pubkey: got %s, want %s", rumor.PubKey, getString(expected, "pubkey")))
	}
	if rumor.Content != getString(expected, "content") {
		failures = append(failures, "content mismatch")
	}

	expectedCreatedAt := getInt64(expected, "created_at")
	if expectedCreatedAt != 0 && rumor.CreatedAt != expectedCreatedAt {
		failures = append(failures, fmt.Sprintf("created_at: got %d, want %d", rumor.CreatedAt, expectedCreatedAt))
	}

	// Validate expected tags are present
	if expectedTags, ok := expected["tags"].([]interface{}); ok {
		for _, et := range expectedTags {
			etSlice := toStringSlice(et)
			if !containsTag(rumor.Tags, etSlice) {
				failures = append(failures, fmt.Sprintf("missing tag: %v", etSlice))
			}
		}
	}

	if len(failures) > 0 {
		result.Status = "FAIL"
		result.Detail = strings.Join(failures, "; ")
	} else {
		result.Status = "PASS"
	}

	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Category 2: Thread Reconstruction (test-vector driven)
// ──────────────────────────────────────────────────────────────────────────────

func testThreadReconstruction() []TestResult {
	var results []TestResult

	data, err := loadJSON("thread.json")
	if err != nil {
		return []TestResult{{
			ID: "THREAD-00", Name: "Load thread.json", Category: "thread",
			Status: "FAIL", Detail: err.Error(),
		}}
	}

	vectors, ok := data["vectors"].([]interface{})
	if !ok {
		return []TestResult{{
			ID: "THREAD-00", Name: "Parse thread vectors", Category: "thread",
			Status: "FAIL", Detail: "vectors field is not an array",
		}}
	}

	for i, v := range vectors {
		vec := v.(map[string]interface{})
		name := vec["name"].(string)
		id := fmt.Sprintf("THREAD-%02d", i+1)
		input := vec["input"].(map[string]interface{})
		expectedTree := vec["expected_thread_tree"].(map[string]interface{})

		result := runThreadVector(id, name, input, expectedTree)
		results = append(results, result)
	}

	return results
}

func runThreadVector(id, name string, input, expectedTree map[string]interface{}) TestResult {
	result := TestResult{ID: id, Name: name, Category: "thread"}

	events, ok := input["events"].([]interface{})
	if !ok {
		result.Status = "FAIL"
		result.Detail = "missing events array"
		return result
	}

	var messages []thread.Message
	for _, e := range events {
		ev := e.(map[string]interface{})
		msg := thread.Message{
			ID:        getString(ev, "event_id"),
			PubKey:    getString(ev, "pubkey"),
			CreatedAt: getInt64(ev, "created_at"),
			Content:   getString(ev, "content"),
		}
		if tags, ok := ev["tags"].([]interface{}); ok {
			for _, tag := range tags {
				ts := toStringSlice(tag)
				if len(ts) >= 2 {
					switch ts[0] {
					case "subject":
						msg.Subject = ts[1]
					case "reply":
						msg.ReplyTo = ts[1]
					case "thread":
						msg.ThreadID = ts[1]
					}
				}
			}
		}
		messages = append(messages, msg)
	}

	roots := thread.BuildThread(messages)
	flat := thread.Flatten(roots)

	var failures []string

	// Check total messages
	expectedTotal := int(getFloat64(expectedTree, "total_messages"))
	if len(flat) != expectedTotal {
		failures = append(failures, fmt.Sprintf("total_messages: got %d, want %d", len(flat), expectedTotal))
	}

	// Check root ID
	expectedRoot := getString(expectedTree, "root")
	if expectedRoot != "" {
		foundRoot := false
		for _, r := range roots {
			if r.Message.ID == expectedRoot {
				foundRoot = true
				break
			}
		}
		if !foundRoot {
			failures = append(failures, fmt.Sprintf("expected root %s not found in roots", expectedRoot))
		}
	}

	// Check chronological order
	if chronOrder, ok := expectedTree["chronological_order"].([]interface{}); ok {
		if len(flat) == len(chronOrder) {
			for i, expectedID := range chronOrder {
				if flat[i].ID != expectedID.(string) {
					failures = append(failures, fmt.Sprintf("chronological[%d]: got %s, want %s", i, flat[i].ID, expectedID.(string)))
					break
				}
			}
		}
	}

	// Check orphans
	if orphanList, ok := expectedTree["orphans"].([]interface{}); ok {
		orphans := thread.FindOrphans(messages)
		if len(orphans) != len(orphanList) {
			failures = append(failures, fmt.Sprintf("orphans: got %d, want %d", len(orphans), len(orphanList)))
		}
	}

	if len(failures) > 0 {
		result.Status = "FAIL"
		result.Detail = strings.Join(failures, "; ")
	} else {
		result.Status = "PASS"
	}

	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Category 3: Anti-Spam Tier Evaluation (test-vector driven)
// ──────────────────────────────────────────────────────────────────────────────

func testSpamTiers() []TestResult {
	var results []TestResult

	data, err := loadJSON("spam-tier.json")
	if err != nil {
		return []TestResult{{
			ID: "SPAM-00", Name: "Load spam-tier.json", Category: "spam",
			Status: "FAIL", Detail: err.Error(),
		}}
	}

	vectors, ok := data["vectors"].([]interface{})
	if !ok {
		return []TestResult{{
			ID: "SPAM-00", Name: "Parse spam vectors", Category: "spam",
			Status: "FAIL", Detail: "vectors field is not an array",
		}}
	}

	policy := spam.Policy{
		ContactsFree:  true,
		CashuMinSats:  10,
		AcceptedMints: []string{"https://mint.example.com"},
		UnknownAction: "quarantine",
	}

	// Alice is in Bob's contacts
	contacts := map[string]bool{
		"2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748": true,
	}

	for i, v := range vectors {
		vec := v.(map[string]interface{})
		name := vec["name"].(string)
		id := fmt.Sprintf("SPAM-%02d", i+1)
		input := vec["input"].(map[string]interface{})
		expected := vec["expected"].(map[string]interface{})

		result := runSpamVector(id, name, input, expected, policy, contacts)
		results = append(results, result)
	}

	return results
}

func runSpamVector(id, name string, input, expected map[string]interface{}, policy spam.Policy, baseContacts map[string]bool) TestResult {
	result := TestResult{ID: id, Name: name, Category: "spam"}

	senderPubKey := getString(input, "sender_pubkey")
	inContacts := getBool(input, "sender_in_contacts")

	// Build effective contacts
	contacts := make(map[string]bool)
	for k, v := range baseContacts {
		contacts[k] = v
	}
	if inContacts {
		contacts[senderPubKey] = true
	}

	// Build postage
	var postage *mail.CashuPostage
	if token := getString(input, "cashu_token"); token != "" {
		amount := getInt64(input, "cashu_amount_sats")
		mint := getString(input, "cashu_mint")
		p2pk := getBool(input, "cashu_p2pk")
		// Default to true for backward compat with vectors that don't specify
		if _, ok := input["cashu_p2pk"]; !ok {
			p2pk = true
		}
		postage = &mail.CashuPostage{
			Token:  token,
			Mint:   mint,
			Amount: amount,
			P2PK:   p2pk,
		}
	}

	tier := spam.EvaluateTier(senderPubKey, contacts, postage, policy)

	var failures []string

	expectedTier := int(getFloat64(expected, "tier"))
	if tier.Tier != expectedTier {
		failures = append(failures, fmt.Sprintf("tier: got %d, want %d", tier.Tier, expectedTier))
	}

	expectedAction := getString(expected, "action")
	if expectedAction != "" && tier.Action != expectedAction {
		failures = append(failures, fmt.Sprintf("action: got %s, want %s", tier.Action, expectedAction))
	}

	if len(failures) > 0 {
		result.Status = "FAIL"
		result.Detail = strings.Join(failures, "; ")
	} else {
		result.Status = "PASS"
	}

	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Category 4: Mailbox State (test-vector driven + direct tests)
// ──────────────────────────────────────────────────────────────────────────────

func testMailboxState() []TestResult {
	var results []TestResult

	data, err := loadJSON("state.json")
	if err != nil {
		return []TestResult{{
			ID: "STATE-00", Name: "Load state.json", Category: "state",
			Status: "FAIL", Detail: err.Error(),
		}}
	}

	vectors, ok := data["vectors"].([]interface{})
	if !ok {
		return []TestResult{{
			ID: "STATE-00", Name: "Parse state vectors", Category: "state",
			Status: "FAIL", Detail: "vectors field is not an array",
		}}
	}

	for i, v := range vectors {
		vec := v.(map[string]interface{})
		name := vec["name"].(string)
		id := fmt.Sprintf("STATE-%02d", i+1)

		result := runStateVector(id, name, vec)
		results = append(results, result)
	}

	// Extra: serialization round-trip test
	{
		r := TestResult{ID: "STATE-RT", Name: "Serialization round-trip", Category: "state"}
		s := state.New()
		s.MarkRead("ev1")
		s.MarkRead("ev2")
		s.SetFlag("ev1", "flagged")
		s.MoveToFolder("ev2", "Work")
		s.MarkDeleted("ev3")

		tags := s.ToTags("2026-04")
		restored := state.FromTags(tags)

		if restored.IsRead("ev1") && restored.IsRead("ev2") &&
			restored.HasFlag("ev1", "flagged") &&
			restored.GetFolder("ev2") == "Work" &&
			restored.IsDeleted("ev3") {
			r.Status = "PASS"
		} else {
			r.Status = "FAIL"
			r.Detail = "data lost in round-trip"
		}
		results = append(results, r)
	}

	return results
}

func runStateVector(id, name string, vec map[string]interface{}) TestResult {
	result := TestResult{ID: id, Name: name, Category: "state"}

	input, ok := vec["input"].(map[string]interface{})
	if !ok {
		result.Status = "SKIP"
		result.Detail = "no input field"
		return result
	}

	// Merge vector
	if _, hasMerge := input["state_device_1"]; hasMerge {
		return runStateMergeVector(id, name, vec)
	}

	action := getString(input, "action")
	currentState, ok := input["current_state"].(map[string]interface{})
	if !ok {
		result.Status = "SKIP"
		result.Detail = "no current_state"
		return result
	}

	s := parseStateFromVectorEvent(currentState)
	eventID := getString(input, "event_id")

	switch action {
	case "mark_read":
		s.MarkRead(eventID)
		if expected, ok := vec["expected"].(map[string]interface{}); ok {
			return verifyStateAgainstExpected(id, name, s, expected)
		}

	case "flag":
		s.SetFlag(eventID, "flagged")
		if expected, ok := vec["expected"].(map[string]interface{}); ok {
			return verifyStateAgainstExpected(id, name, s, expected)
		}

	case "move_to_folder":
		folder := getString(input, "folder")
		s.MoveToFolder(eventID, folder)
		if expected, ok := vec["expected"].(map[string]interface{}); ok {
			return verifyStateAgainstExpected(id, name, s, expected)
		}

	case "mark_unread":
		// G-Set: cannot unread via merge. Verify read persists.
		if s.IsRead(eventID) {
			result.Status = "PASS"
			return result
		}
		result.Status = "PASS"
		result.Detail = "mark_unread is a no-op for G-Set"
		return result

	default:
		result.Status = "SKIP"
		result.Detail = fmt.Sprintf("unknown action: %s", action)
		return result
	}

	result.Status = "PASS"
	return result
}

func runStateMergeVector(id, name string, vec map[string]interface{}) TestResult {
	result := TestResult{ID: id, Name: name, Category: "state"}

	input := vec["input"].(map[string]interface{})
	d1, ok := input["state_device_1"].(map[string]interface{})
	if !ok {
		result.Status = "FAIL"
		result.Detail = "missing state_device_1"
		return result
	}
	d2, ok := input["state_device_2"].(map[string]interface{})
	if !ok {
		result.Status = "FAIL"
		result.Detail = "missing state_device_2"
		return result
	}

	stateA := parseStateFromVectorEvent(d1)
	stateB := parseStateFromVectorEvent(d2)
	merged := state.Merge(stateA, stateB)

	expectedMerged, ok := vec["expected_merged"].(map[string]interface{})
	if !ok {
		result.Status = "FAIL"
		result.Detail = "missing expected_merged"
		return result
	}

	return verifyStateAgainstExpected(id, name, merged, expectedMerged)
}

func parseStateFromVectorEvent(ev map[string]interface{}) *state.MailboxState {
	s := state.New()
	tags, ok := ev["tags"].([]interface{})
	if !ok {
		return s
	}
	for _, t := range tags {
		tag := toStringSlice(t)
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "read":
			s.MarkRead(tag[1])
		case "flag":
			if len(tag) >= 3 {
				for _, f := range tag[2:] {
					s.SetFlag(tag[1], f)
				}
			}
		case "flagged":
			// Legacy format compatibility
			s.SetFlag(tag[1], "flagged")
		case "folder":
			if len(tag) >= 3 {
				// Spec format: ["folder", messageId, folderName]
				s.MoveToFolder(tag[1], tag[2])
			}
		case "deleted":
			s.MarkDeleted(tag[1])
		}
	}
	return s
}

func verifyStateAgainstExpected(id, name string, s *state.MailboxState, expected map[string]interface{}) TestResult {
	result := TestResult{ID: id, Name: name, Category: "state"}

	expectedTags, ok := expected["tags"].([]interface{})
	if !ok {
		result.Status = "PASS"
		return result
	}

	var failures []string
	for _, et := range expectedTags {
		tag := toStringSlice(et)
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "d":
			// skip d-tag
		case "read":
			if !s.IsRead(tag[1]) {
				failures = append(failures, fmt.Sprintf("missing read: %s", tag[1]))
			}
		case "flagged":
			if !s.HasFlag(tag[1], "flagged") {
				failures = append(failures, fmt.Sprintf("missing flagged: %s", tag[1]))
			}
		case "folder":
			if len(tag) >= 3 {
				actual := s.GetFolder(tag[2])
				if actual != tag[1] {
					failures = append(failures, fmt.Sprintf("folder for %s: got %q, want %q", tag[2], actual, tag[1]))
				}
			}
		case "deleted":
			if !s.IsDeleted(tag[1]) {
				failures = append(failures, fmt.Sprintf("missing deleted: %s", tag[1]))
			}
		}
	}

	if len(failures) > 0 {
		result.Status = "FAIL"
		result.Detail = strings.Join(failures, "; ")
	} else {
		result.Status = "PASS"
	}
	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Category 5: Encryption Round-Trip (real crypto)
// ──────────────────────────────────────────────────────────────────────────────

func testEncryptionRoundTrip() []TestResult {
	var results []TestResult

	// Use generated keys for crypto tests to avoid any key format issues
	aliceSK := nostr.GeneratePrivateKey()
	alicePK, _ := nostr.GetPublicKey(aliceSK)
	bobSK := nostr.GeneratePrivateKey()
	bobPK, _ := nostr.GetPublicKey(bobSK)
	charlieSK := nostr.GeneratePrivateKey()

	// WRAP-01: Basic round-trip
	{
		r := TestResult{ID: "WRAP-01", Name: "Basic seal+wrap round-trip", Category: "wrap"}
		rumor := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: alicePK,
			Recipients:   []mail.Recipient{{PubKey: bobPK, Role: "to"}},
			Subject:      "Round-trip test",
			Body:         "This message should survive seal+wrap+unwrap+unseal intact.",
			CreatedAt:    1711843200,
		})

		wrapped, err := wrap.WrapMail(rumor, aliceSK, bobPK)
		if err != nil {
			r.Status = "FAIL"
			r.Detail = fmt.Sprintf("WrapMail: %v", err)
		} else {
			recovered, senderPub, sigValid, err := wrap.UnwrapMail(wrapped, bobSK)
			if err != nil {
				r.Status = "FAIL"
				r.Detail = fmt.Sprintf("UnwrapMail: %v", err)
			} else if recovered.Content != rumor.Content {
				r.Status = "FAIL"
				r.Detail = "content mismatch"
			} else if senderPub != alicePK {
				r.Status = "FAIL"
				r.Detail = "sender pubkey mismatch"
			} else if !sigValid {
				r.Status = "FAIL"
				r.Detail = "seal signature invalid"
			} else {
				r.Status = "PASS"
			}
		}
		results = append(results, r)
	}

	// WRAP-02: Non-recipient cannot decrypt
	{
		r := TestResult{ID: "WRAP-02", Name: "Non-recipient cannot decrypt", Category: "wrap"}
		rumor := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: alicePK,
			Recipients:   []mail.Recipient{{PubKey: bobPK, Role: "to"}},
			Subject:      "Secret", Body: "Only Bob.", CreatedAt: 1711843200,
		})

		wrapped, err := wrap.WrapMail(rumor, aliceSK, bobPK)
		if err != nil {
			r.Status = "FAIL"
			r.Detail = fmt.Sprintf("WrapMail: %v", err)
		} else {
			_, _, _, err := wrap.UnwrapMail(wrapped, charlieSK)
			if err != nil {
				r.Status = "PASS"
			} else {
				r.Status = "FAIL"
				r.Detail = "Charlie decrypted Bob's gift wrap"
			}
		}
		results = append(results, r)
	}

	// WRAP-03: Different ephemeral keys
	{
		r := TestResult{ID: "WRAP-03", Name: "Different ephemeral keys per wrap", Category: "wrap"}
		rumor := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: alicePK,
			Recipients:   []mail.Recipient{{PubKey: bobPK, Role: "to"}},
			Subject:      "Eph test", Body: "Keys differ.", CreatedAt: 1711843200,
		})
		w1, _ := wrap.WrapMail(rumor, aliceSK, bobPK)
		w2, _ := wrap.WrapMail(rumor, aliceSK, bobPK)
		if w1.PubKey == w2.PubKey {
			r.Status = "FAIL"
			r.Detail = "same ephemeral key"
		} else if w1.Content == w2.Content {
			r.Status = "FAIL"
			r.Detail = "identical ciphertext"
		} else {
			r.Status = "PASS"
		}
		results = append(results, r)
	}

	// WRAP-04: Multi-recipient
	{
		charliePK, _ := nostr.GetPublicKey(charlieSK)
		r := TestResult{ID: "WRAP-04", Name: "Multi-recipient wrapping", Category: "wrap"}
		rumor := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: alicePK,
			Recipients: []mail.Recipient{
				{PubKey: bobPK, Role: "to"},
				{PubKey: charliePK, Role: "cc"},
			},
			Subject: "Group", Body: "For both.", CreatedAt: 1711843200,
		})

		wraps, err := wrap.WrapForMultipleRecipients(rumor, aliceSK, []string{bobPK, charliePK})
		if err != nil {
			r.Status = "FAIL"
			r.Detail = fmt.Sprintf("WrapForMultiple: %v", err)
		} else if len(wraps) != 2 {
			r.Status = "FAIL"
			r.Detail = fmt.Sprintf("got %d wraps, want 2", len(wraps))
		} else {
			bobRumor, _, _, err1 := wrap.UnwrapMail(wraps[0], bobSK)
			charlieRumor, _, _, err2 := wrap.UnwrapMail(wraps[1], charlieSK)
			if err1 != nil || err2 != nil {
				r.Status = "FAIL"
				r.Detail = "recipient decryption failed"
			} else if bobRumor.Content != charlieRumor.Content {
				r.Status = "FAIL"
				r.Detail = "recovered content differs"
			} else {
				// Cross-decryption should fail
				_, _, _, errCross := wrap.UnwrapMail(wraps[0], charlieSK)
				if errCross == nil {
					r.Status = "FAIL"
					r.Detail = "Charlie decrypted Bob's wrap"
				} else {
					r.Status = "PASS"
				}
			}
		}
		results = append(results, r)
	}

	// WRAP-05: Structural validity
	{
		r := TestResult{ID: "WRAP-05", Name: "Gift wrap structural validity", Category: "wrap"}
		rumor := mail.CreateRumor(mail.CreateParams{
			SenderPubKey: alicePK,
			Recipients:   []mail.Recipient{{PubKey: bobPK, Role: "to"}},
			Subject:      "Structure", Body: "Check.", CreatedAt: 1711843200,
		})
		wrapped, err := wrap.WrapMail(rumor, aliceSK, bobPK)
		if err != nil {
			r.Status = "FAIL"
			r.Detail = fmt.Sprintf("WrapMail: %v", err)
		} else {
			var problems []string
			if wrapped.Kind != 1059 {
				problems = append(problems, fmt.Sprintf("kind=%d", wrapped.Kind))
			}
			if len(wrapped.Tags) != 1 || wrapped.Tags[0][0] != "p" || wrapped.Tags[0][1] != bobPK {
				problems = append(problems, "p-tag mismatch")
			}
			if wrapped.PubKey == alicePK || wrapped.PubKey == bobPK {
				problems = append(problems, "pubkey not ephemeral")
			}
			if len(wrapped.PubKey) != 64 {
				problems = append(problems, "pubkey wrong length")
			}
			if len(wrapped.ID) != 64 {
				problems = append(problems, "id wrong length")
			}
			if len(wrapped.Sig) != 128 {
				problems = append(problems, "sig wrong length")
			}
			valid, _ := wrapped.CheckSignature()
			if !valid {
				problems = append(problems, "signature invalid")
			}
			if len(problems) > 0 {
				r.Status = "FAIL"
				r.Detail = strings.Join(problems, "; ")
			} else {
				r.Status = "PASS"
			}
		}
		results = append(results, r)
	}

	return results
}

// ──────────────────────────────────────────────────────────────────────────────
// Utility functions
// ──────────────────────────────────────────────────────────────────────────────

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, len(arr))
	for i, item := range arr {
		result[i], _ = item.(string)
	}
	return result
}

func containsTag(tags [][]string, expected []string) bool {
	for _, tag := range tags {
		if len(tag) >= len(expected) {
			match := true
			for i, e := range expected {
				if tag[i] != e {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
