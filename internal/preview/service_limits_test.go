// ABOUTME: Verifies public service connection and request bounds at the socket boundary.
// ABOUTME: Keeps all traffic on a synthetic foreground fixture's loopback listener.
package preview

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRT006_8_ActiveHTTPConnectionsAreBounded(t *testing.T) {
	s := startTestService(t, NativeHost())
	addr := strings.TrimPrefix(s.origin, "http://")
	var connections []net.Conn
	t.Cleanup(func() {
		for _, conn := range connections {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	request := fmt.Sprintf("GET /_health HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	for range 128 {
		conn, err := net.DialTimeout("tcp4", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		if err = conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(conn, request); err != nil {
			t.Fatal(err)
		}
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.Copy(io.Discard, response.Body); err != nil {
			t.Fatal(err)
		}
		if err = response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			t.Fatal("admitted connection failed")
		}
	}
	extra, err := net.DialTimeout("tcp4", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	connections = append(connections, extra)
	if err = extra.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(extra, request); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		response, err := http.ReadResponse(bufio.NewReader(extra), nil)
		if err == nil {
			err = response.Body.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("129th connection was admitted while all slots were occupied: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = connections[0].Close(); err != nil {
		t.Fatal(err)
	}
	connections = connections[1:]
	if err = <-result; err != nil {
		t.Fatalf("released connection slot was not reused: %v", err)
	}
}
