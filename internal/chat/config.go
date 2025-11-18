package chat

import (
	"io"
	"os"

	ma "github.com/multiformats/go-multiaddr"
)

const DefaultTopicName = "my_applesauce"

type Config struct {
	TopicName string
	UserName  string
	NoDHT     bool
	Peers     []ma.Multiaddr
	Relays    []ma.Multiaddr
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
}

func (c Config) withDefaults() Config {
	if c.TopicName == "" {
		c.TopicName = DefaultTopicName
	}
	if c.In == nil {
		c.In = os.Stdin
	}
	if c.Out == nil {
		c.Out = os.Stdout
	}
	if c.Err == nil {
		c.Err = os.Stderr
	}
	return c
}
