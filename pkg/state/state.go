// Package state implements the NOSTR Mail mailbox state (kind 30099) using
// CRDT-based conflict resolution for multi-device sync.
//
// The state model uses:
//   - G-Set (grow-only set) for reads and deletes — once added, never removed
//   - LWW (last-writer-wins) for flags — the latest state event determines flag presence
//   - LWW for folders — each message can be in at most one folder, latest state wins
//
// Kind 30099 events carry state as an encrypted JSON payload in the content
// field (NIP-44 self-encrypted). The only visible tag is ["d", "YYYY-MM"].
package state

import "encoding/json"

// MailboxState represents the synchronized state of a user's mailbox.
// It tracks which messages have been read, flagged, moved to folders,
// or deleted. All IDs are message-id values (not gift-wrap event IDs).
type MailboxState struct {
	Reads   map[string]bool     // G-Set: message IDs that have been read
	Flags   map[string][]string // message ID -> list of flag names (e.g., "starred")
	Folders map[string]string   // message ID -> folder name (LWW)
	Deleted map[string]bool     // G-Set: message IDs that have been deleted
}

// StatePayload is the JSON schema for the encrypted kind 30099 content field.
type StatePayload struct {
	Read    []string            `json:"read"`
	Flag    map[string][]string `json:"flag"`
	Folder  map[string]string   `json:"folder"`
	Deleted []string            `json:"deleted"`
}

// New creates an empty MailboxState with initialized maps.
func New() *MailboxState {
	return &MailboxState{
		Reads:   make(map[string]bool),
		Flags:   make(map[string][]string),
		Folders: make(map[string]string),
		Deleted: make(map[string]bool),
	}
}

// MarkRead adds a message ID to the read set. This is a G-Set operation:
// once marked read, the entry cannot be removed through state merges.
func (s *MailboxState) MarkRead(messageID string) {
	s.Reads[messageID] = true
}

// IsRead returns true if the given message ID is in the read set.
func (s *MailboxState) IsRead(messageID string) bool {
	return s.Reads[messageID]
}

// ToggleFlag adds a flag to a message if not present, or removes it if already
// present. Common flags include "starred", "important", "flagged".
func (s *MailboxState) ToggleFlag(messageID, flag string) {
	flags := s.Flags[messageID]
	for i, f := range flags {
		if f == flag {
			// Remove the flag.
			s.Flags[messageID] = append(flags[:i], flags[i+1:]...)
			if len(s.Flags[messageID]) == 0 {
				delete(s.Flags, messageID)
			}
			return
		}
	}
	// Add the flag.
	s.Flags[messageID] = append(flags, flag)
}

// SetFlag adds a specific flag to a message without toggling.
func (s *MailboxState) SetFlag(messageID, flag string) {
	flags := s.Flags[messageID]
	for _, f := range flags {
		if f == flag {
			return // already set
		}
	}
	s.Flags[messageID] = append(flags, flag)
}

// HasFlag returns true if the given message has the specified flag.
func (s *MailboxState) HasFlag(messageID, flag string) bool {
	for _, f := range s.Flags[messageID] {
		if f == flag {
			return true
		}
	}
	return false
}

// MoveToFolder assigns a message to a folder. A message can only be in one
// folder at a time; this overwrites any previous folder assignment.
func (s *MailboxState) MoveToFolder(messageID, folder string) {
	s.Folders[messageID] = folder
}

// GetFolder returns the folder name for a message, or empty string if the
// message is in the inbox (no explicit folder).
func (s *MailboxState) GetFolder(messageID string) string {
	return s.Folders[messageID]
}

// MarkDeleted adds a message ID to the deleted set. This is a G-Set operation:
// once deleted, the entry cannot be removed through state merges.
func (s *MailboxState) MarkDeleted(messageID string) {
	s.Deleted[messageID] = true
}

// IsDeleted returns true if the given message ID is in the deleted set.
func (s *MailboxState) IsDeleted(messageID string) bool {
	return s.Deleted[messageID]
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
	for id := range a.Reads {
		result.Reads[id] = true
	}
	for id := range b.Reads {
		result.Reads[id] = true
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

// ToPayload converts the mailbox state to a StatePayload for JSON serialization.
// The caller is responsible for NIP-44 encrypting the JSON before publishing.
func (s *MailboxState) ToPayload() *StatePayload {
	p := &StatePayload{
		Read:    make([]string, 0, len(s.Reads)),
		Flag:    make(map[string][]string),
		Folder:  make(map[string]string),
		Deleted: make([]string, 0, len(s.Deleted)),
	}

	for id := range s.Reads {
		p.Read = append(p.Read, id)
	}

	for id, flags := range s.Flags {
		if len(flags) > 0 {
			p.Flag[id] = flags
		}
	}

	for id, folder := range s.Folders {
		p.Folder[id] = folder
	}

	for id := range s.Deleted {
		p.Deleted = append(p.Deleted, id)
	}

	return p
}

// FromPayload creates a MailboxState from a parsed StatePayload.
// The caller is responsible for NIP-44 decrypting the content first.
func FromPayload(p *StatePayload) *MailboxState {
	s := New()

	for _, id := range p.Read {
		s.Reads[id] = true
	}

	for id, flags := range p.Flag {
		if len(flags) > 0 {
			s.Flags[id] = flags
		}
	}

	for id, folder := range p.Folder {
		s.Folders[id] = folder
	}

	for _, id := range p.Deleted {
		s.Deleted[id] = true
	}

	return s
}

// SerializeState produces the tags and JSON content for a kind 30099 event.
// The caller MUST NIP-44 encrypt the content string to the user's own pubkey.
func (s *MailboxState) SerializeState(partition string) (tags [][]string, content string, err error) {
	payload := s.ToPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	return [][]string{{"d", partition}}, string(data), nil
}

// DeserializeState parses a decrypted kind 30099 content string into a MailboxState.
func DeserializeState(content string) (*MailboxState, error) {
	var p StatePayload
	if err := json.Unmarshal([]byte(content), &p); err != nil {
		return nil, err
	}
	return FromPayload(&p), nil
}

// ── Legacy compatibility ────────────────────────────────────────────────────
// These functions support the old plaintext-tags format for migration.

// ToTags serializes the mailbox state to kind 30099 event tags (legacy format).
// Deprecated: Use SerializeState() instead.
func (s *MailboxState) ToTags(partition string) [][]string {
	var tags [][]string

	tags = append(tags, []string{"d", partition})

	for id := range s.Reads {
		tags = append(tags, []string{"read", id})
	}

	for id, flags := range s.Flags {
		if len(flags) > 0 {
			tag := []string{"flag", id}
			tag = append(tag, flags...)
			tags = append(tags, tag)
		}
	}

	for id, folder := range s.Folders {
		tags = append(tags, []string{"folder", id, folder})
	}

	for id := range s.Deleted {
		tags = append(tags, []string{"deleted", id})
	}

	return tags
}

// FromTags deserializes kind 30099 event tags into a MailboxState (legacy format).
// Deprecated: Use DeserializeState() instead.
func FromTags(tags [][]string) *MailboxState {
	s := New()

	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "read":
			s.Reads[tag[1]] = true
		case "flag":
			if len(tag) >= 3 {
				s.Flags[tag[1]] = tag[2:]
			}
		case "folder":
			if len(tag) >= 3 {
				s.Folders[tag[1]] = tag[2]
			}
		case "deleted":
			s.Deleted[tag[1]] = true
		}
	}

	return s
}
