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
	PhoneModel       string  `json:"phone_model"`
	Battery          float64 `json:"battery"`
	Temperature      float64 `json:"temperature"`
	MaxSize          int64   `json:"max_size"`
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

	details := s.scheduler.GetTracker().GetServerDetails()

	for _, name := range data.ConnectedDevices {
		var phoneModel string
		var battery, temp float64
		var maxSize int64
		if node, ok := details[name]; ok {
			phoneModel = node.Model
			battery = node.Battery
			temp = node.Temperature
			maxSize = node.MaxSize
		}
		data.Devices[name] = DeviceDebugData{
			PhoneModel:  phoneModel,
			Battery:     battery,
			Temperature: temp,
			MaxSize:     maxSize,
		}
	}

	snap, _ := s.scheduler.GetDebugInfo()

	if snap.Model != "" {
		data.Model = snap.Model
		data.TotalBufferMiB = snap.TotalBufferMiB
		data.PromptTokensPerSec = snap.PromptTokensPerSec
		data.EvalTokensPerSec = snap.EvalTokensPerSec

		for name, d := range snap.Devices {
			devData := data.Devices[name]
			devData.ModelBufferMiB = d.ModelBufferMiB
			devData.KvBufferMiB = d.KvBufferMiB
			devData.ComputeBufferMiB = d.ComputeBufferMiB
			devData.TotalMiB = d.ModelBufferMiB + d.KvBufferMiB + d.ComputeBufferMiB
			data.Devices[name] = devData
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
    <title>SWAG DEBUG</title>
    <style>
        body {
            background-color: #000;
            color: #ccc;
            font-family: monospace;
            padding: 20px;
            line-height: 1.5;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
        }
        h1 {
            color: #fff;
            font-size: 1.2rem;
            border-bottom: 1px solid #333;
            padding-bottom: 10px;
            margin-bottom: 20px;
        }
        .status {
            color: #0f0;
            margin-bottom: 20px;
        }
        .section {
            margin-bottom: 30px;
        }
        .section-title {
            color: #aaa;
            text-transform: uppercase;
            font-size: 0.8rem;
            margin-bottom: 10px;
            border-left: 3px solid #333;
            padding-left: 10px;
        }
        .data-row {
            display: flex;
            justify-content: space-between;
            border-bottom: 1px solid #111;
            padding: 5px 0;
        }
        .label { color: #666; }
        .value { color: #eee; }
        
        .device-box {
            border: 1px solid #222;
            padding: 15px;
            margin-bottom: 15px;
        }
        .device-header {
            display: flex;
            justify-content: space-between;
            margin-bottom: 10px;
            border-bottom: 1px solid #222;
            padding-bottom: 5px;
        }
        .device-name { color: #fff; font-weight: bold; }
        .device-meta { color: #444; font-size: 0.8rem; }
        
        .bar-container {
            height: 12px;
            background: #111;
            margin-bottom: 8px;
            display: flex;
        }
        .bar { height: 100%; }
        .bar-model   { background-color: #2563eb; }
        .bar-kv      { background-color: #059669; }
        .bar-compute { background-color: #d97706; }
        
        .legend {
            display: flex;
            gap: 15px;
            font-size: 0.7rem;
            margin-top: 10px;
        }
        .legend-item { display: flex; align-items: center; gap: 5px; }
        .dot { width: 8px; height: 8px; }

        pre { font-size: 0.9rem; margin-top: 20px; color: #444; }
    </style>
</head>
<body>
    <div class="container">
        <h1>SWAG_SERVER_DEBUG_PROBE</h1>
        <div class="status">[ SYSTEM_ONLINE ]</div>
        
        <div class="section">
            <div class="section-title">Inference_Stats</div>
            <div class="data-row">
                <span class="label">model_active</span>
                <span class="value" id="model-name">...</span>
            </div>
            <div class="data-row">
                <span class="label">tps_prompt</span>
                <span class="value" id="prompt-tps">0.00</span>
            </div>
            <div class="data-row">
                <span class="label">tps_eval</span>
                <span class="value" id="eval-tps">0.00</span>
            </div>
        </div>

        <div class="section">
            <div class="section-title">Memory_Usage</div>
            <div class="data-row">
                <span class="label">total_vram_mapped</span>
                <span class="value" id="total-mem">0.0 MiB</span>
            </div>
            <div class="data-row">
                <span class="label">active_nodes</span>
                <span class="value" id="num-devices">0</span>
            </div>
        </div>

        <div class="section">
            <div class="section-title">Device_Probe_Map</div>
            <div id="devices-container">
                <div style="color: #333;">// waiting for device data...</div>
            </div>
            
            <div class="legend" id="legend" style="display:none">
                <div class="legend-item"><div class="dot bar-model"></div> MODEL</div>
                <div class="legend-item"><div class="dot bar-kv"></div> KV_CACHE</div>
                <div class="legend-item"><div class="dot bar-compute"></div> COMPUTE</div>
            </div>
        </div>

        <pre>
-- 
SWAG Debug Interface v1.0.2
polling_rate: 1500ms
        </pre>
    </div>

    <script>
        function mib(v) { return (v || 0).toFixed(1) + ' MiB'; }
        function tps(v) { return v > 0 ? v.toFixed(2) : '0.00'; }

        async function update() {
            try {
                const r = await fetch('/api/debug_data');
                const d = await r.json();

                document.getElementById('model-name').textContent = d.model || 'NONE';
                document.getElementById('prompt-tps').textContent = tps(d.prompt_tps);
                document.getElementById('eval-tps').textContent = tps(d.eval_tps);
                document.getElementById('total-mem').textContent = mib(d.total_buffer_mib);
                document.getElementById('num-devices').textContent = d.connected_devices ? d.connected_devices.length : 0;

                const container = document.getElementById('devices-container');
                const connected = d.connected_devices || [];
                const devMap = d.devices || {};
                
                if (connected.length === 0 && !devMap['CPU']) {
                    container.innerHTML = '<div style="color: #333;">// no_active_nodes</div>';
                    document.getElementById('legend').style.display = 'none';
                    return;
                }

                document.getElementById('legend').style.display = 'flex';
                let html = '';
                
                const allNames = connected.slice();
                if (devMap['CPU'] && !allNames.includes('CPU')) allNames.unshift('CPU');

                let maxMib = 0;
                allNames.forEach(n => { if(devMap[n]) maxMib = Math.max(maxMib, devMap[n].total_mib || 0); });
                if (maxMib === 0) maxMib = 1;

                allNames.forEach(n => {
                    const stats = devMap[n] || {};
                    const total = stats.total_mib || 0;
                    const pModel = ((stats.model_mib || 0) / maxMib * 100).toFixed(1);
                    const pKv = ((stats.kv_mib || 0) / maxMib * 100).toFixed(1);
                    const pCmp = ((stats.compute_mib || 0) / maxMib * 100).toFixed(1);

                    const devModel = stats.phone_model || (n === 'CPU' ? 'HOST_CPU' : n);
                    const statsHtml = n !== 'CPU' ? 
                        '<span class="device-meta">Batt: ' + (stats.battery>0?stats.battery.toFixed(0)+'%':'N/A') + 
                        ' | Temp: ' + (stats.temperature>0?stats.temperature.toFixed(1)+'°C':'N/A') +
                        ' | Max: ' + (stats.max_size>0?mib(stats.max_size/(1024*1024)):'N/A') + '</span>' 
                        : '';

                    html += '<div class="device-box">' +
                        '<div class="device-header">' +
                            '<span class="device-name">' + devModel + (stats.phone_model ? ' (' + n + ')' : '') + '</span>' +
                            '<div>' + statsHtml + ' <span class="device-meta" style="margin-left:10px;color:#eee;">' + mib(total) + '</span></div>' +
                        '</div>' +
                        '<div class="bar-container">' +
                            '<div class="bar bar-model" style="width:' + pModel + '%"></div>' +
                            '<div class="bar bar-kv" style="width:' + pKv + '%"></div>' +
                            '<div class="bar bar-compute" style="width:' + pCmp + '%"></div>' +
                        '</div>' +
                    '</div>';
                });
                container.innerHTML = html;

            } catch(e) { console.error(e); }
        }

        update();
        setInterval(update, 1500);
    </script>
</body>
</html>
`

func (s *Server) serveDebugUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, debugUIHTML)
}
