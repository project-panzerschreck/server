package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wk-y/rama-swap/server/scheduler"
)

type DeviceDebugData struct {
	ModelBufferMiB   float64 `json:"model_mib"`
	KvBufferMiB      float64 `json:"kv_mib"`
	ComputeBufferMiB float64 `json:"compute_mib"`
	TotalMiB         float64 `json:"total_mib"`
}

type DebugData struct {
	Model              string                     `json:"model"`
	TotalBufferMiB     float64                    `json:"total_buffer_mib"`
	Devices            map[string]DeviceDebugData `json:"devices"`
	ConnectedDevices   []string                   `json:"connected_devices"`
	PromptTokensPerSec float64                    `json:"prompt_tps"`
	EvalTokensPerSec   float64                    `json:"eval_tps"`
}

func (s *Server) handleDebugData(w http.ResponseWriter, r *http.Request) {
	data := DebugData{
		Devices:          make(map[string]DeviceDebugData),
		ConnectedDevices: s.scheduler.GetTracker().GetServers(),
	}

	snap, _ := s.scheduler.GetDebugInfo()

	if snap.Model != "" {
		data.Model = snap.Model
		data.TotalBufferMiB = snap.TotalBufferMiB
		data.PromptTokensPerSec = snap.PromptTokensPerSec
		data.EvalTokensPerSec = snap.EvalTokensPerSec

		for name, d := range snap.Devices {
			data.Devices[name] = DeviceDebugData{
				ModelBufferMiB:   d.ModelBufferMiB,
				KvBufferMiB:      d.KvBufferMiB,
				ComputeBufferMiB: d.ComputeBufferMiB,
				TotalMiB:         d.ModelBufferMiB + d.KvBufferMiB + d.ComputeBufferMiB,
			}
		}
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

// Ensure unused import is used
var _ = scheduler.DebugSnapshot{}

const debugUIHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SWAG Server | Live Debug</title>
    <style>
        :root {
            --bg: #0f172a;
            --surface: #1e293b;
            --surface2: #263348;
            --primary: #3b82f6;
            --primary-glow: rgba(59, 130, 246, 0.4);
            --success: #10b981;
            --warn: #f59e0b;
            --text-main: #f8fafc;
            --text-muted: #94a3b8;
            --border: #334155;
            --card-radius: 14px;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Inter', system-ui, -apple-system, sans-serif;
            background: var(--bg);
            color: var(--text-main);
            min-height: 100vh;
            padding: 2rem 1rem;
            display: flex;
            flex-direction: column;
            align-items: center;
        }
        .container { width: 100%; max-width: 1060px; display: flex; flex-direction: column; gap: 1.5rem; }
        header { text-align: center; animation: fadeIn 0.8s ease-out; }
        h1 {
            font-size: 2.4rem;
            font-weight: 800;
            background: linear-gradient(135deg, #60a5fa 0%, #a78bfa 60%, #f472b6 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 0.4rem;
        }
        .badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 0.3rem 0.85rem;
            border-radius: 9999px;
            font-size: 0.8rem;
            font-weight: 600;
            background: rgba(16, 185, 129, 0.12);
            color: var(--success);
            border: 1px solid rgba(16, 185, 129, 0.25);
            margin-bottom: 0.6rem;
        }
        .subtitle { color: var(--text-muted); font-size: 1rem; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1.25rem; }
        .card {
            background: var(--surface);
            border-radius: var(--card-radius);
            padding: 1.4rem;
            border: 1px solid var(--border);
            box-shadow: 0 4px 20px rgba(0,0,0,0.2);
            transition: transform 0.2s, box-shadow 0.2s;
            animation: slideUp 0.5s ease-out backwards;
        }
        .card:hover { transform: translateY(-2px); box-shadow: 0 8px 30px rgba(59,130,246,0.15); }
        .card-title {
            font-size: 0.72rem;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            color: var(--text-muted);
            margin-bottom: 1rem;
        }
        .big-num { font-size: 2.2rem; font-weight: 700; line-height: 1; }
        .unit { font-size: 1rem; color: var(--text-muted); font-weight: 400; margin-left: 2px; }
        .row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 0.6rem 0;
            border-bottom: 1px solid var(--border);
        }
        .row:last-child { border-bottom: none; padding-bottom: 0; }
        .lbl { color: var(--text-muted); font-size: 0.9rem; }
        .val { font-family: monospace; font-size: 1rem; }
        /* Device cards */
        .device-card {
            background: var(--surface2);
            border-radius: 10px;
            padding: 1.1rem 1.2rem;
            border: 1px solid var(--border);
            margin-bottom: 0.9rem;
        }
        .device-card:last-child { margin-bottom: 0; }
        .device-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 0.85rem;
        }
        .device-name { font-weight: 600; font-size: 0.95rem; }
        .device-tag {
            font-size: 0.72rem;
            background: rgba(59,130,246,0.15);
            color: #60a5fa;
            border-radius: 4px;
            padding: 2px 7px;
        }
        .mem-bars { display: flex; flex-direction: column; gap: 6px; }
        .bar-row { display: flex; align-items: center; gap: 10px; }
        .bar-lbl { font-size: 0.75rem; color: var(--text-muted); width: 60px; text-align: right; flex-shrink: 0; }
        .bar-track { flex: 1; height: 7px; background: rgba(255,255,255,0.07); border-radius: 4px; overflow: hidden; }
        .bar-fill { height: 100%; border-radius: 4px; transition: width 0.9s cubic-bezier(0.4,0,0.2,1); }
        .bar-model  { background: linear-gradient(90deg, #3b82f6, #6366f1); }
        .bar-kv     { background: linear-gradient(90deg, #10b981, #34d399); }
        .bar-cmp    { background: linear-gradient(90deg, #f59e0b, #fb923c); }
        .bar-val { font-size: 0.75rem; color: var(--text-muted); width: 68px; flex-shrink: 0; text-align: left; font-family: monospace; }
        .no-data { color: var(--text-muted); text-align: center; padding: 2.5rem 0; font-size: 0.95rem; }
        /* tps cards */
        .tps-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
        .tps-box { background: rgba(255,255,255,0.03); border-radius: 8px; padding: 0.9rem 1rem; border: 1px solid var(--border); }
        .tps-box .lbl { font-size: 0.75rem; margin-bottom: 4px; }
        .tps-box .big-num { font-size: 1.8rem; }
        @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
        @keyframes slideUp { from { opacity: 0; transform: translateY(16px); } to { opacity: 1; transform: translateY(0); } }
        .pulse {
            width: 8px; height: 8px;
            background: var(--success);
            border-radius: 50%;
            box-shadow: 0 0 6px var(--success);
            animation: pulse-anim 2s infinite;
        }
        @keyframes pulse-anim {
            0%   { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16,185,129,0.7); }
            70%  { transform: scale(1);    box-shadow: 0 0 0 6px rgba(16,185,129,0); }
            100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16,185,129,0); }
        }
        .legend { display: flex; gap: 16px; margin-top: 10px; flex-wrap: wrap; }
        .legend-item { display: flex; align-items: center; gap: 6px; font-size: 0.75rem; color: var(--text-muted); }
        .legend-dot { width: 9px; height: 9px; border-radius: 2px; }
    </style>
</head>
<body>
<div class="container">
    <header>
        <h1>SWAG Dashboard</h1>
        <div class="badge"><span class="pulse"></span>System Online</div>
        <p class="subtitle" id="model-name">Waiting for model data...</p>
    </header>

    <div class="grid">
        <div class="card" style="animation-delay:0.05s">
            <div class="card-title">Inference throughput (tokens/s)</div>
            <div class="tps-grid">
                <div class="tps-box">
                    <div class="lbl">Prompt Eval</div>
                    <div class="big-num" id="prompt-tps">—</div>
                </div>
                <div class="tps-box">
                    <div class="lbl">Generation</div>
                    <div class="big-num" id="eval-tps">—</div>
                </div>
            </div>
        </div>
        <div class="card" style="animation-delay:0.12s">
            <div class="card-title">Memory Overview</div>
            <div class="row"><span class="lbl">Model tensors loaded</span><span class="val" id="total-mem">0.0 MiB</span></div>
            <div class="row"><span class="lbl">Connected RPC devices</span><span class="big-num" id="num-devices" style="font-size:1.4rem">0</span></div>
        </div>
    </div>

    <div class="card" style="animation-delay:0.2s">
        <div class="card-title" style="display:flex;justify-content:space-between">
            <span>Device Memory Breakdown</span>
        </div>
        <div class="legend">
            <span class="legend-item"><span class="legend-dot" style="background:linear-gradient(90deg,#3b82f6,#6366f1)"></span>Model tensors</span>
            <span class="legend-item"><span class="legend-dot" style="background:linear-gradient(90deg,#10b981,#34d399)"></span>KV cache</span>
            <span class="legend-item"><span class="legend-dot" style="background:linear-gradient(90deg,#f59e0b,#fb923c)"></span>Compute buffer</span>
        </div>
        <div style="margin-top:1rem" id="devices-container">
            <div class="no-data">No active inference. Load a model to see device breakdown.</div>
        </div>
    </div>
</div>

<script>
function mib(v) { return (v || 0).toFixed(1) + ' MiB'; }
function tps(v)  { return v > 0 ? v.toFixed(2) : '—'; }

function barRow(label, cls, value, maxMib) {
    const pct = maxMib > 0 ? Math.min(100, (value / maxMib) * 100).toFixed(1) : 0;
    return '<div class="bar-row">' +
        '<span class="bar-lbl">' + label + '</span>' +
        '<div class="bar-track"><div class="bar-fill ' + cls + '" style="width:' + pct + '%"></div></div>' +
        '<span class="bar-val">' + mib(value) + '</span>' +
        '</div>';
}

async function fetchDebugData() {
    try {
        const res = await fetch('/api/debug_data');
        const data = await res.json();

        document.getElementById('model-name').textContent =
            data.model ? 'Loaded: ' + data.model : 'No model currently loaded';
        document.getElementById('prompt-tps').textContent = tps(data.prompt_tps);
        document.getElementById('eval-tps').textContent   = tps(data.eval_tps);
        document.getElementById('total-mem').textContent  = mib(data.total_buffer_mib);
        document.getElementById('num-devices').textContent =
            data.connected_devices ? data.connected_devices.length : 0;

        const container = document.getElementById('devices-container');
        const connected  = data.connected_devices || [];

        if (connected.length === 0) {
            container.innerHTML = '<div class="no-data">No RPC devices connected.</div>';
            return;
        }

        const devMap = data.devices || {};

        // Largest total across all devices for scaling bars
        let maxMib = 0;
        connected.forEach(d => { if (devMap[d]) maxMib = Math.max(maxMib, devMap[d].total_mib || 0); });
        if (devMap['CPU']) maxMib = Math.max(maxMib, devMap['CPU'].total_mib || 0);
        if (maxMib === 0) maxMib = 1;

        let html = '';
        const allDevices = connected.slice();
        if (devMap['CPU'] && !allDevices.includes('CPU')) allDevices.unshift('CPU');

        allDevices.forEach(name => {
            const d = devMap[name] || {};
            const totalMib = d.total_mib || 0;
            const isCpu = name === 'CPU';
            html += '<div class="device-card">' +
                '<div class="device-header">' +
                    '<span class="device-name">' + (isCpu ? 'Local CPU' : name) + '</span>' +
                    '<span class="device-tag">' + (isCpu ? 'Host' : 'RPC') + ' &mdash; ' + mib(totalMib) + ' total</span>' +
                '</div>' +
                '<div class="mem-bars">' +
                    barRow('Model', 'bar-model', d.model_mib || 0, maxMib) +
                    barRow('KV', 'bar-kv', d.kv_mib || 0, maxMib) +
                    barRow('Compute', 'bar-cmp', d.compute_mib || 0, maxMib) +
                '</div></div>';
        });

        container.innerHTML = html;
    } catch(e) { console.error('debug data fetch failed', e); }
}

fetchDebugData();
setInterval(fetchDebugData, 1500);
</script>
</body>
</html>
`

func (s *Server) serveDebugUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, debugUIHTML)
}
