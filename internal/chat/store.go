package chat

import (
	"context"
	"io"
)

const DefaultHistoryLimit = 50

type Store interface {
	SaveMessage(context.Context, ChatMessage) (bool, error)
	MessagesByRoom(ctx context.Context, room string, limit int) ([]ChatMessage, error)
	SetSetting(ctx context.Context, key, value string) error
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

type noopStore struct{}

func NewNoopStore() Store {
	return noopStore{}
}

func (noopStore) SaveMessage(context.Context, ChatMessage) (bool, error) {
	return true, nil
}

func (noopStore) MessagesByRoom(context.Context, string, int) ([]ChatMessage, error) {
	return nil, nil
}

func (noopStore) SetSetting(context.Context, string, string) error {
	return nil
}

func (noopStore) GetSetting(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func CloseStore(store Store) error {
	closer, ok := store.(io.Closer)
	if !ok {
		return nil
	}
	return closer.Close()
}
