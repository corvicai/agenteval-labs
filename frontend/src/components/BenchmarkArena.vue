<template>
  <div class="benchmark-arena">
    <!-- Summary Section (if open) -->
    <SummarySection 
      v-if="showSummary && currentRun"
      :run="currentRun"
      :agents="agents"
      :results="runResults"
      @close="showSummary = false"
    />

    <!-- Start Run Error Toast -->
    <div v-if="startRunError" class="error-toast">
      <span class="icon">⚠️</span>
      <span class="message">{{ startRunError }}</span>
      <button class="close-btn" @click="startRunError = null">×</button>
    </div>

    <!-- Modals -->
    <RunSetupModal
       v-if="showRunSetup"
       :question-set="currentQuestionSet"
       :agents="mergedAgents"
       @start="handleStartRun"
       @save="handleRunSave"
       @cancel="showRunSetup = false"
    />

    <QuestionEditorModal 
        v-if="showQuestionEditor"
        :question-set="currentQuestionSet"
        :workspace-id="workspaceId"
        @close="onQuestionEditorClose"
        @saved="onQuestionSetSaved"
      />

    <!-- Questions Panel -->
    <div v-if="!isZenMode" class="questions-panel">
      <div class="questions-header-top">
        <h3>📋 Question Sets</h3>
        <div class="questions-header-actions">
           <button class="btn btn-secondary btn-sm" @click="createNewQuestionSet">
             <span class="icon">➕</span> New Set
           </button>
        </div>
      </div>
      
      <div class="qs-list">
        <div 
          v-for="qs in questionSets" 
          :key="qs.id" 
          class="qs-item" 
          :class="{ active: currentQuestionSet?.id === qs.id }"
          @click="selectQuestionSet(qs)"
        >
          <span class="qs-name">{{ qs.name }}</span>
          <span v-if="wsState.runningQuestionSetId === qs.id" class="running-indicator-dot"></span>
          <span class="qs-meta">{{ getQuestionCount(qs) }} qs</span>
          <div class="qs-actions">
            <button class="btn-icon-small" @click.stop="$emit('view-history', qs)" title="View History">📜</button>
          </div>
        </div>
      </div>
      <div class="questions-select-row">
        <select v-model="selectedQuestionId" class="questions-select">
          <option value="">All Questions</option>
          <option v-for="q in flatQuestions" :key="q.id" :value="q.id">
            {{ q.question.slice(0, 60) }}{{ q.question.length > 60 ? '...' : '' }}
          </option>
        </select>
        <button v-if="!agents.length" class="btn btn-warning" @click="$emit('configure-agents')">
          🤖 Configure Agents
        </button>
        <button v-else class="btn btn-primary" @click="startRunSetup" :disabled="isRunning">
          {{ isRunning ? '⏳ Running...' : '▶️ Run Benchmark' }}
        </button>
        <button class="btn btn-secondary btn-history-arena" @click="$emit('view-history', currentQuestionSet || {})">
          📚 History
        </button>
        <button class="btn btn-secondary" @click="showQuestionEditor = true" :disabled="!currentQuestionSet">
          ✏️ Edit Questions
        </button>
        <button class="btn btn-secondary btn-pdf" @click="exportToPdf" :disabled="!currentRun">
          📄 PDF
        </button>
        <button class="btn btn-secondary" @click="$emit('toggle-zen', true)">
          🧘 Zen
        </button>
        <button v-if="isRunning" class="btn btn-danger" @click="cancelBenchmark">
          ⛔ Cancel
        </button>
      </div>
    </div>

    <!-- Progress Bar -->
    <div v-if="isRunning && activeRunQuestionSetId === currentQuestionSet?.id" class="progress-bar">
      <div class="progress-fill-started" :style="{ width: progressPercentStarted + '%' }"></div>
      <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
      <span class="progress-text">{{ progressStatusText }}</span>
    </div>

    <!-- Loading Indicator -->
    <div v-if="isLoadingResults" class="loading-overlay">
      <div class="loading-spinner"></div>
      <span>Loading results...</span>
    </div>

    <!-- Chat Panels -->
    <div class="document-body">
      <div v-if="!currentQuestionSet" class="benchmarks-empty-state">
         <div class="empty-icon">📋</div>
         <h3>Select a Question Set</h3>
         <p>Choose a question set from the left panel to start benchmarking.</p>
      </div>

      <div v-else class="chat-container">
        <div class="chat-panels-bar">
          <div v-for="agent in displayAgents" :key="agent.id" class="chat-panel-wrapper">
            <ChatPanel 
              :agent-name="agent.name"
              :agent-id="agent.id"
              :agent-url="agent.config?.url || agent.config?.prompt_id || ''"
              :model="agent.config?.model || ''"
              :provider="agent.provider_type"
              :results="getAgentResults(agent.id)"
              :messages-ref="{ value: null }"
              :message-refs="{ value: {} }"
              :on-scroll="() => {}"
              :selected-question-id="selectedQuestionId"
              :history-by-question="{}" 
              :get-question-key="getQuestionKey"
              :on-validation="(idx, val) => onValidation(agent.id, idx, val)"
              :on-retry="(idx) => onRetry(agent.id, idx)"
              :extract-answer-text="extractAnswerText"
              :extract-answer-meta="extractAnswerMeta"
              @rerun="rerunQuestion"
              @rate="rateResult"
              :dev-mode="isDev"
              @show-details="(idx) => handleShowDetails(agent.id, idx)"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Details Modal -->
    <DetailsModal 
      :is-open="showDetailsModal" 
      :details="selectedDetails"
      @close="showDetailsModal = false"
    />

    <!-- Question Navigation -->
    <div v-if="flatQuestions.length > 1" class="question-navigation-floating">
      <button class="nav-btn-floating" @click="prevQuestion" :disabled="currentQuestionIndex <= 0">
        <span class="nav-icon">←</span>
        <span class="nav-label">Prev</span>
      </button>
      <div class="nav-current-floating">
        <span class="nav-index">{{ currentQuestionIndex + 1 }}</span>
        <span class="nav-total">of {{ flatQuestions.length }}</span>
      </div>
      <button class="nav-btn-floating" @click="nextQuestion" :disabled="currentQuestionIndex >= flatQuestions.length - 1">
        <span class="nav-icon">→</span>
        <span class="nav-label">Next</span>
      </button>
    </div>

    <!-- Zen Mode Exit Button -->
    <div v-if="isZenMode" class="zen-mode-exit-overlay">
      <button class="btn btn-secondary btn-exit-zen" @click="emit('toggle-zen', false)">
        ✕ Exit Zen Mode (Esc)
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import SummarySection from './SummarySection.vue'
import RunSetupModal from './RunSetupModal.vue'
import ChatPanel from './ChatPanel.vue'
import QuestionEditorModal from './QuestionEditorModal.vue'
import DetailsModal from './modals/DetailsModal.vue'
import PrintReport from './PrintReport.vue'
import wsService from '../services/websocket.js'
import { exportResultsReport } from '../utils/exporters.js'
import { downloadManager } from '../services/DownloadManager.js'
import { contentCache } from '../services/ContentCache.js'
import { useWSStore } from '../stores/wsStore'

const props = defineProps({
  workspaceId: String,
  agents: {
    type: Array,
    default: () => []
  },
  questionSets: {
    type: Array,
    default: () => []
  },
  initialQuestionSetId: String,
  isZenMode: Boolean
})

const emit = defineEmits(['update:currentQuestionSet', 'configure-agents', 'view-history', 'trigger-print', 'toggle-zen'])

watch(() => props.questionSets, (sets) => {
  console.log('[Arena] Question sets updated:', sets.length)
}, { immediate: true })

watch(() => props.agents, (agents) => {
  console.log('[Arena] Agents updated:', agents.length)
}, { immediate: true })

const wsStore = useWSStore()
const { state: wsState } = wsStore

// State
const currentQuestionSet = ref(null)
const previousQuestionSet = ref(null)
const currentRun = ref(null)
const runResults = ref({})
const isRunning = ref(false)
const activeRunQuestionSetId = ref(null)
const isLoadingResults = ref(false)
const startedTasks = ref(0)
const completedTasks = ref(0)
const totalTasks = ref(0)
const selectedQuestionId = ref('')
const showSummary = ref(false)
const showRunSetup = ref(false)
const showQuestionEditor = ref(false)
const showDetailsModal = ref(false)
const selectedDetails = ref(null)
const isDev = import.meta.env.DEV
const latestRunCache = new Map()
const pendingResultsBuffer = ref([])
const startRunError = ref(null)

// Init logic for Question Set
watch(() => props.questionSets, (sets) => {
  if (!sets || sets.length === 0) return
  
  if (!currentQuestionSet.value) {
     initQuestionSet(sets)
  } else {
    // Sync current set with updated data from props
    const updated = sets.find(s => s.id === currentQuestionSet.value.id)
    if (updated) {
      // console.log('[Arena] Syncing currentQuestionSet with updated props data')
      currentQuestionSet.value = updated
    }
  }
}, { immediate: true, deep: true })

// Watch for parent-driven selection changes
watch(() => props.initialQuestionSetId, (newId) => {
  if (newId && newId !== currentQuestionSet.value?.id) {
    console.log('[Arena] Parent changed question set ID:', newId)
    const found = props.questionSets.find(s => s.id === newId)
    if (found) {
      currentQuestionSet.value = found
    }
  }
})

function initQuestionSet(sets) {
    if (props.initialQuestionSetId) {
        const found = sets.find(s => s.id === props.initialQuestionSetId)
        if (found) {
            currentQuestionSet.value = found
            return
        }
    }
    // Fallback: localStorage
    const lastId = localStorage.getItem('lastQuestionSetId')
    if (lastId) {
        const found = sets.find(s => s.id === lastId)
        if (found) {
            currentQuestionSet.value = found
            return
        }
    }
    // Fallback: First one (or last created)
    // if (sets.length > 0) {
    //     currentQuestionSet.value = sets[sets.length - 1]
    // }
}

watch(currentQuestionSet, (newSet) => {
  emit('update:currentQuestionSet', newSet)
  if (newSet) {
    localStorage.setItem('lastQuestionSetId', newSet.id)
    if (!isRunning.value) {
       fetchLatestResultsForQS(newSet.id)
    }
  } else {
    localStorage.removeItem('lastQuestionSetId')
  }
})

// Computed
const flatQuestions = computed(() => {
  if (!currentQuestionSet.value?.data) return []
  
  let data = currentQuestionSet.value.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch (e) {
      console.error('Failed to parse question set data:', e)
      return []
    }
  }

  const questions = []
  const categories = data.categories || []
  for (let catIdx = 0; catIdx < categories.length; catIdx++) {
    const cat = categories[catIdx]
    const catQuestions = cat.questions || []
    for (let qIdx = 0; qIdx < catQuestions.length; qIdx++) {
      const q = catQuestions[qIdx]
      const questionText = q.question || q.text || ''
      // Generate ID matching backend format if not present: "{catIdx+1}-{qIdx+1}"
      const qId = q.id != null && q.id !== '' ? String(q.id) : `${catIdx + 1}-${qIdx + 1}`
      questions.push({ ...q, id: qId, category: cat.name, question: questionText })
    }
  }
  return questions
})

const currentQuestionIndex = computed(() => {
  if (!selectedQuestionId.value) return 0
  return flatQuestions.value.findIndex(q => q.id === selectedQuestionId.value)
})

// Granular progress: started tasks count for half (0.5x), completed count for full (1x)
// This gives visual feedback that tasks are running before they complete
const progressPercent = computed(() => {
  if (totalTasks.value === 0) return 0
  return Math.round((completedTasks.value / totalTasks.value) * 100)
})

const progressPercentStarted = computed(() => {
  if (totalTasks.value === 0) return 0
  // Started but not completed tasks contribute half progress
  const inProgress = startedTasks.value - completedTasks.value
  const inProgressContribution = (inProgress / totalTasks.value) * 50 // 0.5x weight
  const completedContribution = (completedTasks.value / totalTasks.value) * 100
  return Math.min(100, Math.round(completedContribution + inProgressContribution))
})

const progressStatusText = computed(() => {
  const inProgress = startedTasks.value - completedTasks.value
  if (completedTasks.value === 0 && inProgress > 0) {
    return `${inProgress} task${inProgress > 1 ? 's' : ''} running... (${progressPercentStarted.value}%)`
  }
  if (inProgress > 0) {
    return `${completedTasks.value}/${totalTasks.value} done, ${inProgress} running (${progressPercentStarted.value}%)`
  }
  return `${completedTasks.value}/${totalTasks.value} tasks (${progressPercent.value}%)`
})

const mergedAgents = computed(() => {
  if (!props.agents || props.agents.length === 0) return []
  
  const qs = currentQuestionSet.value
  if (!qs) return props.agents

  // If the question set has NO agents array at all, it's definitely uninitialized
  if (!qs.agents || !Array.isArray(qs.agents)) {
    console.log(`[Arena] mergedAgents: No agents array for QS "${qs.name}", using defaults`)
    return props.agents
  }

  // If it has an agents array but it's empty, we must decide:
  // Is it empty because nothing was ever saved, or because the user saved "nothing enabled"?
  // Usually, saveSelection sends ALL agents. So an empty array means "never saved".
  if (qs.agents.length === 0) {
    console.log(`[Arena] mergedAgents: Empty agents array for QS "${qs.name}", using defaults`)
    return props.agents
  }

  console.log(`[Arena] mergedAgents: Merging ${qs.agents.length} overrides for QS "${qs.name}"`)

  // Map of overrides for fast lookup
  const overrideMap = {}
  qs.agents.forEach(oa => {
    // Check both agent_id and agentID just in case of inconsistency
    const aid = oa.agent_id || oa.agentID
    if (aid) overrideMap[aid] = oa
  })

  const merged = props.agents.map(a => {
    const override = overrideMap[a.id]
    if (override) {
      return {
        ...a,
        enabled: override.enabled !== undefined ? !!override.enabled : !!a.enabled,
        position: override.position !== undefined ? override.position : a.position,
        config: override.config || a.config
      }
    }
    // If not in overrides list (e.g. new agent added to workspace after this QS was saved),
    // respect its global enabled state instead of forcing it false.
    // This fixed the "I added an agent but it's disabled" confusion.
    return { ...a, enabled: a.enabled }
  })

  // console.log(`[Arena] mergedAgents: result enabled count:`, merged.filter(a => a.enabled).length)
  return merged
})

const enabledAgents = computed(() => mergedAgents.value.filter(a => a.enabled))

const displayAgents = computed(() => {
  let list = []
  
  if (isRunning.value && currentRun.value?.agentIds) {
    // 1. If running, show agents participating in this specific run
    const runIds = currentRun.value.agentIds
    list = mergedAgents.value.filter(a => runIds.includes(a.id))
    
    // Fallback for agents not in props.agents
    const missingIds = runIds.filter(id => !list.some(a => a.id === id))
    if (missingIds.length > 0) {
      const missing = missingIds.map(id => {
        const globalAgent = props.agents.find(ga => ga.id === id)
        return globalAgent || { id, name: 'Agent (loading...)', provider_type: 'unknown' }
      })
      list = [...list, ...missing]
    }
  } else {
// 2. Not running - check if we have results to show (History mode)
    const resultAgentIds = Object.keys(runResults.value || {})
    if (resultAgentIds.length > 0) {
      // Filter enabledAgents to only those in results OR explicitly enabled for NEXT run
      // The user wants deselected agents to disappear from this screen.
      const agentsWithResults = mergedAgents.value.filter(a => resultAgentIds.includes(a.id) && a.enabled)
      const oldAgentIds = resultAgentIds.filter(id => !mergedAgents.value.some(a => a.id === id))
      const oldAgents = oldAgentIds.map(id => {
        const found = mergedAgents.value.find(a => a.id === id)
        return found || { id, name: 'Agent (historical)', provider_type: 'unknown' }
      })
      list = [...agentsWithResults, ...oldAgents]
    } else {
      // 3. Setup mode - show locally enabled agents
      list = [...enabledAgents.value]
    }
  }

  const isEvaluator = (a) => a.provider_type === 'openai' || a.provider_type === 'evaluator'
  return list.sort((a, b) => {
    if (isEvaluator(a) && !isEvaluator(b)) return 1
    if (!isEvaluator(a) && isEvaluator(b)) return -1
    return (a.position || 0) - (b.position || 0)
  })
})

// Methods
function selectQuestionSet(qs) {
    currentQuestionSet.value = qs
}

function getQuestionCount(set) {
  if (!set || !set.data) return 0
  let data = set.data
  if (typeof data === 'string') {
    try { data = JSON.parse(data) } catch (e) { return 0 }
  }
  if (!data.categories) return 0
  return data.categories.reduce((acc, cat) => acc + (cat.questions ? cat.questions.length : 0), 0)
}

function startRunSetup() {
  if (!currentQuestionSet.value) return
  
  const primaryAgents = enabledAgents.value.filter(a => a.provider_type !== 'evaluator')
  
  if (primaryAgents.length === 0) {
    console.warn('[Arena] Start blocked. Enabled count:', enabledAgents.value.length, 'Primary count:', primaryAgents.length)
    console.log('[Arena] Merged Agents dump:', mergedAgents.value)
    alert('Please enable at least one primary agent to start.')
    emit('configure-agents')
    return
  }
  showRunSetup.value = true
}

function createNewQuestionSet() {
  previousQuestionSet.value = currentQuestionSet.value
  currentQuestionSet.value = null 
  showQuestionEditor.value = true
}

function onQuestionEditorClose() {
  if (currentQuestionSet.value === null && previousQuestionSet.value) {
    currentQuestionSet.value = previousQuestionSet.value
  }
  previousQuestionSet.value = null
  showQuestionEditor.value = false
}

function onQuestionSetSaved(updated) {
  currentQuestionSet.value = updated
  previousQuestionSet.value = null
  showQuestionEditor.value = false
}

function prevQuestion() {
  const idx = currentQuestionIndex.value
  if (idx > 0) {
    selectedQuestionId.value = flatQuestions.value[idx - 1].id
    if (!props.isZenMode) emit('toggle-zen', true)
  }
}

function nextQuestion() {
  const idx = currentQuestionIndex.value
  if (idx < flatQuestions.value.length - 1) {
    selectedQuestionId.value = flatQuestions.value[idx + 1].id
    if (!props.isZenMode) emit('toggle-zen', true)
  }
}

function getQuestionKey(text) { return text }

function extractAnswerText(result) {
  if (typeof result === 'string') return result
  return result?.answer || ''
}

function extractAnswerMeta(result) {
  return {
    duration: result?.duration,
    timestamp: result?.timestamp,
    error: result?.error,
    loading: result?.loading
  }
}

function handleShowDetails(agentId, index) {
  const results = getAgentResults(agentId)
  const item = results[index]
  if (!item) return

  selectedDetails.value = {
    run_id: currentRun.value?.id,
    agent_id: agentId,
    question_id: item.question?.id,
    result_id: item.id,
    question_set_id: currentQuestionSet.value?.id,
    question: item.question,
    result: item,
    metadata: item.metadata
  }
  showDetailsModal.value = true
}

// Data Fetching & WebSockets
async function fetchLatestResultsForQS(qsId) {
  if (!props.workspaceId) return
  isLoadingResults.value = true
  downloadManager.cancelAll()

  try {
    const cached = getCachedRunForQS(qsId)
    if (cached) {
      applyRunLiteData(cached)
      return
    }

    const data = await wsService.getLatestRunByQuestionSet(qsId)
    setCachedRunForQS(qsId, data)
    applyRunLiteData(data)
  } catch (e) {
    console.error('[Arena] Failed to load latest results:', e)
  } finally {
    isLoadingResults.value = false
  }
}

function getRecentRunIdForQS(qsId) {
  const runs = wsState.recentRuns || []
  const latest = runs.find(r => r.question_set_id === qsId && r.status !== 'running')
  return latest ? latest.id : null
}

function getCachedRunForQS(qsId) {
  const cached = latestRunCache.get(qsId)
  if (!cached) return null

  const recentRunId = getRecentRunIdForQS(qsId)
  if (recentRunId && cached.runId !== recentRunId) return null

  return cached.data
}

function setCachedRunForQS(qsId, data) {
  latestRunCache.set(qsId, { data, runId: data?.run?.id || null })
}

function applyRunLiteData(data) {
  runResults.value = {}
  currentRun.value = null
  totalTasks.value = 0
  completedTasks.value = 0

  if (!data || !data.run || !data.run.id) {
    return
  }

  currentRun.value = data.run
  const skeletonResults = {}
  const allResultIds = []

  data.results.forEach(res => {
    const agentId = res.agent_id
    const qIdStr = String(res.question_id)

    if (!skeletonResults[agentId]) skeletonResults[agentId] = {}

    const cached = contentCache.get(res.content_hash)

    skeletonResults[agentId][qIdStr] = {
      id: res.id,
      content_hash: res.content_hash,
      loading: !cached,
      success: res.status === 'success',
      answer: cached ? cached.answer : '',
      error: res.status === 'error' ? 'Error in run' : null,
      duration: res.duration_ms / 1000,
      timestamp: res.created_at,
      evaluations: cached ? (cached.evaluations || []) : [],
      humanValidation: cached ? cached.evaluations?.find(e => e.rater_type === 'user')?.rating : null
    }

    if (!cached) allResultIds.push(res.id)
  })

  runResults.value = skeletonResults
  totalTasks.value = data.run.total_tasks || 0
  completedTasks.value = data.run.total_tasks || 0

  if (allResultIds.length > 0) {
    downloadManager.enqueue(allResultIds)
  }

  if (selectedQuestionId.value) {
    prioritizeQuestionInQueue(selectedQuestionId.value)
  }
}

watch(() => wsState.recentRuns, () => {
  if (!currentQuestionSet.value || isRunning.value || isLoadingResults.value) return
  const qsId = currentQuestionSet.value.id
  const latestId = getRecentRunIdForQS(qsId)
  const cached = latestRunCache.get(qsId)
  if (latestId && cached?.runId !== latestId) {
    fetchLatestResultsForQS(qsId)
  }
}, { deep: true })

function getAgentResults(agentId) {
  const results = runResults.value[agentId] || {}
  
  // Filter questions if a specific one is selected
  const targetQuestions = selectedQuestionId.value 
    ? flatQuestions.value.filter(q => String(q.id) === String(selectedQuestionId.value))
    : flatQuestions.value

  const isAgentRunning = isRunning.value && 
                       activeRunQuestionSetId.value === currentQuestionSet.value?.id && 
                       currentRun.value?.agentIds?.includes(agentId)

  return targetQuestions.map(q => {
    const qIdStr = String(q.id)
    const res = results[qIdStr]
    const ratingMap = { 'like': 'positive', 'dislike': 'negative', 'valid': 'alternative', 'wrong': 'partial' }
    return {
      question: q,
      answer: res ? res.answer : null,
      loading: res ? res.loading : isAgentRunning,
      queued: res ? res.queued : false,
      duration: res ? res.duration : null,
      timestamp: res ? res.timestamp : null,
      id: res ? res.id : null,
      error: res ? res.error : null,
      evaluations: res ? (res.evaluations || []) : [],
      humanValidation: res ? (ratingMap[res.humanValidation] || res.humanValidation) : null
    }
  })
}

function prioritizeQuestionInQueue(questionId) {
    if (!runResults.value) return
    const idsToPrioritize = []
    for (const agentId in runResults.value) {
        const agentResults = runResults.value[agentId]
        const qIdStr = String(questionId)
        if (agentResults[qIdStr]) {
            idsToPrioritize.push(agentResults[qIdStr].id)
        }
    }
    if (idsToPrioritize.length > 0) {
        downloadManager.prioritize(idsToPrioritize[0])
    }
}

watch(selectedQuestionId, (newId) => {
  if (newId) prioritizeQuestionInQueue(newId)
})

// Actions
async function handleStartRun({ questionSetId, agentIds }) {
  showRunSetup.value = false
  if (flatQuestions.value.length === 0) {
    alert('The current question set is empty.')
    return
  }
  
  const selectedAgentsCount = agentIds.length
  isRunning.value = true
  activeRunQuestionSetId.value = questionSetId
  wsStore.setRunningQuestionSetId(questionSetId)
  startedTasks.value = 0
  completedTasks.value = 0
  totalTasks.value = selectedAgentsCount * flatQuestions.value.length
  runResults.value = {}
  
  try {
    const result = await wsService.startRun(questionSetId, agentIds)
    currentRun.value = { id: result.run_id || result.id, status: 'running', agentIds }
    localStorage.setItem('activeRunId', currentRun.value.id)
    
    // Process buffered results
    if (pendingResultsBuffer.value.length > 0) {
      console.log(`[Arena] Processing ${pendingResultsBuffer.value.length} buffered results for run ${currentRun.value.id}`)
      pendingResultsBuffer.value.forEach(data => {
        if (data.run_id === currentRun.value.id) {
          processTaskCompleted(data)
        }
      })
      pendingResultsBuffer.value = []
    }
  } catch (e) {
    console.error('Failed to start run:', e)
    // Show toast error
    startRunError.value = e.message || 'Failed to start run. Please check your agent configurations.'
    // Auto-clear after 5 seconds
    setTimeout(() => {
      if (startRunError.value) startRunError.value = null
    }, 5000)
    
    isRunning.value = false
    pendingResultsBuffer.value = []
  }

}

function processTaskCompleted(data) {
    completedTasks.value++
    if (!runResults.value[data.agent_id]) runResults.value[data.agent_id] = {}
    
    runResults.value[data.agent_id] = {
        ...runResults.value[data.agent_id],
        [data.question_id]: {
            id: data.run_result_id,
            loading: false,
            success: data.success,
            answer: data.answer,
            error: data.error,
            duration: data.duration_ms / 1000,
            timestamp: new Date().toISOString(),
            evaluations: data.evaluations || []
        }
    }

    // Check if run completed for this question set
    if (isRunning.value && completedTasks.value === totalTasks.value) {
       isRunning.value = false
       if (activeRunQuestionSetId.value === currentQuestionSet.value?.id) {
         wsStore.setRunningQuestionSetId(null)
       }
    }
}

async function handleRunSave(payload) {
  const savedQuestionSet = payload?.questionSet
  if (!currentQuestionSet.value || !savedQuestionSet) return

  console.log('[Arena] handleRunSave: Received saved QS with', savedQuestionSet.agents?.length, 'agents')

  if (savedQuestionSet.id === currentQuestionSet.value.id) {
    // Extract agents carefully
    const newAgents = savedQuestionSet.agents || payload?.agents || currentQuestionSet.value.agents
    
    currentQuestionSet.value = {
      ...currentQuestionSet.value,
      ...savedQuestionSet,
      agents: newAgents
    }
    console.log('[Arena] handleRunSave: Updated currentQuestionSet.agents count:', currentQuestionSet.value.agents?.length)
  }
}

async function cancelBenchmark() {
  if (!currentRun.value) return
  try {
    await wsService.cancelRun(currentRun.value.id)
    isRunning.value = false
    currentRun.value.status = 'cancelled'
  } catch (e) {
    console.error('Failed to cancel run:', e)
  }
}

async function rerunQuestion(agentId, questionId) {
  if (!currentRun.value) return
  
  // Optimistic update
  const qIdStr = String(questionId)
  if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
     runResults.value[agentId][qIdStr].loading = true
     runResults.value[agentId][qIdStr].error = null
  }

  // Find the question object to get full context
  const question = flatQuestions.value.find(q => String(q.id) === qIdStr)
  const resultItem = runResults.value[agentId]?.[qIdStr]

  // Determine if this is an evaluator and if we need to target another result
  let resultIdToUse = resultItem?.id
  const agent = mergedAgents.value.find(a => a.id === agentId)
      
  if (agent && (agent.provider_type === 'evaluator' || agent.config?.target_agent_id)) {
      // It's an evaluator. Use heuristic to find the target answer.
      const targetAgentId = agent.config?.target_agent_id
      
      // Look for candidates in runResults
      // runResults structure is { agentId: { qId: { ... } } }
      const candidates = []
      
      for (const aid in runResults.value) {
          if (aid === agentId) continue
          const res = runResults.value[aid][qIdStr]
          if (res && res.answer) {
              candidates.push({ ...res, agent_id: aid })
          }
      }
      
      let targetMatch = null
      if (targetAgentId) {
          targetMatch = candidates.find(c => c.agent_id === targetAgentId)
      } else if (candidates.length === 1) {
          targetMatch = candidates[0]
      } else if (candidates.length > 0) {
          // Ambiguous
          targetMatch = candidates[0] 
      }

      if (targetMatch) {
          console.log(`[Frontend] Resolved target result for evaluator retry: ${targetMatch.id} (Agent ${targetMatch.agent_id})`)
          resultIdToUse = targetMatch.id
      } else {
          console.warn('[Frontend] Could not resolve target result for evaluator retry. Letting backend heuristic handle it.')
          resultIdToUse = '' // Reset to empty so backend heuristic kicks in
      }
  }

  try {
    await wsService.rerunTask(currentRun.value.id, agentId, questionId, {
      questionSetId: currentQuestionSet.value?.id,
      resultId: resultIdToUse,
      originalQuestion: question?.question || '',
      expectedAnswer: question?.expected || question?.expected_answer || ''
    })
  } catch (e) {
    console.error('Failed to rerun:', e)
     // Revert on error
      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
         runResults.value[agentId][qIdStr].loading = false
      }
  }
}

async function rateResult(resultId, rating) {
  try {
    return await wsService.createEvaluation(resultId, rating)
  } catch (e) {
    console.error('Failed to rate:', e)
    return null
  }
}

function onValidation(agentId, index, validation) {
  const results = getAgentResults(agentId)
  const result = results[index]
  if (result && result.id) {
    const qIdStr = String(result.question.id)
    rateResult(result.id, validation).then(newEval => {
      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
        const item = runResults.value[agentId][qIdStr]
        item.humanValidation = validation
        if (newEval) {
          if (!item.evaluations) item.evaluations = []
          const existingIdx = item.evaluations.findIndex(e => e.rater_type === 'user')
          if (existingIdx !== -1) {
            item.evaluations[existingIdx] = newEval
          } else {
            item.evaluations.push(newEval)
          }
        }
      }
    })
  }
}

function onRetry(agentId, index) {
  const question = flatQuestions.value[index]
  if (question && currentRun.value) {
    rerunQuestion(agentId, question.id)
  }
}

// Global Listeners for THIS component
// We need to listen to WS events to update live results
onMounted(async () => {
    // Check for active run restoration
    const activeRunId = localStorage.getItem('activeRunId')
    if (activeRunId) {
        try {
            const data = await wsService.getRunDetails(activeRunId)
            if (data && data.run && data.run.status === 'running') {
                const runAgentIds = data.agents ? Object.keys(data.agents) : []
                currentRun.value = { ...data.run, agentIds: runAgentIds }
                console.log('Restored active run:', activeRunId, 'with agents:', runAgentIds)
                isRunning.value = true
                totalTasks.value = data.run.total_tasks
                
                if (data.results) {
                    const restored = {}
                    data.results.forEach(res => {
                        const agentId = res.agent_id
                        const qIdStr = String(res.question_id)
                        if (!restored[agentId]) restored[agentId] = {}
                        restored[agentId][qIdStr] = {
                             id: res.id,
                             loading: false,
                             success: res.status === 'success',
                             answer: res.answer,
                             error: res.status === 'error' ? 'Error' : null,
                             duration: res.duration_ms / 1000,
                             timestamp: res.created_at,
                             evaluations: res.evaluations || [],
                             humanValidation: res.evaluations?.find(e => e.rater_type === 'user')?.rating
                        }
                    })
                    runResults.value = restored
                    completedTasks.value = data.results.length
                 }
                 
                 if (data.question_set && (!currentQuestionSet.value || currentQuestionSet.value.id !== data.question_set.id)) {
                     currentQuestionSet.value = data.question_set
                 }
            } else if (activeRunId) {
                 localStorage.removeItem('activeRunId')
            }
        } catch (e) { console.error('Failed to restore active run:', e) }
    }

    wsService.on('EVT_TASK_QUEUED', (data) => {
        const agentId = data.agent_id
        const qIdStr = String(data.question_id)
        
        if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
            runResults.value[agentId][qIdStr].queued = true
            runResults.value[agentId][qIdStr].loading = true
            runResults.value[agentId][qIdStr].error = null
        }
    })

    wsService.on('EVT_TASK_STARTED', (data) => {
        if (isRunning.value) {
            startedTasks.value++
        }
        
        // Update specific item status if it exists (for reruns)
        // This should happen regardless of global isRunning state
        const agentId = data.agent_id
        const qIdStr = String(data.question_id)
        
        if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
            runResults.value[agentId][qIdStr].queued = false
            runResults.value[agentId][qIdStr].loading = true
            runResults.value[agentId][qIdStr].error = null
        }
    })

    wsService.on('EVT_TASK_COMPLETED', (data) => {
        if (!currentRun.value) {
            if (isRunning.value) {
                console.log('[Arena] Buffering result for pending run:', data.run_id)
                pendingResultsBuffer.value.push(data)
            }
            return
        }
        if (data.run_id !== currentRun.value.id) return
        processTaskCompleted(data)
    })

    wsService.on('DATA_RESULT_DETAILS', (payload) => {
        if (!payload.results || !currentRun.value?.id) return
        const runId = String(currentRun.value.id)
        payload.results.forEach(res => {
            if (String(res.run_id) !== runId) return
            const agentId = res.agent_id
            const qIdStr = String(res.question_id)
            if (!runResults.value[agentId]) return // Agent not in current results
            
            const skeleton = runResults.value[agentId][qIdStr]
            // Only update if the skeleton exists and has the same result ID
            // This prevents stale downloads from overwriting correct data
            if (!skeleton || skeleton.id !== res.id) return
            
            if (skeleton.content_hash) {
                contentCache.set(skeleton.content_hash, {
                    answer: res.answer,
                    evaluations: res.evaluations
                })
            }
            runResults.value[agentId][qIdStr] = {
                id: res.id,
                content_hash: skeleton.content_hash,
                loading: false,
                success: res.status === 'success',
                answer: res.answer,
                error: res.status === 'error' ? 'Error' : null,
                duration: res.duration_ms / 1000,
                timestamp: res.created_at,
                evaluations: res.evaluations || [],
                humanValidation: res.evaluations?.find(e => e.rater_type === 'user')?.rating,
            }
        })
    })

    wsService.on('EVT_RUN_FINISHED', () => {
        isRunning.value = false
        localStorage.removeItem('activeRunId')
    })
})

function exportToPdf() {
  if (!currentRun.value) return
  
  // Build agents array from displayAgents with their results
  const agentsArray = displayAgents.value.map(agent => ({
    id: agent.id,
    name: agent.name || agent.config?.name || 'Agent',
    provider: agent.provider_type,
    config: agent.config,
    results: getAgentResults(agent.id)
  }))

  const pData = exportResultsReport({
    agentsRef: agentsArray,
    calculateStats: calculateStats
  })
  
  if (!pData) {
    console.error('Export failed: No data returned')
    return
  }
  
  emit('trigger-print', {
    workspaceName: currentQuestionSet.value?.name || 'Benchmark',
    summary: pData.summary,
    results: pData.results
  })
}

// Removing local triggerBrowserPrint as it's handled by parent App.vue

function calculateStats(results) {
  if (!results || !Array.isArray(results)) return {}
  
  let answered = 0
  let errors = 0
  let total = results.length
  let totalDuration = 0
  let validations = { positive: 0, negative: 0, alternative: 0, partial: 0, notEvaluated: 0 }
  
  results.forEach(r => {
    if (r.answer || r.error) answered++
    if (r.error) errors++
    if (r.duration) totalDuration += parseFloat(r.duration) || 0
    
    if (r.humanValidation) {
      const v = r.humanValidation.toLowerCase()
      validations[v] = (validations[v] || 0) + 1
    } else {
      validations.notEvaluated++
    }
  })
  
  return {
    answered,
    totalQuestions: total,
    errors,
    avgDuration: answered ? (totalDuration / answered).toFixed(2) : 0,
    validations,
    percentages: {
      positive: total ? Math.round((validations.positive || 0) / total * 100) : 0,
      negative: total ? Math.round((validations.negative || 0) / total * 100) : 0,
      alternative: total ? Math.round((validations.alternative || 0) / total * 100) : 0,
      partial: total ? Math.round((validations.partial || 0) / total * 100) : 0,
    }
  }
}

onUnmounted(() => {
    // Remove listeners? wsService might need off() or we just accept they pile up if not careful?
    // wsService is global singleton, we MUST remove listeners to avoid duplication/memory leak
    // Current wsService implemention might not support proper off() or ID based clear
    // But let's assume standard behavior or add a way.
    // Ideally we should use a composite subscription that cleans itself up.
    // For now, doing nothing as App.vue didn't do it either except on logout.
})

// Expose methods that parent might need?
defineExpose({
    initQuestionSet
})
</script>

<style scoped>
.benchmark-arena {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

.document-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden; /* Container doesn't scroll, child bar does */
  padding: 1rem;
  background: #f8fafc;
  min-height: 0;
}

/* Scoped styles that were specifically for the runner/arena */
/* Zen Mode Exit Overlay */
.zen-mode-exit-overlay {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 2000;
  pointer-events: auto;
}

.btn-exit-zen {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(8px);
  border: 1px solid #e2e8f0;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  color: #475569;
  font-weight: 600;
  padding: 0.6rem 1.2rem;
  border-radius: 999px;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  transition: all 0.2s;
}

.btn-exit-zen:hover {
  background: white;
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.15);
  color: #1e293b;
}

/* Running Indicator Dot */
.running-indicator-dot {
  width: 8px;
  height: 8px;
  background-color: #ef4444;
  border-radius: 50%;
  display: inline-block;
  margin: 0 4px;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.5);
  animation: pulse-dot 1.5s infinite;
}

@keyframes pulse-dot {
  0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.7); }
  70% { transform: scale(1); box-shadow: 0 0 0 5px rgba(239, 68, 68, 0); }
  100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(239, 68, 68, 0); }
}

.chat-container {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.chat-panels-bar {
  display: flex;
  gap: 1.5rem;
  overflow-x: auto;
  padding: 1rem 0 2rem 0;
  flex: 1;
}

.chat-panel-wrapper {
  min-width: 450px;
  flex: 1;
  display: flex;
}

.loading-overlay {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  gap: 1rem;
}

/* Add any other specific styles from App.css if you want them scoped, 
   otherwise they inherit from global App.css */

.error-toast {
  position: fixed;
  top: 20px;
  left: 50%;
  transform: translateX(-50%);
  background: #fee2e2;
  color: #7f1d1d;
  padding: 1rem 1.5rem;
  border-radius: 8px;
  border: 1px solid #fca5a5;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  z-index: 9999;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-weight: 500;
  max-width: 90vw;
  animation: slide-down 0.3s ease-out;
}

.error-toast .icon {
  font-size: 1.25rem;
}

.error-toast .close-btn {
  background: transparent;
  border: none;
  color: #991b1b;
  font-size: 1.5rem;
  line-height: 1;
  padding: 0;
  margin-left: 0.5rem;
  cursor: pointer;
  opacity: 0.7;
  transition: opacity 0.2s;
}

.error-toast .close-btn:hover {
  opacity: 1;
}

@keyframes slide-down {
  from { top: -50px; opacity: 0; }
  to { top: 20px; opacity: 1; }
}

</style>
