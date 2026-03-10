package scheduler

import (
	"context"
	"errors"

	"github.com/wk-y/rama-swap/tracker"
)

// ErrModelLoading is returned by Lock when the requested backend is still
// starting up.  Callers should treat this as a transient condition and retry
// after a short delay (the HTTP handlers return 503 + Retry-After for this).
var ErrModelLoading = errors.New("model is loading")

type ModelScheduler interface {
	// Lock waits for the model to be ready.
	// Scheduler implementations should keep the model loaded until Unlock is called.
	// Lock does not imply access to the backend will be mutually exclusive.
	Lock(ctx context.Context, model string) (*backend, error)

	// Unlock must after a successful Lock call to signal that the backend is no longer in use.
	Unlock(*backend)

	GetTracker() *tracker.Tracker

	// GetDebugInfo returns a snapshot of the active backend's metrics, and the port it's on (0 if none).
	GetDebugInfo() (snap DebugSnapshot, port int)
}
