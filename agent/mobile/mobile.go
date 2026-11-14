// Package mobile exposes a small gomobile-compatible lifecycle. Tokens remain
// inside the application's private storage and are used for loopback API calls.
package mobile

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/AaronChou313/open-remote-net/agent/internal/agentapp"
)

var mu sync.Mutex
var running *agentapp.App

func Start(directory string) error {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		return nil
	}
	app, err := agentapp.Start(filepath.Join(directory, "config.json"), "0.1.0", "", "", false)
	if err != nil {
		return err
	}
	running = app
	return nil
}
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if running == nil {
		return nil
	}
	err := running.Close()
	running = nil
	return err
}
func Token() string {
	mu.Lock()
	defer mu.Unlock()
	if running == nil {
		return ""
	}
	return running.Token
}
func Rebind() error {
	mu.Lock()
	app := running
	mu.Unlock()
	if app == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return app.Node.Rebind(ctx)
}
