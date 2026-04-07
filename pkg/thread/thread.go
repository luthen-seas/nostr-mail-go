// Package thread implements thread reconstruction for NOSTR Mail conversations.
//
// Messages reference their parent via ["reply", parentEventId, relayHint] and
// the conversation root via ["thread", rootEventId, relayHint]. This package
// builds a tree from a flat list of messages, handling missing parents
// (orphans) gracefully.
package thread

import "sort"

// Message represents a decrypted mail message with the fields needed for
// thread reconstruction.
type Message struct {
	ID        string
	MessageID string // Stable identity from message-id tag (used for threading)
	PubKey    string
	ReplyTo   string // parent message-id from the "reply" tag; empty if root
	ThreadID  string // root message-id from the "thread" tag; empty if root
	CreatedAt int64
	Subject   string
	Content   string
}

// ThreadNode is a node in a thread tree, linking a message to its parent and
// children.
type ThreadNode struct {
	Message  Message
	Children []*ThreadNode
	Parent   *ThreadNode
}

// BuildThread constructs a thread tree from a set of messages.
//
// The algorithm:
//  1. Index all messages by their event ID.
//  2. For each message with a "reply" tag, link it as a child of the parent.
//  3. Messages with no parent, or whose parent is not in the provided set,
//     are treated as root nodes.
//  4. Children at each level are sorted by created_at (chronological order).
//
// Returns a slice of root nodes. Orphaned messages (whose parent is unknown)
// appear as separate roots.
func BuildThread(messages []Message) []*ThreadNode {
	// Step 1: Create a node for each message, indexed by message-id.
	nodeIndex := make(map[string]*ThreadNode, len(messages))
	for i := range messages {
		key := messages[i].MessageID
		if key == "" {
			key = messages[i].ID // fallback for messages without message-id
		}
		nodeIndex[key] = &ThreadNode{
			Message: messages[i],
		}
	}

	// Step 2: Link children to parents.
	var roots []*ThreadNode
	for _, node := range nodeIndex {
		parentID := node.Message.ReplyTo
		if parentID == "" {
			// No reply tag — this is a root message.
			roots = append(roots, node)
			continue
		}
		parentNode, found := nodeIndex[parentID]
		if !found {
			// Parent not in the provided set — treat as orphan root.
			roots = append(roots, node)
			continue
		}
		node.Parent = parentNode
		parentNode.Children = append(parentNode.Children, node)
	}

	// Step 3: Sort children at each level by created_at.
	for _, node := range nodeIndex {
		if len(node.Children) > 1 {
			sort.Slice(node.Children, func(i, j int) bool {
				return node.Children[i].Message.CreatedAt < node.Children[j].Message.CreatedAt
			})
		}
	}

	// Sort roots by created_at as well.
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Message.CreatedAt < roots[j].Message.CreatedAt
	})

	return roots
}

// Flatten returns messages in chronological depth-first order, starting from
// the given root nodes. Each root's subtree is traversed depth-first, with
// children visited in chronological order.
func Flatten(roots []*ThreadNode) []Message {
	var result []Message
	for _, root := range roots {
		flattenDFS(root, &result)
	}
	return result
}

// flattenDFS performs a depth-first traversal of the thread tree, appending
// each message to the result slice.
func flattenDFS(node *ThreadNode, result *[]Message) {
	if node == nil {
		return
	}
	*result = append(*result, node.Message)
	for _, child := range node.Children {
		flattenDFS(child, result)
	}
}

// FindOrphans returns messages whose parent ID is not present in the message
// set. These are messages that reference parents which have not been received.
func FindOrphans(messages []Message) []Message {
	ids := make(map[string]bool, len(messages))
	for _, m := range messages {
		ids[m.ID] = true
	}

	var orphans []Message
	for _, m := range messages {
		if m.ReplyTo != "" && !ids[m.ReplyTo] {
			orphans = append(orphans, m)
		}
	}
	return orphans
}

// MissingParents returns the set of event IDs referenced as parents that are
// not present in the message set. This is useful for fetching missing messages
// from relays.
func MissingParents(messages []Message) map[string]bool {
	ids := make(map[string]bool, len(messages))
	for _, m := range messages {
		ids[m.ID] = true
	}

	missing := make(map[string]bool)
	for _, m := range messages {
		if m.ReplyTo != "" && !ids[m.ReplyTo] {
			missing[m.ReplyTo] = true
		}
	}
	return missing
}
