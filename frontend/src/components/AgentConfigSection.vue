<template>
  <div class="config-section summary-section">
    <div class="summary-header">
      <h3>🤖 Agent Configuration</h3>
      <button class="btn-close-summary" @click="$emit('close')">×</button>
    </div>
    
    <div class="agents-list">
      <div v-for="(agent, index) in localAgents" :key="agent.id" class="agent-card">
        <div class="agent-header">
          <div class="agent-drag" @mousedown="startDrag(index)">⠿</div>
          <input 
            v-model="agent.name" 
            class="agent-name-input"
            @blur="saveAgent(agent)"
          />
          <span class="agent-type-badge" :class="agent.provider_type">
            {{ agent.provider_type }}
          </span>
          <div class="agent-actions">
            <button class="btn-icon" @click="toggleAgent(agent)" :title="agent.enabled ? 'Disable' : 'Enable'">
              {{ agent.enabled ? '✅' : '⏸️' }}
            </button>
            <button class="btn-icon" @click="showSpyModal(agent)" title="Spy Payload">
              🔍
            </button>
            <button class="btn-icon btn-danger-icon" @click="deleteAgent(agent)" title="Delete">
              🗑️
            </button>
          </div>
        </div>
        
        <div class="agent-config">
          <div v-if="agent.provider_type === 'mcp'" class="config-fields-mcp">
            <div class="field full-width">
              <label>Connection Mode</label>
              <select v-model="agent.config.mode" @change="saveAgent(agent)">
                <option value="http">Remote (HTTP / SSE)</option>
                <option value="stdio">Local (Stdio / Process)</option>
              </select>
            </div>
            
            <template v-if="agent.config.mode !== 'stdio'">
              <div class="field">
                <label>Endpoint URL</label>
                <input v-model="agent.config.endpoint" placeholder="https://..." @blur="saveAgent(agent)" />
              </div>
              <div class="field">
                <label>Auth Token</label>
                <input v-model="agent.config.token" type="password" placeholder="Token..." @blur="saveAgent(agent)" />
              </div>
            </template>
            
            <template v-else>
              <div class="field">
                <label>Command</label>
                <input v-model="agent.config.command" placeholder="e.g. npx, python, node" @blur="saveAgent(agent)" />
              </div>
              <div class="field">
                <label>Arguments (JSON Array)</label>
                <input 
                  :value="JSON.stringify(agent.config.args || [])" 
                  @blur="e => { try { agent.config.args = JSON.parse(e.target.value); saveAgent(agent); } catch(err) { agent.config.args = []; } }" 
                  placeholder='["-y", "@modelcontextprotocol/server-everything"]'
                />
              </div>
              <div class="field full-width">
                <label>Environment Variables (JSON Object)</label>
                <input 
                  :value="JSON.stringify(agent.config.env || {})" 
                  @blur="e => { try { agent.config.env = JSON.parse(e.target.value); saveAgent(agent); } catch(err) { agent.config.env = {}; } }" 
                  placeholder='{"DEBUG": "true", "API_KEY": "..."}'
                />
              </div>
              <div class="field full-width">
                <p class="config-help">
                  💡 <strong>Stdio Mode:</strong> The command must be available inside the container. 
                  Node.js (v20) and Python (3.11) are pre-installed.
                </p>
              </div>
            </template>
          </div>
          <div v-else-if="agent.provider_type === 'openai'" class="config-fields">
            <div class="field">
              <label>API Key</label>
              <input v-model="agent.config.api_key" type="password" @blur="saveAgent(agent)" />
            </div>
            <div class="field">
              <label>Prompt ID</label>
              <input v-model="agent.config.prompt_id" @blur="saveAgent(agent)" />
            </div>
            <div class="field">
              <label>Prompt Version</label>
              <input v-model="agent.config.prompt_version" @blur="saveAgent(agent)" />
            </div>
          </div>
          <div v-else-if="agent.provider_type === 'evaluator'" class="config-fields">
            <div class="field">
              <label>Target Agent</label>
              <select v-model="agent.config.target_agent_id" @change="saveAgent(agent)">
                <option value="">All Agents</option>
                <option v-for="a in agents.filter(x => x.id !== agent.id && x.provider_type !== 'evaluator')" :key="a.id" :value="a.id">
                  {{ a.name }}
                </option>
              </select>
            </div>
            <div class="field">
              <label>OpenAI API Key (Required)</label>
              <input v-model="agent.config.api_key" type="password" placeholder="sk-..." @blur="saveAgent(agent)" />
            </div>
          </div>

        </div>
      </div>
    </div>

    <div class="agent-config-actions">
      <button class="btn btn-primary" @click="addAgent('mcp')">+ MCP Agent</button>
      <button class="btn btn-primary" @click="addAgent('openai')">+ OpenAI Agent</button>
      <button class="btn btn-secondary" @click="addAgent('evaluator')">+ Evaluator</button>
    </div>

    <!-- Spy Modal -->
    <div v-if="spyAgent" class="modal-overlay" @click.self="spyAgent = null">
      <div class="modal-container payload-modal">
        <div class="modal-header">
          <h3>🔍 Spy Payload: {{ spyAgent.name }}</h3>
          <button class="btn-close" @click="spyAgent = null">×</button>
        </div>
        <div class="modal-body">
          <p class="modal-description">
            This shows what will be sent to the Python runner (secrets are redacted):
          </p>
          <pre class="payload-content">{{ JSON.stringify(spyPayload, null, 2) }}</pre>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="spyAgent = null">Close</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ref, watch } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  agents: Array,
  workspaceId: String
})

const emit = defineEmits(['update', 'close'])

const localAgents = ref([])
const spyAgent = ref(null)
const spyPayload = ref(null)

watch(() => props.agents, (newAgents) => {
  localAgents.value = newAgents.map(a => ({
    ...a,
    config: typeof a.config === 'string' ? JSON.parse(a.config) : (a.config || {})
  }))
}, { immediate: true })

async function addAgent(providerType) {
  if (!props.workspaceId) return
  
  const defaultConfigs = {
    mcp: { mode: 'http', endpoint: '', token: '', command: '', args: [], env: {} },
    openai: { api_key: '', prompt_id: '', prompt_version: '' },
    evaluator: { target_agent_id: '', api_key: '' }
  }
  
  try {
    // Using WebSocket Service
    await wsService.createAgent(props.workspaceId, {
      name: `New ${providerType} Agent`,
      provider_type: providerType,
      config: defaultConfigs[providerType] || {}
    })
    // No need to emit update manually if WS events handle it, but for safety/UI reactivity we can keep emitting or rely on store updates.
    // However, wsStore listens to events.
    emit('update')
  } catch (e) {
    console.error('Failed to create agent:', e)
  }
}

async function saveAgent(agent) {
  try {
    await wsService.updateAgent(agent.id, {
      name: agent.name,
      provider_type: agent.provider_type,
      config: agent.config,
      enabled: agent.enabled,
      position: agent.position
    })
  } catch (e) {
    console.error('Failed to save agent:', e)
  }
}

async function toggleAgent(agent) {
  agent.enabled = !agent.enabled
  await saveAgent(agent)
}

async function deleteAgent(agent) {
  if (!confirm(`Delete agent "${agent.name}"?`)) return
  try {
    await wsService.deleteAgent(agent.id)
    emit('update')
  } catch (e) {
    console.error('Failed to delete agent:', e)
  }
}

async function showSpyModal(agent) {
  try {
    spyPayload.value = await wsService.getSpyPayload(agent.id, '[Sample question]')
    spyAgent.value = agent
  } catch (e) {
    console.error('Failed to get spy payload:', e)
  }
}

function startDrag(index) {
  // TODO: Implement drag-and-drop reordering
  console.log('Drag started for index:', index)
}
</script>

<style scoped>
.config-section {
  max-height: 50vh;
  overflow-y: auto;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 1.5rem;
}

.agents-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 0;
  margin-bottom: 1.5rem;
}

.agent-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 1rem;
  transition: all 0.2s ease;
}

.agent-card:hover {
  border-color: #cbd5e1;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.agent-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.agent-drag {
  cursor: grab;
  color: #94a3b8;
  font-size: 1.2rem;
}

.agent-name-input {
  flex: 1;
  border: none;
  background: transparent;
  font-size: 1rem;
  font-weight: 600;
  color: #1e293b;
  padding: 0.25rem;
}

.agent-name-input:focus {
  outline: none;
  background: white;
  border-radius: 4px;
  box-shadow: 0 0 0 2px #e2e8f0;
}

.agent-type-badge {
  font-size: 0.7rem;
  padding: 0.2rem 0.5rem;
  border-radius: 999px;
  font-weight: 600;
  text-transform: uppercase;
}

.agent-type-badge.mcp {
  background: #dbeafe;
  color: #1e40af;
}

.agent-type-badge.openai {
  background: #dcfce7;
  color: #166534;
}

.agent-type-badge.evaluator {
  background: #fef3c7;
  color: #92400e;
}

.agent-actions {
  display: flex;
  gap: 0.25rem;
}

.btn-icon {
  background: white;
  border: 1px solid #e2e8f0;
  font-size: 1rem;
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 4px;
  transition: all 0.2s;
}

.btn-icon:hover {
  background: #f1f5f9;
}

.btn-danger-icon:hover {
  background: #fef2f2;
  border-color: #fecaca;
}

.config-fields, .config-fields-mcp {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0.75rem;
}

.config-fields-mcp {
  grid-template-columns: 1fr 1fr;
}

.field.full-width {
  grid-column: 1 / -1;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.field label {
  font-size: 0.75rem;
  color: #64748b;
  font-weight: 500;
}

.field input,
.field select {
  padding: 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.875rem;
  color: #1e293b;
  background: white;
}

.field input:focus,
.field select:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.config-help {
  margin: 0.5rem 0 0;
  font-size: 0.75rem;
  color: #64748b;
  line-height: 1.4;
}
</style>
