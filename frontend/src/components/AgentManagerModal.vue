<template>
  <div class="modal-overlay" @click.self="handleClose">
    <div class="modal-container agent-manager-modal">
      <div class="modal-header">
        <h3>🤖 Manage Agents</h3>
        <button class="btn-close" @click="handleClose">×</button>
      </div>
      
      <div class="modal-body">
        <div class="agents-list">
          <div v-for="(agent, index) in localAgents" :key="agent.id" class="agent-card">
            <div class="agent-header">
              <div class="agent-drag" @mousedown="startDrag(index)">⠿</div>
              <input 
                v-model="agent.name" 
                class="agent-name-input"
                @focus="startEditing"
                @input="markPendingChanges(agent)"
                @blur="saveAgent(agent); stopEditing()"
              />
                <span class="agent-type-badge" :class="agent.provider_type">
                {{ agent.provider_type === 'mcp' ? 'Corvic' : (agent.provider_type === 'evaluator' ? 'Evaluator (OpenAI)' : agent.provider_type) }}
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
                <!-- Mode selection removed, defaulting to HTTP -->
                <div class="field full-width">
                  <span class="mode-badge">Remote (HTTP / SSE)</span>
                </div>
                
                <div class="field">
                  <label>Endpoint URL</label>
                  <input v-model="agent.config.endpoint" placeholder="https://..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Token</label>
                  <input v-model="agent.config.token" type="password" placeholder="Token..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                
                <div v-if="isDev" class="alert alert-info mt-2 full-width">
                  <strong>💡 Dev Tip:</strong> Set Token to <code>MOCK</code> to simulate responses.
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'openai'" class="config-fields">
                <div class="field">
                  <label>API Key</label>
                  <input v-model="agent.config.api_key" type="password" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Prompt ID</label>
                  <input v-model="agent.config.prompt_id" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Prompt Version</label>
                  <input v-model="agent.config.prompt_version" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
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
                <!-- Evaluator is explicitly OpenAI, so we also need API Key here if not global -->
                 <div class="field">
                  <label>OpenAI API Key</label>
                  <input v-model="agent.config.api_key" type="password" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" placeholder="sk-..." />
                </div>
                <div class="field">
                  <label>Prompt ID (Optional)</label>
                  <input v-model="agent.config.prompt_id" placeholder="e.g. prompt_..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Prompt Version (Optional)</label>
                  <input v-model="agent.config.prompt_version" placeholder="e.g. v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <div class="agent-config-actions">
          <button class="btn btn-primary" @click="addAgent('mcp')">+ Corvic Agent</button>
          <button class="btn btn-secondary" @click="addAgent('evaluator')">+ Evaluator (OpenAI)</button>
        </div>
        <div class="footer-right">
          <span v-if="saveStatus" class="save-status" :class="saveStatus">{{ saveStatusText }}</span>
          <button class="btn btn-primary" @click="saveAllAgents" :disabled="!pendingChanges || saveStatus === 'saving'">Save Changes</button>
          <button class="btn btn-secondary" @click="handleClose">Close</button>
        </div>
      </div>

      <!-- Spy Modal (Nested) -->
      <div v-if="spyAgent" class="modal-overlay" style="z-index: 1001;" @click.self="spyAgent = null">
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
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'

import { wsService } from '../services/websocket.js'
import { generateAgentName } from '../utils/nameGenerator.js'

const isDev = import.meta.env.DEV

const props = defineProps({
  agents: Array,
  workspaceId: String,
  questionSet: Object // Pass the whole object to get preloaded agents mapping
})

const emit = defineEmits(['update', 'close'])

const localAgents = ref([])
const spyAgent = ref(null)
const spyPayload = ref(null)
const saveStatus = ref(null) // 'saving', 'saved', 'error'
const saveStatusText = ref('')
const pendingChanges = ref(false)
const dirtyAgentIds = ref(new Set())
const pendingCreateIds = ref(new Set())
const isEditing = ref(false) // Block watcher updates when editing

function normalizeConfig(rawConfig, providerType) {
  let config = rawConfig
  if (typeof config === 'string') {
    try {
      config = JSON.parse(config)
    } catch (e) {
      config = {}
    }
  }
  if (!config || typeof config !== 'object' || Array.isArray(config)) {
    config = {}
  }

  if (providerType === 'mcp') {
    config = {
      mode: 'http',
      endpoint: '',
      token: '',
      ...config
    }
  } else if (providerType === 'openai') {
    config = {
      api_key: '',
      prompt_id: '',
      prompt_version: '',
      ...config
    }
  } else if (providerType === 'evaluator') {
    config = {
      target_agent_id: '',
      api_key: '',
      prompt_id: '',
      prompt_version: '',
      ...config
    }
  }

  return config
}

function mergeAgentsWithOverrides(globalAgents, qs) {
  if (!globalAgents) return []
  if (!qs?.agents || qs.agents.length === 0) {
    return (globalAgents || []).map(a => ({
      ...a,
      config: normalizeConfig(a.config, a.provider_type)
    }))
  }

  const overrideMap = {}
  qs.agents.forEach(oa => {
    overrideMap[oa.agent_id] = oa
  })

  return globalAgents.map(a => {
    const override = overrideMap[a.id]
    if (override) {
      return {
        ...a,
        enabled: override.enabled !== undefined ? override.enabled : a.enabled,
        position: override.position !== undefined ? override.position : a.position,
        config: normalizeConfig(override.config || a.config, a.provider_type)
      }
    }
    return { 
      ...a, 
      enabled: false,
      config: normalizeConfig(a.config, a.provider_type)
    }
  }).sort((a, b) => (a.position || 0) - (b.position || 0))
}

watch([() => props.agents, () => props.questionSet], ([newAgents, newQS]) => {
  // Don't overwrite local state if user is currently editing
  if (isEditing.value) {
    console.log('[AgentManagerModal] Skipping sync while editing')
    return
  }
  
  const merged = mergeAgentsWithOverrides(newAgents, newQS)
  const mergedIds = new Set(merged.map(a => a.id))

  // Preserve pending creates that haven't hit the server/broadcast yet
  for (const id of pendingCreateIds.value) {
    if (mergedIds.has(id)) {
      pendingCreateIds.value.delete(id)
    }
  }

  if (pendingCreateIds.value.size > 0) {
    const pendingAgents = localAgents.value.filter(a => pendingCreateIds.value.has(a.id))
    localAgents.value = [...pendingAgents, ...merged]
    return
  }

  localAgents.value = merged
}, { immediate: true })

function startEditing() {
  isEditing.value = true
}

function stopEditing() {
  // Delay stopping to allow save to complete first
  setTimeout(() => {
    isEditing.value = false
  }, 500)
}

async function addAgent(providerType) {
  if (!props.workspaceId) return
  
  const defaultConfigs = {
    mcp: { mode: 'http', endpoint: '', token: '' },
    openai: { api_key: '', prompt_id: '', prompt_version: '' },
    evaluator: { target_agent_id: '', api_key: '' }
  }
  
  try {
    const newAgent = await wsService.createAgent(props.workspaceId, {
      name: generateAgentName(providerType),
      provider_type: providerType,
      config: defaultConfigs[providerType] || {}
    })
    
    // Add the new agent to localAgents immediately so it appears in the list
    const newAgentWithParsedConfig = {
      ...newAgent,
      config: normalizeConfig(newAgent.config, newAgent.provider_type || providerType),
      enabled: true
    }
    if (newAgent?.id) {
      pendingCreateIds.value.add(newAgent.id)
    }
    localAgents.value.unshift(newAgentWithParsedConfig)
    
    // If we have an active QS, we should also initialize this agent for this QS
    if (props.questionSet?.id) {
      await saveQuestionSetAgents()
    }
    
    emit('update')
    showSaveStatus('saved', 'Agent created!')
  } catch (e) {
    console.error('Failed to create agent:', e)
    showSaveStatus('error', 'Failed to create')
  }
}

function showSaveStatus(status, text) {
  saveStatus.value = status
  saveStatusText.value = text
  setTimeout(() => {
    saveStatus.value = null
    saveStatusText.value = ''
  }, 2000)
}

function markPendingChanges(agent) {
  if (agent?.id) {
    dirtyAgentIds.value.add(agent.id)
  }
  pendingChanges.value = true
}

function clearPendingChanges(agentId) {
  if (agentId) {
    dirtyAgentIds.value.delete(agentId)
  }
  pendingChanges.value = dirtyAgentIds.value.size > 0
}

async function saveQuestionSetAgents() {
  if (!props.questionSet?.id) return
  const payload = localAgents.value.map((a, i) => ({
    agent_id: a.id,
    enabled: a.enabled !== undefined ? a.enabled : true,
    position: i,
    config: normalizeConfig(a.config, a.provider_type)
  }))
  await wsService.updateQuestionSetAgents(props.questionSet.id, payload)
}

async function handleClose() {
  // Save any pending changes before closing
  if (pendingChanges.value && localAgents.value.length > 0) {
    const saved = await saveAllAgents()
    if (!saved) return
  }
  emit('close')
}

async function saveAgent(agent) {
  saveStatus.value = 'saving'
  saveStatusText.value = 'Saving...'
  
  try {
    agent.config = normalizeConfig(agent.config, agent.provider_type)
    // Always persist agent config/credentials at the workspace level
    await wsService.updateAgent(agent.id, {
      name: agent.name,
      provider_type: agent.provider_type,
      config: agent.config,
      enabled: agent.enabled,
      position: agent.position
    })

    // Keep question-set mapping in sync when applicable
    if (props.questionSet?.id) {
      await saveQuestionSetAgents()
    }
    // NOTE: We intentionally do NOT emit('update') here to avoid the watcher
    // overwriting localAgents with stale server data. Updates are synced on close.
    clearPendingChanges(agent.id)
    showSaveStatus('saved', 'Saved ✓')
  } catch (e) {
    console.error('Failed to save agent:', e)
    markPendingChanges(agent)
    showSaveStatus('error', 'Save failed')
  }
}

async function saveAllAgents() {
  saveStatus.value = 'saving'
  saveStatusText.value = 'Saving...'

  try {
    const dirtyIds = Array.from(dirtyAgentIds.value)
    const agentsToSave = dirtyIds.length
      ? localAgents.value.filter(a => dirtyIds.includes(a.id))
      : localAgents.value

    for (const agent of agentsToSave) {
      agent.config = normalizeConfig(agent.config, agent.provider_type)
      await wsService.updateAgent(agent.id, {
        name: agent.name,
        provider_type: agent.provider_type,
        config: agent.config,
        enabled: agent.enabled,
        position: agent.position
      })
    }

    if (props.questionSetId) {
      await saveQuestionSetAgents()
    }

    dirtyAgentIds.value.clear()
    pendingChanges.value = false
    emit('update')
    showSaveStatus('saved', 'All changes saved')
    return true
  } catch (e) {
    console.error('Failed to save agents:', e)
    pendingChanges.value = true
    showSaveStatus('error', 'Save failed')
    return false
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
    // Update local state immediately
    localAgents.value = localAgents.value.filter(a => a.id !== agent.id)
    dirtyAgentIds.value.delete(agent.id)
    pendingCreateIds.value.delete(agent.id)
    pendingChanges.value = dirtyAgentIds.value.size > 0
    emit('update')
    showSaveStatus('saved', 'Agent deleted')
  } catch (e) {
    console.error('Failed to delete agent:', e)
    showSaveStatus('error', e.message || 'Delete failed')
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
.agent-manager-modal {
  width: 90%;
  max-width: 800px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
}

.modal-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #f8fafc;
}

.agents-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.agent-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 1rem;
  transition: all 0.2s ease;
}

.agent-card:hover {
  border-color: #cbd5e1;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
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
  border: 1px solid transparent;
  background: transparent;
  font-size: 1rem;
  font-weight: 600;
  color: #1e293b;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.agent-name-input:hover {
  border-color: #e2e8f0;
}

.agent-name-input:focus {
  outline: none;
  background: white;
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
}

.agent-type-badge {
  font-size: 0.7rem;
  padding: 0.2rem 0.6rem;
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
  padding: 0.4rem;
  border-radius: 6px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
}

.btn-icon:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.btn-danger-icon:hover {
  background: #fee2e2;
  border-color: #fca5a5;
  color: #ef4444;
}

.agent-config-actions {
  display: flex;
  gap: 0.5rem;
}

/* Config Fields */
.config-fields, .config-fields-mcp {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0.75rem;
  background: #f8fafc;
  padding: 1rem;
  border-radius: 8px;
}

.config-fields-mcp {
  grid-template-columns: 1fr 1fr;
}

.field.full-width,
.alert.full-width {
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
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.config-help {
  margin: 0.5rem 0 0;
  font-size: 0.75rem;
  color: #64748b;
  line-height: 1.4;
}

.mode-badge {
  display: inline-block;
  background: #e2e8f0;
  color: #475569;
  font-size: 0.8rem;
  font-weight: 600;
  padding: 0.3rem 0.6rem;
  border-radius: 6px;
  width: fit-content;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.save-status {
  font-size: 0.8rem;
  font-weight: 500;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  animation: fadeIn 0.2s ease;
}

.save-status.saving {
  color: #6b7280;
  background: #f3f4f6;
}

.save-status.saved {
  color: #059669;
  background: #d1fae5;
}

.save-status.error {
  color: #dc2626;
  background: #fee2e2;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}

.alert-info {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
  border: 1px solid rgba(59, 130, 246, 0.2);
  padding: 0.75rem;
  border-radius: 6px;
  font-size: 0.85rem;
}

.mt-2 { margin-top: 0.5rem; }
</style>
