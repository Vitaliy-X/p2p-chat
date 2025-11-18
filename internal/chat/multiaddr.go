package chat

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type MultiaddrList []ma.Multiaddr

func (m *MultiaddrList) String() string {
	if m == nil {
		return ""
	}
	addrs := make([]string, 0, len(*m))
	for _, addr := range *m {
		addrs = append(addrs, addr.String())
	}
	return fmt.Sprint(addrs)
}

func (m *MultiaddrList) Set(value string) error {
	addr, err := ma.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*m = append(*m, addr)
	return nil
}

func (m MultiaddrList) Addrs() []ma.Multiaddr {
	return append([]ma.Multiaddr(nil), m...)
}

func addrInfosFromP2PAddrs(addrs []ma.Multiaddr) ([]peer.AddrInfo, error) {
	infos := make([]peer.AddrInfo, 0, len(addrs))
	for _, addr := range addrs {
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", addr, err)
		}
		infos = append(infos, *info)
	}
	return infos, nil
}
