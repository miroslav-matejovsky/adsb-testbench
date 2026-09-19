package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body)
}

func TestHostServesTwoIndependentBenches(t *testing.T) {
	host, err := newHost(slog.New(slog.DiscardHandler), "test-a", "test-b")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(host.handler)
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- host.run(ctx) }()

	for _, check := range []struct{ path, want string }{
		{"/bench/a/manager/", `"apiBaseUrl":"/bench/a/api/simulator/"`},
		{"/bench/b/aircraft/", `"apiBaseUrl":"/bench/b/api/display/"`},
		{"/bench/a/api/simulator/metadata", `"runId":"test-a"`},
		{"/bench/b/api/simulator/metadata", `"runId":"test-b"`},
		{"/bench/a/assets/components.js", "mountAircraftDisplay"},
		{"/custom/", "mountManager"},
	} {
		status, body := get(t, server.Client(), server.URL+check.path)
		if status != http.StatusOK || !strings.Contains(body, check.want) {
			t.Fatalf("GET %s = %d, want 200 containing %q", check.path, status, check.want)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("clean cancellation returned %v", err)
	}
}

func TestHostRejectsEmptyRunIdentity(t *testing.T) {
	if _, err := newHost(slog.New(slog.DiscardHandler), "", "b"); err == nil {
		t.Fatal("an empty run identity was accepted")
	}
}

func TestRunRequiresAnExplicitAddress(t *testing.T) {
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "-listen is required") {
		t.Fatalf("run without -listen = %v", err)
	}
}
