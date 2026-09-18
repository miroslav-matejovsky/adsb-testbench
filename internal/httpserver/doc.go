// Package httpserver supervises HTTP serving and background work.
//
// Serve applies shared server settings and exposes explicit graceful shutdown.
// Run coordinates a server with optional background work. On cancellation or
// failure it drains HTTP requests first, then cancels and joins the work. Serve,
// shutdown, and work failures are returned with their original causes.
package httpserver
