package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wk-y/rama-swap/ramalama"
)

// fcfsScheduler is a ModelScheduler that implements (roughly) first-come-first-serve
// access with at most one model loaded at a time.
type fcfsScheduler struct {
	port        int // port to attach the backend to
	idleTimeout time.Duration

	lock     sync.Mutex
	ramalama ramalama.Ramalama

	// rules for using the backend properties:
	// backendCond must be held while changing any of the backend properties
	// backend may only be changed when backendUsers is 0
	backendCond    sync.Cond
	backend        *backend
	backendModel   string
	backendUsers   int
	backendIdleAt  time.Time
	backendLocking bool

	// cached set of valid model names
	ramalamaModelsCache     map[string]struct{}
	ramalamaModelsCacheLock sync.Mutex
}

// Lock implements ModelScheduler.
func (f *fcfsScheduler) Lock(ctx context.Context, model string) (*backend, error) {
	exists, err := f.modelExists(model)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, errors.New("nonexistent model")
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	select {
	case <-ctx.Done():
		return nil, errors.New("context cancelled")
	default:
	}

	f.backendCond.L.Lock()
	defer f.backendCond.L.Unlock()

	if f.backend != nil && f.backendModel == model {
		// if it is exited, don't return the backend
		select {
		case <-f.backend.Exited:
		default:
			f.backendUsers++
			f.backendCond.Broadcast()
			return f.backend, nil
		}
	}

	for f.backendUsers > 0 {
		f.backendCond.Wait()
	}

	if f.backend != nil {
		f.backend.cancel()
		<-f.backend.Exited
		f.backend = nil
	}

	backend, err := f.startBackend(model)
	if err != nil {
		return nil, err
	}
	f.backend = backend
	f.backendModel = model

	select {
	case <-ctx.Done():
		return nil, errors.New("context cancelled")
	case <-backend.Ready:
		f.backendUsers++
		return f.backend, nil
	}
}

// Unlock implements ModelScheduler.
func (f *fcfsScheduler) Unlock(backend *backend) {
	f.backendCond.L.Lock()
	defer f.backendCond.L.Unlock()
	if f.backend == backend {
		f.backendUsers--
		if f.backendUsers == 0 {
			f.backendIdleAt = time.Now()
		}
		f.backendCond.Broadcast()
	}
}

func (f *fcfsScheduler) modelExists(modelName string) (bool, error) {
	f.ramalamaModelsCacheLock.Lock()
	defer f.ramalamaModelsCacheLock.Unlock()

	_, ok := f.ramalamaModelsCache[modelName]
	if ok {
		return true, nil
	}

	models, err := f.ramalama.GetModels()
	if err != nil {
		return false, err
	}

	f.ramalamaModelsCache = make(map[string]struct{}, len(models))
	for _, model := range models {
		f.ramalamaModelsCache[model.Name] = struct{}{}
	}

	_, ok = f.ramalamaModelsCache[modelName]
	return ok, nil
}

func (f *fcfsScheduler) startBackend(modelName string) (*backend, error) {
	back := &backend{}
	back.port = f.port

	ctx, cancel := context.WithCancel(context.Background())
	back.cancel = cancel

	cmd := f.ramalama.ServeCommand(ctx, ramalama.ServeArgs{
		Model: modelName,
		Port:  back.port,
		RpcNodes: []ramalama.RpcNode{
			{
				Host: "192.168.0.198:50052", // FIXME
			},
		},
	})
	cmd.Stderr = os.Stderr

	switch runtime.GOOS {
	case "linux":
		// By default, Go sends SIGKILL, which causes ramalama to exit without stopping the container.
		// Instead, let ramalama gracefully exit by sending SIGINT
		cmd.Cancel = func() error {
			return cmd.Process.Signal(os.Interrupt)
		}
	default:
		log.Println("[WARN] Graceful shutdown of ramalama not supported for OS, switching may not work correctly")
	}

	err := cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start ramalama: %v\n", err)
	}

	back.Ready = make(chan struct{})
	back.Exited = make(chan struct{})

	// waits for ready
	go func() {
		defer close(back.Ready)

		for !back.healthCheck() {
			select {
			case <-back.Exited:
				return
			default:
			}

			time.Sleep(time.Second) // fixme
		}
	}()

	// waits for exit
	go func() {
		err := cmd.Wait()
		back.cancel()

		back.Lock()
		back.err = err

		back.portLock.Lock()
		back.port = 0
		back.portLock.Unlock()

		close(back.Exited) // must be after portLock unlock

		back.Unlock()
	}()

	return back, nil
}

func (f *fcfsScheduler) startIdleTimeout() {
	f.backendCond.L.Lock()
	for {
		if f.backend == nil || f.backendUsers > 0 {
			f.backendCond.Wait()
			continue
		}

		if waitingTime := time.Until(f.backendIdleAt.Add(f.idleTimeout)); waitingTime > 0 {
			go func() {
				time.Sleep(waitingTime)
				f.backendCond.L.Lock()
				defer f.backendCond.L.Unlock()
				f.backendCond.Broadcast()
			}()
			f.backendCond.Wait()
			continue
		}

		log.Printf("Stopping backend after being idle for %v\n", f.idleTimeout)
		f.backend.cancel()
		<-f.backend.Exited
		f.backend = nil
	}
}

func NewFcfsScheduler(ramalama ramalama.Ramalama, port int, idleTimeout time.Duration) *fcfsScheduler {
	scheduler := &fcfsScheduler{
		ramalama:            ramalama,
		port:                port,
		idleTimeout:         idleTimeout,
		ramalamaModelsCache: map[string]struct{}{},
		backendCond:         *sync.NewCond(&sync.Mutex{}),
	}

	if idleTimeout != 0 {
		go scheduler.startIdleTimeout()
	}
	return scheduler
}

var _ ModelScheduler = (*fcfsScheduler)(nil)
