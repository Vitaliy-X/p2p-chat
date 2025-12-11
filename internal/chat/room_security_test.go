package chat

import (
	"errors"
	"strings"
	"testing"
)

func TestPrivateRoomTopic(t *testing.T) {
	a, err := privateRoomTopic("local", "shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := privateRoomTopic("local", "shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("topic must be deterministic")
	}
	if !strings.HasPrefix(a, privateTopicPrefix) {
		t.Fatalf("topic = %q, want prefix %q", a, privateTopicPrefix)
	}
	if strings.Contains(a, "local") || strings.Contains(a, "shared-secret") {
		t.Fatalf("topic leaks room or key: %q", a)
	}

	other, err := privateRoomTopic("local", "another-secret")
	if err != nil {
		t.Fatal(err)
	}
	if other == a {
		t.Fatal("different keys must produce different topics")
	}
}

func TestValidateRoomKey(t *testing.T) {
	if err := ValidateRoomKey("shared-secret"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoomKey("short"); !errors.Is(err, ErrInvalidRoomKey) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidRoomKey)
	}
	if err := ValidateRoomKey("   shared-secret   "); err != nil {
		t.Fatal(err)
	}
}
