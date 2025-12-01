package chat

import (
	"sync"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
)

var bootstrapPeers = defaultBootstrapPeers()

type peerSet struct {
	mu    sync.Mutex
	peers map[peer.ID]int
}

func newPeerSet() *peerSet {
	return &peerSet{peers: make(map[peer.ID]int)}
}

func (s *peerSet) add(id peer.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[id]++
	return s.peers[id] == 1
}

func (s *peerSet) remove(id peer.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	count, ok := s.peers[id]
	if !ok {
		return false
	}
	if count > 1 {
		s.peers[id] = count - 1
		return false
	}
	delete(s.peers, id)
	return true
}

func isBootstrapPeer(id peer.ID) bool {
	_, ok := bootstrapPeers[id]
	return ok
}

func defaultBootstrapPeers() map[peer.ID]struct{} {
	ids := make(map[peer.ID]struct{}, len(dht.DefaultBootstrapPeers))
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		info, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			continue
		}
		ids[info.ID] = struct{}{}
	}
	return ids
}
