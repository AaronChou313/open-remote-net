package forward

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var devicePattern = regexp.MustCompile(`^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)

type consoleConn struct {
	connection net.Conn
	hijacked   bool
}
type connectionKey struct{}

// CreateConsole provides a distinct browser origin per ORH device. Requests
// still require the remote ORH login. The controller forwards HTTP and WebSocket
// through the encrypted mesh, with no public listener or Cloud data relay.
func (m *Manager) CreateConsole(ctx context.Context, ip, expectedDevice string) (Entry, error) {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(ip, "8765")}
	transport := &http.Transport{DialContext: m.dialer.Dial, ResponseHeaderTimeout: 12 * time.Second, IdleConnTimeout: 60 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, "GET", target.String()+"/.well-known/openremotehub", nil)
	if err != nil {
		return Entry{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		transport.CloseIdleConnections()
		return Entry{}, fmt.Errorf("目标设备未提供 ORH Web 控制台")
	}
	var identity struct {
		Service    string `json:"service"`
		InstanceID string `json:"instance_id"`
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&identity)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || identity.Service != "openremotehub" || !devicePattern.MatchString(identity.InstanceID) || expectedDevice != "" && expectedDevice != identity.InstanceID {
		transport.CloseIdleConnections()
		return Entry{}, fmt.Errorf("目标控制台身份不匹配")
	}
	// Reuse an existing browser origin for this device while this process lives.
	m.mu.Lock()
	for _, tunnel := range m.tunnels {
		if tunnel.Kind == "console" && tunnel.DeviceID == identity.InstanceID && tunnel.PeerIP == ip {
			entry := tunnel.Entry
			m.mu.Unlock()
			transport.CloseIdleConnections()
			return entry, nil
		}
	}
	m.mu.Unlock()
	tunnel, err := m.allocate(ip, 8765, 0)
	if err != nil {
		transport.CloseIdleConnections()
		return Entry{}, err
	}
	origin := &url.URL{Scheme: "http", Host: "orh-" + identity.InstanceID + ".localhost:" + strconv.Itoa(tunnel.LocalPort)}
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.Out.Host = target.Host
			if request.In.Header.Get("Origin") != "" {
				request.Out.Header.Set("Origin", target.String())
			}
			request.Out.Header.Del("Forwarded")
		},
		ModifyResponse: func(response *http.Response) error {
			location := response.Header.Get("Location")
			if location == "" {
				return nil
			}
			redirect, err := url.Parse(location)
			if err != nil {
				return err
			}
			if redirect.Host == target.Host {
				redirect.Host = origin.Host
				redirect.Scheme = origin.Scheme
			}
			fragment, err := url.ParseQuery(redirect.Fragment)
			if err == nil && fragment.Has("authorize") {
				var intent map[string]any
				if json.Unmarshal([]byte(fragment.Get("authorize")), &intent) == nil && intent["device"] == identity.InstanceID {
					intent["address"] = origin.String()
					data, err := json.Marshal(intent)
					if err != nil {
						return err
					}
					fragment.Set("authorize", string(data))
					redirect.Fragment = fragment.Encode()
				}
			}
			response.Header.Set("Location", redirect.String())
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "控制台连接中断，请返回 ORN 重新连接", http.StatusBadGateway)
		},
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	server.ConnContext = func(ctx context.Context, connection net.Conn) context.Context {
		return context.WithValue(ctx, connectionKey{}, &consoleConn{connection: connection})
	}
	server.ConnState = func(connection net.Conn, status http.ConnState) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch status {
		case http.StateNew:
			if tunnel.closed {
				connection.Close()
				return
			}
			tunnel.active[connection] = struct{}{}
			tunnel.Connections++
			tunnel.last = time.Now()
		case http.StateClosed:
			if _, ok := tunnel.active[connection]; ok {
				delete(tunnel.active, connection)
				tunnel.Connections--
			}
			tunnel.last = time.Now()
		}
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != origin.Host || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin.String() {
			http.Error(w, "控制台请求来源无效", http.StatusForbidden)
			return
		}
		link := r.Context().Value(connectionKey{}).(*consoleConn)
		// ReverseProxy serves a hijacked WebSocket until its bidirectional relay ends.
		websocket := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		if websocket {
			defer func() { link.connection.Close(); server.ConnState(link.connection, http.StateClosed) }()
		}
		proxy.ServeHTTP(w, r)
	})
	m.mu.Lock()
	tunnel.Kind = "console"
	tunnel.DeviceID = identity.InstanceID
	tunnel.URL = origin.String()
	tunnel.cleanup = func() { server.Close(); transport.CloseIdleConnections() }
	entry := tunnel.Entry
	m.mu.Unlock()
	go server.Serve(tunnel.listener)
	return entry, nil
}
