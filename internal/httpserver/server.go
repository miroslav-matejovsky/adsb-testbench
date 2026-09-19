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

var (
	// ErrInvalidConfig identifies a missing dependency or setting.
	ErrInvalidConfig = errors.New("invalid HTTP server configuration")
	// ErrWorkStopped identifies background work that returned before Run
	// canceled it, which is never an expected shutdown.
	ErrWorkStopped = errors.New("background work stopped before cancellation")
)

// Config holds every HTTP server limit. All fields are required and positive;
// the package supplies no defaults.
type Config struct {
	// ReadHeaderTimeout bounds reading request headers.
	ReadHeaderTimeout time.Duration
	// ReadTimeout bounds reading a whole request, including its body.
	ReadTimeout time.Duration
	// WriteTimeout bounds writing a response, measured from the end of the
	// request header read. It must exceed every handler deadline.
	WriteTimeout time.Duration
	// IdleTimeout bounds waiting for the next request on a keep-alive
	// connection.
	IdleTimeout time.Duration
	// ShutdownTimeout is the graceful drain budget Run grants in-flight
	// requests before force-closing connections.
	ShutdownTimeout time.Duration
	// MaxHeaderBytes bounds request header size in bytes.
	MaxHeaderBytes int
}

// Validate reports every non-positive setting.
func (c Config) Validate() error {
	var errs []error
	for _, field := range []struct {
		name  string
		value time.Duration
	}{
		{"ReadHeaderTimeout", c.ReadHeaderTimeout},
		{"ReadTimeout", c.ReadTimeout},
		{"WriteTimeout", c.WriteTimeout},
		{"IdleTimeout", c.IdleTimeout},
		{"ShutdownTimeout", c.ShutdownTimeout},
	} {
		if field.value <= 0 {
			errs = append(errs, fmt.Errorf("%w: %s must be positive, got %s", ErrInvalidConfig, field.name, field.value))
		}
	}
	if c.MaxHeaderBytes <= 0 {
		errs = append(errs, fmt.Errorf("%w: MaxHeaderBytes must be positive, got %d", ErrInvalidConfig, c.MaxHeaderBytes))
	}
	return errors.Join(errs...)
}

// Server serves one caller-bound listener in a background goroutine.
type Server struct {
	server *http.Server
	done   chan struct{}
	err    error
}

// Serve validates its inputs, starts handler on listener, and returns
// immediately. On success the server owns listener and closes it during
// Shutdown. On error nothing is started and the caller still owns listener.
func Serve(config Config, logger *slog.Logger, listener net.Listener, handler http.Handler) (*Server, error) {
	if err := validateInputs(config, logger, listener, handler); err != nil {
		return nil, err
	}
	server := &Server{
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: config.ReadHeaderTimeout,
			ReadTimeout:       config.ReadTimeout,
			WriteTimeout:      config.WriteTimeout,
			IdleTimeout:       config.IdleTimeout,
			MaxHeaderBytes:    config.MaxHeaderBytes,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		done: make(chan struct{}),
	}
	go func() {
		server.err = server.server.Serve(listener)
		close(server.done)
	}()
	return server, nil
}

func validateInputs(config Config, logger *slog.Logger, listener net.Listener, handler http.Handler) error {
	errs := []error{config.Validate()}
	if logger == nil {
		errs = append(errs, fmt.Errorf("%w: logger is nil", ErrInvalidConfig))
	}
	if listener == nil {
		errs = append(errs, fmt.Errorf("%w: listener is nil", ErrInvalidConfig))
	}
	if handler == nil {
		errs = append(errs, fmt.Errorf("%w: handler is nil", ErrInvalidConfig))
	}
	return errors.Join(errs...)
}

// Done closes when serving ends through shutdown or failure.
func (s *Server) Done() <-chan struct{} { return s.done }

// Shutdown drains in-flight requests until ctx ends, then force-closes
// remaining connections, joins the serving goroutine, and reports serving
// failures. A drain cut short by ctx returns an error wrapping ctx's error.
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

// Run supervises HTTP serving and optional background work until ctx is
// canceled or either of them ends.
//
// Shutdown order is fixed: HTTP requests drain for config.ShutdownTimeout
// while work keeps running, remaining connections are then force-closed, and
// only afterwards is work canceled and joined. Work must return once its
// context is canceled. Inputs are validated before anything starts; on a
// validation error the caller still owns listener.
//
// Cancellation of ctx is a clean stop and returns nil when nothing failed.
// Serving failures, drain failures, work errors, and work that returns before
// Run cancels it (ErrWorkStopped) are joined with their original causes. Work
// returning context.Canceled after Run canceled it is a clean stop.
func Run(ctx context.Context, config Config, logger *slog.Logger, listener net.Listener, handler http.Handler, work func(context.Context) error) error {
	server, err := Serve(config, logger, listener, handler)
	if err != nil {
		return err
	}
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

	workEndedFirst := false
	select {
	case <-ctx.Done():
	case <-server.Done():
	case <-workDone:
		workEndedFirst = true
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), config.ShutdownTimeout)
	defer cancelShutdown()
	serverErr := server.Shutdown(shutdownCtx)
	stopWork()
	if workDone != nil {
		<-workDone
	}
	return errors.Join(serverErr, classifyWork(workErr, workEndedFirst))
}

// classifyWork turns the work result into Run's error contribution.
func classifyWork(err error, endedFirst bool) error {
	switch {
	case endedFirst && err == nil:
		return ErrWorkStopped
	case endedFirst:
		return fmt.Errorf("%w: %w", ErrWorkStopped, err)
	case errors.Is(err, context.Canceled):
		return nil
	case err != nil:
		return fmt.Errorf("background work: %w", err)
	}
	return nil
}
