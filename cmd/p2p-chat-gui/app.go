//go:build desktop || bindings

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"p2p-chat/internal/chat"
	sqlitestore "p2p-chat/internal/store/sqlite"
)

const (
	appDirName    = "p2p-chat"
	defaultDBName = "p2p-chat-gui.db"
)

type App struct {
	ctx context.Context

	mu      sync.Mutex
	session *chat.Session
	store   chat.Store
	status  string
	lastErr string
}

func NewApp() *App {
	return &App{status: "disconnected"}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {
	if err := a.Disconnect(); err != nil {
		log.Println("shutdown warning:", err)
	}
}

func (a *App) Connect(req ConnectRequest) (State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.disconnectLocked(); err != nil {
		return a.stateLocked(nil), err
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Room = strings.TrimSpace(req.Room)
	req.DBPath = strings.TrimSpace(req.DBPath)
	if req.Room == "" {
		req.Room = chat.DefaultTopicName
	}
	dbPath, err := resolveDBPath(req.DBPath)
	if err != nil {
		a.status = "error"
		a.lastErr = err.Error()
		a.emitStatusLocked(nil)
		return a.stateLocked(nil), err
	}

	a.status = "connecting"
	a.lastErr = ""
	a.emitStatusLocked(nil)

	store, err := sqlitestore.Open(context.Background(), dbPath)
	if err != nil {
		a.status = "error"
		a.lastErr = fmt.Sprintf("open sqlite database %q: %v", dbPath, err)
		a.emitStatusLocked(nil)
		return a.stateLocked(nil), errors.New(a.lastErr)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	session, err := chat.StartSession(context.Background(), chat.SessionConfig{
		Room:     req.Room,
		Username: req.Username,
		NoDHT:    req.NoDHT,
		Store:    store,
		Logger:   logger,
	})
	if err != nil {
		chat.CloseStore(store)
		a.status = "error"
		a.lastErr = err.Error()
		a.emitStatusLocked(nil)
		return a.stateLocked(nil), err
	}

	a.session = session
	a.store = store
	a.status = "connected"

	messages, err := a.historyLocked()
	if err != nil {
		a.lastErr = err.Error()
	}
	go a.forwardEvents(session)
	a.emitStatusLocked(messages)
	return a.stateLocked(messages), nil
}

func (a *App) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.disconnectLocked()
}

func (a *App) Send(text string) (MessageDTO, error) {
	a.mu.Lock()
	session := a.session
	a.mu.Unlock()
	if session == nil {
		return MessageDTO{}, errors.New("chat is not connected")
	}
	message, err := session.Send(context.Background(), text)
	if err != nil {
		a.setError(err)
		return MessageDTO{}, err
	}
	return toMessageDTO(message), nil
}

func (a *App) ManualConnect(addr string) error {
	a.mu.Lock()
	session := a.session
	a.mu.Unlock()
	if session == nil {
		return errors.New("chat is not connected")
	}
	if err := session.ConnectPeer(context.Background(), addr); err != nil {
		a.setError(err)
		return err
	}
	a.emitStatus()
	return nil
}

func (a *App) CopyPeerInfo() error {
	a.mu.Lock()
	session := a.session
	a.mu.Unlock()
	if session == nil {
		return errors.New("chat is not connected")
	}
	info := session.PeerInfo()
	text := strings.Join(info.Addresses, "\n")
	if text == "" {
		text = info.ID
	}
	return runtime.ClipboardSetText(a.ctx, text)
}

func (a *App) Status() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stateLocked(nil)
}

func (a *App) disconnectLocked() error {
	var err error
	if a.session != nil {
		err = a.session.Close()
		a.session = nil
	}
	if a.store != nil {
		if closeErr := chat.CloseStore(a.store); closeErr != nil && err == nil {
			err = closeErr
		}
		a.store = nil
	}
	a.status = "disconnected"
	a.emitStatusLocked(nil)
	return err
}

func (a *App) forwardEvents(session *chat.Session) {
	for {
		select {
		case event := <-session.Events():
			switch event.Type {
			case "message":
				if event.Message != nil {
					runtime.EventsEmit(a.ctx, "chat:message", toMessageDTO(*event.Message))
				}
			case "peer_connected", "peer_disconnected":
				a.emitStatus()
			case "error":
				a.setError(errors.New(event.Error))
			}
		case <-session.Done():
			a.emitStatus()
			return
		}
	}
}

func (a *App) historyLocked() ([]MessageDTO, error) {
	if a.session == nil {
		return nil, nil
	}
	messages, err := a.session.History(context.Background(), chat.DefaultHistoryLimit)
	if err != nil {
		return nil, err
	}
	return toMessageDTOs(messages), nil
}

func (a *App) stateLocked(messages []MessageDTO) State {
	state := State{
		Connected: a.session != nil,
		Status:    a.status,
		LastError: a.lastErr,
		Messages:  messages,
	}
	if a.session != nil {
		state.PeerCount = a.session.PeerCount()
		state.PeerInfo = a.session.PeerInfo()
	}
	return state
}

func (a *App) emitStatus() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.emitStatusLocked(nil)
}

func (a *App) emitStatusLocked(messages []MessageDTO) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "chat:status", a.stateLocked(messages))
}

func (a *App) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastErr = err.Error()
	a.emitStatusLocked(nil)
}

func resolveDBPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultDBName
	}
	if isSQLiteDSN(path) {
		return path, nil
	}
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		baseDir, err := appDataDir()
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(baseDir, expanded)
	}
	expanded = filepath.Clean(expanded)
	if err := os.MkdirAll(filepath.Dir(expanded), 0o700); err != nil {
		return "", fmt.Errorf("create sqlite directory: %w", err)
	}
	return expanded, nil
}

func isSQLiteDSN(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file:")
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func appDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err == nil {
		return filepath.Join(configDir, appDirName), nil
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return "", fmt.Errorf("resolve app data directory: %w", err)
	}
	return filepath.Join(home, "."+appDirName), nil
}
