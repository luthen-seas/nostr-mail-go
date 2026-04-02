// Package state implements the NOSTR Mail mailbox state (kind 10099) using
// CRDT-based conflict resolution for multi-device sync.
//
// The state model uses:
//   - G-Set (grow-only set) for reads and deletes — once added, never removed
//   - LWW (last-writer-wins) for flags — the latest state event determines flag presence
//   - LWW for folders — each message can be in at most one folder, latest state wins
package state

// MailboxState represents the synchronized state of a user's mailbox.
// It tracks which messages have been read, flagged, moved to folders,
// or deleted.
type MailboxState struct {
	Reads   map[string]string   // G-Set: event ID -> timestamp when read
	Flags   map[string][]string // event ID -> list of flag names (e.g., "flagged")
	Folders map[string]string   // event ID -> folder name (LWW)
	Deleted map[string]bool     // G-Set: event IDs that have been deleted
}

// New creates an empty MailboxState with initialized maps.
func New() *MailboxState {
	return &MailboxState{
		Reads:   make(map[string]string),
		Flags:   make(map[string][]string),
		Folders: make(map[string]string),
		Deleted: make(map[string]bool),
	}
}

// MarkRead adds an event ID to the read set. This is a G-Set operation:
// once marked read, the entry cannot be removed through state merges.
// The timestamp records when the message was read.
func (s *MailboxState) MarkRead(eventID string, timestamp string) {
	if _, exists := s.Reads[eventID]; !exists {
		s.Reads[eventID] = timestamp
	}
}

// IsRead returns true if the given event ID is in the read set.
func (s *MailboxState) IsRead(eventID string) bool {
	_, exists := s.Reads[eventID]
	return exists
}

// ToggleFlag adds a flag to a message if not present, or removes it if already
// present. Common flags include "flagged" for starred/important messages.
func (s *MailboxState) ToggleFlag(eventID, flag string) {
	flags := s.Flags[eventID]
	for i, f := range flags {
		if f == flag {
			// Remove the flag.
			s.Flags[eventID] = append(flags[:i], flags[i+1:]...)
			if len(s.Flags[eventID]) == 0 {
				delete(s.Flags, eventID)
			}
			return
		}
	}
	// Add the flag.
	s.Flags[eventID] = append(flags, flag)
}

// SetFlag adds a specific flag to a message without toggling.
func (s *MailboxState) SetFlag(eventID, flag string) {
	flags := s.Flags[eventID]
	for _, f := range flags {
		if f == flag {
			return // already set
		}
	}
	s.Flags[eventID] = append(flags, flag)
}

// HasFlag returns true if the given event has the specified flag.
func (s *MailboxState) HasFlag(eventID, flag string) bool {
	for _, f := range s.Flags[eventID] {
		if f == flag {
			return true
		}
	}
	return false
}

// MoveToFolder assigns a message to a folder. A message can only be in one
// folder at a time; this overwrites any previous folder assignment.
func (s *MailboxState) MoveToFolder(eventID, folder string) {
	s.Folders[eventID] = folder
}

// GetFolder returns the folder name for a message, or empty string if the
// message is in the inbox (no explicit folder).
func (s *MailboxState) GetFolder(eventID string) string {
	return s.Folders[eventID]
}

// MarkDeleted adds an event ID to the deleted set. This is a G-Set operation:
// once deleted, the entry cannot be removed through state merges.
func (s *MailboxState) MarkDeleted(eventID string) {
	s.Deleted[eventID] = true
}

// IsDeleted returns true if the given event ID is in the deleted set.
func (s *MailboxState) IsDeleted(eventID string) bool {
	return s.Deleted[eventID]
}

// Merge combines two mailbox states using CRDT merge rules:
//   - Reads: G-Set union (all reads from both states are preserved)
//   - Deleted: G-Set union (all deletes from both states are preserved)
//   - Flags: both preserved (union of all flags from both states)
//   - Folders: LWW — state b wins when it has a later created_at (caller
//     is responsible for passing b as the newer state)
//
// The convention is that b is the state with the later created_at timestamp.
// For folders (LWW), b's assignments take precedence over a's.
func Merge(a, b *MailboxState) *MailboxState {
	result := New()

	// Reads: G-Set union.
	for id, ts := range a.Reads {
		result.Reads[id] = ts
	}
	for id, ts := range b.Reads {
		if _, exists := result.Reads[id]; !exists {
			result.Reads[id] = ts
		}
	}

	// Deleted: G-Set union.
	for id := range a.Deleted {
		result.Deleted[id] = true
	}
	for id := range b.Deleted {
		result.Deleted[id] = true
	}

	// Flags: union of all flags from both states.
	flagSet := make(map[string]map[string]bool)
	for id, flags := range a.Flags {
		if flagSet[id] == nil {
			flagSet[id] = make(map[string]bool)
		}
		for _, f := range flags {
			flagSet[id][f] = true
		}
	}
	for id, flags := range b.Flags {
		if flagSet[id] == nil {
			flagSet[id] = make(map[string]bool)
		}
		for _, f := range flags {
			flagSet[id][f] = true
		}
	}
	for id, fs := range flagSet {
		for f := range fs {
			result.Flags[id] = append(result.Flags[id], f)
		}
	}

	// Folders: LWW — b (newer) wins. Start with a's folders, then overwrite
	// with b's.
	for id, folder := range a.Folders {
		result.Folders[id] = folder
	}
	for id, folder := range b.Folders {
		result.Folders[id] = folder
	}

	return result
}

// ToTags serializes the mailbox state to kind 10099 event tags.
// The output format follows the NOSTR Mail specification:
//   - ["d", "mailbox-state"] — required d-tag for addressable events
//   - ["read", eventId, timestamp] — one per read message
//   - ["flagged", eventId] — one per flagged message
//   - ["folder", folderName, eventId] — one per folder assignment
//   - ["deleted", eventId] — one per deleted message
func (s *MailboxState) ToTags() [][]string {
	var tags [][]string

	// d-tag for addressable event (kind 10099 is in the replaceable range).
	tags = append(tags, []string{"d", "mailbox-state"})

	// Read tags (G-Set).
	for id, ts := range s.Reads {
		tags = append(tags, []string{"read", id, ts})
	}

	// Flagged tags.
	for id, flags := range s.Flags {
		for _, f := range flags {
			if f == "flagged" {
				tags = append(tags, []string{"flagged", id})
			}
		}
	}

	// Folder tags.
	for id, folder := range s.Folders {
		tags = append(tags, []string{"folder", folder, id})
	}

	// Deleted tags (G-Set).
	for id := range s.Deleted {
		tags = append(tags, []string{"deleted", id})
	}

	return tags
}

// FromTags deserializes kind 10099 event tags into a MailboxState.
// Unknown tags are silently ignored.
func FromTags(tags [][]string) *MailboxState {
	s := New()

	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "read":
			ts := ""
			if len(tag) >= 3 {
				ts = tag[2]
			}
			s.Reads[tag[1]] = ts
		case "flagged":
			s.SetFlag(tag[1], "flagged")
		case "folder":
			if len(tag) >= 3 {
				// Format: ["folder", folderName, eventId]
				s.Folders[tag[2]] = tag[1]
			}
		case "deleted":
			s.Deleted[tag[1]] = true
		}
	}

	return s
}
