package chat

import "testing"

func TestDefaultUserName(t *testing.T) {
	id := mustDecodePeerID(t)

	got := defaultUserName(id, "tester")
	want := "tester-y1so13up"
	if got != want {
		t.Fatalf("defaultUserName() = %q, want %q", got, want)
	}
}

func TestShortIDHandlesShortValues(t *testing.T) {
	got := shortID("")
	if got != "" {
		t.Fatalf("shortID() = %q, want empty string", got)
	}
}
