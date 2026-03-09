package scheduler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

// DeviceStats holds all known memory and performance stats for a single device.
type DeviceStats struct {
	ModelBufferMiB   float64
	KvBufferMiB      float64
	ComputeBufferMiB float64
}

type backend struct {
	sync.RWMutex
	Ready    chan struct{}
	Exited   chan struct{}
	port     int
	portLock sync.RWMutex
	err      error
	cancel   func()

	// Set at spawn time
	Model string

	// Filled in by stderr parser as llama-server loads
	Devices         map[string]*DeviceStats // key: device name e.g. "192.168.1.187:50052" or "CPU"
	TotalBufferSize float64                 // sum of model buffer MiB across all devices

	// Updated after each inference
	PromptTokensPerSec float64
	EvalTokensPerSec   float64
}

func newBackend(model string) *backend {
	return &backend{
		Model:   model,
		Devices: make(map[string]*DeviceStats),
	}
}

func (b *backend) getOrCreateDevice(name string) *DeviceStats {
	if d, ok := b.Devices[name]; ok {
		return d
	}
	d := &DeviceStats{}
	b.Devices[name] = d
	return d
}

func (b *backend) healthCheck() bool {
	// /health is more accurate but might be llama-server specific
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%v/health", b.port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WithClient runs callback with a client configured to use the backend.
// Because the backend's port may be freed and reused by another backend,
// it is not safe to save the client given to callback.
func (b *backend) WithClient(callback func(openai.Client) error) error {
	b.portLock.RLock()
	defer b.portLock.RUnlock()

	if b.port == 0 { // port was freed
		return errors.New("backend is dead")
	}

	client := openai.NewClient(
		option.WithAPIKey(""),
		option.WithOrganization(""),
		option.WithProject(""),
		option.WithWebhookSecret(""),
		option.WithBaseURL(fmt.Sprintf("http://127.0.0.1:%v", b.port)),
	)

	return callback(client)
}

func (b *backend) Proxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			*pr.Out.URL = *pr.In.URL
			pr.Out.URL.Host = fmt.Sprintf("127.0.0.1:%v", b.port)
			pr.Out.URL.Scheme = "http"
		},
	}
}

// DebugSnapshot is a point-in-time copy of backend metrics.
type DebugSnapshot struct {
	Model              string
	TotalBufferMiB     float64
	Devices            map[string]DeviceStats
	PromptTokensPerSec float64
	EvalTokensPerSec   float64
}

func (b *backend) Snapshot() DebugSnapshot {
	b.RLock()
	defer b.RUnlock()

	snap := DebugSnapshot{
		Model:              b.Model,
		TotalBufferMiB:     b.TotalBufferSize,
		Devices:            make(map[string]DeviceStats, len(b.Devices)),
		PromptTokensPerSec: b.PromptTokensPerSec,
		EvalTokensPerSec:   b.EvalTokensPerSec,
	}
	for k, v := range b.Devices {
		snap.Devices[k] = *v
	}
	return snap
}
