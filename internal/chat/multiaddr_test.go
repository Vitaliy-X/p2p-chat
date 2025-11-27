package chat

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func TestMultiaddrListSetStringAndAddrs(t *testing.T) {
	var addrs MultiaddrList
	if err := addrs.Set("/ip4/127.0.0.1/tcp/4001"); err != nil {
		t.Fatal(err)
	}

	if got := addrs.String(); got != "[/ip4/127.0.0.1/tcp/4001]" {
		t.Fatalf("String() = %q", got)
	}
	values := addrs.Addrs()
	if len(values) != 1 {
		t.Fatalf("len(Addrs()) = %d, want 1", len(values))
	}
	if values[0].String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Fatalf("Addrs()[0] = %q", values[0])
	}

	values[0] = nil
	if addrs[0] == nil {
		t.Fatal("Addrs() should return a copy")
	}
}

func TestMultiaddrListRejectsInvalid(t *testing.T) {
	var addrs MultiaddrList
	if err := addrs.Set("not-a-multiaddr"); err == nil {
		t.Fatal("expected invalid multiaddr to fail")
	}
}

func TestAddrInfosFromP2PAddrs(t *testing.T) {
	addr := mustMultiaddr(t, "/ip4/127.0.0.1/tcp/4001/p2p/"+testPeerID)

	infos, err := addrInfosFromP2PAddrs([]ma.Multiaddr{addr})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("len(infos) = %d, want 1", len(infos))
	}
	if infos[0].ID.String() != testPeerID {
		t.Fatalf("peer id = %s, want %s", infos[0].ID, testPeerID)
	}
	if len(infos[0].Addrs) != 1 || infos[0].Addrs[0].String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Fatalf("peer addrs = %v", infos[0].Addrs)
	}
}

func TestAddrInfosRequirePeerID(t *testing.T) {
	addr := mustMultiaddr(t, "/ip4/127.0.0.1/tcp/4001")
	if _, err := addrInfosFromP2PAddrs([]ma.Multiaddr{addr}); err == nil {
		t.Fatal("expected address without /p2p peer id to fail")
	}
}

func mustMultiaddr(t *testing.T, value string) ma.Multiaddr {
	t.Helper()
	addr, err := ma.NewMultiaddr(value)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
