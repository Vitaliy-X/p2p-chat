package chat

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	ma "github.com/multiformats/go-multiaddr"
)

type discoveryNotifee struct {
	ctx    context.Context
	h      host.Host
	logger *log.Logger
	out    io.Writer
}

func (n discoveryNotifee) HandlePeerFound(info peer.AddrInfo) {
	connectToPeer(n.ctx, n.h, info, n.logger, n.out)
}

func startMDNS(ctx context.Context, h host.Host, logger *log.Logger, out io.Writer) (io.Closer, error) {
	service := mdns.NewMdnsService(h, mdns.ServiceName, discoveryNotifee{
		ctx:    ctx,
		h:      h,
		logger: logger,
		out:    out,
	})
	if err := service.Start(); err != nil {
		return nil, err
	}
	return service, nil
}

func printHostInfo(out io.Writer, h host.Host) {
	fmt.Fprintln(out, "Peer ID:", h.ID())
	fmt.Fprintln(out, "Addresses:")
	for _, addr := range h.Addrs() {
		fmt.Fprintf(out, "  %s/p2p/%s\n", addr, h.ID())
	}
}

func connectToConfiguredPeers(ctx context.Context, h host.Host, addrs []ma.Multiaddr, logger *log.Logger, out io.Writer) {
	infos, err := addrInfosFromP2PAddrs(addrs)
	if err != nil {
		logger.Println("Configured peer warning:", err)
		return
	}
	for _, info := range infos {
		connectToPeer(ctx, h, info, logger, out)
	}
}

func connectToPeer(ctx context.Context, h host.Host, info peer.AddrInfo, logger *log.Logger, out io.Writer) {
	if info.ID == h.ID() {
		return
	}
	if h.Network().Connectedness(info.ID) == network.Connected {
		return
	}
	if err := h.Connect(ctx, info); err != nil {
		logger.Printf("Failed connecting to %s: %v", info.ID, err)
		return
	}
	fmt.Fprintln(out, "Connected to:", info.ID)
}

func initDHT(ctx context.Context, h host.Host, logger *log.Logger) (*dht.IpfsDHT, error) {
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			logger.Println("Bootstrap peer parse warning:", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerInfo); err != nil {
				logger.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT, nil
}

func searchPeers(ctx context.Context, h host.Host, topicName string, logger *log.Logger, out io.Writer) {
	kademliaDHT, err := initDHT(ctx, h, logger)
	if err != nil {
		if ctx.Err() == nil {
			logger.Println("DHT warning:", err)
		}
		return
	}
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, topicName)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		fmt.Fprintln(out, "Searching for peers with DHT...")
		peerChan, err := routingDiscovery.FindPeers(ctx, topicName)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Println("DHT find peers warning:", err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		for info := range peerChan {
			connectToPeer(ctx, h, info, logger, out)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
