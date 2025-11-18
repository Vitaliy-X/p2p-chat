package chat

import "testing"

func TestMessageDeduperSeenOrAdd(t *testing.T) {
	deduper := NewMessageDeduper(2)

	if deduper.SeenOrAdd("a") {
		t.Fatal("first id should not be seen")
	}
	if !deduper.SeenOrAdd("a") {
		t.Fatal("duplicate id should be seen")
	}
	if deduper.SeenOrAdd("b") {
		t.Fatal("new id should not be seen")
	}
	if deduper.SeenOrAdd("c") {
		t.Fatal("new id should not be seen")
	}
	if deduper.SeenOrAdd("a") {
		t.Fatal("oldest id should be evicted")
	}
	if !deduper.SeenOrAdd("c") {
		t.Fatal("newer id should still be tracked")
	}
}

func TestNewMessageDeduperUsesDefaultLimit(t *testing.T) {
	deduper := NewMessageDeduper(0)
	if deduper.limit != 1024 {
		t.Fatalf("limit = %d, want 1024", deduper.limit)
	}
}
