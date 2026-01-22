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
          <label class="section-label">Select Agents</label>
          <div class="agents-checklist">
            <div 
              v-for="(agent, index) in localAgents" 
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
            :disabled="selectedAgentIds.length === 0 || isSaving"
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

// Initialize local agents list (sorted by position)
const sortedAgents = [...props.agents].sort((a, b) => (a.position || 0) - (b.position || 0))
localAgents.value = sortedAgents

const selectedAgentIds = ref([])

// Initialize selected IDs from agents prop (which already reflects database state via mergedAgents)
// The agents prop comes from BenchmarkArena's mergedAgents which applies QS overrides
function initializeSelection() {
  // Primary source: check the 'enabled' field on each agent provided in props
  // These props already come pre-merged with QS overrides from BenchmarkArena
  const enabledIds = props.agents.filter(a => a.enabled).map(a => a.id)
  
  // If we have specific enabled agents, use them.
  // If ALL are disabled but we have overrides in the question set, we should respect that (keep 0 selected)
  const hasOverrides = props.questionSet?.agents && props.questionSet.agents.length > 0
  
  if (enabledIds.length > 0 || hasOverrides) {
    selectedAgentIds.value = enabledIds
    console.log(`[RunSetupModal] Initialized specialized selection. Enabled: ${enabledIds.length}, Has Overrides: ${hasOverrides}`)
  } else {
    // Fallback: if no agents enabled and NO overrides exist yet, default to all primary agents
    console.log('[RunSetupModal] No overrides found, defaulting to all active agents')
    selectedAgentIds.value = props.agents.filter(a => a.provider_type !== 'evaluator').map(a => a.id)
  }
}

// Watchers to keep local state in sync if props change while modal is open
watch(() => props.agents, (newAgents) => {
  if (newAgents) {
    const sorted = [...newAgents].sort((a, b) => (a.position || 0) - (b.position || 0))
    localAgents.value = sorted
    initializeSelection()
  }
}, { deep: true })

watch(() => props.questionSet, (newQs) => {
  if (newQs) {
    initializeSelection()
  }
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
  if (selectedAgentIds.value.includes(id)) {
    selectedAgentIds.value = selectedAgentIds.value.filter(aid => aid !== id)
  } else {
    selectedAgentIds.value.push(id)
  }
}

// Build payload for question set agents API
function buildAgentsPayload() {
  return localAgents.value.map((a, i) => ({
    agent_id: a.id,
    enabled: selectedAgentIds.value.includes(a.id),
    position: i,
    config: a.config || {}
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
    if (saved && !saved.agents) {
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
  
  // Save selection to database before starting the run
  const savedQuestionSet = await saveToDatabase()
  if (!savedQuestionSet) return
  
  emit('save', {
    selectedIds: selectedAgentIds.value,
    agents: localAgents.value,
    questionSet: savedQuestionSet
  })

  emit('start', {
    questionSetId: props.questionSet.id,
    agentIds: selectedAgentIds.value
  })
}

async function saveSelection() {
  // Save selection to database
  const savedQuestionSet = await saveToDatabase()
  if (!savedQuestionSet) return
  
  emit('save', {
    selectedIds: selectedAgentIds.value,
    agents: localAgents.value, // Pass reordered agents
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
  font-size: 0.7rem;
  color: #64748b;
  text-transform: uppercase;
}

.error-text {
  color: #ef4444;
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
