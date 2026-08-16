// Command server is the Where to Bird Today HTTP server.
//
// This file does wiring and lifecycle only: load config, build dependencies,
// start listening, shut down cleanly. Business logic belongs in internal/.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/api"
	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// shutdownTimeout bounds how long in-flight requests get to finish once a
// termination signal arrives. Azure Container Apps sends SIGTERM and then
// kills the container, so this must stay comfortably under that grace period.
const shutdownTimeout = 10 * time.Second

// main is deliberately tiny. All real work happens in run, which returns an
// error instead of exiting — the "os.Exit only in main" idiom.
//
// The reason is defer: os.Exit terminates the process immediately and does NOT
// run deferred functions. Any code that both defers cleanup and exits directly
// silently skips that cleanup. Keeping os.Exit in main, where nothing is
// deferred, makes that impossible to get wrong.
func main() {
	if err := run(); err != nil {
		slog.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Dependencies are constructed here and passed down explicitly. There is
	// no container, no service registration, and no lifetime scopes — the
	// wiring graph is just this function, readable top to bottom.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	// NotifyContext returns a context that is cancelled when one of these
	// signals arrives — turning "a signal happened" into ordinary context
	// cancellation that the rest of the code already knows how to handle.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	// Deferred here rather than in store.New: whoever opens the pool decides
	// when it closes, and that is this function's whole lifetime.
	defer pool.Close()

	logger.Info("database connected")

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: api.NewRouter(logger, pool),

		// The zero value of http.Server has NO timeouts. A client that opens
		// a connection and never sends anything would occupy a goroutine
		// forever — the classic Slowloris exposure. Kestrel ships with
		// defaults for this; net/http does not, and it is on you to set them.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ListenAndServe blocks, so it runs in a goroutine while main waits on
	// either it failing or a shutdown signal.
	//
	// The channel is buffered (capacity 1) on purpose: if nobody is left
	// receiving, an unbuffered send would block forever and leak the
	// goroutine. With a buffer of one, the send always completes.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", slog.String("addr", srv.Addr))
		// Shutdown makes ListenAndServe return ErrServerClosed. That is the
		// expected, successful path — not a failure — so it is filtered out.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("listening on %s: %w", srv.Addr, err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// A fresh context here, not ctx: ctx is already cancelled, and passing a
	// cancelled context to Shutdown would abort instantly, dropping in-flight
	// requests — exactly what graceful shutdown is meant to prevent.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}
