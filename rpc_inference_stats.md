# Investigation: RPC Inference Statistics and Server Knowledge

This document outlines the statistics and information exchanged between the Android RPC nodes (phones) and the SWAG server during inference.

## 1. Data Sent from Android Devices to the Server

The Android application (`llama-rpc-app`) communicates with the server through two primary channels: a high-level **Discovery/Tracker** service and the low-level **GGML RPC Protocol**.

### A. Discovery & Heartbeat (Tracker)
The Android app proactively announces its presence to the server's tracker.
*   **Endpoint**: `/announce?port=<rpc_port>`
*   **Payload**: The RPC server port (default 50052). The server identifies the device's IP from the request headers.
*   **Frequency**: Every 30 seconds (configurable heartbeat).
*   **Information Shared**: Device IP, RPC Port, and "Online" status.

### B. GGML RPC Protocol
Once the server (acting as the RPC client) connects to the phone, it uses the standard GGML RPC commands:
*   **Memory Status (`RPC_CMD_GET_DEVICE_MEMORY`)**: The device returns its current `free_mem` and `total_mem`.
*   **Hardware Info**: During backend initialization, the device reports its available devices (e.g., "CPU", "Vulkan"), their names, and descriptions.
*   **Computation Results**: After a `GRAPH_COMPUTE` command, the device returns the computed tensor results.

---

## 2. Information the Server Innately Knows

The SWAG server (Go backend) gathers deep insights into the inference process by parsing the `stderr` output of the underlying `llama-server` process.

### A. Workload & Memory Split
The server tracks exactly how the model is distributed across devices. It captures the following MiB allocations per device (captured via regex from `llama-server` logs):
*   **Model Weights**: Memory consumed by the model tensors loaded onto the device.
*   **KV Cache**: Memory allocated for the Key-Value cache (context memory).
*   **Compute Buffers**: Temporary scratchpad memory used during mathematical operations.

### B. Live Inference Progress
While there is no "percentage complete" bar for a single mathematical graph evaluation (as RPC calls are synchronous), the server tracks progress at the **token level**:
*   **Prompt Throughput**: Tokens per second (tps) during the initial prompt processing phase.
*   **Eval Throughput**: Tokens per second (tps) during the generation (autoregressive) phase.

### C. Device Orchestration
The server maintains a global view of:
*   **Active Nodes**: Which RPC nodes are currently participating in the current inference session.
*   **Total VRAM Mapped**: The aggregate memory footprint across the entire cluster.

---

## 3. Summary of Statistics Monitoring

| Feature | Source | Data Type |
| :--- | :--- | :--- |
| **Node Telemetry & Availability** | Tracker (/announce) | IP:Port, Last Seen, Phone Model, Battery %, Temp, Max Tensor Size |
| **Total/Free Memory** | RPC Call (GET_DEVICE_MEMORY) | Bytes |
| **Workload Split** | `llama-server` logs | MiB (Model, KV, Compute) |
| **Inference Speed** | `llama-server` logs | Tokens/sec (Prompt & Eval) |
| **Hardware Desc** | RPC Initialization | Backend Name, Description |
| **Status** | HTTP Health Check | Healthy/Unhealthy |

---

## 4. Potential Dashboard Enhancements (Hidden Metrics)

Based on the `ggml-rpc` code analysis, we could extract these additional "hidden" metrics to make the dashboard more informative:

### A. Efficiency Metrics
*   **Network Efficiency (Deduplication Rate)**:
    *   **How it works**: For any tensor larger than 10 MiB (`HASH_THRESHOLD`), the server calculates a 64-bit FNV-1a hash. It sends this hash to the phone via `RPC_CMD_SET_TENSOR_HASH`.
    *   **Persistence (Disk vs. RAM)**: 
        *   **Protocol Support**: The `ggml-rpc` code supports a `cache_dir`. If provided, it saves hashed tensors to disk (e.g., `cache_dir/<hash_str>`).
        *   **Current Android State**: The Android app currently passes `nullptr` for this directory, meaning **it is currently RAM-only** and tensors are lost when the app process stops.
        *   **Future Optimization**: By passing the Android "Internal Cache" path to the native layer, we could achieve **persistent cross-session caching**. This would allow zero-network model loading on subsequent runs.
    *   **Dashboard Value**: We can track **"Logical Transfer"** (total model size) vs. **"Physical Transfer"** (bytes actually sent).
    *   **Example**: Scaling 1GB of weights over Wi-Fi might take 30s. A cache hit reduces this to <1s of hash checking.

*   **Graph Reuse**: The `RPC_CMD_GRAPH_RECOMPUTE` command indicates that the computational structure is being reused. A "Graph Hit Rate" would show how stable the workload distribution is.

### B. Compatibility & Health
*   **Protocol Version**: Display the `major.minor.patch` version (from `RPC_CMD_HELLO`) for each node to identify out-of-date apps.
*   **Max Tensor Size**: Each device reports a `max_size` (via `RPC_CMD_GET_MAX_SIZE`). This could be displayed to explain why certain large models might fail to offload to specific phones.

### C. Hardware Metadata via Heartbeat (Implemented)
The standard RPC protocol does **not** currently allow the server to ask the phone "What is your GPU name?" (e.g., "Adreno 740") or environment stats.
*   **Solution**: We modified the Android app's `/announce` call (Heartbeat) to include a `&model=` parameter (e.g., `Pixel 8 Pro`), `&battery=`, `&temperature=`, and `&max_size=`. This side-channels critical telemetry to the debug dashboard without having to change the low-level C/C++ RPC protocol.
