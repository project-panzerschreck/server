package scheduler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wk-y/rama-swap/llama"
	"github.com/wk-y/rama-swap/tracker"
)

// fcfsScheduler is a ModelScheduler that implements (roughly) first-come-first-serve
// access with at most one model loaded at a time.
type fcfsScheduler struct {
	tracker     *tracker.Tracker
	port        int // port to attach the backend to
	idleTimeout time.Duration

	lock     sync.Mutex
	ramalama llama.Llama

	// rules for using the backend properties:
	// backendCond must be held while changing any of the backend properties
	// backend may only be changed when backendUsers is 0
	backendCond     sync.Cond
	backend         *backend
	backendModel    string
	backendRpcNodes []string
	backendUsers    int
	backendIdleAt   time.Time
	backendLocking  bool

	// cached set of valid model names
	ramalamaModelsCache     map[string]struct{}
	ramalamaModelsCacheLock sync.Mutex
}

// decrementBackendUsers decrements backendUsers for the given backend if it is
// still the active one, broadcasts, and records the idle time.
func (f *fcfsScheduler) decrementBackendUsers(back *backend) {
	f.backendCond.L.Lock()
	defer f.backendCond.L.Unlock()
	if f.backend == back {
		f.backendUsers--
		if f.backendUsers == 0 {
			f.backendIdleAt = time.Now()
		}
	}
	f.backendCond.Broadcast()
}

// Lock implements ModelScheduler.
//
// It blocks until the requested backend is ready, then returns it with its
// user-count incremented.  Call Unlock when done.
//
// Returns ErrModelLoading if ctx is cancelled while the model is loading
// (e.g. the HTTP client disconnected) — the backend keeps loading in the
// background and the next call will either wait again or return immediately
// once ready.  Returns a plain error if the backend exits before becoming
// ready or if some other permanent failure occurs.
func (f *fcfsScheduler) Lock(ctx context.Context, model string) (*backend, error) {
	exists, err := f.modelExists(model)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("nonexistent model")
	}

	select {
	case <-ctx.Done():
		return nil, errors.New("context cancelled")
	default:
	}

	// --- Critical section: find or start the backend, claim a user slot. ---
	// f.lock serialises concurrent attempts to start/replace the backend.
	// f.backendCond.L protects all backend fields.
	// Both are released before the potentially long wait for Ready below.
	f.lock.Lock()
	f.backendCond.L.Lock()

	servers := f.tracker.GetServers()
	if len(servers) == 0 {
		f.backendCond.L.Unlock()
		f.lock.Unlock()
		return nil, errors.New("no servers")
	}

	var back *backend

	// Reuse the existing backend if it matches and hasn't exited.
	// GetServers is sorted so direct slice comparison is valid.
	if f.backend != nil && f.backendModel == model && slices.Equal(f.backendRpcNodes, servers) {
		select {
		case <-f.backend.Exited:
			// Exited — fall through to restart below.
		default:
			// Still alive (may still be loading): claim a user slot so the
			// idle-timeout goroutine cannot evict it while we wait.
			back = f.backend
			f.backendUsers++
			f.backendCond.Broadcast()
		}
	}

	if back == nil {
		// Wait for any current users to drain before we replace the backend.
		for f.backendUsers > 0 {
			f.backendCond.Wait()
		}
		if f.backend != nil {
			f.backend.cancel()
			// <-f.backend.Exited is fast here: cancel() sent SIGINT and llama-server
			// is expected to exit within a few seconds.
			<-f.backend.Exited
			f.backend = nil
		}
		newBackend, startErr := f.startBackend(model, servers)
		if startErr != nil {
			f.backendCond.L.Unlock()
			f.lock.Unlock()
			return nil, startErr
		}
		f.backend = newBackend
		f.backendModel = model
		f.backendRpcNodes = servers
		back = newBackend
		f.backendUsers++
		f.backendCond.Broadcast()
		log.Printf("Backend started for model %s with %d RPC nodes", model, len(servers))
	}

	// Release both locks BEFORE the potentially very long wait for Ready
	// (~minutes over Wi-Fi).  Other requests can reach the reuse path above
	// and join the wait concurrently.
	f.backendCond.L.Unlock()
	f.lock.Unlock()

	// --- Wait for backend to become ready (no locks held) ---
	select {
	case <-ctx.Done():
		// HTTP client disconnected or request timed out.  The backend keeps
		// loading — the next request will come back and wait again.
		f.decrementBackendUsers(back)
		return nil, ErrModelLoading

	case <-back.Exited:
		// Backend process died before it became ready.
		f.decrementBackendUsers(back)
		return nil, fmt.Errorf("backend exited during startup: %v", back.Err())

	case <-back.Ready:
		// Double-check: Ready and Exited may close at the same instant.
		select {
		case <-back.Exited:
			f.decrementBackendUsers(back)
			return nil, fmt.Errorf("backend exited during startup: %v", back.Err())
		default:
			return back, nil
		}
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

// modelNameVariants returns the model name plus common prefix variants to check against the ramalama cache.
// This handles the case where a client sends "unsloth/foo" but ramalama stores "hf:unsloth/foo".
func modelNameVariants(name string) []string {
	const hfPrefix = "hf:"
	if strings.HasPrefix(name, hfPrefix) {
		// Client sent hf:..., also try without prefix
		return []string{name, strings.TrimPrefix(name, hfPrefix)}
	}
	// Client sent bare name, also try with hf: prefix
	return []string{name, hfPrefix + name}
}

func (f *fcfsScheduler) modelExists(modelName string) (bool, error) {
	f.ramalamaModelsCacheLock.Lock()
	defer f.ramalamaModelsCacheLock.Unlock()

	for _, v := range modelNameVariants(modelName) {
		if _, ok := f.ramalamaModelsCache[v]; ok {
			return true, nil
		}
	}

	models, err := f.ramalama.GetModels()
	if err != nil {
		return false, err
	}

	f.ramalamaModelsCache = make(map[string]struct{}, len(models))
	for _, model := range models {
		f.ramalamaModelsCache[model.Name] = struct{}{}
	}

	for _, v := range modelNameVariants(modelName) {
		if _, ok := f.ramalamaModelsCache[v]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (f *fcfsScheduler) waitForNodes(rpcNodes []string) ([]string, error) {
	var reachable []string
	for _, node := range rpcNodes {
		log.Printf("Checking connectivity to RPC node %s...\n", node)
		success := false
		for i := 0; i < 5; i++ {
			conn, err := net.DialTimeout("tcp", node, 2*time.Second)
			if err == nil {
				conn.Close()
				success = true
				break
			}
			log.Printf("Node %s not reachable yet, retrying... (%d/5)\n", node, i+1)
			time.Sleep(2 * time.Second)
		}
		if !success {
			log.Printf("node %s is not reachable after wait, dropping it", node)
		} else {
			reachable = append(reachable, node)
		}
	}

	if len(reachable) == 0 {
		return nil, fmt.Errorf("no RPC nodes are reachable")
	}

	return reachable, nil
}

func (f *fcfsScheduler) startBackend(modelName string, rpcNodes []string) (*backend, error) {
	// Filter to only nodes that are currently TCP-reachable before handing
	// them to llama-server.  A node that is in the tracker but whose RPC port
	// is not yet listening would cause llama-server to block indefinitely
	// inside ggml-rpc's socket_connect() / check_server_version().
	reachable, err := f.waitForNodes(rpcNodes)
	if err != nil {
		return nil, fmt.Errorf("no reachable RPC nodes: %w", err)
	}
	rpcNodes = reachable

	back := newBackend(modelName)
	back.port = f.port

	ctx, cancel := context.WithCancel(context.Background())
	back.cancel = cancel

	rpc := make([]llama.RpcNode, len(rpcNodes))
	for i, node := range rpcNodes {
		rpc[i] = llama.RpcNode{Host: node}
	}

	cmd := f.ramalama.ServeCommand(ctx, llama.ServeArgs{
		Model:    modelName,
		Port:     back.port,
		RpcNodes: rpc,
	})

	log.Printf("Starting ramalama with command: %v %v\n", cmd.Path, cmd.Args)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get stderr pipe: %v\n", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get stdout pipe: %v\n", err)
	}

	switch runtime.GOOS {
	case "linux":
		// Ensure the child is killed automatically if the Go parent process dies
		// (e.g. SIGKILL). Without this, llama-server processes survive parent
		// death and occupy the RPC node connections and port on the next run.
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
		// By default, Go sends SIGKILL, which causes ramalama to exit without stopping the container.
		// Instead, let ramalama gracefully exit by sending SIGINT
		cmd.Cancel = func() error {
			return cmd.Process.Signal(os.Interrupt)
		}
	default:
		log.Println("[WARN] Graceful shutdown of ramalama not supported for OS, switching may not work correctly")
	}

	err = cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start ramalama: %v\n", err)
	}

	back.Ready = make(chan struct{})
	back.Exited = make(chan struct{})

	// Parse stderr for per-device metrics and inference timings.
	// Uses a 4 MiB scanner buffer because some llama-server lines (token lists) are very long
	// and would cause the default 64 KiB bufio.Scanner to error out mid-stream, dropping all
	// subsequent lines including the buffer-size lines we care about.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

		// load_tensors: RPC0[192.168.1.187:50052] model buffer size =   438.60 MiB
		// load_tensors:   CPU_Mapped model buffer size =    20.51 MiB
		reModel := regexp.MustCompile(`load_tensors:\s*(?:RPC\d+\[([\d\.]+:\d+)\]|CPU_Mapped)\s*model buffer size =\s*([\d\.]+)\s*MiB`)

		// llama_kv_cache: RPC0[192.168.1.187:50052] KV buffer size =    44.00 MiB
		reKv := regexp.MustCompile(`llama_kv_cache:\s*(?:RPC\d+\[([\d\.]+:\d+)\]|.*Host)\s*KV buffer size =\s*([\d\.]+)\s*MiB`)

		// sched_reserve: RPC0[192.168.1.187:50052] compute buffer size =    66.50 MiB
		reCompute := regexp.MustCompile(`sched_reserve:\s*(?:RPC\d+\[([\d\.]+:\d+)\]|.*CPU)\s*compute buffer size =\s*([\d\.]+)\s*MiB`)

		// prompt eval time =    3227.58 ms /    24 tokens ... 7.44 tokens per second)
		// eval time =    3661.85 ms /     9 tokens ... 2.46 tokens per second)
		rePromptTps := regexp.MustCompile(`prompt eval time =.*?([\d\.]+)\s*tokens per second`)
		reEvalTps := regexp.MustCompile(`^\s*eval time =.*?([\d\.]+)\s*tokens per second`)

		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)

			// Model buffer sizes
			if m := reModel.FindStringSubmatch(line); m != nil {
				device := m[1] // empty string if CPU_Mapped
				if device == "" {
					device = "CPU"
				}
				if size, err2 := strconv.ParseFloat(m[2], 64); err2 == nil {
					back.Lock()
					back.getOrCreateDevice(device).ModelBufferMiB = size
					back.TotalBufferSize += size
					back.Unlock()
				}
			}

			// KV cache buffer sizes
			if m := reKv.FindStringSubmatch(line); m != nil {
				device := m[1]
				if device == "" {
					device = "CPU"
				}
				if size, err2 := strconv.ParseFloat(m[2], 64); err2 == nil {
					back.Lock()
					back.getOrCreateDevice(device).KvBufferMiB = size
					back.Unlock()
				}
			}

			// Compute buffer sizes
			if m := reCompute.FindStringSubmatch(line); m != nil {
				device := m[1]
				if device == "" {
					device = "CPU"
				}
				if size, err2 := strconv.ParseFloat(m[2], 64); err2 == nil {
					back.Lock()
					back.getOrCreateDevice(device).ComputeBufferMiB = size
					back.Unlock()
				}
			}

			// Live throughput from slot timing lines
			if m := rePromptTps.FindStringSubmatch(line); m != nil {
				if tps, err2 := strconv.ParseFloat(m[1], 64); err2 == nil {
					back.Lock()
					back.PromptTokensPerSec = tps
					back.Unlock()
				}
			}
			if m := reEvalTps.FindStringSubmatch(line); m != nil {
				if tps, err2 := strconv.ParseFloat(m[1], 64); err2 == nil {
					back.Lock()
					back.EvalTokensPerSec = tps
					back.Unlock()
				}
			}
		}
		if err2 := scanner.Err(); err2 != nil {
			log.Printf("stderr scanner error: %v", err2)
		}
	}()

	// Capture stdout as well
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			fmt.Fprintln(os.Stderr, scanner.Text())
		}
	}()

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

func NewFcfsScheduler(ramalama llama.Llama, port int, idleTimeout time.Duration, tracker *tracker.Tracker) *fcfsScheduler {
	scheduler := &fcfsScheduler{
		ramalama:            ramalama,
		port:                port,
		idleTimeout:         idleTimeout,
		ramalamaModelsCache: map[string]struct{}{},
		backendCond:         *sync.NewCond(&sync.Mutex{}),
		tracker:             tracker,
	}

	if idleTimeout != 0 {
		go scheduler.startIdleTimeout()
	}
	return scheduler
}

func (f *fcfsScheduler) GetTracker() *tracker.Tracker {
	return f.tracker
}

func (f *fcfsScheduler) GetDebugInfo() (snap DebugSnapshot, port int) {
	f.backendCond.L.Lock()
	defer f.backendCond.L.Unlock()

	if f.backend != nil {
		snap = f.backend.Snapshot()

		f.backend.portLock.RLock()
		port = f.backend.port
		f.backend.portLock.RUnlock()
	}
	return
}

var _ ModelScheduler = (*fcfsScheduler)(nil)
