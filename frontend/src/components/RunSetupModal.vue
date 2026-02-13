<template>
  <div class="modal-overlay" @click.self="$emit('cancel')">
    <div class="modal-container run-setup-modal">
      <div class="modal-header">
        <h3>🚀 Run Benchmark Setup</h3>
        <button class="btn-close" @click="$emit('cancel')">×</button>
      </div>

      <div class="modal-body">
        <div class="setup-section">
          <label class="section-label">Question Set</label>
          <div class="info-box">
            <span class="info-icon">📋</span>
            <div class="info-content">
              <strong>{{ questionSet?.name || 'Unknown Set' }}</strong>
              <span class="info-meta">{{ totalQuestions }} questions</span>
            </div>
          </div>
        </div>

        <div class="setup-section">
          <label class="section-label">Select Benchmark Agents</label>
          <p class="section-hint">These agents answer questions. If none is selected, evaluator-only mode uses the latest run answers.</p>
          <div class="agents-checklist">
            <div 
              v-for="(agent, index) in localAgents.filter(a => !isEvaluatorAgent(a))" 
              :key="agent.id" 
              class="agent-check-item"
              :class="{ selected: selectedAgentIds.includes(agent.id) }"
              draggable="true"
              @dragstart="onDragStart($event, index)"
              @dragover="onDragOver($event, index)"
              @drop="onDrop($event, index)"
              @click="toggleAgent(agent.id)"
            >
              <div class="drag-handle">⋮⋮</div>
              <div class="checkbox">
                {{ selectedAgentIds.includes(agent.id) ? '☑️' : '⬜' }}
              </div>
              <div class="agent-info">
                <span class="agent-name">{{ agent.name }}</span>
                <span class="agent-type">{{ agent.provider_type }}</span>
              </div>
            </div>
          </div>
          <p v-if="selectedAgentIds.length === 0" class="error-text">
            ⚠️ Please select at least one agent to run.
          </p>
          <p v-else-if="selectedPrimaryAgentIds.length === 0 && selectedEvaluatorAgentIds.length > 0" class="warning-text">
            ℹ️ Evaluator-only mode: this will evaluate answers from the latest run for this question set.
          </p>
        </div>

        <div class="setup-section" v-if="localAgents.some(a => isEvaluatorAgent(a))">
          <label class="section-label">Configure Evaluators</label>
          <p class="section-hint">Evaluators run automatically after agents finish.</p>
          <div class="agents-checklist">
            <div 
              v-for="(agent, index) in localAgents.filter(a => isEvaluatorAgent(a))" 
              :key="agent.id" 
              class="agent-check-item"
              :class="{ selected: selectedAgentIds.includes(agent.id) }"
              @click="toggleAgent(agent.id)"
            >
              <div class="checkbox">
                {{ selectedAgentIds.includes(agent.id) ? '☑️' : '⬜' }}
              </div>
              <div class="agent-info">
                <span class="agent-name">{{ agent.name }} ({{ agent.provider_type }})</span>
                
                <!-- Target Agent Selection for Evaluators -->
                <div class="target-agent-select" @click.stop>
                  <label>Target:</label>
                  <select v-model="agent.config.target_agent_id" @change="isDirty = true">
                    <option value="">All Agents</option>
                    <option v-for="a in localAgents.filter(x => x.id !== agent.id && !isEvaluatorAgent(x))" :key="a.id" :value="a.id">
                      {{ a.name }}
                    </option>
                  </select>
                </div>
              </div>
            </div>
          </div>
          <p v-if="selectedEvaluatorsMissingTarget.length > 0" class="error-text">
            ⚠️ Select a target agent for evaluator(s): {{ selectedEvaluatorsMissingTarget.map(a => a.name).join(', ') }}.
          </p>
        </div>
      </div>

      <div class="modal-footer">
        <button class="btn btn-secondary" @click="saveSelection" :disabled="isSaving">
          {{ isSaving ? 'Saving...' : 'Save Selection' }}
        </button>
        <div class="footer-actions">
          <button class="btn btn-ghost" @click="$emit('cancel')" :disabled="isSaving">Cancel</button>
          <button 
            class="btn btn-primary btn-lg" 
            :disabled="selectedAgentIds.length === 0 || selectedEvaluatorsMissingTarget.length > 0 || isSaving"
            @click="confirmRun"
          >
            {{ isSaving ? 'Saving...' : `Start Run (${selectedAgentIds.length})` }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  questionSet: Object,
  agents: Array
})

const emit = defineEmits(['start', 'cancel', 'save'])

const localAgents = ref([])
const isSaving = ref(false)
const isDirty = ref(false)

const selectedAgentIds = ref([])

function toAgentID(entry) {
  if (!entry || typeof entry !== 'object') return ''
  return String(entry.agent_id || entry.agentID || entry.id || '')
}

function cloneConfig(rawConfig) {
  if (typeof rawConfig === 'string') {
    try {
      rawConfig = JSON.parse(rawConfig)
    } catch (e) {
      rawConfig = {}
    }
  }
  if (!rawConfig || typeof rawConfig !== 'object' || Array.isArray(rawConfig)) {
    return {}
  }
  try {
    return JSON.parse(JSON.stringify(rawConfig))
  } catch (e) {
    return { ...rawConfig }
  }
}

function sortAgents(agents = []) {
  return [...agents].sort((a, b) => (a.position || 0) - (b.position || 0))
}

function buildOverrideMap() {
  const map = {}
  const overrides = Array.isArray(props.questionSet?.agents) ? props.questionSet.agents : []
  overrides.forEach((override) => {
    const id = toAgentID(override)
    if (id) map[id] = override
  })
  return map
}

function hydrateLocalAgents(sourceAgents) {
  const overrideMap = buildOverrideMap()
  const list = Array.isArray(sourceAgents) ? sourceAgents : []
  return sortAgents(list.map((agent) => {
    const override = overrideMap[agent.id]
    return {
      ...agent,
      position: override?.position !== undefined ? override.position : agent.position,
      enabled: override?.enabled !== undefined ? !!override.enabled : !!agent.enabled,
      config: cloneConfig(override?.config || agent.config)
    }
  }))
}

function isTargetConfigured(agent) {
  return typeof agent?.config?.target_agent_id === 'string' && agent.config.target_agent_id.trim() !== ''
}

function isLegacyEvaluatorConfig(config) {
  if (!config || typeof config !== 'object') return false
  const hasTargetField = Object.prototype.hasOwnProperty.call(config, 'target_agent_id')
  const hasOpenAIMode = typeof config.openai_mode === 'string' && config.openai_mode.trim() !== ''
  const hasSystemPrompt = typeof config.system_prompt === 'string' && config.system_prompt.trim() !== ''
  return hasTargetField || hasOpenAIMode || hasSystemPrompt
}

function isEvaluatorAgent(agent) {
  if (!agent) return false
  if (agent.provider_type === 'evaluator') return true
  if (agent.provider_type !== 'openai') return false
  if (isLegacyEvaluatorConfig(agent.config || {})) return true
  const name = String(agent.name || '').toLowerCase()
  return name.includes('evaluator')
}

const selectedPrimaryAgentIds = computed(() => {
  return selectedAgentIds.value.filter((id) => {
    const agent = localAgents.value.find((a) => a.id === id)
    return agent && !isEvaluatorAgent(agent)
  })
})

const selectedEvaluatorAgentIds = computed(() => {
  return selectedAgentIds.value.filter((id) => {
    const agent = localAgents.value.find((a) => a.id === id)
    return agent && isEvaluatorAgent(agent)
  })
})

const selectedEvaluatorsMissingTarget = computed(() => {
  return localAgents.value.filter((agent) => {
    if (!isEvaluatorAgent(agent)) return false
    if (!selectedAgentIds.value.includes(agent.id)) return false
    return !isTargetConfigured(agent)
  })
})

// Initialize selected IDs from agents prop (which already reflects database state via mergedAgents)
// The agents prop comes from BenchmarkArena's mergedAgents which applies QS overrides
function initializeSelection() {
  localAgents.value = hydrateLocalAgents(props.agents)

  const overrides = Array.isArray(props.questionSet?.agents) ? props.questionSet.agents : []
  const overrideIDs = overrides.map((item) => toAgentID(item)).filter(Boolean)
  if (overrideIDs.length > 0) {
    selectedAgentIds.value = overrides
      .filter((item) => item?.enabled)
      .map((item) => toAgentID(item))
      .filter(Boolean)
    return
  }

  const enabledIDs = localAgents.value.filter((a) => a.enabled).map((a) => a.id)
  if (enabledIDs.length > 0) {
    selectedAgentIds.value = [...enabledIDs]
    return
  }

  // Fresh question set with no mapping yet: default to primary agents only.
  selectedAgentIds.value = localAgents.value.filter((a) => !isEvaluatorAgent(a)).map((a) => a.id)
}

// Watchers to keep local state in sync if props change while modal is open
watch(() => props.agents, (newAgents) => {
  if (!newAgents || isDirty.value) return
  initializeSelection()
}, { deep: true })

watch(() => props.questionSet, (newQs) => {
  if (!newQs || isDirty.value) return
  initializeSelection()
}, { deep: true })

onMounted(() => {
  initializeSelection()
})

const draggedIndex = ref(null)

function onDragStart(event, index) {
  draggedIndex.value = index
  event.dataTransfer.effectAllowed = 'move'
  // Optional: set drag image ghost
}

function onDragOver(event, index) {
  event.preventDefault() // Required to allow dropping
  event.dataTransfer.dropEffect = 'move'
}

function onDrop(event, index) {
  event.preventDefault()
  const fromIndex = draggedIndex.value
  
  if (fromIndex === null || fromIndex === index) return

  isDirty.value = true
  // Clone array to modify
  const newAgents = [...localAgents.value]
  const [movedItem] = newAgents.splice(fromIndex, 1)
  newAgents.splice(index, 0, movedItem)
  
  // Update positions based on new index
  newAgents.forEach((a, idx) => {
    a.position = idx
  })
  
  // Assign back to reactive ref
  localAgents.value = newAgents
  draggedIndex.value = null
}

const totalQuestions = computed(() => {
  if (!props.questionSet?.data) return 0
  let data = props.questionSet.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch { return 0 }
  }
  return data.categories?.reduce((acc, cat) => acc + (cat.questions?.length || 0), 0) || 0
})

function toggleAgent(id) {
  isDirty.value = true
  const alreadySelected = selectedAgentIds.value.includes(id)
  if (alreadySelected) {
    selectedAgentIds.value = selectedAgentIds.value.filter(aid => aid !== id)
  } else {
    selectedAgentIds.value.push(id)
    const agent = localAgents.value.find((a) => a.id === id)
    if (agent && isEvaluatorAgent(agent) && !isTargetConfigured(agent)) {
      alert(`Evaluator "${agent.name}" requires a target agent. Select one in the Target field before running.`)
    }
  }
}

// Build payload for question set agents API
function buildAgentsPayload() {
  return localAgents.value.map((a, i) => ({
    agent_id: a.id,
    enabled: selectedAgentIds.value.includes(a.id),
    position: i,
    config: cloneConfig(a.config)
  }))
}

async function saveToDatabase() {
  if (!props.questionSet?.id) return
  
  try {
    isSaving.value = true
    const payload = buildAgentsPayload()
    console.log('[RunSetupModal] Sending payload to DB:', payload.map(a => ({ id: a.agent_id.slice(0,8), enabled: a.enabled })))
    const saved = await wsService.updateQuestionSetAgents(props.questionSet.id, payload)
    console.log('[RunSetupModal] Saved agent selection to database')
    isDirty.value = false
    if (saved && (!Array.isArray(saved.agents) || saved.agents.length === 0)) {
      saved.agents = payload
    }
    return saved || { id: props.questionSet.id, agents: payload }
  } catch (error) {
    console.error('[RunSetupModal] Failed to save to database:', error)
    alert('Failed to save agent selection: ' + (error?.message || 'Unknown error'))
    return null
  } finally {
    isSaving.value = false
  }
}

async function confirmRun() {
  if (selectedAgentIds.value.length === 0) return
  if (selectedEvaluatorsMissingTarget.value.length > 0) {
    const names = selectedEvaluatorsMissingTarget.value.map((a) => a.name).join(', ')
    alert(`Set a target agent before running for: ${names}.`)
    return
  }

  if (selectedPrimaryAgentIds.value.length === 0 && selectedEvaluatorAgentIds.value.length > 0) {
    const confirmed = confirm('Evaluator-only mode will use the latest run answers from this question set. Continue?')
    if (!confirmed) return
  }
  
  // Save selection to database before starting the run
  const savedQuestionSet = await saveToDatabase()
  if (!savedQuestionSet) return

  const agentsPayload = buildAgentsPayload()
  
  emit('save', {
    selectedIds: [...selectedAgentIds.value],
    agents: agentsPayload,
    questionSet: savedQuestionSet
  })

  emit('start', {
    questionSetId: props.questionSet.id,
    agentIds: [...selectedAgentIds.value],
    primaryAgentIds: [...selectedPrimaryAgentIds.value],
    evaluatorAgentIds: [...selectedEvaluatorAgentIds.value]
  })
}

async function saveSelection() {
  // Save selection to database
  const savedQuestionSet = await saveToDatabase()
  if (!savedQuestionSet) return
  const agentsPayload = buildAgentsPayload()
  
  emit('save', {
    selectedIds: [...selectedAgentIds.value],
    agents: agentsPayload,
    questionSet: savedQuestionSet
  })
}
</script>

<style scoped>
.run-setup-modal {
  width: 90%;
  max-width: 500px;
}

.modal-body {
  padding: 1.5rem;
}

.setup-section {
  margin-bottom: 1.5rem;
}

.section-label {
  display: block;
  font-weight: 600;
  color: #475569;
  margin-bottom: 0.5rem;
  font-size: 0.9rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.section-hint {
  font-size: 0.75rem;
  color: #64748b;
  margin-top: -0.25rem;
  margin-bottom: 0.75rem;
}

.info-box {
  display: flex;
  align-items: center;
  gap: 1rem;
  background: #f1f5f9;
  padding: 0.75rem 1rem;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}

.info-icon {
  font-size: 1.5rem;
}

.info-content {
  display: flex;
  flex-direction: column;
}

.info-content strong {
  color: #1e293b;
}

.info-meta {
  font-size: 0.8rem;
  color: #64748b;
}

.agents-checklist {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 300px;
  overflow-y: auto;
}

.agent-check-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.agent-check-item:hover {
  background: #f8fafc;
  border-color: #cbd5e1;
}

.agent-check-item.selected {
  background: #eff6ff;
  border-color: #bfdbfe;
}

.checkbox {
  font-size: 1.2rem;
  width: 24px;
}

.agent-info {
  display: flex;
  flex-direction: column;
}

.agent-name {
  font-weight: 500;
  color: #0f172a;
}

.agent-type {
  color: #64748b;
  text-transform: uppercase;
}

.target-agent-select {
  margin-top: 0.4rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: white;
  padding: 0.3rem 0.6rem;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
}

.target-agent-select label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
}

.target-agent-select select {
  font-size: 0.75rem;
  border: none;
  background: transparent;
  color: #1e293b;
  padding: 0;
  cursor: pointer;
}

.target-agent-select select:focus {
  outline: none;
}

.error-text {
  color: #ef4444;
  font-size: 0.85rem;
  margin-top: 0.5rem;
}

.warning-text {
  color: #b45309;
  font-size: 0.85rem;
  margin-top: 0.5rem;
}

.footer-actions {
  display: flex;
  gap: 0.75rem;
}

.btn-lg {
  padding: 0.75rem 1.5rem;
  font-size: 1rem;
}
.drag-handle {
  cursor: grab;
  color: #bdc3c7;
  font-size: 1.2rem;
  padding-right: 0.5rem;
  user-select: none;
}
</style>
