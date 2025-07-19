package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

var (
	topicNameFlag = flag.String("topicName", "my_applesauce", "name of topic to join")
	userNameFlag  = flag.String("userName", "", "username to authenticate with")
	user          User
)

func main() {
	flag.Parse()
	ctx := context.Background()

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0", "/ip6/::/tcp/0"),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.DefaultTransports,
	)
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}
	go searchPeers(ctx, h)

	user.ID = h.ID()
	user.Username = *userNameFlag
	if len(user.Username) == 0 {
		user.Username = defaultUserName(h.ID())
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		log.Fatalf("Failed to create pubsub service: %v", err)
	}
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		log.Fatalf("Failed to join topic: %v", err)
	}

	go startChat(ctx, topic)

	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("Failed to subscribe to topic: %v", err)
	}
	receiveMessages(ctx, sub)
}

func initDHT(ctx context.Context, h host.Host) *dht.IpfsDHT {
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		log.Fatalf("Failed to create DHT: %v", err)
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		log.Fatalf("Failed to bootstrap DHT: %v", err)
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				log.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func searchPeers(ctx context.Context, h host.Host) {
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, *topicNameFlag)

	anyConnected := false
	for !anyConnected {
		fmt.Println("Searching for peers...")
		peerChan, err := routingDiscovery.FindPeers(ctx, *topicNameFlag)
		if err != nil {
			log.Fatalf("Failed to find peers: %v", err)
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue // No self connection
			}
			err := h.Connect(ctx, peer)
			if err != nil {
				//fmt.Printf("Failed connecting to %s, error: %s\n", peer.ID, err)
			} else {
				fmt.Println("Connected to:", peer.ID)
				anyConnected = true
			}
		}
	}
	fmt.Println("Peer discovery complete")
}

func defaultUserName(id peer.ID) string {
	return fmt.Sprintf("%s-%s", os.Getenv("USER"), shortID(id))
}

func shortID(id peer.ID) string {
	pretty := id.String()
	return pretty[len(pretty)-8:]
}

func packMessage(message string) []byte {
	mes := ChatMessage{
		Message:        message,
		SenderID:       user.ID,
		SenderUsername: user.Username,
	}
	data, err := json.Marshal(mes)
	if err != nil {
		log.Println("Failed to encode message:", err)
	}

	return data
}

func startChat(ctx context.Context, topic *pubsub.Topic) {
	fmt.Println("P2P chat launched")
	reader := bufio.NewReader(os.Stdin)
	for {
		mes, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Failed to read input:", err)
		}
		if err := topic.Publish(ctx, packMessage(mes)); err != nil {
			log.Println("### Publish error:", err)
		}
	}
}

func receiveMessages(ctx context.Context, sub *pubsub.Subscription) {
	for {
		mes, err := sub.Next(ctx)
		if err != nil {
			log.Println("Failed to read next message:", err)
		}
		cm := new(ChatMessage)
		if err := json.Unmarshal(mes.Data, cm); err != nil {
			log.Println("Failed to decode message:", err)
		}

		fmt.Println(cm.SenderUsername, ": ", cm.Message)
	}
}
