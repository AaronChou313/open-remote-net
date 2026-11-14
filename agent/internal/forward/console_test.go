package forward

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type mappedDialer struct{ target string }

func (d mappedDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, d.target)
}

func TestConsoleOriginCookieRedirectAndWebSocket(t *testing.T) {
	const id = "f57bc9e0-1f1e-4c96-b4f8-c9e3be1f41c9"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "100.64.0.20:8765" {
			t.Errorf("upstream Host %q", r.Host)
		}
		switch r.URL.Path {
		case "/.well-known/openremotehub":
			json.NewEncoder(w).Encode(map[string]string{"service": "openremotehub", "instance_id": id})
		case "/auth/cloud/start":
			data, _ := json.Marshal(map[string]string{"device": id, "address": "http://100.64.0.20:8765", "state": "nonce"})
			location := "https://cloud.example/#" + url.Values{"authorize": {string(data)}}.Encode()
			http.SetCookie(w, &http.Cookie{Name: "orh_cloud_state", Value: "nonce", HttpOnly: true})
			http.Redirect(w, r, location, http.StatusFound)
		case "/echo":
			if r.Header.Get("Origin") != "http://100.64.0.20:8765" {
				t.Error("upstream Origin was not translated")
			}
			if cookie, err := r.Cookie("orh"); err != nil || cookie.Value != "session" {
				t.Error("session cookie lost")
			}
			connection, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer connection.CloseNow()
			kind, data, err := connection.Read(r.Context())
			if err == nil {
				connection.Write(r.Context(), kind, data)
			}
		default:
			io.WriteString(w, "ready")
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	manager := New(mappedDialer{target: target.Host})
	defer manager.Close()
	entry, err := manager.CreateConsole(context.Background(), "100.64.0.20", id)
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := url.Parse(entry.URL)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", origin.Port()))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(entry.URL + "/auth/cloud/start")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	redirect, _ := url.Parse(response.Header.Get("Location"))
	fragment, _ := url.ParseQuery(redirect.Fragment)
	var intent map[string]string
	json.Unmarshal([]byte(fragment.Get("authorize")), &intent)
	if intent["address"] != entry.URL || intent["state"] != "nonce" {
		t.Fatal("Cloud authorization callback was not mapped")
	}
	if len(response.Cookies()) != 1 || response.Cookies()[0].Name != "orh_cloud_state" {
		t.Fatal("nonce cookie missing")
	}
	request, _ := http.NewRequest("GET", entry.URL+"/", nil)
	request.Host = "attacker.example"
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("foreign Host accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	socket, _, err := websocket.Dial(ctx, strings.Replace(entry.URL, "http:", "ws:", 1)+"/echo", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {entry.URL}, "Cookie": {"orh=session"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	if err = socket.Write(ctx, websocket.MessageText, []byte("终端 WebSocket")); err != nil {
		t.Fatal(err)
	}
	_, data, err := socket.Read(ctx)
	if err != nil || string(data) != "终端 WebSocket" {
		t.Fatalf("WebSocket relay failed: %v", err)
	}
	same, err := manager.CreateConsole(context.Background(), "100.64.0.20", id)
	if err != nil || same.ID != entry.ID {
		t.Fatal("console origin was not reused")
	}
	manager.Delete(entry.ID)
}
