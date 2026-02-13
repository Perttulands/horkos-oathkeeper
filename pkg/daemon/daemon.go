// Package daemon provides the Oathkeeper daemon lifecycle: signal handling,
// graceful shutdown, and foreground execution suitable for systemd.
package daemon

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrShutdownTimeout is returned when OnStart does not return within the
// configured shutdown timeout after the context is cancelled.
var ErrShutdownTimeout = errors.New("shutdown timed out")

// Config holds daemon configuration.
type Config struct {
	// ShutdownTimeout is the maximum time to wait for OnStart to return
	// after a shutdown signal. Zero means use DefaultConfig value.
	ShutdownTimeout time.Duration

	// OnStart is called when the daemon starts. It receives a context that
	// is cancelled on shutdown. OnStart should block until work is done or
	// ctx is cancelled. If nil, the daemon blocks until shutdown.
	OnStart func(ctx context.Context) error

	// OnStop is called after OnStart returns (or after shutdown timeout).
	// Used for cleanup. If nil, no cleanup is performed.
	OnStop func()
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ShutdownTimeout: 10 * time.Second,
	}
}

// Daemon manages the lifecycle of the Oathkeeper process.
type Daemon struct {
	cfg     Config
	cancel  context.CancelFunc
	once    sync.Once
	running atomic.Bool
}

// New creates a new Daemon with the given config.
func New(cfg Config) *Daemon {
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = DefaultConfig().ShutdownTimeout
	}
	return &Daemon{cfg: cfg}
}

// Run starts the daemon, blocks until shutdown, and returns any error from
// OnStart. It listens for SIGINT and SIGTERM to trigger graceful shutdown.
func (d *Daemon) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.running.Store(true)

	// Listen for OS signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	defer func() {
		cancel()
		d.running.Store(false)
		if d.cfg.OnStop != nil {
			d.cfg.OnStop()
		}
	}()

	if d.cfg.OnStart == nil {
		// No work function — just wait for shutdown
		<-ctx.Done()
		return nil
	}

	// Run OnStart in a goroutine so we can enforce the shutdown timeout
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.cfg.OnStart(ctx)
	}()

	// Wait for OnStart to finish or context cancellation (shutdown requested)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// Context cancelled — wait for OnStart to finish with timeout
	select {
	case err := <-errCh:
		return err
	case <-time.After(d.cfg.ShutdownTimeout):
		return ErrShutdownTimeout
	}
}

// Shutdown triggers a graceful shutdown. Safe to call multiple times.
func (d *Daemon) Shutdown() {
	d.once.Do(func() {
		if d.cancel != nil {
			d.cancel()
		}
	})
}

// IsRunning returns true if the daemon is currently running.
func (d *Daemon) IsRunning() bool {
	return d.running.Load()
}
