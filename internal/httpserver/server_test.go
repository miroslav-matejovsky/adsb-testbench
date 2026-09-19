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
	"time"

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

func testConfig() httpserver.Config {
	return httpserver.Config{
		ReadHeaderTimeout: time.Minute,
		ReadTimeout:       time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       time.Minute,
		ShutdownTimeout:   time.Minute,
		MaxHeaderBytes:    1 << 16,
	}
}

func TestServerServesAndShutsDown(t *testing.T) {
	t.Parallel()

	listener := newCloseListener(t)
	server, err := httpserver.Serve(testConfig(), slog.New(slog.DiscardHandler), listener, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ok")
	}))
	require.NoError(t, err)
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
		done <- httpserver.Run(ctx, testConfig(), slog.New(slog.DiscardHandler), listener, handler, work)
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
		err := httpserver.Run(t.Context(), testConfig(), slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), func(ctx context.Context) error {
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
		err := httpserver.Run(t.Context(), testConfig(), slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), func(context.Context) error {
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
	require.NoError(t, httpserver.Run(ctx, testConfig(), slog.New(slog.DiscardHandler), listener, http.NotFoundHandler(), nil))
	<-listener.closed
}

func TestConfigValidateRequiresEverySetting(t *testing.T) {
	t.Parallel()

	require.NoError(t, testConfig().Validate())
	cases := map[string]func(*httpserver.Config){
		"ReadHeaderTimeout": func(c *httpserver.Config) { c.ReadHeaderTimeout = 0 },
		"ReadTimeout":       func(c *httpserver.Config) { c.ReadTimeout = -time.Second },
		"WriteTimeout":      func(c *httpserver.Config) { c.WriteTimeout = 0 },
		"IdleTimeout":       func(c *httpserver.Config) { c.IdleTimeout = 0 },
		"ShutdownTimeout":   func(c *httpserver.Config) { c.ShutdownTimeout = 0 },
		"MaxHeaderBytes":    func(c *httpserver.Config) { c.MaxHeaderBytes = 0 },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			config := testConfig()
			mutate(&config)
			err := config.Validate()
			require.ErrorIs(t, err, httpserver.ErrInvalidConfig)
			require.ErrorContains(t, err, field)
		})
	}
	require.ErrorIs(t, httpserver.Config{}.Validate(), httpserver.ErrInvalidConfig)
}

func TestServeRejectsInvalidInputsWithoutStarting(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	cases := map[string]func(*closeListener) error{
		"config": func(l *closeListener) error {
			_, err := httpserver.Serve(httpserver.Config{}, logger, l, http.NotFoundHandler())
			return err
		},
		"logger": func(l *closeListener) error {
			_, err := httpserver.Serve(testConfig(), nil, l, http.NotFoundHandler())
			return err
		},
		"listener": func(*closeListener) error {
			_, err := httpserver.Serve(testConfig(), logger, nil, http.NotFoundHandler())
			return err
		},
		"handler": func(l *closeListener) error {
			_, err := httpserver.Serve(testConfig(), logger, l, nil)
			return err
		},
		"run": func(l *closeListener) error {
			worked := false
			err := httpserver.Run(t.Context(), testConfig(), nil, l, http.NotFoundHandler(), func(context.Context) error {
				worked = true
				return nil
			})
			require.False(t, worked)
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			listener := newCloseListener(t)
			require.ErrorIs(t, call(listener), httpserver.ErrInvalidConfig)
			select {
			case <-listener.closed:
				t.Fatal("rejected input closed the caller-owned listener")
			default:
			}
			require.NoError(t, listener.Close())
		})
	}
}

func TestShutdownForceClosesAfterExpiredDrain(t *testing.T) {
	t.Parallel()

	listener := newCloseListener(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	server, err := httpserver.Serve(testConfig(), slog.New(slog.DiscardHandler), listener, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	require.NoError(t, err)
	clientErr := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if err != nil {
			clientErr <- err
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			err = response.Body.Close()
		}
		clientErr <- err
	}()
	<-entered
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	shutdownErr := server.Shutdown(expired)
	require.ErrorIs(t, shutdownErr, context.Canceled)
	require.Error(t, <-clientErr)
	close(release)
}

func TestRunClassifiesWorkTermination(t *testing.T) {
	t.Parallel()

	failure := errors.New("pacing failed")

	t.Run("work error before cancellation keeps cause", func(t *testing.T) {
		t.Parallel()

		err := httpserver.Run(t.Context(), testConfig(), slog.New(slog.DiscardHandler), newCloseListener(t), http.NotFoundHandler(), func(context.Context) error {
			return failure
		})
		require.ErrorIs(t, err, failure)
		require.ErrorIs(t, err, httpserver.ErrWorkStopped)
	})

	t.Run("successful work return before cancellation", func(t *testing.T) {
		t.Parallel()

		err := httpserver.Run(t.Context(), testConfig(), slog.New(slog.DiscardHandler), newCloseListener(t), http.NotFoundHandler(), func(context.Context) error {
			return nil
		})
		require.ErrorIs(t, err, httpserver.ErrWorkStopped)
	})

	t.Run("canceled work is clean", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		started := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- httpserver.Run(ctx, testConfig(), slog.New(slog.DiscardHandler), newCloseListener(t), http.NotFoundHandler(), func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return ctx.Err()
			})
		}()
		<-started
		cancel()
		require.NoError(t, <-done)
	})

	t.Run("work failure after cancellation keeps cause", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		started := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- httpserver.Run(ctx, testConfig(), slog.New(slog.DiscardHandler), newCloseListener(t), http.NotFoundHandler(), func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return failure
			})
		}()
		<-started
		cancel()
		err := <-done
		require.ErrorIs(t, err, failure)
		require.NotErrorIs(t, err, httpserver.ErrWorkStopped)
	})
}
