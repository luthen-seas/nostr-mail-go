package state

import (
	"encoding/json"
	"testing"
)

const (
	idA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestMarkRead_Append(t *testing.T) {
	s := New()
	s.MarkRead(idA)

	if !s.IsRead(idA) {
		t.Errorf("expected %s to be read", idA)
	}
	if s.IsRead(idB) {
		t.Errorf("expected %s to NOT be read", idB)
	}

	// Add a second read
	s.MarkRead(idB)
	if !s.IsRead(idB) {
		t.Errorf("expected %s to be read after adding", idB)
	}

	// Both should be in the read set
	if len(s.Reads) != 2 {
		t.Errorf("expected 2 reads, got %d", len(s.Reads))
	}
}

func TestMarkRead_Idempotent(t *testing.T) {
	s := New()
	s.MarkRead(idA)
	s.MarkRead(idA) // second mark — idempotent

	if len(s.Reads) != 1 {
		t.Errorf("expected 1 read entry (idempotent), got %d", len(s.Reads))
	}
}

func TestMarkRead_CannotUnread(t *testing.T) {
	// G-Set property: once read, cannot be un-read through normal operations.
	s := New()
	s.MarkRead(idA)

	// There is no "unread" method because G-Set is grow-only.
	// Verify the read persists.
	if !s.IsRead(idA) {
		t.Errorf("G-Set read should persist -- cannot be reverted")
	}
}

func TestToggleFlag(t *testing.T) {
	s := New()

	// Initially not flagged
	if s.HasFlag(idA, "starred") {
		t.Errorf("should not be starred initially")
	}

	// Toggle on
	s.ToggleFlag(idA, "starred")
	if !s.HasFlag(idA, "starred") {
		t.Errorf("should be starred after toggle on")
	}

	// Toggle off
	s.ToggleFlag(idA, "starred")
	if s.HasFlag(idA, "starred") {
		t.Errorf("should not be starred after toggle off")
	}
}

func TestSetFlag(t *testing.T) {
	s := New()
	s.SetFlag(idA, "starred")

	if !s.HasFlag(idA, "starred") {
		t.Errorf("expected starred after SetFlag")
	}

	// SetFlag is idempotent
	s.SetFlag(idA, "starred")
	count := 0
	for _, f := range s.Flags[idA] {
		if f == "starred" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("SetFlag should be idempotent, got %d entries", count)
	}
}

func TestMoveToFolder(t *testing.T) {
	s := New()

	// Initially no folder
	if s.GetFolder(idA) != "" {
		t.Errorf("expected no folder initially, got %s", s.GetFolder(idA))
	}

	// Move to Work
	s.MoveToFolder(idA, "Work")
	if s.GetFolder(idA) != "Work" {
		t.Errorf("expected folder 'Work', got %s", s.GetFolder(idA))
	}

	// Move to Personal (replaces Work)
	s.MoveToFolder(idA, "Personal")
	if s.GetFolder(idA) != "Personal" {
		t.Errorf("expected folder 'Personal' after move, got %s", s.GetFolder(idA))
	}
}

func TestMerge_GSetUnionReads(t *testing.T) {
	a := New()
	a.MarkRead(idA)
	a.MarkRead(idB)

	b := New()
	b.MarkRead(idA)
	b.MarkRead(idC)

	merged := Merge(a, b)

	// Union should contain A, B, C
	if !merged.IsRead(idA) {
		t.Errorf("merged should contain read %s", idA)
	}
	if !merged.IsRead(idB) {
		t.Errorf("merged should contain read %s (from state a)", idB)
	}
	if !merged.IsRead(idC) {
		t.Errorf("merged should contain read %s (from state b)", idC)
	}
	if len(merged.Reads) != 3 {
		t.Errorf("expected 3 reads in merge, got %d", len(merged.Reads))
	}
}

func TestMerge_LWWFolders(t *testing.T) {
	// Device 1 (older): moves A to "Work"
	a := New()
	a.MarkRead(idA)
	a.MarkRead(idB)
	a.MarkRead(idC)
	a.MoveToFolder(idA, "Work")

	// Device 2 (newer): moves A to "Personal", flags A
	b := New()
	b.MarkRead(idA)
	b.MarkRead(idB)
	b.SetFlag(idA, "starred")
	b.MoveToFolder(idA, "Personal")

	// b is newer, so Merge(a, b) should use b's folder for conflicts
	merged := Merge(a, b)

	// Folder: LWW -- b wins, so A should be in "Personal"
	if merged.GetFolder(idA) != "Personal" {
		t.Errorf("LWW folder should be 'Personal' (from newer state b), got %s", merged.GetFolder(idA))
	}

	// Reads: G-Set union should have all 3
	if len(merged.Reads) != 3 {
		t.Errorf("expected 3 reads after merge, got %d", len(merged.Reads))
	}

	// Flags: should have starred from b
	if !merged.HasFlag(idA, "starred") {
		t.Errorf("merged should have starred from state b")
	}
}

func TestMerge_FlagsUnion(t *testing.T) {
	a := New()
	a.SetFlag(idA, "starred")

	b := New()
	// b does NOT have idA starred

	// When merging, flags from both states are union'd
	merged := Merge(a, b)

	// Since a has the flag and b doesn't, the union still contains it
	if !merged.HasFlag(idA, "starred") {
		t.Errorf("merged flags should include 'starred' from state a")
	}
}

// ── Encrypted payload serialization tests ───────────────────────────────────

func TestPayloadRoundTrip(t *testing.T) {
	s := New()
	s.MarkRead(idA)
	s.MarkRead(idB)
	s.SetFlag(idA, "starred")
	s.MoveToFolder(idA, "Work")
	s.MarkDeleted(idC)

	// Serialize to payload
	payload := s.ToPayload()

	// Deserialize back
	restored := FromPayload(payload)

	// Verify reads
	if !restored.IsRead(idA) || !restored.IsRead(idB) {
		t.Errorf("restored reads mismatch")
	}

	// Verify flags
	if !restored.HasFlag(idA, "starred") {
		t.Errorf("restored should have A starred")
	}

	// Verify folders
	if restored.GetFolder(idA) != "Work" {
		t.Errorf("restored folder for A should be 'Work', got %s", restored.GetFolder(idA))
	}

	// Verify deleted
	if !restored.IsDeleted(idC) {
		t.Errorf("restored should have C deleted")
	}
}

func TestPayloadJSONSchema(t *testing.T) {
	s := New()
	s.MarkRead(idA)
	s.SetFlag(idA, "starred")
	s.MoveToFolder(idB, "archive")
	s.MarkDeleted(idC)

	payload := s.ToPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	// Verify it parses back to the same structure
	var parsed StatePayload
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if len(parsed.Read) != 1 || parsed.Read[0] != idA {
		t.Errorf("expected read [%s], got %v", idA, parsed.Read)
	}
	if len(parsed.Flag[idA]) != 1 || parsed.Flag[idA][0] != "starred" {
		t.Errorf("expected flag {%s: [starred]}, got %v", idA, parsed.Flag)
	}
	if parsed.Folder[idB] != "archive" {
		t.Errorf("expected folder {%s: archive}, got %v", idB, parsed.Folder)
	}
	if len(parsed.Deleted) != 1 || parsed.Deleted[0] != idC {
		t.Errorf("expected deleted [%s], got %v", idC, parsed.Deleted)
	}
}

func TestSerializeState_DTagOnly(t *testing.T) {
	s := New()
	s.MarkRead(idA)

	tags, content, err := s.SerializeState("2026-04")
	if err != nil {
		t.Fatalf("SerializeState failed: %v", err)
	}

	// Only the d-tag should be visible
	if len(tags) != 1 {
		t.Errorf("expected 1 tag (d-tag only), got %d", len(tags))
	}
	if tags[0][0] != "d" || tags[0][1] != "2026-04" {
		t.Errorf("expected d-tag [d, 2026-04], got %v", tags[0])
	}

	// Content should be valid JSON
	var payload StatePayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if len(payload.Read) != 1 || payload.Read[0] != idA {
		t.Errorf("payload read mismatch")
	}
}

func TestDeserializeState_RoundTrip(t *testing.T) {
	s := New()
	s.MarkRead(idA)
	s.MarkRead(idB)
	s.SetFlag(idA, "starred")
	s.MoveToFolder(idB, "archive")
	s.MarkDeleted(idC)

	_, content, err := s.SerializeState("2026-04")
	if err != nil {
		t.Fatalf("SerializeState failed: %v", err)
	}

	restored, err := DeserializeState(content)
	if err != nil {
		t.Fatalf("DeserializeState failed: %v", err)
	}

	if !restored.IsRead(idA) || !restored.IsRead(idB) {
		t.Errorf("restored reads mismatch")
	}
	if !restored.HasFlag(idA, "starred") {
		t.Errorf("restored should have A starred")
	}
	if restored.GetFolder(idB) != "archive" {
		t.Errorf("restored folder mismatch")
	}
	if !restored.IsDeleted(idC) {
		t.Errorf("restored should have C deleted")
	}
}

// ── Legacy tag tests ────────────────────────────────────────────────────────

func TestLegacyTagsRoundTrip(t *testing.T) {
	s := New()
	s.MarkRead(idA)
	s.MarkRead(idB)
	s.SetFlag(idA, "starred")
	s.MoveToFolder(idA, "Work")
	s.MarkDeleted(idC)

	// Serialize to legacy tags
	tags := s.ToTags("2026-04")

	// Deserialize back
	restored := FromTags(tags)

	if !restored.IsRead(idA) || !restored.IsRead(idB) {
		t.Errorf("restored reads mismatch")
	}
	if !restored.HasFlag(idA, "starred") {
		t.Errorf("restored should have A starred")
	}
	if restored.GetFolder(idA) != "Work" {
		t.Errorf("restored folder for A should be 'Work', got %s", restored.GetFolder(idA))
	}
	if !restored.IsDeleted(idC) {
		t.Errorf("restored should have C deleted")
	}
}

func TestFromTags_ParsesSpecFormat(t *testing.T) {
	// Tag format per NIP spec: ["flag", messageId, flag1, flag2, ...]
	// ["folder", messageId, folderName]
	tags := [][]string{
		{"d", "2026-04"},
		{"read", idA},
		{"read", idB},
		{"flag", idA, "starred"},
		{"folder", idA, "Work"},
	}

	s := FromTags(tags)

	if !s.IsRead(idA) {
		t.Errorf("should have read A")
	}
	if !s.IsRead(idB) {
		t.Errorf("should have read B")
	}
	if !s.HasFlag(idA, "starred") {
		t.Errorf("should have A starred")
	}
	if s.GetFolder(idA) != "Work" {
		t.Errorf("expected folder 'Work' for A, got %s", s.GetFolder(idA))
	}
}

func TestMarkDeleted_GSet(t *testing.T) {
	s := New()
	s.MarkDeleted(idA)

	if !s.IsDeleted(idA) {
		t.Errorf("should be deleted")
	}
	if s.IsDeleted(idB) {
		t.Errorf("B should not be deleted")
	}

	// Idempotent
	s.MarkDeleted(idA)
	count := 0
	for range s.Deleted {
		count++
	}
	if count != 1 {
		t.Errorf("delete should be idempotent, got %d entries", count)
	}
}

func TestMerge_DeletedGSetUnion(t *testing.T) {
	a := New()
	a.MarkDeleted(idA)

	b := New()
	b.MarkDeleted(idB)

	merged := Merge(a, b)
	if !merged.IsDeleted(idA) {
		t.Errorf("merged should have A deleted")
	}
	if !merged.IsDeleted(idB) {
		t.Errorf("merged should have B deleted")
	}
}
