// ABOUTME: Verifies public service connection and request bounds at the socket boundary.
// ABOUTME: Keeps all traffic on a synthetic foreground fixture's loopback listener.
package preview

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

func TestRT006_8_CapabilityCapacityRetainsExistingGrants(t *testing.T) {
	s := startTestService(t, NativeHost())
	file := source(t, s.root, "entry.html", "<h1>Capacity fixture</h1>")
	var first string
	for i := range 129 {
		data, err := json.Marshal(map[string]any{"paths": []string{file}, "format_contract": 1, "settings": map[string]any{"deadline": fmt.Sprintf("%dms", 60000-i)}})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), "POST", "http://control/v1/previews", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		data, code, err := controlResponse(s.control, req)
		var result registrationResponse
		if err != nil || code != 200 || json.Unmarshal(data, &result) != nil || len(result.Results) != 1 {
			t.Fatalf("capacity registration: status=%d err=%v", code, err)
		}
		if i < 128 && (result.Results[0].URL == "" || result.Results[0].Error != "") {
			t.Fatalf("capability %d refused within limit", i+1)
		}
		if i == 0 {
			first = result.Results[0].URL
		}
		if i == 128 && (result.Results[0].URL != "" || result.Results[0].Error != "capacity_exceeded") {
			t.Fatal("129th context did not report capability capacity")
		}
	}
	code, _, body := responseAsset(t, s, "GET", first)
	if code != 200 || !bytes.Contains(body, []byte("Capacity fixture")) {
		t.Fatal("capacity exhaustion revoked an existing grant")
	}
}

func TestRT006_8_CancelledLastWaiterReleasesWorker(t *testing.T) {
	host := NativeHost()
	actual := host.Execute
	entered := make(chan struct{}, 2)
	reaped := make(chan struct{}, 2)
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && strings.HasPrefix(cmd.Args[0], "--defaults=") {
			entered <- struct{}{}
			<-ctx.Done()
			reaped <- struct{}{}
			return nil, ctx.Err()
		}
		return actual(ctx, cmd)
	}
	s := startTestService(t, host)
	url := s.register(t, "cancel.md", "# Cancelled source")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		resp, err := s.client.Do(req)
		if err == nil {
			err = resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("converter did not enter")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled request unexpectedly succeeded")
	}
	select {
	case <-reaped:
	case <-time.After(3 * time.Second):
		t.Fatal("last disconnected waiter did not cancel converter")
	}
	// A fresh, independent native representation must still be serviceable.
	url = s.register(t, "after.html", "<h1>After cancellation</h1>")
	code, _, body := responseAsset(t, s, "GET", url)
	if code != 200 || !bytes.Contains(body, []byte("After cancellation")) {
		t.Fatal("cancelled work prevented subsequent requests")
	}
}
