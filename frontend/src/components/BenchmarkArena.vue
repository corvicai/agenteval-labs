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

    <!-- Left Sidebar with Tabs -->
    <LeftSidebar
      :question-sets="questionSets"
      :current-question-set="currentQuestionSet"
      :agents="mergedAgents"
      :running-question-set-id="wsState.runningQuestionSetId"
      :workspace-id="workspaceId"
      @create-question-set="createNewQuestionSet"
      @select-question-set="selectQuestionSet"
      @question-set-updated="handleQuestionSetUpdated"
    />

    
    <!-- Main Content Area -->
    <div class="benchmark-arena-content">
    <!-- Action Buttons Row -->
    <div class="action-buttons-row">
      <button class="btn btn-primary" @click="startRunSetup" :disabled="isRunning || !currentQuestionSet">
        {{ isRunning ? '⏳ Running...' : '▶️ Run Benchmark' }}
      </button>
      <button class="btn btn-secondary btn-pdf" @click="exportToPdf" :disabled="!currentRun">
        📄 PDF
      </button>
      <button class="btn btn-secondary" @click="reloadResults" title="Reload Results">
        🔄 Reload Results
      </button>
      <button v-if="isRunning" class="btn btn-danger" @click="cancelBenchmark">
        ⛔ Cancel
      </button>
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

      <!-- Main Content Area: Questions + Chat Panels side by side -->
      <div v-else-if="flatQuestions.length > 0" class="main-content-area">
        <!-- Arena Label -->
        
        
        <!-- Questions List View -->
        <div class="questions-list-view">
          <div class="questions-list-header">
            <h2>{{ currentQuestionSet.name }}</h2>
            <p class="questions-count">{{ flatQuestions.length }} question{{ flatQuestions.length !== 1 ? 's' : '' }}</p>
          </div>
          <div class="questions-list-container">
            <div 
              v-for="(question, index) in flatQuestions" 
              :key="question.id || index"
              class="question-item"
              :class="{ 'selected': selectedQuestionId === question.id }"
              @click="selectedQuestionId = question.id"
            >
              <div class="question-number">Q{{ index + 1 }}</div>
              <div class="question-content">
                <div class="question-text">{{ question.question || question.text }}</div>
                <div v-if="question.category" class="question-category">{{ question.category }}</div>
              </div>
              <div class="question-actions">
                <span class="question-status" :class="getQuestionStatus(question.id)">
                  {{ getQuestionStatusText(question.id) }}
                </span>
                <button 
                  v-if="hasQuestionBeenRun(question.id)"
                  class="btn-retry" 
                  @click.stop="retryQuestionForAllAgents(question.id)"
                  title="Retry this question"
                >
                  🔄 Retry
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Chat Panels (when agents/results are available) -->
        <div v-if="displayAgents.length > 0" class="chat-container">
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

      <!-- Fallback: Chat Panels only (when results are available but no questions) -->
      <div v-else-if="hasResults && displayAgents.length > 0" class="chat-container">
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

    </div>
    <!-- Details Modal -->
    <DetailsModal 
      :is-open="showDetailsModal" 
      :details="selectedDetails"
      @close="showDetailsModal = false"
    />
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import SummarySection from './SummarySection.vue'
import RunSetupModal from './RunSetupModal.vue'
import ChatPanel from './ChatPanel.vue'
import DetailsModal from './modals/DetailsModal.vue'
import PrintReport from './PrintReport.vue'
import LeftSidebar from './LeftSidebar.vue'
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
  initialQuestionSetId: String
})

const emit = defineEmits(['update:currentQuestionSet', 'trigger-print'])

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

// Watch for workspaceId changes to trigger fetch if it was skipped
watch(() => props.workspaceId, (newId) => {
  if (newId && currentQuestionSet.value && !isRunning.value) {
    console.log('[Arena] WorkspaceID arrived, fetching results for QS:', currentQuestionSet.value.id)
    fetchLatestResultsForQS(currentQuestionSet.value.id)
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

const hasResults = computed(() => {
  return runResults.value && Object.keys(runResults.value).length > 0
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

// Check if a question has been run (has results from any agent)
function hasQuestionBeenRun(questionId) {
  if (!runResults.value || !questionId) return false
  const qIdStr = String(questionId)
  
  for (const agentId in runResults.value) {
    const agentResults = runResults.value[agentId]
    if (agentResults && agentResults[qIdStr]) {
      const result = agentResults[qIdStr]
      // Consider it run if it has an answer, error, or was completed
      if (result.answer || result.error || result.timestamp) {
        return true
      }
    }
  }
  return false
}

// Get status class for question
function getQuestionStatus(questionId) {
  if (!runResults.value || !questionId) return 'status-not-run'
  const qIdStr = String(questionId)
  
  let hasError = false
  let hasAnswer = false
  let isLoading = false
  
  for (const agentId in runResults.value) {
    const agentResults = runResults.value[agentId]
    if (agentResults && agentResults[qIdStr]) {
      const result = agentResults[qIdStr]
      if (result.loading) isLoading = true
      if (result.error) hasError = true
      if (result.answer) hasAnswer = true
    }
  }
  
  if (isLoading) return 'status-loading'
  if (hasError) return 'status-error'
  if (hasAnswer) return 'status-completed'
  return 'status-not-run'
}

// Get status text for question
function getQuestionStatusText(questionId) {
  const status = getQuestionStatus(questionId)
  switch (status) {
    case 'status-loading':
      return '⏳ Running'
    case 'status-error':
      return '❌ Error'
    case 'status-completed':
      return '✅ Completed'
    default:
      return '⭕ Not Run'
  }
}

// Retry a question for all agents
async function retryQuestionForAllAgents(questionId) {
  if (!currentRun.value || !questionId) {
    alert('No active run. Please start a benchmark first.')
    return
  }
  
  const qIdStr = String(questionId)
  const enabledAgents = mergedAgents.value.filter(a => a.enabled && a.provider_type !== 'evaluator')
  
  if (enabledAgents.length === 0) {
    alert('No enabled agents found. Please enable at least one agent.')
    return
  }
  
  // Retry for each enabled agent
  for (const agent of enabledAgents) {
    await rerunQuestion(agent.id, questionId)
  }
}

function selectQuestionSet(qs) {
    currentQuestionSet.value = qs
    emit('update:currentQuestionSet', qs)
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
    return
  }
  showRunSetup.value = true
}

function createNewQuestionSet() {
  currentQuestionSet.value = null 
  // Question editor is now handled in LeftSidebar
}

function handleQuestionSetUpdated(updated) {
  currentQuestionSet.value = updated
}

function prevQuestion() {
  const idx = currentQuestionIndex.value
  if (idx > 0) {
    selectedQuestionId.value = flatQuestions.value[idx - 1].id
  }
}

function nextQuestion() {
  const idx = currentQuestionIndex.value
  if (idx < flatQuestions.value.length - 1) {
    selectedQuestionId.value = flatQuestions.value[idx + 1].id
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

function reloadResults() {
  if (currentQuestionSet.value) {
    console.log('[Arena] Manually reloading results...')
    fetchLatestResultsForQS(currentQuestionSet.value.id)
  }
}

function ensureResultsLoaded() {
  setTimeout(() => {
    if (currentQuestionSet.value && !currentRun.value && !isLoadingResults.value && !isRunning.value) {
       console.log('[Arena] Auto-healing: Results missing, triggering reload...')
       reloadResults()
    }
  }, 1000)
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
    
    // Safety check for loaded results
    ensureResultsLoaded()

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
  flex-direction: row;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

.questions-list-view {
  width: 400px;
  background: #ffffff;
  border-right: 1px solid #e2e8f0;
  padding: 1rem 1.5rem;
  overflow-y: auto;
  flex-shrink: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  z-index: 1;
}

.questions-list-view h3 {
  margin: 0 0 1rem 0;
  color: #1e293b;
  font-size: 1.1rem;
  font-weight: 600;
}

.questions-list-view .questions-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  flex: 1;
  overflow-y: auto;
}

.questions-list-view .question-item {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 0.75rem;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  transition: background 0.2s ease;
}

.questions-list-view .question-item:hover {
  background: rgba(99, 102, 241, 0.05);
}

.questions-list-view .question-number {
  flex-shrink: 0;
  font-weight: 600;
  color: #6366f1;
  min-width: 2rem;
}

.questions-list-view .question-content-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}

.questions-list-view .question-text {
  color: #1e293b;
  line-height: 1.5;
}

.questions-list-view .question-category {
  flex-shrink: 0;
  padding: 0.25rem 0.5rem;
  background: #6366f1;
  color: white;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
  width: fit-content;
}

.questions-list-view .question-actions {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.5rem;
  flex-shrink: 0;
}

.questions-list-view .question-status {
  font-size: 0.75rem;
  font-weight: 500;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  white-space: nowrap;
}

.questions-list-view .question-status.status-not-run {
  background: #e2e8f0;
  color: #64748b;
}

.questions-list-view .question-status.status-loading {
  background: #dbeafe;
  color: #1e40af;
}

.questions-list-view .question-status.status-completed {
  background: #d1fae5;
  color: #065f46;
}

.questions-list-view .question-status.status-error {
  background: #fee2e2;
  color: #991b1b;
}

.questions-list-view .btn-retry {
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
  background: #6366f1;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.2s ease;
  white-space: nowrap;
}

.questions-list-view .btn-retry:hover {
  background: #4f46e5;
}

.questions-list-view .btn-retry:active {
  background: #4338ca;
}

.questions-list-view .empty-state {
  padding: 2rem;
  text-align: center;
  color: #64748b;
}

.questions-list-view .empty-state p {
  margin: 0;
}

.benchmark-arena-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
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
.main-content-area {
  position: relative;
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

.benchmark-arena {
  display: flex;
  height: 100%;
  overflow: hidden;
}

/* Main Content Area - Side by side layout */
.main-content-area {
  flex: 1;
  display: flex;
  flex-direction: row;
  gap: 16px;
  overflow: hidden;
  padding: 16px;
  position: relative;
}

.arena-label {
  position: absolute;
  top: 16px;
  left: 16px;
  font-size: 18px;
  font-weight: 600;
  color: #666;
  z-index: 10;
  background: rgba(255, 255, 255, 0.9);
  padding: 8px 16px;
  border-radius: 6px;
  border: 1px solid #e0e0e0;
  pointer-events: none;
}

/* Questions List View */
.questions-list-view {
  flex: auto;
  overflow-y: auto;
  padding: 24px;
  background: #f8f9fa;
  border-radius: 8px;
  border: 1px solid #e0e0e0;
}

.questions-list-header {
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 2px solid #e0e0e0;
}

.questions-list-header h2 {
  margin: 0 0 8px 0;
  font-size: 24px;
  color: #333;
}

.questions-count {
  margin: 0;
  color: #666;
  font-size: 14px;
}

.questions-list-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.question-item {
  background: white;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  gap: 16px;
}

.question-item:hover {
  border-color: #007bff;
  box-shadow: 0 2px 8px rgba(0, 123, 255, 0.1);
}

.question-item.selected {
  border-color: #007bff;
  background: #e7f3ff;
  box-shadow: 0 2px 8px rgba(0, 123, 255, 0.15);
}

.question-number {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f0f0f0;
  border-radius: 6px;
  font-weight: 600;
  color: #666;
  font-size: 14px;
}

.question-item.selected .question-number {
  background: #007bff;
  color: white;
}

.question-content {
  flex: 1;
}

.question-text {
  font-size: 15px;
  line-height: 1.5;
  color: #333;
  margin-bottom: 8px;
}

.question-category {
  font-size: 12px;
  color: #666;
  font-style: italic;
}

.action-buttons-row {
  position: inherit;
  padding: 2px 16px;
  background: white;
  display: flex;
  gap: 8px;
  z-index: 100;
  border-bottom: 1px solid #e0e0e0
}

.action-buttons-row .btn {
  padding: 8px 16px;
  font-size: 14px;
}

.chat-container {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-width: 0; /* Allow flex shrinking */
}

</style>
