package chat

import (
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/peer"
)

type User struct {
	ID       peer.ID
	Username string
}

func DefaultUserName(id peer.ID) string {
	return defaultUserName(id, os.Getenv("USER"))
}

func defaultUserName(id peer.ID, systemUser string) string {
	return fmt.Sprintf("%s-%s", systemUser, shortID(id))
}

func shortID(id peer.ID) string {
	pretty := id.String()
	if len(pretty) <= 8 {
		return pretty
	}
	return pretty[len(pretty)-8:]
}
