package forward

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

type Dialer interface {
	Dial(context.Context, string, string) (net.Conn, error)
}
type Entry struct {
	ID          string    `json:"id"`
	PeerIP      string    `json:"peer_ip"`
	RemotePort  int       `json:"remote_port"`
	LocalPort   int       `json:"local_port"`
	Created     time.Time `json:"created"`
	Connections int       `json:"connections"`
	Kind        string    `json:"kind"`
	URL         string    `json:"url,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
}
type tunnel struct {
	Entry
	listener net.Listener
	active   map[net.Conn]struct{}
	last     time.Time
	cleanup  func()
	closed   bool
}
type Manager struct {
	mu      sync.Mutex
	dialer  Dialer
	tunnels map[string]*tunnel
	ctx     context.Context
	cancel  context.CancelFunc
}

func New(d Dialer) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{dialer: d, tunnels: map[string]*tunnel{}, ctx: ctx, cancel: cancel}
	go m.expire()
	return m
}
func (m *Manager) allocate(ip string, remote, local int) (*tunnel, error) {
	if net.ParseIP(ip) == nil || remote < 1 || remote > 65535 || local < 0 || local > 65535 {
		return nil, fmt.Errorf("IP 或端口无效")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", local))
	if err != nil {
		return nil, fmt.Errorf("本地端口不可用: %w", err)
	}
	var id [12]byte
	rand.Read(id[:])
	t := &tunnel{Entry: Entry{ID: hex.EncodeToString(id[:]), PeerIP: ip, RemotePort: remote, LocalPort: listener.Addr().(*net.TCPAddr).Port, Created: time.Now(), Kind: "tcp"}, listener: listener, last: time.Now(), active: map[net.Conn]struct{}{}}
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		listener.Close()
		return nil, fmt.Errorf("转发管理器已停止")
	}
	m.tunnels[t.ID] = t
	m.mu.Unlock()
	return t, nil
}
func (m *Manager) Create(ip string, remote, local int) (Entry, error) {
	tunnel, err := m.allocate(ip, remote, local)
	if err != nil {
		return Entry{}, err
	}
	entry := tunnel.Entry
	go m.accept(tunnel)
	return entry, nil
}

func (m *Manager) accept(t *tunnel) {
	for {
		local, err := t.listener.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		if t.closed {
			m.mu.Unlock()
			local.Close()
			return
		}
		t.active[local] = struct{}{}
		t.Connections++
		t.last = time.Now()
		m.mu.Unlock()
		go m.pipe(t, local)
	}
}
func (m *Manager) pipe(t *tunnel, local net.Conn) {
	defer func() {
		local.Close()
		m.mu.Lock()
		delete(t.active, local)
		t.Connections--
		t.last = time.Now()
		m.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(m.ctx, 12*time.Second)
	defer cancel()
	remote, err := m.dialer.Dial(ctx, "tcp", net.JoinHostPort(t.PeerIP, fmt.Sprint(t.RemotePort)))
	if err != nil {
		return
	}
	defer remote.Close()
	m.mu.Lock()
	if t.closed {
		m.mu.Unlock()
		return
	}
	t.active[remote] = struct{}{}
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(t.active, remote); m.mu.Unlock() }()
	done := make(chan struct{}, 2)
	copySide := func(dst, src net.Conn) {
		buffer := make([]byte, 32*1024)
		for {
			src.SetReadDeadline(time.Now().Add(10 * time.Minute))
			n, e := src.Read(buffer)
			if n > 0 {
				dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, w := dst.Write(buffer[:n]); w != nil {
					break
				}
			}
			if e != nil {
				break
			}
		}
		if half, ok := dst.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		}
		done <- struct{}{}
	}
	go copySide(local, remote)
	go copySide(remote, local)
	<-done
	<-done
}
func (m *Manager) List() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Entry{}
	for _, t := range m.tunnels {
		out = append(out, t.Entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	tunnel, ok := m.tunnels[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("转发已关闭")
	}
	tunnel.closed = true
	delete(m.tunnels, id)
	connections := make([]net.Conn, 0, len(tunnel.active))
	for connection := range tunnel.active {
		connections = append(connections, connection)
	}
	cleanup := tunnel.cleanup
	m.mu.Unlock()
	tunnel.listener.Close()
	for _, connection := range connections {
		connection.Close()
	}
	if cleanup != nil {
		cleanup()
	}
	return nil
}

func (m *Manager) expire() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			for _, entry := range m.List() {
				m.mu.Lock()
				t := m.tunnels[entry.ID]
				expired := t != nil && t.Connections == 0 && time.Since(t.last) > 10*time.Minute
				m.mu.Unlock()
				if expired {
					m.Delete(entry.ID)
				}
			}
		}
	}
}
func (m *Manager) Close() {
	m.cancel()
	m.CloseAll()
}

func (m *Manager) CloseAll() {
	for _, t := range m.List() {
		m.Delete(t.ID)
	}
}
