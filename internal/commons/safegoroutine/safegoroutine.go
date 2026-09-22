package safegoroutine

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/yourusername/astra-backend/internal/commons/logger"
)

// SafeGo launches f in a goroutine with panic recovery.
// A recovered panic is logged as an error with a stack trace;
// it does NOT propagate — the server stays alive.
//
// Use this everywhere instead of bare `go func() { ... }()`.
func SafeGo(name string, f func()) {
	go func() {
		defer recoverPanic(name)
		f()
	}()
}

// SafeGoWithContext launches f in a goroutine. If ctx is already cancelled
// before f is called, the goroutine exits immediately without running f.
// Panics inside f are still recovered.
func SafeGoWithContext(ctx context.Context, name string, f func(ctx context.Context)) {
	go func() {
		defer recoverPanic(name)
		select {
		case <-ctx.Done():
			return
		default:
			f(ctx)
		}
	}()
}

// recoverPanic is the shared deferred recovery logic.
// It logs the panic value and full stack trace via the existing logger.
func recoverPanic(name string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		logger.Error("[PANIC RECOVERED] goroutine=%s panic=%v\n%s", name, fmt.Sprintf("%v", r), string(stack))
	}
}
