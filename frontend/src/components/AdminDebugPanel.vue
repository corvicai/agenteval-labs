<template>
  <div class="debug-panel">
    <div class="debug-header">
      <div class="debug-title-row">
        <h2>🔧 Super Admin – Debug Panel</h2>
        <span class="badge-restricted">Restricted Access</span>
      </div>
      <p class="debug-subtitle">Diagnostics, MCP tester and system probes.</p>
    </div>

    <div class="debug-tabs">
      <button :class="{ active: activeTab === 'mcp' }" @click="activeTab = 'mcp'">🔌 MCP Tester</button>
      <button :class="{ active: activeTab === 'afk' }" @click="activeTab = 'afk'">⏱ AFK Debug</button>
    </div>

    <!-- ─────────────────────────── MCP TESTER ─────────────────────────── -->
    <div v-if="activeTab === 'mcp'" class="tab-content">

      <div class="section-card">
        <h3>Endpoint Configuration</h3>
        <div class="form-grid">
          <div class="field">
            <label>MCP Endpoint URL</label>
            <input
              v-model="form.endpoint"
              type="text"
              placeholder="https://api.example.com/agents/<id>/"
              class="code-input"
              spellcheck="false"
            />
          </div>
          <div class="field">
            <label>Authorization Token</label>
            <div class="token-row">
              <input
                v-model="form.token"
                :type="showToken ? 'text' : 'password'"
                placeholder="Bearer sk-... or raw token"
                class="code-input"
                spellcheck="false"
              />
              <button class="btn-icon" @click="showToken = !showToken" :title="showToken ? 'Hide' : 'Show'">
                {{ showToken ? '🙈' : '👁' }}
              </button>
            </div>
          </div>
          <div class="field">
            <label>Test Question <span class="hint">(optional)</span></label>
            <input
              v-model="form.question"
              type="text"
              placeholder="Hello, what tools do you have available?"
              class="code-input"
            />
          </div>
        </div>

        <div class="test-selector">
          <h4>Tests to run</h4>
          <div class="checkbox-group">
            <label v-for="t in availableTests" :key="t.key" class="checkbox-label">
              <input type="checkbox" :value="t.key" v-model="selectedTests" />
              <span class="test-label">
                <span class="test-icon">{{ t.icon }}</span>
                <span>
                  <strong>{{ t.name }}</strong>
                  <span class="test-desc">{{ t.desc }}</span>
                </span>
              </span>
            </label>
          </div>
        </div>

        <div class="run-row">
          <button
            class="btn-run"
            @click="runTests"
            :disabled="isRunning || !form.endpoint || selectedTests.length === 0"
          >
            <span v-if="isRunning" class="spinner-inline"></span>
            <span v-else>▶</span>
            {{ isRunning ? 'Running tests…' : 'Run Selected Tests' }}
          </button>
          <button
            v-if="results.length"
            class="btn-clear"
            @click="results = []; lastEndpoint = ''"
          >
            🗑 Clear
          </button>
        </div>

        <div v-if="runError" class="run-error">
          ⚠️ {{ runError }}
        </div>
      </div>

      <!-- Results -->
      <div v-if="results.length" class="results-section">
        <div class="results-header">
          <h3>Results</h3>
          <span class="endpoint-tag">{{ lastEndpoint }}</span>
        </div>
        <div class="result-cards">
          <div
            v-for="r in results"
            :key="r.name"
            class="result-card"
            :class="r.success ? 'success' : 'failure'"
          >
            <div class="result-head">
              <span class="result-status">{{ r.success ? '✅' : '❌' }}</span>
              <span class="result-name">{{ r.name }}</span>
              <span class="result-timing">{{ r.duration_ms }}ms</span>
              <span v-if="r.status_code" class="result-code" :class="statusCodeClass(r.status_code)">
                HTTP {{ r.status_code }}
              </span>
            </div>

            <div v-if="r.answer" class="result-answer">
              <span class="field-label">Answer:</span>
              <div class="answer-text">{{ r.answer }}</div>
            </div>

            <div v-if="r.error" class="result-error">
              <span class="field-label">Error:</span>
              <code>{{ r.error }}</code>
            </div>

            <div v-if="r.request_body || r.response_body" class="result-raw">
              <div v-if="r.request_body">
                <button class="toggle-raw" @click="toggleRaw(r, 'req')">
                  {{ r._showReq ? '▼' : '▶' }} Request Body
                </button>
                <pre v-if="r._showReq" class="raw-block">{{ r.request_body }}</pre>
              </div>
              <div v-if="r.response_body">
                <button class="toggle-raw" @click="toggleRaw(r, 'res')">
                  {{ r._showRes ? '▼' : '▶' }} Response Body
                </button>
                <pre v-if="r._showRes" class="raw-block">{{ tryFormatJSON(r.response_body) }}</pre>
              </div>
            </div>

            <div v-if="r.details && Object.keys(r.details).length" class="result-raw">
              <button class="toggle-raw" @click="toggleRaw(r, 'det')">
                {{ r._showDet ? '▼' : '▶' }} Details / Metadata
              </button>
              <pre v-if="r._showDet" class="raw-block">{{ JSON.stringify(r.details, null, 2) }}</pre>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ─────────────────────────── AFK DEBUG ─────────────────────────── -->
    <div v-if="activeTab === 'afk'" class="tab-content">
      <div class="section-card">
        <h3>AFK Debug Panel</h3>
        <p class="hint-text">
          Enables the AFK debug overlay at the bottom-left of the screen. You can also
          activate it via <code>?afkDebug=1</code> in the URL or
          <code>localStorage.setItem('afk_debug', '1')</code> in the browser console.
        </p>
        <div class="afk-actions">
          <button class="btn-run" @click="enableAfkDebug">Enable AFK Debug Overlay</button>
          <button class="btn-clear" @click="disableAfkDebug">Disable</button>
        </div>
        <div v-if="afkMsg" class="afk-feedback">{{ afkMsg }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import wsService from '../services/websocket.js'

const activeTab = ref('mcp')

const form = reactive({
  endpoint: '',
  token: '',
  question: '',
})

const showToken = ref(false)
const isRunning = ref(false)
const runError = ref('')
const results = ref([])
const lastEndpoint = ref('')

const availableTests = [
  {
    key: 'go_sdk',
    name: 'Go SDK (production path)',
    icon: '🏭',
    desc: 'Exercises the exact same code used in production runs.',
  },
  {
    key: 'raw_2025_06_18',
    name: 'Raw HTTP – MCP 2025-06-18',
    icon: '🔬',
    desc: 'Direct HTTP POST bypassing the Go MCP SDK. Uses latest protocol version.',
  },
  {
    key: 'raw_2024_11_05',
    name: 'Raw HTTP – MCP 2024-11-05',
    icon: '🔬',
    desc: 'Direct HTTP POST bypassing the Go MCP SDK. Uses older protocol version.',
  },
]

const selectedTests = ref(['go_sdk', 'raw_2025_06_18', 'raw_2024_11_05'])

async function runTests() {
  if (!form.endpoint || selectedTests.value.length === 0) return
  isRunning.value = true
  runError.value = ''
  results.value = []
  lastEndpoint.value = form.endpoint

  try {
    const resp = await wsService.adminDebugMCPTest({
      endpoint: form.endpoint,
      token: form.token,
      question: form.question || undefined,
      tests: selectedTests.value,
    })
    results.value = (resp.results || []).map(r => ({
      ...r,
      _showReq: false,
      _showRes: false,
      _showDet: false,
    }))
  } catch (err) {
    runError.value = err?.message || String(err)
  } finally {
    isRunning.value = false
  }
}

function toggleRaw(result, key) {
  if (key === 'req') result._showReq = !result._showReq
  if (key === 'res') result._showRes = !result._showRes
  if (key === 'det') result._showDet = !result._showDet
}

function tryFormatJSON(str) {
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}

function statusCodeClass(code) {
  if (code >= 200 && code < 300) return 'code-ok'
  if (code >= 400 && code < 500) return 'code-client-err'
  return 'code-server-err'
}

// AFK Debug
const afkMsg = ref('')
function enableAfkDebug() {
  localStorage.setItem('afk_debug', '1')
  afkMsg.value = 'AFK debug enabled — reload the page to see the overlay.'
}
function disableAfkDebug() {
  localStorage.removeItem('afk_debug')
  afkMsg.value = 'AFK debug disabled — reload to hide the overlay.'
}
</script>

<style scoped>
.debug-panel {
  max-width: 960px;
  margin: 0 auto;
  padding: 24px 20px;
  color: #e2e8f0;
}

.debug-header {
  margin-bottom: 20px;
}
.debug-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.debug-title-row h2 {
  margin: 0;
  font-size: 1.4rem;
  font-weight: 700;
}
.badge-restricted {
  background: rgba(239, 68, 68, 0.15);
  color: #fca5a5;
  border: 1px solid rgba(239, 68, 68, 0.35);
  border-radius: 6px;
  padding: 2px 8px;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.debug-subtitle {
  margin: 4px 0 0;
  color: #94a3b8;
  font-size: 0.88rem;
}

.debug-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.2);
  margin-bottom: 20px;
}
.debug-tabs button {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  color: #94a3b8;
  padding: 8px 16px;
  cursor: pointer;
  font-size: 0.9rem;
  transition: color 0.15s, border-color 0.15s;
  margin-bottom: -1px;
}
.debug-tabs button.active {
  color: #60a5fa;
  border-bottom-color: #60a5fa;
}
.debug-tabs button:hover:not(.active) {
  color: #cbd5e1;
}

.tab-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.section-card {
  background: rgba(15, 23, 42, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.15);
  border-radius: 10px;
  padding: 20px;
}
.section-card h3 {
  margin: 0 0 16px;
  font-size: 1rem;
  font-weight: 600;
  color: #cbd5e1;
}
.section-card h4 {
  margin: 16px 0 10px;
  font-size: 0.85rem;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.form-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.field label {
  display: block;
  font-size: 0.8rem;
  font-weight: 500;
  color: #94a3b8;
  margin-bottom: 5px;
}
.hint {
  opacity: 0.6;
  font-weight: 400;
}
.code-input {
  width: 100%;
  background: rgba(2, 6, 23, 0.8);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 6px;
  color: #e2e8f0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.82rem;
  padding: 8px 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s;
}
.code-input:focus {
  border-color: #3b82f6;
}
.token-row {
  display: flex;
  gap: 6px;
}
.token-row .code-input {
  flex: 1;
}
.btn-icon {
  background: rgba(2, 6, 23, 0.8);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 6px;
  color: #94a3b8;
  padding: 0 10px;
  cursor: pointer;
  font-size: 1rem;
}

.checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.checkbox-label {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  cursor: pointer;
}
.checkbox-label input {
  margin-top: 3px;
  accent-color: #3b82f6;
  cursor: pointer;
}
.test-label {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.test-icon {
  font-size: 1rem;
  line-height: 1.4;
}
.test-label strong {
  display: block;
  font-size: 0.87rem;
  color: #e2e8f0;
}
.test-desc {
  display: block;
  font-size: 0.78rem;
  color: #64748b;
  margin-top: 1px;
}

.run-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 18px;
}
.btn-run {
  display: flex;
  align-items: center;
  gap: 6px;
  background: #2563eb;
  color: #fff;
  border: none;
  border-radius: 7px;
  padding: 9px 18px;
  font-size: 0.9rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, opacity 0.15s;
}
.btn-run:hover:not(:disabled) {
  background: #1d4ed8;
}
.btn-run:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.btn-clear {
  background: rgba(148, 163, 184, 0.08);
  color: #94a3b8;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 7px;
  padding: 8px 14px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: background 0.15s;
}
.btn-clear:hover {
  background: rgba(148, 163, 184, 0.15);
}
.run-error {
  margin-top: 10px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: 6px;
  padding: 8px 12px;
  color: #fca5a5;
  font-size: 0.85rem;
}

.spinner-inline {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
  display: inline-block;
}
@keyframes spin { to { transform: rotate(360deg); } }

/* Results */
.results-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.results-header {
  display: flex;
  align-items: center;
  gap: 10px;
}
.results-header h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: #cbd5e1;
}
.endpoint-tag {
  background: rgba(15, 23, 42, 0.8);
  border: 1px solid rgba(148, 163, 184, 0.15);
  border-radius: 5px;
  padding: 2px 8px;
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
  color: #94a3b8;
  max-width: 480px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.result-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.result-card {
  border-radius: 10px;
  border: 1px solid rgba(148, 163, 184, 0.12);
  padding: 16px;
  background: rgba(15, 23, 42, 0.6);
}
.result-card.success {
  border-left: 3px solid #22c55e;
}
.result-card.failure {
  border-left: 3px solid #ef4444;
}
.result-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.result-status {
  font-size: 1.1rem;
}
.result-name {
  font-weight: 600;
  font-size: 0.92rem;
  color: #e2e8f0;
  flex: 1;
}
.result-timing {
  font-size: 0.8rem;
  color: #64748b;
  font-family: ui-monospace, monospace;
}
.result-code {
  font-family: ui-monospace, monospace;
  font-size: 0.8rem;
  padding: 1px 7px;
  border-radius: 4px;
  font-weight: 600;
}
.code-ok { background: rgba(34, 197, 94, 0.15); color: #86efac; }
.code-client-err { background: rgba(239, 68, 68, 0.15); color: #fca5a5; }
.code-server-err { background: rgba(251, 146, 60, 0.15); color: #fdba74; }

.result-answer {
  margin-top: 10px;
  font-size: 0.85rem;
}
.field-label {
  color: #64748b;
  font-size: 0.78rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  display: block;
  margin-bottom: 4px;
}
.answer-text {
  color: #a5f3fc;
  font-size: 0.87rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.result-error {
  margin-top: 8px;
}
.result-error code {
  display: block;
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 5px;
  padding: 6px 10px;
  color: #fca5a5;
  font-size: 0.82rem;
  white-space: pre-wrap;
  word-break: break-word;
}
.result-raw {
  margin-top: 8px;
}
.toggle-raw {
  background: none;
  border: none;
  color: #475569;
  font-size: 0.78rem;
  cursor: pointer;
  padding: 2px 0;
  font-family: ui-monospace, monospace;
  transition: color 0.15s;
}
.toggle-raw:hover {
  color: #94a3b8;
}
.raw-block {
  background: rgba(2, 6, 23, 0.9);
  border: 1px solid rgba(148, 163, 184, 0.1);
  border-radius: 6px;
  padding: 10px 12px;
  margin-top: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.78rem;
  color: #94a3b8;
  overflow-x: auto;
  white-space: pre;
  max-height: 300px;
  overflow-y: auto;
}

/* AFK tab */
.hint-text {
  color: #94a3b8;
  font-size: 0.87rem;
  line-height: 1.6;
  margin-bottom: 14px;
}
.hint-text code {
  background: rgba(2, 6, 23, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.15);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 0.82rem;
  color: #93c5fd;
}
.afk-actions {
  display: flex;
  gap: 10px;
}
.afk-feedback {
  margin-top: 10px;
  color: #86efac;
  font-size: 0.85rem;
}
</style>
