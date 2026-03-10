# SWAG Project: model loading & RPC Reliability Debugging
## Summary for AI Migration

### 1. Objective
Enable reliable LLM model offloading from a Go backend to multiple Android devices via RPC, specifically handling 32-bit/64-bit architecture mix and low-bandwidth/high-latency Wi-Fi environments.

### 2. Core Fixes (Go Backend)

#### A. Scheduler Deadlock Prevention
**Issue**: The FCFS scheduler held a global mutex while waiting for a model to finish loading (`<-backend.Ready`). Over Wi-Fi, this can take 10-15 minutes, during which the entire API (including Debug/Dashboard metrics) was deadlocked.
**Fix**: Released the `backendCond.L` mutex during the `select` wait for the `Ready` channel. Added re-locking logic in the return paths.

#### B. Tracker Expiry & Health Probes
**Issue**: 
1. The tracker pinged devices every 10s. During dense tensor uploads, phones were too busy to respond, causing the tracker to mark them unreachable and cancel the context.
2. Device expiry was set to 60s. Phones busy with RPC often missed heartbeats, causing them to be evicted mid-load.
**Fix**:
1. Increased health-check interval to 30s and dial timeout to 5s.
2. Set `reachable=true` immediately on `/announce` (restoring trust-first semantics).
3. Increased device expiry to 1 hour to survive the entire model loading window.

#### C. Model Name & Error Handling
**Issue**: Mismatch between `hf:` prefixed model names in the tracker vs bare names from the client. Also, a missing `return` in the API handler caused panic on model start failures.
**Fix**: Implemented `modelNameVariants` to check both prefixed and bare names. Added missing control flow returns.

#### D. KV Cache OOM (Low-RAM Phones)
**Issue**: `llama.cpp` auto-fitting was picking large context sizes (~20k). The KV cache (multi-hundred MB) was being allocated on phones with <100MB free RAM, causing immediate RPC crashes.
**Fix**: Forced `-c 4096 --no-kv-offload` in `serve.go`. This keeps the KV cache on the Host CPU (rich RAM) while only weights stay on the phones.

### 3. Core Fixes (Android / Native)

#### A. Zombie Process Port Collision
**Issue**: If the Service/Activity restarted, overlapping threads would try to bind to the same RPC port. The second thread would fail but enter an infinite retry loop, which became a zombie thread that stole the port as soon as the first thread was stopped.
**Fix**: Implemented a "Token/Generation" system in `native-lib.cpp`. Every start generates a new token. If a thread sees a newer token has been issued, it kills itself and releases its socket immediately.

#### B. 32-bit Architecture (SM-G900V)
**Issue**: Initial hangs were suspected to be ABI issues. 
**Resolution**: Confirmed `ggml-rpc` uses packed fixed-width structs. The actual issue was the 32-bit device having very little available RAM (~129MB free), which was triggering the KV cache OOM described above. 

### 4. Current State
- **Stability**: High. The server no longer deadlocks.
- **Model Loading**: Functional but slow over Wi-Fi (approx 1MB/s).
- **Concurrency**: Successfully manages 3+ devices with automatic deduplication by IP.
- **Verification**: Confirmed `llama-server` successfully uploads tensors to all devices and survives the allocation phase without crashing.

### 5. Deployment Instructions
Rebuild the Go server: `go run . -host 0.0.0.0 -ramalama ./llama.cpp/build/bin/llama-server \;`
The binary will now correctly cap context and handle 32-bit devices alongside 64-bit devices.
