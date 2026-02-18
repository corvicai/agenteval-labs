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
          <label class="section-label">Active Agents For This Question Set</label>
          <p class="section-hint">Only these agents are stored as active for this question set.</p>

          <div v-if="isLoadingEnvelope" class="empty-text">Loading agent selection...</div>

          <div v-else class="agents-checklist">
            <div
              v-for="(agent, index) in selectedAgents"
              :key="agent.id"
              class="agent-check-item selected"
              draggable="true"
              @dragstart="onSelectedDragStart($event, index)"
              @dragover="onSelectedDragOver($event)"
              @drop="onSelectedDrop($event, index)"
            >
              <div class="drag-handle">⋮⋮</div>
              <div class="agent-info">
                <span class="agent-name">{{ agent.name }}</span>
                <span class="agent-type">{{ agent.provider_type }}</span>

                <div v-if="isEvaluatorAgent(agent)" class="target-agent-select">
                  <label>Target:</label>
                  <select v-model="agent.config.target_agent_id" @change="isDirty = true">
                    <option
                      v-for="target in selectedPrimaryAgents"
                      :key="target.id"
                      :value="target.id"
                    >
                      {{ target.name }}
                    </option>
                  </select>
                </div>
              </div>
              <button class="btn btn-ghost btn-remove" @click="removeAgent(agent.id)">Remove</button>
            </div>
          </div>

          <p v-if="!isLoadingEnvelope && selectedAgents.length === 0" class="error-text">
            ⚠️ Select at least one agent to run.
          </p>
          <p v-else-if="selectedPrimaryAgentIds.length === 0 && selectedEvaluatorAgentIds.length > 0" class="warning-text">
            ℹ️ Evaluator-only mode: this will evaluate answers from the latest run for this question set.
          </p>
          <p v-if="selectedEvaluatorsMissingTarget.length > 0" class="error-text">
            ⚠️ Select a target agent for evaluator(s): {{ selectedEvaluatorsMissingTarget.map(a => a.name).join(', ') }}.
          </p>
        </div>

        <div class="setup-section">
          <label class="section-label">Available Agents</label>
          <p class="section-hint">These workspace agents are available to add to this question set.</p>

          <div class="agents-checklist">
            <div
              v-for="agent in availableAgents"
              :key="agent.id"
              class="agent-check-item"
            >
              <div class="agent-info">
                <span class="agent-name">{{ agent.name }}</span>
                <span class="agent-type">{{ agent.provider_type }}</span>
              </div>
              <button class="btn btn-secondary btn-add" @click="addAgent(agent.id)">Add</button>
            </div>
          </div>

          <p v-if="availableAgents.length === 0" class="empty-text">No more agents available.</p>
        </div>
      </div>

      <div class="modal-footer">
        <button class="btn btn-secondary" @click="saveSelection" :disabled="isSaving || isLoadingEnvelope">
          {{ isSaving ? 'Saving...' : 'Save Selection' }}
        </button>
        <div class="footer-actions">
          <button class="btn btn-ghost" @click="$emit('cancel')" :disabled="isSaving">Cancel</button>
          <button
            class="btn btn-primary btn-lg"
            :disabled="selectedAgents.length === 0 || selectedEvaluatorsMissingTarget.length > 0 || isSaving || isLoadingEnvelope"
            @click="confirmRun"
          >
            {{ isSaving ? 'Saving...' : `Start Run (${selectedAgents.length})` }}
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

const selectedAgents = ref([])
const availableAgents = ref([])
const isSaving = ref(false)
const isLoadingEnvelope = ref(false)
const isDirty = ref(false)
const draggedSelectedIndex = ref(null)

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

function normalizeAgent(agent = {}, fallbackPosition = 0) {
  return {
    ...agent,
    id: String(agent.id || ''),
    position: Number.isFinite(agent.position) ? agent.position : fallbackPosition,
    config: cloneConfig(agent.config)
  }
}

function sortAgents(list = []) {
  return [...list].sort((a, b) => (a.position || 0) - (b.position || 0))
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

function normalizeSelectedPositions() {
  selectedAgents.value = selectedAgents.value.map((agent, index) => ({
    ...agent,
    position: index
  }))
}

function hydrateFromFallback() {
  const baseAgents = sortAgents((props.agents || []).map((agent, index) => normalizeAgent(agent, index)))
  const overrideRows = Array.isArray(props.questionSet?.agents) ? props.questionSet.agents : []
  const overrideIDs = overrideRows.map((item) => toAgentID(item)).filter(Boolean)

  if (overrideIDs.length > 0) {
    const overrideMap = {}
    overrideRows.forEach((row) => {
      const id = toAgentID(row)
      if (id) overrideMap[id] = row
    })

    const selected = []
    const available = []

    for (const base of baseAgents) {
      const row = overrideMap[base.id]
      if (row && row.enabled !== false) {
        selected.push({
          ...base,
          position: row.position !== undefined ? row.position : base.position,
          config: cloneConfig(row.config || base.config)
        })
      } else {
        available.push(base)
      }
    }

    selectedAgents.value = sortAgents(selected)
    normalizeSelectedPositions()
    availableAgents.value = sortAgents(available)
    return
  }

  const selected = baseAgents.filter((agent) => !!agent.enabled)
  const selectedIDSet = new Set(selected.map((agent) => agent.id))
  const available = baseAgents.filter((agent) => !selectedIDSet.has(agent.id))

  selectedAgents.value = sortAgents(selected)
  normalizeSelectedPositions()
  availableAgents.value = sortAgents(available)
}

async function loadEnvelope() {
  if (!props.questionSet?.id) {
    hydrateFromFallback()
    return
  }

  isLoadingEnvelope.value = true
  try {
    const response = await wsService.getQuestionSetAgentEnvelope(props.questionSet.id)
    const selected = sortAgents((response?.selected_agents || []).map((agent, index) => normalizeAgent(agent, index)))
    const selectedIDSet = new Set(selected.map((agent) => agent.id))
    const available = sortAgents((response?.available_agents || [])
      .map((agent, index) => normalizeAgent(agent, index))
      .filter((agent) => !selectedIDSet.has(agent.id)))

    selectedAgents.value = selected
    normalizeSelectedPositions()
    availableAgents.value = available
    isDirty.value = false
  } catch (error) {
    console.error('[RunSetupModal] Failed to load question-set agent envelope:', error)
    hydrateFromFallback()
  } finally {
    isLoadingEnvelope.value = false
  }
}

const selectedAgentIds = computed(() => selectedAgents.value.map((agent) => agent.id))

const selectedPrimaryAgents = computed(() =>
  selectedAgents.value.filter((agent) => !isEvaluatorAgent(agent))
)

const selectedPrimaryAgentIds = computed(() => selectedPrimaryAgents.value.map((agent) => agent.id))

const selectedEvaluatorAgents = computed(() =>
  selectedAgents.value.filter((agent) => isEvaluatorAgent(agent))
)

const selectedEvaluatorAgentIds = computed(() => selectedEvaluatorAgents.value.map((agent) => agent.id))

const selectedEvaluatorsMissingTarget = computed(() => {
  return selectedEvaluatorAgents.value.filter((agent) => !isTargetConfigured(agent))
})

const totalQuestions = computed(() => {
  if (!props.questionSet?.data) return 0
  let data = props.questionSet.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch {
      return 0
    }
  }
  return data.categories?.reduce((acc, cat) => acc + (cat.questions?.length || 0), 0) || 0
})

function addAgent(agentID) {
  const id = String(agentID || '')
  if (!id) return

  const index = availableAgents.value.findIndex((agent) => agent.id === id)
  if (index === -1) return

  const [agent] = availableAgents.value.splice(index, 1)
  selectedAgents.value.push({
    ...agent,
    position: selectedAgents.value.length
  })
  normalizeSelectedPositions()
  availableAgents.value = sortAgents(availableAgents.value)
  isDirty.value = true

  if (isEvaluatorAgent(agent) && !isTargetConfigured(agent)) {
    alert(`Evaluator "${agent.name}" requires a target agent. Select one before running.`)
  }
}

function removeAgent(agentID) {
  const id = String(agentID || '')
  if (!id) return

  const index = selectedAgents.value.findIndex((agent) => agent.id === id)
  if (index === -1) return

  const [agent] = selectedAgents.value.splice(index, 1)
  normalizeSelectedPositions()

  availableAgents.value = sortAgents([
    ...availableAgents.value,
    {
      ...agent,
      position: Number.isFinite(agent.position) ? agent.position : availableAgents.value.length
    }
  ])

  isDirty.value = true
}

function onSelectedDragStart(event, index) {
  draggedSelectedIndex.value = index
  event.dataTransfer.effectAllowed = 'move'
}

function onSelectedDragOver(event) {
  event.preventDefault()
  event.dataTransfer.dropEffect = 'move'
}

function onSelectedDrop(event, index) {
  event.preventDefault()
  const fromIndex = draggedSelectedIndex.value
  if (fromIndex === null || fromIndex === index) return

  const reordered = [...selectedAgents.value]
  const [moved] = reordered.splice(fromIndex, 1)
  reordered.splice(index, 0, moved)
  selectedAgents.value = reordered
  normalizeSelectedPositions()

  draggedSelectedIndex.value = null
  isDirty.value = true
}

function buildAgentsPayload() {
  return selectedAgents.value.map((agent, index) => ({
    agent_id: agent.id,
    enabled: true,
    position: index,
    config: cloneConfig(agent.config)
  }))
}

async function saveToDatabase() {
  if (!props.questionSet?.id) return null

  try {
    isSaving.value = true
    const payload = buildAgentsPayload()
    const saved = await wsService.updateQuestionSetAgents(props.questionSet.id, payload)
    isDirty.value = false

    if (saved && (!Array.isArray(saved.agents) || saved.agents.length === 0)) {
      saved.agents = payload
    }

    return saved || { id: props.questionSet.id, agents: payload }
  } catch (error) {
    console.error('[RunSetupModal] Failed to save agent selection:', error)
    alert('Failed to save agent selection: ' + (error?.message || 'Unknown error'))
    return null
  } finally {
    isSaving.value = false
  }
}

async function confirmRun() {
  if (selectedAgents.value.length === 0) return

  if (selectedEvaluatorsMissingTarget.value.length > 0) {
    const names = selectedEvaluatorsMissingTarget.value.map((agent) => agent.name).join(', ')
    alert(`Set a target agent before running for: ${names}.`)
    return
  }

  if (selectedPrimaryAgentIds.value.length === 0 && selectedEvaluatorAgentIds.value.length > 0) {
    const confirmed = confirm('Evaluator-only mode will use the latest run answers from this question set. Continue?')
    if (!confirmed) return
  }

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
  const savedQuestionSet = await saveToDatabase()
  if (!savedQuestionSet) return

  const agentsPayload = buildAgentsPayload()

  emit('save', {
    selectedIds: [...selectedAgentIds.value],
    agents: agentsPayload,
    questionSet: savedQuestionSet
  })
}

watch(
  () => props.questionSet?.id,
  async (nextID, prevID) => {
    if (!nextID) {
      hydrateFromFallback()
      return
    }
    if (nextID !== prevID || !isDirty.value) {
      await loadEnvelope()
    }
  },
  { immediate: true }
)

onMounted(() => {
  if (!props.questionSet?.id) {
    hydrateFromFallback()
  }
})
</script>

<style scoped>
.run-setup-modal {
  width: 90%;
  max-width: 620px;
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
  max-height: 260px;
  overflow-y: auto;
}

.agent-check-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.agent-check-item.selected {
  background: #eff6ff;
  border-color: #bfdbfe;
}

.agent-info {
  flex: 1;
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

.btn-add,
.btn-remove {
  min-width: 88px;
}

.drag-handle {
  cursor: grab;
  color: #bdc3c7;
  font-size: 1.2rem;
  padding-right: 0.5rem;
  user-select: none;
}

.empty-text {
  color: #64748b;
  font-size: 0.85rem;
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
</style>
