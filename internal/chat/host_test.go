package chat

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func TestHostOptionsWithoutRelays(t *testing.T) {
	opts, err := hostOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) = %d, want 6", len(opts))
	}
}

func TestHostOptionsRequireRelayPeerID(t *testing.T) {
	relay := mustMultiaddr(t, "/ip4/127.0.0.1/tcp/4001")
	if _, err := hostOptions([]ma.Multiaddr{relay}); err == nil {
		t.Fatal("expected relay without /p2p peer id to fail")
	}
}
