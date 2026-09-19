// Package httpserver supervises HTTP serving and background work.
//
// Config carries every server limit: header, read, write and idle timeouts,
// the graceful drain budget, and the maximum header size. All fields are
// required and positive; the package supplies no defaults.
//
// Serve validates the configuration and its logger, listener and handler
// before starting anything and returns (*Server, error). On success the
// server owns the caller-bound listener; on error the caller still owns it.
// Server.Shutdown drains in-flight requests until its context ends and then
// force-closes remaining connections.
//
// Run coordinates a server with optional background work. On cancellation or
// failure it drains HTTP requests first, then cancels and joins the work.
// Work must honor cancellation. Serve, drain, and work failures are joined with
// their original causes. Validation failures satisfy errors.Is with
// ErrInvalidConfig; work that returns before Run cancels it satisfies
// errors.Is with ErrWorkStopped. Neither function stores a context, registers
// signals, or exits the process.
package httpserver
