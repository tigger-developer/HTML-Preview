// ABOUTME: Guards the annotation request budget through the HTTP interface.
// ABOUTME: A slow converter must not trip the former five-second deadline.
package preview

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// W009: slow annotation requests retain a fifteen-second response budget.
func TestAnnotationReadAllowsSlowConversion(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	var delayed atomic.Bool
	host.Execute = func(ctx context.Context, command Command) ([]byte, error) {
		if len(command.Input) > 0 && delayed.CompareAndSwap(false, true) {
			timer := time.NewTimer(6 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return execute(ctx, command)
	}
	s := startTestService(t, host)
	s.client.Timeout = 20 * time.Second
	path := source(t, s.root, "slow.org", "* Heading\nA paragraph for annotation.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	status, body := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || body["writable"] != true || !delayed.Load() {
		t.Fatalf("slow annotation read: want writable HTTP 200; got %d, %v", status, body)
	}
}
