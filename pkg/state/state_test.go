package state

import (
	"testing"
)

const (
	idA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestMarkRead_Append(t *testing.T) {
	s := New()
	s.MarkRead(idA, "1711840000")

	if !s.IsRead(idA) {
		t.Errorf("expected %s to be read", idA)
	}
	if s.IsRead(idB) {
		t.Errorf("expected %s to NOT be read", idB)
	}

	// Add a second read
	s.MarkRead(idB, "1711846800")
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
	s.MarkRead(idA, "1711840000")
	s.MarkRead(idA, "1711846800") // second mark with different timestamp

	// G-Set idempotent: should keep the first timestamp
	if s.Reads[idA] != "1711840000" {
		t.Errorf("idempotent MarkRead should keep first timestamp, got %s", s.Reads[idA])
	}
	if len(s.Reads) != 1 {
		t.Errorf("expected 1 read entry (idempotent), got %d", len(s.Reads))
	}
}

func TestMarkRead_CannotUnread(t *testing.T) {
	// G-Set property: once read, cannot be un-read through normal operations.
	s := New()
	s.MarkRead(idA, "1711840000")

	// There is no "unread" method because G-Set is grow-only.
	// Verify the read persists.
	if !s.IsRead(idA) {
		t.Errorf("G-Set read should persist -- cannot be reverted")
	}
}

func TestToggleFlag(t *testing.T) {
	s := New()

	// Initially not flagged
	if s.HasFlag(idA, "flagged") {
		t.Errorf("should not be flagged initially")
	}

	// Toggle on
	s.ToggleFlag(idA, "flagged")
	if !s.HasFlag(idA, "flagged") {
		t.Errorf("should be flagged after toggle on")
	}

	// Toggle off
	s.ToggleFlag(idA, "flagged")
	if s.HasFlag(idA, "flagged") {
		t.Errorf("should not be flagged after toggle off")
	}
}

func TestSetFlag(t *testing.T) {
	s := New()
	s.SetFlag(idA, "flagged")

	if !s.HasFlag(idA, "flagged") {
		t.Errorf("expected flagged after SetFlag")
	}

	// SetFlag is idempotent
	s.SetFlag(idA, "flagged")
	count := 0
	for _, f := range s.Flags[idA] {
		if f == "flagged" {
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
	a.MarkRead(idA, "1711840000")
	a.MarkRead(idB, "1711846800")

	b := New()
	b.MarkRead(idA, "1711840000")
	b.MarkRead(idC, "1711850400")

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
	a.MarkRead(idA, "1711840000")
	a.MarkRead(idB, "1711846800")
	a.MarkRead(idC, "1711850400")
	a.MoveToFolder(idA, "Work")

	// Device 2 (newer): moves A to "Personal", flags A
	b := New()
	b.MarkRead(idA, "1711840000")
	b.MarkRead(idB, "1711846800")
	b.SetFlag(idA, "flagged")
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

	// Flags: should have flagged from b
	if !merged.HasFlag(idA, "flagged") {
		t.Errorf("merged should have flagged from state b")
	}
}

func TestMerge_FlagsUnion(t *testing.T) {
	a := New()
	a.SetFlag(idA, "flagged")

	b := New()
	// b does NOT have idA flagged

	// When merging, flags from both states are union'd
	merged := Merge(a, b)

	// Since a has the flag and b doesn't, the union still contains it
	if !merged.HasFlag(idA, "flagged") {
		t.Errorf("merged flags should include 'flagged' from state a")
	}
}

func TestSerializationRoundTrip(t *testing.T) {
	s := New()
	s.MarkRead(idA, "1711840000")
	s.MarkRead(idB, "1711846800")
	s.SetFlag(idA, "flagged")
	s.MoveToFolder(idA, "Work")
	s.MarkDeleted(idC)

	// Serialize to tags
	tags := s.ToTags("2026-04")

	// Deserialize back
	restored := FromTags(tags)

	// Verify reads
	if !restored.IsRead(idA) || !restored.IsRead(idB) {
		t.Errorf("restored reads mismatch")
	}
	if restored.Reads[idA] != "1711840000" {
		t.Errorf("restored read timestamp mismatch for A: got %s", restored.Reads[idA])
	}

	// Verify flags
	if !restored.HasFlag(idA, "flagged") {
		t.Errorf("restored should have A flagged")
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

func TestFromTags_ParsesTestVectorFormat(t *testing.T) {
	// Simulate the tag format from the test vectors
	tags := [][]string{
		{"d", "mailbox-state"},
		{"read", idA, "1711840000"},
		{"read", idB, "1711846800"},
		{"flagged", idA},
		{"folder", "Work", idA},
	}

	s := FromTags(tags)

	if !s.IsRead(idA) {
		t.Errorf("should have read A")
	}
	if !s.IsRead(idB) {
		t.Errorf("should have read B")
	}
	if !s.HasFlag(idA, "flagged") {
		t.Errorf("should have A flagged")
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
