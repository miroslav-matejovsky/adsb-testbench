package httpserver_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/httpserver"
	"github.com/stretchr/testify/require"
)

type closeListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func newCloseListener(t testing.TB) *closeListener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return &closeListener{Listener: listener, closed: make(chan struct{})}
}

func (l *closeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

type failingListener struct {
	closed chan struct{}
}

func (l *failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (l *failingListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
func (l *failingListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestServerServesAndShutsDown(t *testing.T) {
	t.Parallel()

	listener := newCloseListener(t)
	server := httpserver.Serve(slog.New(slog.DiscardHandler), listener, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ok")
	}))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "ok", string(body))
	require.NoError(t, server.Shutdown(t.Context()))
	<-server.Done()
	<-listener.closed
}

func TestRunDrainsHTTPBeforeStoppingWork(t *testing.T) {
	t.Parallel()

	listener := newCloseListener(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	workStopped := make(chan struct{})
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		select {
		case <-workStopped:
			_, _ = io.WriteString(writer, "stopped")
		default:
			_, _ = io.WriteString(writer, "running")
		}
	})
	work := func(ctx context.Context) error {
		<-ctx.Done()
		close(workStopped)
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- httpserver.Run(ctx, slog.New(slog.DiscardHandler), listener, handler, work)
	}()
	body := make(chan string, 1)
	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if err != nil {
			body <- err.Error()
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			body <- err.Error()
			return
		}
		data, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if joined := errors.Join(readErr, closeErr); joined != nil {
			body <- joined.Error()
			return
		}
		body <- string(data)
	}()
	<-entered
	cancel()
	<-listener.closed
	close(release)
	require.Equal(t, "running", <-body)
	require.NoError(t, <-done)
	<-workStopped
}

func TestRunPropagatesServingAndWorkFailures(t *testing.T) {
	t.Parallel()

	t.Run("serve", func(t *testing.T) {
		listener := &failingListener{closed: make(chan struct{})}
		workStopped := make(chan struct{})
		err := httpserver.Run(t.Context(), slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), func(ctx context.Context) error {
			<-ctx.Done()
			close(workStopped)
			return nil
		})
		require.ErrorContains(t, err, "accept failed")
		<-listener.closed
		<-workStopped
	})

	t.Run("work", func(t *testing.T) {
		listener := newCloseListener(t)
		err := httpserver.Run(t.Context(), slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), func(context.Context) error {
			return errors.New("pacing failed")
		})
		require.ErrorContains(t, err, "pacing failed")
		<-listener.closed
	})
}

func TestRunWithoutWorkStopsOnCancellation(t *testing.T) {
	t.Parallel()

	listener := newCloseListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, httpserver.Run(ctx, slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), nil))
	<-listener.closed
}
