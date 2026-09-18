package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	// ShutdownTimeout is the graceful HTTP drain budget.
	ShutdownTimeout = 5 * time.Second
)

// Server serves one caller-bound listener in a background goroutine.
type Server struct {
	server *http.Server
	done   chan struct{}
	err    error
}

// Serve starts handler on listener and returns immediately. The server owns
// listener. The logger must be non-nil.
func Serve(logger *slog.Logger, listener net.Listener, handler http.Handler) *Server {
	server := &Server{
		server: &http.Server{
			Handler: handler, ReadHeaderTimeout: readHeaderTimeout,
			ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		done: make(chan struct{}),
	}
	go func() {
		server.err = server.server.Serve(listener)
		close(server.done)
	}()
	return server
}

// Done closes when serving ends through shutdown or failure.
func (s *Server) Done() <-chan struct{} { return s.done }

// Shutdown drains in-flight requests until ctx ends, force-closes remaining
// connections, joins the serving goroutine, and reports serving failures.
func (s *Server) Shutdown(ctx context.Context) error {
	var result error
	if err := s.server.Shutdown(ctx); err != nil {
		result = errors.Join(fmt.Errorf("shutdown HTTP server: %w", err), s.server.Close())
	}
	<-s.done
	if !errors.Is(s.err, http.ErrServerClosed) {
		result = errors.Join(result, fmt.Errorf("serve HTTP: %w", s.err))
	}
	return result
}

// Run supervises HTTP serving and optional background work. Cancellation,
// serving termination, or work termination begins HTTP drain while work keeps
// running. Work is canceled and joined only after draining completes.
func Run(ctx context.Context, logger *slog.Logger, listener net.Listener, handler http.Handler, work func(context.Context) error) error {
	server := Serve(logger, listener, handler)
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	defer stopWork()

	var workDone chan struct{}
	var workErr error
	if work != nil {
		workDone = make(chan struct{})
		go func() {
			workErr = work(workCtx)
			close(workDone)
		}()
	}

	select {
	case <-ctx.Done():
	case <-server.Done():
	case <-workDone:
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancelShutdown()
	serverErr := server.Shutdown(shutdownCtx)
	stopWork()
	if workDone != nil {
		<-workDone
	}
	return errors.Join(serverErr, workErr)
}
