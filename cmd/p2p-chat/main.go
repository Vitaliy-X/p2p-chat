package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"p2p-chat/internal/chat"
	sqlitestore "p2p-chat/internal/store/sqlite"
)

func main() {
	var peerAddrs chat.MultiaddrList
	var relayAddrs chat.MultiaddrList

	topicName := flag.String("topicName", chat.DefaultTopicName, "name of topic to join")
	userName := flag.String("userName", "", "username to authenticate with")
	noDHT := flag.Bool("noDHT", false, "disable Kademlia DHT peer discovery")
	dbPath := flag.String("db-path", "", "SQLite database path for message history and settings")
	flag.Var(&peerAddrs, "peer", "peer multiaddr to connect to; can be repeated")
	flag.Var(&relayAddrs, "relay", "static relay multiaddr for AutoRelay; can be repeated")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := chat.NewNoopStore()
	if *dbPath != "" {
		sqliteStore, err := sqlitestore.Open(ctx, *dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer sqliteStore.Close()
		store = sqliteStore
	}

	cfg := chat.Config{
		TopicName: *topicName,
		UserName:  *userName,
		NoDHT:     *noDHT,
		Peers:     peerAddrs.Addrs(),
		Relays:    relayAddrs.Addrs(),
		Store:     store,
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
	}
	if err := chat.Run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
