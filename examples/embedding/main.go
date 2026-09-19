package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run serves the example host on an explicit address until interrupted.
// Shutdown drains HTTP first and then stops the benches.
func run(args []string) error {
	flags := flag.NewFlagSet("embedding", flag.ContinueOnError)
	listen := flags.String("listen", "", "host:port to serve on (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *listen == "" {
		return errors.New("-listen is required")
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	host, err := newHost(logger, "embedding-a-"+rand.Text(), "embedding-b-"+rand.Text())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: host.handler, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	benchCtx, stopBenches := context.WithCancel(context.WithoutCancel(ctx))
	benches := make(chan error, 1)
	go func() { benches <- host.run(benchCtx) }()
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	logger.Info("serving", "address", listener.Addr().String())

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-served:
	}
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(drain)
	stopBenches()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr, <-benches)
}
