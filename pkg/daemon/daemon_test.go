package daemon

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNewDaemon(t *testing.T) {
	d := New(Config{})
	if d == nil {
		t.Fatal("New returned nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("expected 10s shutdown timeout, got %v", cfg.ShutdownTimeout)
	}
}

func TestDaemonRunAndShutdown(t *testing.T) {
	var started atomic.Bool
	var stopped atomic.Bool

	d := New(Config{
		ShutdownTimeout: 2 * time.Second,
		OnStart: func(ctx context.Context) error {
			started.Store(true)
			<-ctx.Done()
			return nil
		},
		OnStop: func() {
			stopped.Store(true)
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for daemon to start
	time.Sleep(50 * time.Millisecond)
	if !started.Load() {
		t.Fatal("OnStart was not called")
	}

	// Trigger shutdown
	d.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}

	if !stopped.Load() {
		t.Error("OnStop was not called")
	}
}

func TestDaemonSignalHandling(t *testing.T) {
	var started atomic.Bool

	d := New(Config{
		ShutdownTimeout: 2 * time.Second,
		OnStart: func(ctx context.Context) error {
			started.Store(true)
			<-ctx.Done()
			return nil
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for start
	time.Sleep(50 * time.Millisecond)
	if !started.Load() {
		t.Fatal("OnStart was not called")
	}

	// Send SIGTERM to self
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after SIGTERM")
	}
}

func TestDaemonSIGINTHandling(t *testing.T) {
	var started atomic.Bool

	d := New(Config{
		ShutdownTimeout: 2 * time.Second,
		OnStart: func(ctx context.Context) error {
			started.Store(true)
			<-ctx.Done()
			return nil
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	if !started.Load() {
		t.Fatal("OnStart was not called")
	}

	// Send SIGINT
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after SIGINT")
	}
}

func TestDaemonOnStartError(t *testing.T) {
	d := New(Config{
		OnStart: func(ctx context.Context) error {
			return context.Canceled
		},
	})

	err := d.Run()
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDaemonShutdownTimeout(t *testing.T) {
	d := New(Config{
		ShutdownTimeout: 100 * time.Millisecond,
		OnStart: func(ctx context.Context) error {
			<-ctx.Done()
			// Simulate slow shutdown — sleep longer than timeout
			time.Sleep(500 * time.Millisecond)
			return nil
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	d.Shutdown()

	select {
	case err := <-errCh:
		if err != ErrShutdownTimeout {
			t.Fatalf("expected ErrShutdownTimeout, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after shutdown timeout")
	}
}

func TestDaemonDoubleShutdown(t *testing.T) {
	d := New(Config{
		ShutdownTimeout: 2 * time.Second,
		OnStart: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	time.Sleep(50 * time.Millisecond)

	// Double shutdown should not panic
	d.Shutdown()
	d.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestDaemonNoCallbacks(t *testing.T) {
	d := New(Config{
		ShutdownTimeout: 1 * time.Second,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	d.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestDaemonLongRunNoFalseTimeout(t *testing.T) {
	// Verify daemon doesn't hit shutdown timeout during normal operation.
	// ShutdownTimeout is very short, but daemon should run until explicitly stopped.
	d := New(Config{
		ShutdownTimeout: 50 * time.Millisecond,
		OnStart: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Run for longer than ShutdownTimeout
	time.Sleep(150 * time.Millisecond)

	d.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestDaemonIsRunning(t *testing.T) {
	d := New(Config{
		ShutdownTimeout: 2 * time.Second,
		OnStart: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	})

	if d.IsRunning() {
		t.Error("daemon should not be running before Run()")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	if !d.IsRunning() {
		t.Error("daemon should be running after Run()")
	}

	d.Shutdown()
	<-errCh

	if d.IsRunning() {
		t.Error("daemon should not be running after shutdown")
	}
}
