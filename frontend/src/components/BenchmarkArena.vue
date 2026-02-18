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

    <div v-if="showPrimaryAgentModal" class="modal-overlay" @click.self="showPrimaryAgentModal = false">
      <div class="modal-container primary-agent-modal">
        <div class="modal-header">
          <h3>Primary agent required</h3>
          <button class="btn-close" @click="showPrimaryAgentModal = false">×</button>
        </div>
        <div class="modal-body">
          <p>Please enable at least one primary agent to start.</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-primary" @click="showPrimaryAgentModal = false">Got it</button>
        </div>
      </div>
    </div>

    <!-- Left Sidebar with Tabs -->
    <LeftSidebar
      :question-sets="questionSets"
      :current-question-set="currentQuestionSet"
      :agents="mergedAgents"
      :running-question-set-id="wsState.runningQuestionSetId"
      :workspace-id="workspaceId"
      @create-question-set="createNewQuestionSet"
      @select-question-set="selectQuestionSet"
      @manage-agents="() => emit('manage-agents')"
      @question-set-updated="handleQuestionSetUpdated"
    />

    <!-- Main Content Area -->
    <div class="benchmark-arena-content">
    <!-- Action Buttons Row -->
    <div class="action-buttons-row">
      <button class="btn btn-primary" @click="startRunSetup" :disabled="isRunning || !currentQuestionSet">
        {{ isRunning ? '⏳ Running...' : '▶️ Run Benchmark' }}
      </button>
      <button
        v-if="hasEnabledEvaluators"
        class="btn btn-secondary btn-eval"
        @click="startEvaluationNow"
        :disabled="!canStartEvaluation"
        :title="startEvaluationDisabledReason"
      >
        🧪 Run Evaluation
      </button>
      <button class="btn btn-secondary btn-pdf" @click="exportToPdf" :disabled="!currentRun || isExportingPdf">
        <span v-if="isExportingPdf" class="pdf-loading-spinner"></span>
        <span v-else>📄</span> PDF
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
      <div
        v-else-if="flatQuestions.length > 0"
        class="main-content-area"
        :class="{ 'single-panel': !showLegacyAgentPanels || displayAgents.length === 0 }"
      >
        <!-- Questions List View -->
        <div class="questions-list-view">
          <div class="questions-list-header">
            <h2>{{ currentQuestionSet.name }}</h2>
            <p class="questions-count">{{ flatQuestions.length }} question{{ flatQuestions.length !== 1 ? 's' : '' }}</p>
          </div>
          
          <!-- Stats Section -->
          <div v-if="currentRun && agentStats.length > 0" class="questions-stats-section">
            <div 
              v-for="agentStat in agentStats" 
              :key="agentStat.id"
              class="agent-stat-card"
            >
              <div class="agent-stat-header">
                <h4>{{ agentStat.name }}</h4>
                <div v-if="agentStat.isEvaluator" class="evaluator-badge-small">Evaluator</div>
                <div
                  v-else
                  class="quality-score-badge"
                  :title="'Quality Index (weighted labels): positive=100, alternative=80, partial=50, negative=0'"
                >
                  Quality {{ agentStat.qualityScore }}%
                </div>
              </div>
              <div class="agent-stat-metrics">
                <div class="metric">
                  <span class="metric-value">{{ agentStat.stats.answered }} / {{ agentStat.stats.totalQuestions }}</span>
                  <span class="metric-label">Answered</span>
                </div>
                <div class="metric">
                  <span class="metric-value">{{ formatDuration(agentStat.stats.avgDuration) }}</span>
                  <span class="metric-label">Avg Speed</span>
                </div>
                <div class="metric">
                  <span class="metric-value">{{ agentStat.isEvaluator ? agentStat.stats.answered : agentStat.avgEvalScoreLabel }}</span>
                  <span class="metric-label">{{ agentStat.isEvaluator ? 'Evaluations' : 'Avg Eval' }}</span>
                </div>
              </div>
              <div v-if="!agentStat.isEvaluator && agentStat.stats.percentages && agentStat.hasAnyEvaluations" class="validations-bar-small">
                <div class="v-segment-small pos" :style="{ width: agentStat.stats.percentages.positive + '%' }"></div>
                <div class="v-segment-small alt" :style="{ width: agentStat.stats.percentages.alternative + '%' }"></div>
                <div class="v-segment-small par" :style="{ width: agentStat.stats.percentages.partial + '%' }"></div>
                <div class="v-segment-small neg" :style="{ width: agentStat.stats.percentages.negative + '%' }"></div>
              </div>
              <div v-if="!agentStat.isEvaluator && agentStat.stats.percentages && agentStat.hasAnyEvaluations" class="validations-legend-small">
                <span class="legend-item-small pos">Positive {{ agentStat.stats.percentages.positive }}%</span>
                <span class="legend-item-small alt">Alternative {{ agentStat.stats.percentages.alternative }}%</span>
                <span class="legend-item-small par">Partial {{ agentStat.stats.percentages.partial }}%</span>
                <span class="legend-item-small neg">Negative {{ agentStat.stats.percentages.negative }}%</span>
              </div>
            </div>
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
                <div v-if="getQuestionResponse(question.id)" class="question-response">
                  <div class="response-label">Response:</div>
                  <div class="response-text">
                    <div 
                      v-if="!expandedResponses[question.id]"
                      v-html="formatResponseHtml(getQuestionResponse(question.id, true))"
                    ></div>
                    <div 
                      v-else
                      v-html="formatResponseHtml(getQuestionResponse(question.id, false))"
                    ></div>
                    <button 
                      v-if="isResponseLong(question.id)"
                      class="btn-expand-response"
                      @click.stop="toggleResponse(question.id)"
                    >
                      {{ expandedResponses[question.id] ? 'Show less' : 'Show more' }}
                    </button>
                  </div>
                </div>
                <div v-if="getQuestionEvaluation(question.id)" class="question-response question-evaluation">
                  <div class="response-label">
                    Evaluation:
                    <span v-if="getQuestionEvaluationScore(question.id)" class="evaluation-score-chip">
                      {{ getQuestionEvaluationScore(question.id) }}
                    </span>
                  </div>
                  <div class="response-text">
                    <div
                      v-if="!expandedEvaluations[question.id]"
                      v-html="formatResponseHtml(getQuestionEvaluation(question.id, true))"
                    ></div>
                    <div
                      v-else
                      v-html="formatResponseHtml(getQuestionEvaluation(question.id, false))"
                    ></div>
                    <button
                      v-if="isEvaluationLong(question.id)"
                      class="btn-expand-response"
                      @click.stop="toggleEvaluation(question.id)"
                    >
                      {{ expandedEvaluations[question.id] ? 'Show less' : 'Show more' }}
                    </button>
                  </div>
                </div>
              </div>
              <div class="question-actions">
                <span
                  class="question-status"
                  :class="getQuestionStatus(question.id)"
                  :title="getQuestionStatusTooltip(question.id) || null"
                >
                  {{ getQuestionStatusText(question.id) }}
                </span>
                <button 
                  v-if="hasQuestionBeenRun(question.id)"
                  class="btn-retry" 
                  @click.stop="retryQuestionForAllAgents(question.id)"
                  :disabled="isQuestionLoading(question.id)"
                  :title="isQuestionLoading(question.id) ? 'Retrying...' : 'Retry this question'"
                >
                  {{ isQuestionLoading(question.id) ? '⏳ Retrying' : '🔄 Retry' }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Chat Panels (when agents/results are available) -->
        <div v-if="showLegacyAgentPanels && displayAgents.length > 0" class="chat-container">
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
      <div v-else-if="showLegacyAgentPanels && hasResults && displayAgents.length > 0" class="chat-container">
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
import { processContent } from '../utils/markdown.js'
import { isEvaluatorAgentObject, toAgentID, uniqueStringIDs, mergeAgentIDs } from '../utils/arena/agents.js'
import { mergeQuestionSetForUI, getRunQuestionSetID } from '../utils/arena/questionSet.js'
import { extractScoreOutOfTen, truncatePreviewText, extractQuestionIdsFromQuestionSet } from '../utils/arena/parsing.js'
import { calculateStats, calculateAverageEvaluationScore, formatDuration } from '../utils/arena/stats.js'
import { flattenQuestionSetQuestions, hasQuestionBeenRun as hasQuestionBeenRunUtil, getQuestionStatus as getQuestionStatusUtil, isQuestionLoading as isQuestionLoadingUtil, getQuestionStatusText as getQuestionStatusTextUtil, getQuestionStatusTooltip as getQuestionStatusTooltipUtil } from '../utils/arena/questions.js'
import { getPrimaryResponseEntry as getPrimaryResponseEntryUtil, getQuestionResponse as getQuestionResponseUtil, getQuestionEvaluation as getQuestionEvaluationUtil } from '../utils/arena/responses.js'
import { splitSelectedAgents as splitSelectedAgentsUtil, getEvaluatorIdsForRun as getEvaluatorIdsForRunUtil, hasEvaluatorResultsLoaded as hasEvaluatorResultsLoadedUtil, resolveRunAgentIds as resolveRunAgentIdsUtil } from '../utils/arena/runs.js'
import { saveRunProgress as saveRunProgressUtil, loadRunProgress as loadRunProgressUtil, clearRunProgress as clearRunProgressUtil, hasLoadingResults as hasLoadingResultsUtil, waitForResultsToLoad as waitForResultsToLoadUtil } from '../utils/arena/progress.js'
import { getAgentResults as getAgentResultsUtil } from '../utils/arena/results.js'
import { registerArenaWsEvents } from '../utils/arena/wsBindings.js'
import { useArenaRetryTracking } from '../composables/useArenaRetryTracking.js'
import { useArenaEvaluatorRuns } from '../composables/useArenaEvaluatorRuns.js'
import { useArenaRetryReconciliation } from '../composables/useArenaRetryReconciliation.js'
import { useArenaRunRestoration } from '../composables/useArenaRunRestoration.js'
import { useArenaRunExecution } from '../composables/useArenaRunExecution.js'
import { useArenaRunResultsLoader } from '../composables/useArenaRunResultsLoader.js'

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

const emit = defineEmits(['update:currentQuestionSet', 'trigger-print', 'manage-agents'])

watch(() => props.questionSets, (sets) => {
  // Question sets updated
}, { immediate: true })

watch(() => props.agents, (agents) => {
  // Agents updated
}, { immediate: true })

const wsStore = useWSStore()
const { state: wsState } = wsStore

// State
const currentQuestionSet = ref(null)
const currentRun = ref(null)
const runResults = ref({})
const taskProgress = ref({})
const isRunning = ref(false)
const activeRunQuestionSetId = ref(null)
const isLoadingResults = ref(false)
const startedTasks = ref(0)
const completedTasks = ref(0)
const totalTasks = ref(0)
const selectedQuestionId = ref('')
const showSummary = ref(false)
const showRunSetup = ref(false)
const showPrimaryAgentModal = ref(false)
const showDetailsModal = ref(false)
const selectedDetails = ref(null)
const expandedResponses = ref({})
const expandedEvaluations = ref({})
const isDev = import.meta.env.DEV
const showLegacyAgentPanels = false
const latestRunCache = new Map()
const pendingResultsBuffer = ref([])
const pendingEvaluatorRuns = ref({})
let arenaWsCleanup = null
const startRunError = ref(null)
const isExportingPdf = ref(false)
const isRestoringRun = ref(false)
const {
  retryingQuestions,
  retryRegistry,
  hasActiveRetryEntries,
  persistRetryRegistry,
  loadRetryRegistry,
  markRetryStarted,
  markRetryFinished: markRetryFinishedState,
  clearRetryTrackingForRun,
  isQuestionRetrying
} = useArenaRetryTracking({
  workspaceId: () => props.workspaceId,
  getRunId: () => currentRun.value?.id || '',
  getQuestionSetId: () => currentQuestionSet.value?.id || ''
})

const {
  getRecentRunIdForQS,
  getCachedRunForQS,
  setCachedRunForQS,
  prioritizeQuestionInQueue,
  applyRunLiteData,
  fetchLatestResultsForQS
} = useArenaRunResultsLoader({
  wsService,
  wsState,
  workspaceId: () => props.workspaceId,
  isLoadingResults,
  latestRunCache,
  downloadManager,
  contentCache,
  runResults,
  taskProgress,
  currentRun,
  totalTasks,
  completedTasks,
  getSelectedQuestionId: () => selectedQuestionId.value
})

const {
  resolveQuestionSetIdForRun,
  popPendingEvaluators,
  getPendingEvaluatorIds,
  resolveLatestRunIDForQuestionSet,
  triggerEvaluatorRun,
  setRunError
} = useArenaEvaluatorRuns({
  wsService,
  wsStore,
  wsState,
  currentRun,
  currentQuestionSet,
  activeRunQuestionSetId,
  pendingEvaluatorRuns,
  isRunning,
  completedTasks,
  totalTasks,
  getFlatQuestions: () => flatQuestions.value,
  startRunError,
  uniqueStringIDs,
  mergeAgentIDs,
  getRunQuestionSetID,
  applyRunLiteData
})

const {
  reconcileRetriesFromServer
} = useArenaRetryReconciliation({
  wsService,
  wsState,
  wsStore,
  retryRegistry,
  loadRetryRegistry,
  markRetryStarted,
  markRetryFinished,
  persistRetryRegistry,
  hasActiveRetryEntries,
  runResults,
  isRunning,
  activeRunQuestionSetId,
  currentQuestionSet,
  currentRun,
  getDisplayAgents: () => displayAgents.value,
  maybeStopRunningWhenIdle,
  fetchLatestResultsForQS
})

const {
  getRunningRunForCurrentQS,
  restoreActiveRun
} = useArenaRunRestoration({
  wsService,
  wsStore,
  wsState,
  currentQuestionSet,
  currentRun,
  runResults,
  taskProgress,
  isRunning,
  activeRunQuestionSetId,
  isRestoringRun,
  startedTasks,
  completedTasks,
  totalTasks,
  latestRunCache,
  getFlatQuestions: () => flatQuestions.value,
  clearRunProgress,
  loadRunProgress,
  saveRunProgress,
  fetchLatestResultsForQS,
  mergeQuestionSetForUI,
  resolveRunAgentIds: resolveRunAgentIdsUtil,
  extractQuestionIdsFromQuestionSet
})

function clearQuestionLoadingState(questionId) {
  if (!questionId) return
  const qIdStr = String(questionId)
  for (const agentId in runResults.value) {
    const item = runResults.value[agentId]?.[qIdStr]
    if (!item) continue
    runResults.value[agentId][qIdStr] = {
      ...item,
      loading: false,
      queued: false
    }
  }
}

function clearAllLoadingStates() {
  for (const agentId in runResults.value) {
    const agentResults = runResults.value[agentId]
    if (!agentResults) continue
    for (const qIdStr in agentResults) {
      const item = agentResults[qIdStr]
      if (!item) continue
      if (!item.loading && !item.queued) continue
      agentResults[qIdStr] = {
        ...item,
        loading: false,
        queued: false
      }
    }
  }
}

function maybeStopRunningWhenIdle() {
  if (hasActiveRetryEntries()) return
  if (String(currentRun.value?.status || '') === 'running') return
  if (!isRunning.value) return

  isRunning.value = false
  taskProgress.value = {}
  localStorage.removeItem('activeRunId')

  if (currentRun.value?.id) {
    clearRunProgress(currentRun.value.id)
  }

  activeRunQuestionSetId.value = null
  wsStore.setRunningQuestionSetId(null)
}

// Init logic for Question Set
watch(() => props.questionSets, (sets) => {
  if (!sets || sets.length === 0) return
  
  if (!currentQuestionSet.value) {
     initQuestionSet(sets)
  } else {
    // Sync current set with updated data from props
    const updated = sets.find(s => s.id === currentQuestionSet.value.id)
    if (updated) {
      currentQuestionSet.value = mergeQuestionSetForUI(updated, currentQuestionSet.value)
    }
  }
}, { immediate: true, deep: true })

// Watch for parent-driven selection changes
watch(() => props.initialQuestionSetId, (newId) => {
  if (newId && newId !== currentQuestionSet.value?.id) {
    const found = props.questionSets.find(s => s.id === newId)
    if (found) {
      currentQuestionSet.value = mergeQuestionSetForUI(found, currentQuestionSet.value)
    }
  }
})

// Watch for workspaceId changes to trigger fetch if it was skipped
watch(() => props.workspaceId, (newId) => {
  loadRetryRegistry()
  if (newId && wsState.isConnected) {
    reconcileRetriesFromServer()
  }
  if (newId && currentQuestionSet.value && !isRunning.value) {
    fetchLatestResultsForQS(currentQuestionSet.value.id)
  }
})

function initQuestionSet(sets) {
    if (props.initialQuestionSetId) {
        const found = sets.find(s => s.id === props.initialQuestionSetId)
        if (found) {
            currentQuestionSet.value = mergeQuestionSetForUI(found, currentQuestionSet.value)
            return
        }
    }
    // Fallback: localStorage
    const lastId = localStorage.getItem('lastQuestionSetId')
    if (lastId) {
        const found = sets.find(s => s.id === lastId)
        if (found) {
            currentQuestionSet.value = mergeQuestionSetForUI(found, currentQuestionSet.value)
            return
        }
    }
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
const flatQuestions = computed(() => flattenQuestionSetQuestions(currentQuestionSet.value?.data))

const currentQuestionIndex = computed(() => {
  if (!selectedQuestionId.value) return 0
  return flatQuestions.value.findIndex(q => q.id === selectedQuestionId.value)
})

const hasResults = computed(() => {
  return runResults.value && Object.keys(runResults.value).length > 0
})

// Granular progress: started tasks add a small baseline, completed tasks count fully
const progressPercent = computed(() => {
  if (totalTasks.value === 0) return 0
  return Math.round((completedTasks.value / totalTasks.value) * 100)
})

const STARTED_TASK_WEIGHT_PERCENT = 10

const progressPercentStarted = computed(() => {
  if (totalTasks.value === 0) return 0
  // Started but not completed tasks contribute a small baseline progress
  const inProgress = Math.max(0, startedTasks.value - completedTasks.value)
  const inProgressContribution = (inProgress / totalTasks.value) * STARTED_TASK_WEIGHT_PERCENT
  const completedContribution = (completedTasks.value / totalTasks.value) * 100
  const softBoost = softProgressBoost.value
  return Math.min(100, Math.round(completedContribution + inProgressContribution + softBoost))
})

const softProgressBoost = computed(() => {
  let boost = 0
  const progress = taskProgress.value || {}
  for (const agentId in progress) {
    const agentProgress = progress[agentId] || {}
    for (const qId in agentProgress) {
      const entry = agentProgress[qId]
      const elapsedMs = typeof entry?.elapsed_ms === 'number' ? entry.elapsed_ms : 0
      const ticks = Math.floor(elapsedMs / 10000)
      boost += ticks * 0.5
    }
  }
  return boost
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

  if (!qs.agents || !Array.isArray(qs.agents) || qs.agents.length === 0) {
    return props.agents
  }

  const overrideMap = {}
  qs.agents.forEach((row) => {
    const aid = row.agent_id || row.agentID || row.id
    if (aid) overrideMap[aid] = row
  })

  return props.agents.map((a) => {
    const override = overrideMap[a.id]
    if (!override) {
      return { ...a, enabled: false }
    }
    return {
      ...a,
      enabled: override.enabled !== false,
      position: override.position !== undefined ? override.position : a.position,
      config: override.config || a.config
    }
  })
})

function getAgentById(agentId) {
  if (!agentId) return null
  return mergedAgents.value.find((a) => a.id === agentId) || props.agents.find((a) => a.id === agentId) || null
}

function isEvaluatorAgentID(agentId) {
  const agent = getAgentById(agentId)
  return isEvaluatorAgentObject(agent)
}

const selectedAgentIdsForQuestionSet = computed(() => {
  const overrides = Array.isArray(currentQuestionSet.value?.agents) ? currentQuestionSet.value.agents : []
  const overrideIDs = overrides.map((item) => toAgentID(item)).filter(Boolean)
  if (overrideIDs.length > 0) {
    return [...new Set(
      overrides
        .filter((item) => !!item?.enabled)
        .map((item) => toAgentID(item))
        .filter(Boolean)
    )]
  }
  return mergedAgents.value.filter((a) => a.enabled).map((a) => a.id)
})

const enabledAgents = computed(() =>
  mergedAgents.value.filter((a) => selectedAgentIdsForQuestionSet.value.includes(a.id))
)
const enabledEvaluatorAgents = computed(() => enabledAgents.value.filter((a) => isEvaluatorAgentObject(a)))
const hasEnabledEvaluators = computed(() => enabledEvaluatorAgents.value.length > 0)

const hasPrimaryRunAnswers = computed(() => {
  for (const agentId in runResults.value || {}) {
    if (isEvaluatorAgentID(agentId)) continue
    const agentResults = runResults.value[agentId] || {}
    for (const qid in agentResults) {
      const result = agentResults[qid]
      const answerText = typeof result?.answer === 'string'
        ? result.answer.trim()
        : String(result?.answer || '').trim()
      if (answerText !== '' && !result?.error) {
        return true
      }
    }
  }
  return false
})

const canStartEvaluation = computed(() => {
  return !!currentQuestionSet.value &&
    !isRunning.value &&
    !isLoadingResults.value &&
    hasEnabledEvaluators.value &&
    hasPrimaryRunAnswers.value
})

const startEvaluationDisabledReason = computed(() => {
  if (!hasEnabledEvaluators.value) return 'No enabled evaluator agents'
  if (isRunning.value) return 'Wait for the current run to finish'
  if (isLoadingResults.value) return 'Wait until results finish loading'
  if (!currentQuestionSet.value) return 'Select a question set first'
  if (!hasPrimaryRunAnswers.value) return 'Not Run: run a primary agent first to generate at least one response'
  return 'Run evaluators on existing answers'
})

const displayAgents = computed(() => {
  let list = []
  
  if (isRunning.value && currentRun.value?.agentIds) {
    // 1. If running, show agents participating in this specific run
    const runIds = new Set(currentRun.value.agentIds)
    mergedAgents.value
      .filter(a => isEvaluatorAgentObject(a) && a.enabled)
      .forEach(a => runIds.add(a.id))
    const runIdList = Array.from(runIds)
    list = mergedAgents.value.filter(a => runIdList.includes(a.id))
    
    // Fallback for agents not in props.agents
    const missingIds = runIdList.filter(id => !list.some(a => a.id === id))
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
      // In history mode, if an agent has results it must stay visible even if currently disabled.
      const agentsWithResults = mergedAgents.value.filter(a => resultAgentIds.includes(a.id))
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

  return list.sort((a, b) => {
    return (a.position || 0) - (b.position || 0)
  })
})

// Computed property for agent stats (similar to PDF export)
const agentStats = computed(() => {
  if (!currentRun.value || displayAgents.value.length === 0) return []
  
  return displayAgents.value.map(agent => {
    const results = getAgentResults(agent.id, true)
    const stats = calculateStats(results)
    const isEvaluator = isEvaluatorAgentObject(agent)
    const evalSummary = calculateAverageEvaluationScore(results)
    const hasAnyEvaluations = results.some((result) => {
      if (result?.humanValidation && String(result.humanValidation).trim() !== '') return true
      return Array.isArray(result?.evaluations) && result.evaluations.length > 0
    })
    
    // Calculate quality score (same logic as PDF export)
    const totalValidations = stats.validations.positive + 
                           stats.validations.negative + 
                           stats.validations.alternative + 
                           stats.validations.partial
    
    const qualityScore = totalValidations > 0
      ? ((stats.validations.positive * 1.0 +
          stats.validations.alternative * 0.8 +
          stats.validations.partial * 0.5) /
         (totalValidations || 1) * 100).toFixed(1)
      : '0.0'
    
    return {
      id: agent.id,
      name: agent.name || agent.config?.name || 'Agent',
      provider: agent.provider_type,
      isEvaluator,
      hasAnyEvaluations,
      stats,
      qualityScore,
      avgEvalScoreLabel: evalSummary.count > 0 ? `${evalSummary.avgScore10.toFixed(1)}/10` : '—'
    }
  })
})

// Methods

// Check if a question has been run (has results from any agent)
function hasQuestionBeenRun(questionId) {
  return hasQuestionBeenRunUtil(runResults.value, questionId)
}

function markRetryFinished(questionId, retryId, status = 'completed') {
  const result = markRetryFinishedState(questionId, retryId, status)
  if (result?.questionCleared) {
    clearQuestionLoadingState(questionId)
  }
  maybeStopRunningWhenIdle()
}

// Get status class for question
function getQuestionStatus(questionId) {
  return getQuestionStatusUtil(runResults.value, questionId, isQuestionRetrying)
}

function isQuestionLoading(questionId) {
  return isQuestionLoadingUtil(runResults.value, questionId, isQuestionRetrying)
}

// Get status text for question
function getQuestionStatusText(questionId) {
  return getQuestionStatusTextUtil(getQuestionStatus(questionId))
}

function getQuestionStatusTooltip(questionId) {
  const status = getQuestionStatus(questionId)
  return getQuestionStatusTooltipUtil(status, questionId, taskProgress.value, isQuestionRetrying)
}

function getPrimaryResponseEntry(questionId) {
  return getPrimaryResponseEntryUtil({
    runResults: runResults.value,
    questionId,
    mergedAgents: mergedAgents.value,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying
  })
}

function getQuestionResponse(questionId, truncated = true) {
  return getQuestionResponseUtil({
    runResults: runResults.value,
    questionId,
    mergedAgents: mergedAgents.value,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying,
    truncatePreviewText,
    truncated
  })
}

function getQuestionEvaluation(questionId, truncated = true) {
  return getQuestionEvaluationUtil({
    runResults: runResults.value,
    questionId,
    mergedAgents: mergedAgents.value,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying,
    truncatePreviewText,
    truncated
  })
}

function getQuestionEvaluationScore(questionId) {
  const fullText = getQuestionEvaluation(questionId, false)
  const score = extractScoreOutOfTen(fullText)
  return score == null ? '' : `${score}/10`
}

function isResponseLong(questionId) {
  const response = getQuestionResponse(questionId, false)
  return !!response && response.length > 150
}

function isEvaluationLong(questionId) {
  const evaluation = getQuestionEvaluation(questionId, false)
  return !!evaluation && evaluation.length > 150
}

function toggleResponse(questionId) {
  expandedResponses.value[questionId] = !expandedResponses.value[questionId]
}

function toggleEvaluation(questionId) {
  expandedEvaluations.value[questionId] = !expandedEvaluations.value[questionId]
}

function formatResponseHtml(text) {
  if (!text || typeof text !== 'string') return ''
  const processed = processContent(text)
  return processed.html || ''
}

function selectQuestionSet(qs) {
    currentQuestionSet.value = mergeQuestionSetForUI(qs, currentQuestionSet.value)
    emit('update:currentQuestionSet', currentQuestionSet.value)
}

function startRunSetup() {
  if (!currentQuestionSet.value) return
  showRunSetup.value = true
}

function createNewQuestionSet() {
  currentQuestionSet.value = null 
  // Question editor is now handled in LeftSidebar
}

function handleQuestionSetUpdated(updated) {
  currentQuestionSet.value = mergeQuestionSetForUI(updated, currentQuestionSet.value)
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
function reloadResults() {
  if (currentQuestionSet.value) {
    fetchLatestResultsForQS(currentQuestionSet.value.id)
  }
}

function ensureResultsLoaded() {
  setTimeout(() => {
    if (currentQuestionSet.value && !currentRun.value && !isLoadingResults.value && !isRunning.value) {
       reloadResults()
    }
  }, 1000)
}

watch(() => wsState.recentRuns, () => {
  if (currentQuestionSet.value && !isRunning.value && !currentRun.value) {
    const running = getRunningRunForCurrentQS()
    if (running?.id) {
      localStorage.setItem('activeRunId', running.id)
      restoreActiveRun(running.id)
      return
    }
  }
  if (!currentQuestionSet.value || isRunning.value || isLoadingResults.value) return
  const qsId = currentQuestionSet.value.id
  const latestId = getRecentRunIdForQS(qsId)
  const cached = latestRunCache.get(qsId)
  if (latestId && cached?.runId !== latestId) {
    fetchLatestResultsForQS(qsId)
  }
}, { deep: true })

function getAgentResults(agentId, includeAllQuestions = false) {
  return getAgentResultsUtil({
    runResults: runResults.value,
    agentId,
    includeAllQuestions,
    selectedQuestionId: selectedQuestionId.value,
    flatQuestions: flatQuestions.value,
    isEvaluatorAgentID,
    isRunning: isRunning.value,
    activeRunQuestionSetId: activeRunQuestionSetId.value,
    currentQuestionSetId: currentQuestionSet.value?.id,
    currentRunAgentIds: currentRun.value?.agentIds || [],
    taskProgress: taskProgress.value
  })
}

watch(selectedQuestionId, (newId) => {
  if (newId) prioritizeQuestionInQueue(newId)
})

function splitSelectedAgents(payload = {}) {
  return splitSelectedAgentsUtil(payload, isEvaluatorAgentID)
}

function getEvaluatorIdsForRun(runLike) {
  return getEvaluatorIdsForRunUtil(runLike, isEvaluatorAgentID)
}

function hasEvaluatorResultsLoaded() {
  return hasEvaluatorResultsLoadedUtil(runResults.value, isEvaluatorAgentID)
}

const {
  startEvaluationNow,
  handleStartRun,
  processTaskCompleted,
  cancelBenchmark,
  rerunQuestion,
  retryQuestionForAllAgents,
  onValidation,
  onRetry
} = useArenaRunExecution({
  wsService,
  wsStore,
  currentQuestionSet,
  currentRun,
  runResults,
  taskProgress,
  isRunning,
  activeRunQuestionSetId,
  startedTasks,
  completedTasks,
  totalTasks,
  showRunSetup,
  pendingResultsBuffer,
  startRunError,
  canStartEvaluation,
  startEvaluationDisabledReason,
  enabledEvaluatorAgents,
  getFlatQuestions: () => flatQuestions.value,
  getMergedAgents: () => mergedAgents.value,
  splitSelectedAgents,
  resolveLatestRunIDForQuestionSet,
  triggerEvaluatorRun,
  setRunError,
  mergeAgentIDs,
  saveRunProgress,
  clearRunProgress,
  markRetryStarted,
  markRetryFinished,
  getPendingEvaluatorIds,
  popPendingEvaluators,
  resolveQuestionSetIdForRun,
  getAgentResults
})

async function handleRunSave(payload) {
  const savedQuestionSet = payload?.questionSet
  if (!currentQuestionSet.value || !savedQuestionSet) return

  console.log('[Arena] handleRunSave: Received saved QS with', savedQuestionSet.agents?.length, 'agents')

  if (savedQuestionSet.id === currentQuestionSet.value.id) {
    const savedAgents = Array.isArray(savedQuestionSet.agents) && savedQuestionSet.agents.length > 0
      ? savedQuestionSet.agents
      : null
    const payloadAgents = Array.isArray(payload?.agents) && payload.agents.length > 0
      ? payload.agents
      : null
    const newAgents = savedAgents || payloadAgents || currentQuestionSet.value.agents
    
    currentQuestionSet.value = mergeQuestionSetForUI({
      ...currentQuestionSet.value,
      ...savedQuestionSet,
      agents: newAgents
    }, currentQuestionSet.value)
  }
}

function saveRunProgress(runId) {
  saveRunProgressUtil(runId, {
    started: startedTasks.value || 0,
    completed: completedTasks.value || 0,
    total: totalTasks.value || 0,
    updatedAt: new Date().toISOString()
  })
}

function loadRunProgress(runId) {
  return loadRunProgressUtil(runId)
}

function clearRunProgress(runId) {
  clearRunProgressUtil(runId)
}

// Global Listeners for THIS component
// We need to listen to WS events to update live results
onMounted(async () => {
  loadRetryRegistry()
  const activeRunId = localStorage.getItem('activeRunId')
  if (activeRunId) {
    await restoreActiveRun(activeRunId)
  }

  await reconcileRetriesFromServer()

  // Safety check for loaded results
  ensureResultsLoaded()

  if (typeof arenaWsCleanup === 'function') {
    arenaWsCleanup()
  }
  arenaWsCleanup = registerArenaWsEvents({
    wsService,
    wsStore,
    contentCache,
    runResults,
    taskProgress,
    isRunning,
    currentRun,
    pendingResultsBuffer,
    currentQuestionSet,
    activeRunQuestionSetId,
    startedTasks,
    totalTasks,
    retryingQuestions,
    saveRunProgress,
    markRetryStarted,
    processTaskCompleted,
    popPendingEvaluators,
    resolveQuestionSetIdForRun,
    triggerEvaluatorRun,
    getEvaluatorIdsForRun,
    hasEvaluatorResultsLoaded,
    clearRunProgress,
    clearRetryTrackingForRun,
    clearAllLoadingStates,
    maybeStopRunningWhenIdle,
    fetchLatestResultsForQS
  })
})

watch(() => wsState.isConnected, async (connected) => {
  if (!connected) return
  const activeRunId = localStorage.getItem('activeRunId')
  if (activeRunId) {
    await restoreActiveRun(activeRunId)
  } else if (currentQuestionSet.value?.id && !isRunning.value) {
    latestRunCache.delete(currentQuestionSet.value.id)
    await fetchLatestResultsForQS(currentQuestionSet.value.id)
  }
  await reconcileRetriesFromServer()
})

// Helper function to check if any results are still loading
function hasLoadingResults() {
  return hasLoadingResultsUtil(runResults.value, currentRun.value)
}

// Wait for all results to finish loading (with timeout)
async function waitForResultsToLoad(maxWaitMs = 5000) {
  await waitForResultsToLoadUtil({
    isLoadingResults: () => isLoadingResults.value,
    hasLoadingResults: () => hasLoadingResults(),
    maxWaitMs
  })
}

async function exportToPdf() {
  if (!currentRun.value || isExportingPdf.value) return
  
  isExportingPdf.value = true
  
  try {
    // Ensure results are loaded before exporting
    if (isLoadingResults.value || hasLoadingResults()) {
      await waitForResultsToLoad()
    }
    
    // Build agents array from displayAgents with their results
    // Always include all questions in PDF export, regardless of selection
    const agentsArray = displayAgents.value.map(agent => ({
      id: agent.id,
      name: agent.name || agent.config?.name || 'Agent',
      provider: agent.provider_type,
      config: agent.config,
      results: getAgentResults(agent.id, true) // true = include all questions
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
  } finally {
    isExportingPdf.value = false
  }
}

// Removing local triggerBrowserPrint as it's handled by parent App.vue

onUnmounted(() => {
    if (typeof arenaWsCleanup === 'function') {
      arenaWsCleanup()
      arenaWsCleanup = null
    }
})

// Expose methods that parent might need?
defineExpose({
    initQuestionSet
})
</script>

<style scoped src="./BenchmarkArena.css"></style>
