package main

import "github.com/libp2p/go-libp2p/core/peer"

type User struct {
	ID       peer.ID
	Username string
}
