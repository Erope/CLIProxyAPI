package auth

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type throttleTestExecutor struct {
	id    string
	mu    sync.Mutex
	calls []time.Time
}

func (f *throttleTestExecutor) Identifier() string {
	return f.id
}

func (f *throttleTestExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, errors.New("execute not implemented in test executor")
}

func (f *throttleTestExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, errors.New("stream not implemented in test executor")
}

func (f *throttleTestExecutor) Refresh(ctx context.Context, auth *Auth) (*Auth, error) {
	f.mu.Lock()
	f.calls = append(f.calls, time.Now())
	f.mu.Unlock()
	return auth, ctx.Err()
}

func (f *throttleTestExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (f *throttleTestExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("http not implemented in test executor")
}

func TestManager_RefreshRateLimitThrottlesStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := &throttleTestExecutor{id: "throttle"}

	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(exec)

	auths := []*Auth{
		{
			ID:       "auth-1",
			Provider: exec.Identifier(),
			Metadata: map[string]any{"refresh_interval_seconds": 1},
		},
		{
			ID:       "auth-2",
			Provider: exec.Identifier(),
			Metadata: map[string]any{"refresh_interval_seconds": 1},
		},
	}

	for _, a := range auths {
		if _, err := m.Register(ctx, a); err != nil {
			t.Fatalf("register auth: %v", err)
		}
	}

	m.SetRefreshRateLimit(600) // roughly one refresh start every ~100ms

	m.checkRefreshes(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for {
		exec.mu.Lock()
		count := len(exec.calls)
		exec.mu.Unlock()
		if count >= len(auths) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	exec.mu.Lock()
	calls := append([]time.Time(nil), exec.calls...)
	exec.mu.Unlock()

	if len(calls) != len(auths) {
		t.Fatalf("expected %d refresh calls, got %d", len(auths), len(calls))
	}

	spacing := calls[1].Sub(calls[0])
	if spacing < 80*time.Millisecond {
		t.Fatalf("expected throttled refresh spacing, got %v", spacing)
	}
}
