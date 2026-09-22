package safegoroutine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yourusername/astra-backend/internal/commons/safegoroutine"
)

// Test: SafeGo does not crash the process on panic
func TestSafeGo_PanicDoesNotCrash(t *testing.T) {
	done := make(chan struct{})

	safegoroutine.SafeGo("test-panic", func() {
		defer close(done)
		panic("intentional test panic")
	})

	select {
	case <-done:
		// goroutine exited cleanly (panic recovered, defers ran)
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine never exited — possible deadlock")
	}
}

// Test: SafeGo runs the function normally when no panic occurs
func TestSafeGo_NormalExecution(t *testing.T) {
	var called bool
	var mu sync.Mutex
	done := make(chan struct{})

	safegoroutine.SafeGo("test-normal", func() {
		defer close(done)
		mu.Lock()
		called = true
		mu.Unlock()
	})

	<-done
	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("expected function to be called")
	}
}

// Test: SafeGoWithContext skips execution when ctx is already cancelled
func TestSafeGoWithContext_CancelledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	var called bool
	var mu sync.Mutex

	safegoroutine.SafeGoWithContext(ctx, "test-cancelled", func(ctx context.Context) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// Give goroutine time to potentially (wrongly) execute
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Fatal("expected function NOT to be called on cancelled context")
	}
}

// Test: SafeGoWithContext still recovers panics
func TestSafeGoWithContext_PanicRecovered(t *testing.T) {
	ctx := context.Background()
	done := make(chan struct{})

	safegoroutine.SafeGoWithContext(ctx, "ctx-panic-test", func(ctx context.Context) {
		defer close(done)
		panic("ctx goroutine panic")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine never exited")
	}
}

// Test: SafeGo preserves defer chain inside the wrapped function (critical for wg.Done())
func TestSafeGo_DefersStillRun(t *testing.T) {
	var wg sync.WaitGroup
	deferRan := false
	var mu sync.Mutex

	wg.Add(1)
	safegoroutine.SafeGo("test-defer-chain", func() {
		defer wg.Done()
		defer func() {
			mu.Lock()
			deferRan = true
			mu.Unlock()
		}()
		panic("panic after defers registered")
	})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !deferRan {
		t.Fatal("inner defers did not run — this would break WaitGroup-based goroutines")
	}
}
