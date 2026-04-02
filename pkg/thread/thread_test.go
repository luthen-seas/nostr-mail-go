package thread

import (
	"testing"
)

const (
	alicePub   = "2c7cc62a697ea3a7826521f3fd34f0cb273693cbe5e9310f35449f43622a6748"
	bobPub     = "98b30d5bfd1e2e751d7a57e7a58e67e15b3f2e0a90f9f7e8e40f7f6e5d4c3b2a"
	charliePub = "d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4"

	idA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	idD = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

func TestBuildThread_LinearThread(t *testing.T) {
	// Alice -> Bob -> Alice (linear chain of 3)
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200, Subject: "Project Update", Content: "Hi Bob, here's the project update."},
		{ID: idB, PubKey: bobPub, ReplyTo: idA, ThreadID: idA, CreatedAt: 1711846800, Subject: "Re: Project Update", Content: "Thanks Alice, looks good."},
		{ID: idC, PubKey: alicePub, ReplyTo: idB, ThreadID: idA, CreatedAt: 1711850400, Subject: "Re: Project Update", Content: "Good question -- we're targeting end of Q2."},
	}

	roots := BuildThread(messages)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	root := roots[0]
	if root.Message.ID != idA {
		t.Errorf("root ID should be %s, got %s", idA, root.Message.ID)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root should have 1 child, got %d", len(root.Children))
	}

	child := root.Children[0]
	if child.Message.ID != idB {
		t.Errorf("child ID should be %s, got %s", idB, child.Message.ID)
	}
	if len(child.Children) != 1 {
		t.Fatalf("child should have 1 child, got %d", len(child.Children))
	}

	grandchild := child.Children[0]
	if grandchild.Message.ID != idC {
		t.Errorf("grandchild ID should be %s, got %s", idC, grandchild.Message.ID)
	}
	if len(grandchild.Children) != 0 {
		t.Errorf("grandchild should have 0 children, got %d", len(grandchild.Children))
	}
}

func TestBuildThread_BranchedThread(t *testing.T) {
	// Alice sends root. Bob and Charlie both reply to root -> 2 branches.
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200, Subject: "Team Decision", Content: "Option A or B?"},
		{ID: idB, PubKey: bobPub, ReplyTo: idA, ThreadID: idA, CreatedAt: 1711846800, Content: "I vote option A."},
		{ID: idD, PubKey: charliePub, ReplyTo: idA, ThreadID: idA, CreatedAt: 1711847000, Content: "I prefer option B."},
	}

	roots := BuildThread(messages)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	root := roots[0]
	if len(root.Children) != 2 {
		t.Fatalf("root should have 2 children (branched), got %d", len(root.Children))
	}

	// Children should be sorted by created_at: Bob (1711846800) before Charlie (1711847000)
	if root.Children[0].Message.ID != idB {
		t.Errorf("first child should be Bob (%s), got %s", idB, root.Children[0].Message.ID)
	}
	if root.Children[1].Message.ID != idD {
		t.Errorf("second child should be Charlie (%s), got %s", idD, root.Children[1].Message.ID)
	}
}

func TestBuildThread_DeepThread5Levels(t *testing.T) {
	id1 := "1000000000000000000000000000000000000000000000000000000000000000"
	id2 := "2000000000000000000000000000000000000000000000000000000000000000"
	id3 := "3000000000000000000000000000000000000000000000000000000000000000"
	id4 := "4000000000000000000000000000000000000000000000000000000000000000"
	id5 := "5000000000000000000000000000000000000000000000000000000000000000"

	messages := []Message{
		{ID: id1, PubKey: alicePub, CreatedAt: 1711843200, Content: "Level 0"},
		{ID: id2, PubKey: bobPub, ReplyTo: id1, ThreadID: id1, CreatedAt: 1711846800, Content: "Level 1"},
		{ID: id3, PubKey: alicePub, ReplyTo: id2, ThreadID: id1, CreatedAt: 1711850400, Content: "Level 2"},
		{ID: id4, PubKey: bobPub, ReplyTo: id3, ThreadID: id1, CreatedAt: 1711854000, Content: "Level 3"},
		{ID: id5, PubKey: alicePub, ReplyTo: id4, ThreadID: id1, CreatedAt: 1711857600, Content: "Level 4"},
	}

	roots := BuildThread(messages)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	// Walk 5 levels deep
	node := roots[0]
	expectedIDs := []string{id1, id2, id3, id4, id5}
	for depth, expectedID := range expectedIDs {
		if node.Message.ID != expectedID {
			t.Errorf("depth %d: expected ID %s, got %s", depth, expectedID, node.Message.ID)
		}
		if depth < 4 {
			if len(node.Children) != 1 {
				t.Fatalf("depth %d: expected 1 child, got %d", depth, len(node.Children))
			}
			node = node.Children[0]
		} else {
			if len(node.Children) != 0 {
				t.Errorf("depth %d: leaf should have 0 children, got %d", depth, len(node.Children))
			}
		}
	}
}

func TestBuildThread_OrphanedReply(t *testing.T) {
	// Event C replies to event B, but B is missing. Event A is the root.
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200, Content: "This is the root message."},
		{ID: idC, PubKey: charliePub, ReplyTo: idB, ThreadID: idA, CreatedAt: 1711850400, Content: "Replying to Bob's message (which we don't have yet)."},
	}

	roots := BuildThread(messages)

	// Should have 2 roots: A (true root) and C (orphan, parent B missing)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (1 real + 1 orphan), got %d", len(roots))
	}

	// Sorted by created_at: A first, then C
	if roots[0].Message.ID != idA {
		t.Errorf("first root should be %s, got %s", idA, roots[0].Message.ID)
	}
	if roots[1].Message.ID != idC {
		t.Errorf("second root (orphan) should be %s, got %s", idC, roots[1].Message.ID)
	}

	// A should have no children (B is missing)
	if len(roots[0].Children) != 0 {
		t.Errorf("root A should have 0 children, got %d", len(roots[0].Children))
	}
}

func TestBuildThread_SingleMessage(t *testing.T) {
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200, Subject: "Standalone", Content: "No replies."},
	}

	roots := BuildThread(messages)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Message.ID != idA {
		t.Errorf("root ID should be %s, got %s", idA, roots[0].Message.ID)
	}
	if len(roots[0].Children) != 0 {
		t.Errorf("single message should have no children")
	}
}

func TestBuildThread_EmptyInput(t *testing.T) {
	roots := BuildThread(nil)
	if len(roots) != 0 {
		t.Errorf("expected 0 roots for nil input, got %d", len(roots))
	}

	roots = BuildThread([]Message{})
	if len(roots) != 0 {
		t.Errorf("expected 0 roots for empty input, got %d", len(roots))
	}
}

func TestFlatten_ChronologicalOrder(t *testing.T) {
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200, Content: "First"},
		{ID: idB, PubKey: bobPub, ReplyTo: idA, ThreadID: idA, CreatedAt: 1711846800, Content: "Second"},
		{ID: idC, PubKey: alicePub, ReplyTo: idB, ThreadID: idA, CreatedAt: 1711850400, Content: "Third"},
	}

	roots := BuildThread(messages)
	flat := Flatten(roots)

	if len(flat) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(flat))
	}

	expectedIDs := []string{idA, idB, idC}
	for i, expected := range expectedIDs {
		if flat[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, flat[i].ID)
		}
	}
}

func TestFindOrphans(t *testing.T) {
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200},
		{ID: idC, PubKey: charliePub, ReplyTo: idB, ThreadID: idA, CreatedAt: 1711850400},
	}

	orphans := FindOrphans(messages)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0].ID != idC {
		t.Errorf("orphan should be %s, got %s", idC, orphans[0].ID)
	}
}

func TestMissingParents(t *testing.T) {
	messages := []Message{
		{ID: idA, PubKey: alicePub, CreatedAt: 1711843200},
		{ID: idC, PubKey: charliePub, ReplyTo: idB, ThreadID: idA, CreatedAt: 1711850400},
	}

	missing := MissingParents(messages)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing parent, got %d", len(missing))
	}
	if !missing[idB] {
		t.Errorf("expected %s to be missing", idB)
	}
}
