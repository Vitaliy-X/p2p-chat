package chat

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPeerSetCountsConnections(t *testing.T) {
	peers := newPeerSet()
	id := peer.ID("peer-a")

	if !peers.add(id) {
		t.Fatal("first connection should be new")
	}
	if peers.add(id) {
		t.Fatal("second connection should not be new")
	}
	if peers.remove(id) {
		t.Fatal("first disconnect should keep peer active")
	}
	if !peers.remove(id) {
		t.Fatal("last disconnect should remove peer")
	}
	if peers.remove(id) {
		t.Fatal("unknown peer should not be removed")
	}
}
