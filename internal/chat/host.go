package chat

import (
	"github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
)

func hostOptions(relayAddrs []ma.Multiaddr) ([]libp2p.Option, error) {
	opts := []libp2p.Option{
		libp2p.NATPortMap(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.EnableRelayService(),
	}
	if len(relayAddrs) == 0 {
		return opts, nil
	}

	relays, err := addrInfosFromP2PAddrs(relayAddrs)
	if err != nil {
		return nil, err
	}
	return append(opts, libp2p.EnableAutoRelayWithStaticRelays(relays)), nil
}
