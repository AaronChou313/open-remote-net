package forward

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type testDialer struct{}

func (testDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}
func TestForwardRoundTripAndClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); io.Copy(conn, conn) }()
		}
	}()
	manager := New(testDialer{})
	defer manager.Close()
	entry, err := manager.Create("127.0.0.1", listener.Addr().(*net.TCPAddr).Port, 0)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(entry.LocalPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(3 * time.Second))
	expected := []byte("ORN真实双向转发")
	if _, err = connection.Write(expected); err != nil {
		t.Fatal(err)
	}
	actual := make([]byte, len(expected))
	if _, err = io.ReadFull(connection, actual); err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(expected) {
		t.Fatal("forward corrupted bytes")
	}
	if err = manager.Delete(entry.ID); err != nil {
		t.Fatal(err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("forward retained after delete")
	}
	if _, err = connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("active connection survived deletion")
	}
}
func TestInvalidPortAndStoppedManager(t *testing.T) {
	manager := New(testDialer{})
	manager.Close()
	for _, port := range []int{-1, 0, 65536} {
		if _, err := manager.Create("127.0.0.1", port, 0); err == nil {
			t.Fatal("invalid port accepted")
		}
	}
	if _, err := manager.Create("127.0.0.1", 80, 0); err == nil {
		t.Fatal("stopped manager accepted a forward")
	}
}
