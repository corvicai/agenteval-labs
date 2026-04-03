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
      @question-set-deleted="handleQuestionSetDeleted"
    />

    <!-- Main Content Area -->
    <div class="benchmark-arena-content">
    <!-- Action Buttons Row -->
    <div class="action-buttons-row">
      <div class="action-buttons-main">
        <button class="btn btn-primary" @click="startRunSetup" :disabled="isRunning || !currentQuestionSet">
          {{ isRunning ? '⏳ Running...' : '▶️ Run Benchmark' }}
        </button>
        <button
          v-if="failedPrimaryRetryCount > 0"
          class="btn btn-secondary btn-retry-failed"
          @click="retryFailedPrimaryResults"
          :disabled="!canRetryFailedPrimary"
          :title="retryFailedPrimaryTitle"
        >
          🔁 Retry Failed Agents
          <span class="btn-inline-count">{{ failedPrimaryRetryCount }}</span>
        </button>
        <button
          v-if="failedEvaluatorRetryCount > 0"
          class="btn btn-secondary btn-retry-failed btn-retry-failed-eval"
          @click="retryFailedEvaluatorResults"
          :disabled="!canRetryFailedEvaluators"
          :title="retryFailedEvaluatorTitle"
        >
          🧪 Retry Failed Evaluations
          <span class="btn-inline-count">{{ failedEvaluatorRetryCount }}</span>
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
        <div class="pdf-export-controls">
          <select
            v-model="pdfExportScope"
            class="pdf-export-select"
            :disabled="!currentRun || isExportingPdf"
            aria-label="PDF export scope"
          >
            <option value="auto">Auto</option>
            <option value="full">Full report</option>
            <option value="selected" :disabled="!canExportSelectedQuestionPdf">Selected question</option>
            <option value="filtered" :disabled="!canExportFilteredQuestionsPdf">{{ filteredQuestionsExportLabel }}</option>
          </select>
          <button
            class="btn btn-secondary btn-pdf"
            @click="exportToPdf()"
            :disabled="!canExportPdf"
            :title="pdfExportDisabledReason"
          >
            <span v-if="isExportingPdf" class="pdf-loading-spinner"></span>
            <span v-else>📄</span> PDF
          </button>
        </div>
        <button v-if="isRunning" class="btn btn-danger" @click="cancelBenchmark">
          ⛔ Cancel
        </button>
        <select
          v-if="currentQuestionSet && flatQuestions.length > 0"
          v-model="questionFilter"
          class="question-filter-select"
          :class="{ 'is-filtered': questionFilter !== 'all' }"
          aria-label="Filter questions"
          title="Filter questions by status"
        >
          <option value="all">All ({{ questionFilterCounts.all }})</option>
          <option value="error">Error ({{ questionFilterCounts.error }})</option>
          <option value="running">Running ({{ questionFilterCounts.running }})</option>
          <option v-if="showEvaluationScoreFilters" value="low_score">Below 5/10 ({{ questionFilterCounts.low_score }})</option>
          <option v-if="showEvaluationScoreFilters" value="critical_score">Below 1/10 ({{ questionFilterCounts.critical_score }})</option>
        </select>
      </div>
    </div>

    <!-- Progress Bar -->
    <div
      v-if="isRunning && activeRunQuestionSetId === currentQuestionSet?.id"
      class="progress-bar"
      :class="`progress-${progressPhase}`"
    >
      <div class="progress-fill-started" :style="{ width: progressTouchedPercent + '%' }"></div>
      <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
      <div class="progress-content">
        <div class="progress-summary">
          <span class="progress-phase-chip" :class="`phase-${progressPhase}`">{{ progressPhaseLabel }}</span>
          <span class="progress-text">{{ progressHeadline }}</span>
        </div>
        <div class="progress-meta">
          <span class="progress-meta-item">{{ progressTaskRatioText }}</span>
          <span v-if="progressRunningCount > 0" class="progress-meta-item">{{ progressRunningCount }} running</span>
          <span v-if="progressQueuedCount > 0" class="progress-meta-item">{{ progressQueuedCount }} queued</span>
          <span v-if="progressErrorCount > 0" class="progress-meta-item is-error">{{ progressErrorCount }} errors</span>
          <span v-if="progressEtaText" class="progress-meta-item">{{ progressEtaText }}</span>
        </div>
      </div>
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
            <div>
              <h2>{{ currentQuestionSet.name }}</h2>
              <p class="questions-count">
                {{ filteredQuestionEntries.length }} of {{ flatQuestions.length }}
                question{{ flatQuestions.length !== 1 ? 's' : '' }}
              </p>
            </div>
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
              v-for="entry in filteredQuestionEntries" 
              :key="entry.question.id || entry.index"
              class="question-item"
              :class="{ 'selected': selectedQuestionId === entry.question.id }"
              @click="selectQuestionForAnalysis(entry.question.id)"
            >
              <div class="question-number">Q{{ entry.index + 1 }}</div>
              <div class="question-content">
                <div class="question-text">{{ entry.question.question || entry.question.text }}</div>
                <div v-if="entry.question.category" class="question-category">{{ entry.question.category }}</div>
                <div
                  v-if="isEditingExpectedAnswer(entry.question.id)"
                  class="question-response question-expected question-expected-editing"
                >
                  <div class="response-label">
                    Expected Answer:
                    <span v-if="savingExpectedAnswerQuestionId === entry.question.id" class="expected-answer-saving">
                      Saving...
                    </span>
                  </div>
                  <div class="expected-answer-editor">
                    <textarea
                      v-model="expectedAnswerDraft"
                      class="expected-answer-textarea"
                      rows="3"
                      placeholder="Enter expected answer..."
                      @click.stop
                      @keydown.esc.stop.prevent="cancelExpectedAnswerEdit"
                      @keydown.enter.ctrl.stop.prevent="saveExpectedAnswer(entry.question)"
                      @keydown.enter.meta.stop.prevent="saveExpectedAnswer(entry.question)"
                    ></textarea>
                    <div class="expected-answer-editor-actions">
                      <button
                        class="btn-expected-inline btn-expected-save"
                        @click.stop="saveExpectedAnswer(entry.question)"
                        :disabled="savingExpectedAnswerQuestionId === entry.question.id"
                      >
                        Save
                      </button>
                      <button
                        class="btn-expected-inline"
                        @click.stop="cancelExpectedAnswerEdit"
                        :disabled="savingExpectedAnswerQuestionId === entry.question.id"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                </div>
                <div v-else-if="getQuestionExpectedAnswer(entry.question)" class="question-response question-expected">
                  <div class="response-label">
                    Expected Answer:
                    <button
                      v-if="isQuestionSelected(entry.question.id)"
                      class="btn-expected-inline btn-expected-edit"
                      @click.stop="startExpectedAnswerEdit(entry.question)"
                    >
                      Edit
                    </button>
                  </div>
                  <div
                    v-if="isQuestionSelected(entry.question.id)"
                    class="response-text"
                    v-html="formatResponseHtml(getQuestionExpectedAnswer(entry.question))"
                  ></div>
                  <div
                    v-else
                    class="response-text response-text-preview"
                  >
                    {{ getQuestionExpectedAnswerPreview(entry.question) }}
                  </div>
                </div>
                <button
                  v-else-if="isQuestionSelected(entry.question.id)"
                  class="btn-add-expected-inline"
                  @click.stop="startExpectedAnswerEdit(entry.question)"
                >
                  + Expected Answer
                </button>
                <div v-if="getQuestionResponse(entry.question.id, false)" class="question-response">
                  <div class="response-label">Response:</div>
                  <div class="response-text">
                    <div
                      v-if="!isQuestionSelected(entry.question.id)"
                      class="response-text-preview"
                    >
                      {{ getQuestionResponsePreview(entry.question.id) }}
                    </div>
                    <div 
                      v-else-if="!expandedResponses[entry.question.id]"
                      v-html="formatResponseHtml(getQuestionResponse(entry.question.id, true))"
                    ></div>
                    <div 
                      v-else
                      v-html="formatResponseHtml(getQuestionResponse(entry.question.id, false))"
                    ></div>
                    <button 
                      v-if="isQuestionSelected(entry.question.id) && isResponseLong(entry.question.id)"
                      class="btn-expand-response"
                      @click.stop="toggleResponse(entry.question.id)"
                    >
                      {{ expandedResponses[entry.question.id] ? 'Show less' : 'Show more' }}
                    </button>
                  </div>
                </div>
                <div
                  v-if="getQuestionEvaluation(entry.question.id)"
                  class="question-response question-evaluation"
                  :class="getQuestionEvaluationSeverityClass(entry.question.id)"
                >
                  <div class="response-label">
                    Evaluation:
                    <span
                      v-if="getQuestionEvaluationScore(entry.question.id)"
                      class="evaluation-score-chip"
                      :class="getQuestionEvaluationScoreChipClass(entry.question.id)"
                    >
                      {{ getQuestionEvaluationScore(entry.question.id) }}
                    </span>
                  </div>
                  <div class="response-text">
                    <div
                      v-if="!isQuestionSelected(entry.question.id)"
                      class="response-text-preview"
                    >
                      {{ getQuestionEvaluationPreview(entry.question.id) }}
                    </div>
                    <div
                      v-else-if="!expandedEvaluations[entry.question.id]"
                      v-html="formatResponseHtml(getQuestionEvaluation(entry.question.id, true))"
                    ></div>
                    <div
                      v-else
                      v-html="formatResponseHtml(getQuestionEvaluation(entry.question.id, false))"
                    ></div>
                    <button
                      v-if="isQuestionSelected(entry.question.id) && isEvaluationLong(entry.question.id)"
                      class="btn-expand-response"
                      @click.stop="toggleEvaluation(entry.question.id)"
                    >
                      {{ expandedEvaluations[entry.question.id] ? 'Show less' : 'Show more' }}
                    </button>
                  </div>
                </div>
              </div>
              <div class="question-actions">
                <span
                  class="question-status"
                  :class="getQuestionStatus(entry.question.id)"
                  :title="getQuestionStatusTooltip(entry.question.id) || null"
                >
                  {{ getQuestionStatusText(entry.question.id) }}
                </span>
                <button
                  v-if="getQuestionEvaluatorRetryCount(entry.question.id) > 0"
                  class="btn-retry btn-retry-eval"
                  @click.stop="retryQuestionForEvaluators(entry.question.id)"
                  :disabled="isQuestionLoading(entry.question.id)"
                  :title="getQuestionEvaluatorRetryTitle(entry.question.id)"
                >
                  {{ isQuestionLoading(entry.question.id) ? '⏳ Eval' : '🧪 Retry Eval' }}
                  <span v-if="getQuestionEvaluatorRetryCount(entry.question.id) > 1" class="btn-inline-count">
                    {{ getQuestionEvaluatorRetryCount(entry.question.id) }}
                  </span>
                </button>
                <button 
                  v-if="hasQuestionBeenRun(entry.question.id)"
                  class="btn-retry" 
                  @click.stop="retryQuestionForAllAgents(entry.question.id)"
                  :disabled="isQuestionLoading(entry.question.id)"
                  :title="isQuestionLoading(entry.question.id) ? 'Retrying...' : 'Retry this question'"
                >
                  {{ isQuestionLoading(entry.question.id) ? '⏳ Retrying' : '🔄 Retry' }}
                </button>
                <button
                  class="btn-retry btn-export-question"
                  @click.stop="exportQuestionEntryToPdf(entry)"
                  :disabled="isExportingPdf"
                  :title="'Export this question as a compact PDF'"
                >
                  {{ isExportingPdf ? '⏳ PDF' : '📄 PDF' }}
                </button>
              </div>
            </div>
            <div v-if="filteredQuestionEntries.length === 0" class="empty-state">
              <p>No questions match the current filter.</p>
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
import LeftSidebar from './LeftSidebar.vue'
import wsService from '../services/websocket.js'
import { exportResultsReport } from '../utils/exporters.js'
import { downloadManager } from '../services/DownloadManager.js'
import { contentCache } from '../services/ContentCache.js'
import { useWSStore } from '../stores/wsStore'
import { extractTextOnly } from '../utils/chatHelpers.js'
import { getContentPreviewText, processContent } from '../utils/markdown.js'
import { isEvaluatorAgentObject, toAgentID, uniqueStringIDs, mergeAgentIDs } from '../utils/arena/agents.js'
import { mergeQuestionSetForUI, getQuestionSetListSyncSignature, getQuestionSetSyncSignature, getRunQuestionSetID } from '../utils/arena/questionSet.js'
import { extractScoreOutOfTen, truncatePreviewText, extractQuestionIdsFromQuestionSet, parseEvaluatorTaskQuestionID } from '../utils/arena/parsing.js'
import { calculateStats, calculateAverageEvaluationScore, formatDuration } from '../utils/arena/stats.js'
import { flattenQuestionSetQuestions, hasQuestionBeenRun as hasQuestionBeenRunUtil, getQuestionStatus as getQuestionStatusUtil, isQuestionLoading as isQuestionLoadingUtil, getQuestionStatusText as getQuestionStatusTextUtil, getQuestionStatusTooltip as getQuestionStatusTooltipUtil } from '../utils/arena/questions.js'
import { getPrimaryResponseEntry as getPrimaryResponseEntryUtil, getQuestionResponse as getQuestionResponseUtil, getQuestionEvaluation as getQuestionEvaluationUtil } from '../utils/arena/responses.js'
import { splitSelectedAgents as splitSelectedAgentsUtil, resolveRunAgentIds as resolveRunAgentIdsUtil } from '../utils/arena/runs.js'
import { saveRunProgress as saveRunProgressUtil, loadRunProgress as loadRunProgressUtil, clearRunProgress as clearRunProgressUtil, hasLoadingResults as hasLoadingResultsUtil, waitForResultsToLoad as waitForResultsToLoadUtil } from '../utils/arena/progress.js'
import { getAgentResults as getAgentResultsUtil } from '../utils/arena/results.js'
import { registerArenaWsEvents } from '../utils/arena/wsBindings.js'
import { getRecentRunsSyncSignature } from '../utils/arena/cache.js'
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
  workspaces: {
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
const editingExpectedQuestionId = ref('')
const expectedAnswerDraft = ref('')
const savingExpectedAnswerQuestionId = ref('')
const isDev = import.meta.env.DEV
const showLegacyAgentPanels = false
const latestRunCache = new Map()
const questionSetStateCache = new Map()
const completionTimeline = ref([])
const pendingResultsBuffer = ref([])
const pendingEvaluatorRuns = ref({})
let arenaWsCleanup = null
const startRunError = ref(null)
const isExportingPdf = ref(false)
const isRestoringRun = ref(false)
const questionFilter = ref('all')
const pdfExportScope = ref('auto')
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

const questionSetListSyncKey = computed(() => getQuestionSetListSyncSignature(props.questionSets))
const recentRunsSyncKey = computed(() => getRecentRunsSyncSignature(wsState.recentRuns))

// Init logic for Question Set
watch(questionSetListSyncKey, () => {
  const nextSets = Array.isArray(props.questionSets) ? props.questionSets : []

  if (!currentQuestionSet.value) {
    if (nextSets.length > 0) {
      initQuestionSet(nextSets)
    }
    return
  }

  // Sync current set with updated data from props
  const updated = nextSets.find(s => s.id === currentQuestionSet.value.id)
  if (updated) {
    const merged = mergeQuestionSetForUI(updated, currentQuestionSet.value)
    if (getQuestionSetSyncSignature(merged) !== getQuestionSetSyncSignature(currentQuestionSet.value)) {
      currentQuestionSet.value = merged
    }
    return
  }

  currentQuestionSet.value = null
}, { immediate: true })

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
  if (newId && currentQuestionSet.value) {
    const selectedQuestionSetId = String(currentQuestionSet.value.id || '')
    const activeRunningQuestionSetId = String(activeRunQuestionSetId.value || '')
    if (!isRunning.value || (activeRunningQuestionSetId && activeRunningQuestionSetId !== selectedQuestionSetId)) {
      fetchLatestResultsForQS(selectedQuestionSetId, {
        force: activeRunningQuestionSetId !== '' && activeRunningQuestionSetId !== selectedQuestionSetId
      })
    }
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

function cloneArenaState(value) {
  if (value == null) return value
  try {
    if (typeof structuredClone === 'function') {
      return structuredClone(value)
    }
    return JSON.parse(JSON.stringify(value))
  } catch (e) {
    return value
  }
}

function cacheQuestionSetState(questionSetId, questionSetSnapshot = currentQuestionSet.value) {
  const qsId = String(questionSetId || '')
  if (!qsId) return

  questionSetStateCache.set(qsId, {
    questionSet: cloneArenaState(questionSetSnapshot),
    currentRun: cloneArenaState(currentRun.value),
    runResults: cloneArenaState(runResults.value),
    taskProgress: cloneArenaState(taskProgress.value),
    isRunning: isRunning.value && String(activeRunQuestionSetId.value || '') === qsId,
    activeRunQuestionSetId: String(activeRunQuestionSetId.value || ''),
    startedTasks: startedTasks.value || 0,
    completedTasks: completedTasks.value || 0,
    totalTasks: totalTasks.value || 0,
    selectedQuestionId: selectedQuestionId.value || '',
    completionTimeline: cloneArenaState(completionTimeline.value) || []
  })
}

function restoreCachedQuestionSetState(questionSetId) {
  const qsId = String(questionSetId || '')
  if (!qsId) return false

  const cached = questionSetStateCache.get(qsId)
  if (!cached) return false

  if (cached.questionSet && String(cached.questionSet.id || '') === qsId) {
    currentQuestionSet.value = mergeQuestionSetForUI(cloneArenaState(cached.questionSet), currentQuestionSet.value)
  }
  currentRun.value = cloneArenaState(cached.currentRun) || null
  runResults.value = cloneArenaState(cached.runResults) || {}
  taskProgress.value = cloneArenaState(cached.taskProgress) || {}
  isRunning.value = !!cached.isRunning
  activeRunQuestionSetId.value = cached.isRunning
    ? (cached.activeRunQuestionSetId || qsId)
    : null
  wsStore.setRunningQuestionSetId(activeRunQuestionSetId.value || null)
  startedTasks.value = cached.startedTasks || 0
  completedTasks.value = cached.completedTasks || 0
  totalTasks.value = cached.totalTasks || 0
  selectedQuestionId.value = cached.selectedQuestionId || selectedQuestionId.value || ''
  completionTimeline.value = cloneArenaState(cached.completionTimeline) || []

  return true
}

async function syncSelectedQuestionSetState(questionSetId) {
  const selectedQuestionSetId = String(questionSetId || '')
  if (!selectedQuestionSetId) return

  const cachedRunData = getCachedRunForQS(selectedQuestionSetId)
  const hasCachedState = restoreCachedQuestionSetState(selectedQuestionSetId)

  const runningForSelectedQS = getRunningRunForCurrentQS()
  if (runningForSelectedQS?.id) {
    localStorage.setItem('activeRunId', runningForSelectedQS.id)
    await restoreActiveRun(runningForSelectedQS.id)
    cacheQuestionSetState(selectedQuestionSetId)
    return
  }

  const activeRunningQuestionSetId = String(activeRunQuestionSetId.value || '')
  if (isRunning.value && activeRunningQuestionSetId === selectedQuestionSetId) {
    const activeRunId = String(localStorage.getItem('activeRunId') || currentRun.value?.id || '')
    if (activeRunId) {
      await restoreActiveRun(activeRunId)
      cacheQuestionSetState(selectedQuestionSetId)
      return
    }
  }

  if (hasCachedState && cachedRunData) {
    return
  }

  await fetchLatestResultsForQS(selectedQuestionSetId, {
    force: activeRunningQuestionSetId !== '' && activeRunningQuestionSetId !== selectedQuestionSetId
  })
  cacheQuestionSetState(selectedQuestionSetId)
}

watch(currentQuestionSet, (newSet, oldSet) => {
  emit('update:currentQuestionSet', newSet)
  const previousId = String(oldSet?.id || '')
  const nextId = String(newSet?.id || '')

  if (previousId && previousId !== nextId) {
    cacheQuestionSetState(previousId, oldSet)
  }

  if (newSet) {
    localStorage.setItem('lastQuestionSetId', newSet.id)
    if (previousId !== nextId) {
      void syncSelectedQuestionSetState(newSet.id)
    }
  } else {
    localStorage.removeItem('lastQuestionSetId')
  }
})

// Computed
const flatQuestions = computed(() => flattenQuestionSetQuestions(currentQuestionSet.value?.data))

function questionHasRunningState(questionId) {
  const qIdStr = String(questionId)
  if (isQuestionRetrying(qIdStr)) return true

  for (const agentId in runResults.value || {}) {
    const agentResults = runResults.value[agentId] || {}
    for (const [resultKey, result] of Object.entries(agentResults)) {
      const parsed = parseEvaluatorTaskQuestionID(String(resultKey))
      const candidateQuestionId = String(parsed?.questionId || resultKey)
      if (candidateQuestionId !== qIdStr) continue
      if (result?.loading || result?.queued) return true
    }
  }
  return false
}

function questionHasErrorState(questionId) {
  const qIdStr = String(questionId)
  for (const agentId in runResults.value || {}) {
    const agentResults = runResults.value[agentId] || {}
    for (const [resultKey, result] of Object.entries(agentResults)) {
      const parsed = parseEvaluatorTaskQuestionID(String(resultKey))
      const candidateQuestionId = String(parsed?.questionId || resultKey)
      if (candidateQuestionId !== qIdStr) continue
      if (result?.error) return true
    }
  }
  return false
}

function questionHasEvaluationBelowThreshold(questionId, threshold) {
  const score = getQuestionEvaluationScoreValue(questionId)
  return score != null && score < threshold
}

function matchesQuestionFilter(questionId, filter = questionFilter.value) {
  switch (filter) {
    case 'error':
      return questionHasErrorState(questionId)
    case 'running':
      return questionHasRunningState(questionId)
    case 'low_score':
      return questionHasEvaluationBelowThreshold(questionId, 5)
    case 'critical_score':
      return questionHasEvaluationBelowThreshold(questionId, 1)
    default:
      return true
  }
}

const questionFilterCounts = computed(() => {
  let error = 0
  let running = 0
  let lowScore = 0
  let criticalScore = 0

  flatQuestions.value.forEach((question) => {
    if (questionHasErrorState(question.id)) error++
    if (questionHasRunningState(question.id)) running++
    if (questionHasEvaluationBelowThreshold(question.id, 5)) lowScore++
    if (questionHasEvaluationBelowThreshold(question.id, 1)) criticalScore++
  })

  return {
    all: flatQuestions.value.length,
    error,
    running,
    low_score: lowScore,
    critical_score: criticalScore
  }
})

const filteredQuestionEntries = computed(() =>
  flatQuestions.value
    .map((question, index) => ({ question, index }))
    .filter((entry) => matchesQuestionFilter(entry.question.id))
)

const selectedQuestionEntry = computed(() => {
  if (!selectedQuestionId.value) return null
  const index = flatQuestions.value.findIndex((question) => String(question.id) === String(selectedQuestionId.value))
  if (index === -1) return null
  return {
    question: flatQuestions.value[index],
    index
  }
})

const canExportSelectedQuestionPdf = computed(() => !!selectedQuestionEntry.value)
const canExportFilteredQuestionsPdf = computed(() => filteredQuestionEntries.value.length > 0)

const filteredQuestionsExportLabel = computed(() => {
  const count = filteredQuestionEntries.value.length
  return questionFilter.value === 'all'
    ? `Visible questions (${count})`
    : `Filtered questions (${count})`
})

const filteredQuestionsButtonLabel = computed(() =>
  questionFilter.value === 'all' ? 'PDF Visible' : 'PDF Filtered'
)

const canExportPdf = computed(() => {
  if (!currentRun.value || isExportingPdf.value) return false
  if (pdfExportScope.value === 'auto') return true
  if (pdfExportScope.value === 'selected') return canExportSelectedQuestionPdf.value
  if (pdfExportScope.value === 'filtered') return canExportFilteredQuestionsPdf.value
  return true
})

const pdfExportDisabledReason = computed(() => {
  if (!currentRun.value) return 'Run a benchmark before exporting'
  if (isExportingPdf.value) return 'Preparing PDF...'
  if (pdfExportScope.value === 'auto') {
    return selectedQuestionEntry.value
      ? 'Export the selected question'
      : (questionFilter.value !== 'all' && canExportFilteredQuestionsPdf.value
          ? 'Export the current filtered questions'
          : 'Export the full report')
  }
  if (pdfExportScope.value === 'selected' && !canExportSelectedQuestionPdf.value) {
    return 'Select a question first'
  }
  if (pdfExportScope.value === 'filtered' && !canExportFilteredQuestionsPdf.value) {
    return 'No questions match the current filter'
  }
  return 'Export PDF'
})

const currentQuestionIndex = computed(() => {
  if (!selectedQuestionId.value) return 0
  return flatQuestions.value.findIndex(q => q.id === selectedQuestionId.value)
})

const hasResults = computed(() => {
  return runResults.value && Object.keys(runResults.value).length > 0
})

const progressCounters = computed(() => {
  const rawTotal = Math.max(Number(totalTasks.value || 0), Number(startedTasks.value || 0), Number(completedTasks.value || 0), 0)
  if (rawTotal === 0) {
    return {
      total: 0,
      started: 0,
      completed: 0,
      running: 0,
      queued: 0
    }
  }

  const completed = Math.min(Math.max(Number(completedTasks.value || 0), 0), rawTotal)
  const started = Math.min(Math.max(Number(startedTasks.value || 0), completed), rawTotal)
  const running = Math.max(started - completed, 0)
  const queued = Math.max(rawTotal - started, 0)

  return {
    total: rawTotal,
    started,
    completed,
    running,
    queued
  }
})

const progressPercent = computed(() => {
  if (progressCounters.value.total === 0) return 0
  return Math.round((progressCounters.value.completed / progressCounters.value.total) * 100)
})

const progressTouchedPercent = computed(() => {
  if (progressCounters.value.total === 0) return 0
  return Math.round((progressCounters.value.started / progressCounters.value.total) * 100)
})

const progressRunningCount = computed(() => progressCounters.value.running)
const progressQueuedCount = computed(() => progressCounters.value.queued)

const progressErrorCount = computed(() => {
  let count = 0
  for (const agentId in runResults.value || {}) {
    const agentResults = runResults.value[agentId] || {}
    for (const resultKey in agentResults) {
      if (agentResults[resultKey]?.error) count++
    }
  }
  return count
})

const primaryTaskCapacity = computed(() => {
  const questionCount = flatQuestions.value.length
  if (questionCount === 0) return 0

  const runAgentIds = Array.isArray(currentRun.value?.agentIds) ? currentRun.value.agentIds : []
  const primaryRunAgents = runAgentIds.filter((agentId) => !isEvaluatorAgentID(agentId))
  if (primaryRunAgents.length > 0) {
    return primaryRunAgents.length * questionCount
  }

  const enabledPrimaryCount = enabledAgents.value.filter((agent) => !isEvaluatorAgentObject(agent)).length
  return enabledPrimaryCount * questionCount
})

const hasActiveEvaluatorActivity = computed(() => {
  for (const agentId in taskProgress.value || {}) {
    const agentProgress = taskProgress.value[agentId] || {}
    for (const questionId in agentProgress) {
      if (String(questionId).startsWith('eval-')) return true
    }
  }

  for (const agentId in runResults.value || {}) {
    const agentResults = runResults.value[agentId] || {}
    for (const resultKey in agentResults) {
      if (!String(resultKey).startsWith('eval-')) continue
      const result = agentResults[resultKey]
      if (result?.loading || result?.queued) {
        return true
      }
    }
  }

  return false
})

const progressPhase = computed(() => {
  if (!isRunning.value) return 'benchmark'

  const phaseHasEvaluationTotals =
    primaryTaskCapacity.value > 0 &&
    progressCounters.value.total > primaryTaskCapacity.value &&
    progressCounters.value.completed >= primaryTaskCapacity.value

  if (hasActiveRetryEntries() && !hasActiveEvaluatorActivity.value) {
    return 'retry'
  }

  if (hasActiveEvaluatorActivity.value || phaseHasEvaluationTotals) {
    return 'evaluation'
  }

  return 'benchmark'
})

const progressPhaseLabel = computed(() => {
  if (progressPhase.value === 'evaluation') return 'Evaluation'
  if (progressPhase.value === 'retry') return 'Retry'
  return 'Benchmark'
})

const progressEvaluationCounters = computed(() => {
  const evaluationTotal = Math.max(progressCounters.value.total - primaryTaskCapacity.value, 0)
  if (evaluationTotal === 0) {
    return {
      total: 0,
      completed: 0
    }
  }

  return {
    total: evaluationTotal,
    completed: Math.min(Math.max(progressCounters.value.completed - primaryTaskCapacity.value, 0), evaluationTotal)
  }
})

const progressHeadline = computed(() => {
  if (progressPhase.value === 'evaluation' && progressEvaluationCounters.value.total > 0) {
    return `Evaluations ${progressEvaluationCounters.value.completed}/${progressEvaluationCounters.value.total}`
  }
  if (progressPhase.value === 'retry') {
    return `Retrying ${progressCounters.value.completed}/${progressCounters.value.total}`
  }
  return `Benchmark ${progressCounters.value.completed}/${progressCounters.value.total}`
})

const progressTaskRatioText = computed(() => {
  if (progressCounters.value.total === 0) return '0%'
  return `${progressPercent.value}% complete`
})

const progressEtaText = computed(() => {
  const remaining = Math.max(progressCounters.value.total - progressCounters.value.completed, 0)
  if (remaining === 0) return ''

  const recent = completionTimeline.value.slice(-12)
  if (recent.length < 2) return ''

  const spanMs = recent[recent.length - 1] - recent[0]
  if (!Number.isFinite(spanMs) || spanMs <= 0) return ''

  const completionsPerMs = (recent.length - 1) / spanMs
  if (!Number.isFinite(completionsPerMs) || completionsPerMs <= 0) return ''

  const etaMs = remaining / completionsPerMs
  if (!Number.isFinite(etaMs) || etaMs <= 0) return ''

  const etaMinutes = Math.round(etaMs / 60000)
  if (etaMinutes >= 2) return `ETA ~${etaMinutes}m`

  const etaSeconds = Math.max(1, Math.round(etaMs / 1000))
  return `ETA ~${etaSeconds}s`
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
const showEvaluationScoreFilters = computed(() => hasEnabledEvaluators.value)

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
      const hasExplicitSelection = Array.isArray(currentQuestionSet.value?.agents)
      if (hasExplicitSelection) {
        const enabledIdSet = new Set(selectedAgentIdsForQuestionSet.value)
        const activeAgentsWithResults = mergedAgents.value.filter((agent) =>
          enabledIdSet.has(agent.id) && resultAgentIds.includes(agent.id)
        )
        list = activeAgentsWithResults.length > 0 ? activeAgentsWithResults : [...enabledAgents.value]
      } else {
        // Legacy fallback when question set has no explicit agent envelope yet.
        const agentsWithResults = mergedAgents.value.filter((agent) => resultAgentIds.includes(agent.id))
        const oldAgentIds = resultAgentIds.filter((id) => !mergedAgents.value.some((agent) => agent.id === id))
        const oldAgents = oldAgentIds.map((id) => ({ id, name: 'Agent (historical)', provider_type: 'unknown' }))
        list = [...agentsWithResults, ...oldAgents]
      }
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
    mergedAgents: enabledAgents.value,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying
  })
}

function getQuestionResponse(questionId, truncated = true) {
  return getQuestionResponseUtil({
    runResults: runResults.value,
    questionId,
    mergedAgents: enabledAgents.value,
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
    mergedAgents: enabledAgents.value,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying,
    truncatePreviewText,
    truncated
  })
}

function formatScoreOutOfTen(score) {
  if (!Number.isFinite(score)) return ''
  const rounded = Math.round(score * 10) / 10
  return Number.isInteger(rounded) ? `${rounded}/10` : `${rounded.toFixed(1)}/10`
}

function getQuestionEvaluationScoreValue(questionId) {
  const fullText = getQuestionEvaluation(questionId, false)
  const score = extractScoreOutOfTen(fullText)
  return Number.isFinite(score) ? score : null
}

function getQuestionEvaluationScore(questionId) {
  const score = getQuestionEvaluationScoreValue(questionId)
  return score == null ? '' : formatScoreOutOfTen(score)
}

function getQuestionEvaluationSeverity(questionId) {
  const score = getQuestionEvaluationScoreValue(questionId)
  if (score == null) return 'ok'
  if (score < 1) return 'danger'
  if (score < 5) return 'warning'
  return 'ok'
}

function getQuestionEvaluationSeverityClass(questionId) {
  const severity = getQuestionEvaluationSeverity(questionId)
  if (severity === 'danger') return 'question-evaluation-danger'
  if (severity === 'warning') return 'question-evaluation-warning'
  return ''
}

function getQuestionEvaluationScoreChipClass(questionId) {
  const severity = getQuestionEvaluationSeverity(questionId)
  if (severity === 'danger') return 'score-chip-danger'
  if (severity === 'warning') return 'score-chip-warning'
  return ''
}

function getQuestionExpectedAnswer(question) {
  if (!question || typeof question !== 'object') return ''
  return question.expected || question.expected_answer || ''
}

function isQuestionSelected(questionId) {
  return String(selectedQuestionId.value || '') === String(questionId || '')
}

function getQuestionExpectedAnswerPreview(question) {
  return getContentPreviewText(getQuestionExpectedAnswer(question), 220)
}

function getQuestionResponsePreview(questionId) {
  return getContentPreviewText(getQuestionResponse(questionId, false), 220)
}

function getQuestionEvaluationPreview(questionId) {
  return getContentPreviewText(getQuestionEvaluation(questionId, false), 220)
}

function isEditingExpectedAnswer(questionId) {
  return String(editingExpectedQuestionId.value || '') === String(questionId || '')
}

function startExpectedAnswerEdit(question) {
  const questionId = String(question?.id || '')
  if (!questionId) return
  editingExpectedQuestionId.value = questionId
  expectedAnswerDraft.value = getQuestionExpectedAnswer(question)
}

function cancelExpectedAnswerEdit() {
  if (savingExpectedAnswerQuestionId.value) return
  editingExpectedQuestionId.value = ''
  expectedAnswerDraft.value = ''
}

function buildQuestionSetDataWithExpectedAnswer(questionSet, questionId, expectedAnswer) {
  let rawData = questionSet?.data
  if (!rawData) return null

  if (typeof rawData === 'string') {
    try {
      rawData = JSON.parse(rawData)
    } catch (e) {
      return null
    }
  }

  const targetQuestionId = String(questionId || '')
  let found = false

  const categories = (rawData.categories || []).map((category, catIdx) => ({
    ...category,
    questions: (category.questions || []).map((question, index) => {
      const currentId = question?.id != null && question.id !== '' ? String(question.id) : `${catIdx + 1}-${index + 1}`
      if (currentId !== targetQuestionId) {
        return { ...question }
      }

      found = true
      const updatedQuestion = {
        ...question,
        expected: expectedAnswer
      }
      if (Object.prototype.hasOwnProperty.call(updatedQuestion, 'expected_answer')) {
        updatedQuestion.expected_answer = expectedAnswer
      }
      if (Object.prototype.hasOwnProperty.call(updatedQuestion, 'expectedAnswer')) {
        updatedQuestion.expectedAnswer = expectedAnswer
      }
      return updatedQuestion
    })
  }))

  if (!found) return null
  return {
    ...rawData,
    categories
  }
}

async function saveExpectedAnswer(question) {
  const questionId = String(question?.id || editingExpectedQuestionId.value || '')
  if (!questionId || !currentQuestionSet.value?.id) return

  const nextExpectedAnswer = expectedAnswerDraft.value.trim()
  const currentExpectedAnswer = getQuestionExpectedAnswer(question)
  if (nextExpectedAnswer === currentExpectedAnswer) {
    cancelExpectedAnswerEdit()
    return
  }

  const updatedData = buildQuestionSetDataWithExpectedAnswer(currentQuestionSet.value, questionId, nextExpectedAnswer)
  if (!updatedData) {
    startRunError.value = 'Failed to update expected answer: question not found in question set.'
    return
  }

  try {
    savingExpectedAnswerQuestionId.value = questionId
    const updated = await wsService.updateQuestionSet(currentQuestionSet.value.id, {
      name: currentQuestionSet.value.name,
      version: currentQuestionSet.value.version || '1.0',
      data: updatedData
    })

    currentQuestionSet.value = mergeQuestionSetForUI(updated, currentQuestionSet.value)
    cacheQuestionSetState(currentQuestionSet.value.id, currentQuestionSet.value)
    editingExpectedQuestionId.value = ''
    expectedAnswerDraft.value = ''
  } catch (e) {
    console.error('[Arena] Failed to save expected answer inline:', e)
    startRunError.value = e?.message || 'Failed to save expected answer.'
  } finally {
    savingExpectedAnswerQuestionId.value = ''
  }
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

function handleQuestionSetDeleted(deleted) {
  const deletedId = String(deleted?.id || '')
  if (!deletedId) return

  questionSetStateCache.delete(deletedId)
  if (String(activeRunQuestionSetId.value || '') === deletedId) {
    activeRunQuestionSetId.value = null
    wsStore.setRunningQuestionSetId(null)
  }

  if (String(currentQuestionSet.value?.id || '') === deletedId) {
    currentRun.value = null
    runResults.value = {}
    taskProgress.value = {}
    selectedQuestionId.value = ''
    isRunning.value = false
    currentQuestionSet.value = null
  }
}

function selectQuestionForAnalysis(questionId, options = {}) {
  const nextId = String(questionId || '')
  if (!nextId) return

  selectedQuestionId.value = nextId
}

function prevQuestion() {
  const idx = currentQuestionIndex.value
  if (idx > 0) {
    selectQuestionForAnalysis(flatQuestions.value[idx - 1].id)
  }
}

function nextQuestion() {
  const idx = currentQuestionIndex.value
  if (idx < flatQuestions.value.length - 1) {
    selectQuestionForAnalysis(flatQuestions.value[idx + 1].id)
  }
}

function getQuestionKey(text) { return text }

function getQuestionFilterLabel(filter = questionFilter.value) {
  switch (filter) {
    case 'error':
      return 'questions with errors'
    case 'running':
      return 'running questions'
    case 'low_score':
      return 'questions below 5/10'
    case 'critical_score':
      return 'questions below 1/10'
    default:
      return 'visible questions'
  }
}

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

watch(recentRunsSyncKey, () => {
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
})

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

watch(showEvaluationScoreFilters, (visible) => {
  if (visible) return
  if (questionFilter.value === 'low_score' || questionFilter.value === 'critical_score') {
    questionFilter.value = 'all'
  }
}, { immediate: true })

watch(selectedQuestionEntry, (entry) => {
  if (entry) return
  if (pdfExportScope.value === 'selected') {
    pdfExportScope.value = 'auto'
  }
})

watch(() => currentRun.value?.id, (newRunId, oldRunId) => {
  if (!newRunId || newRunId === oldRunId) return
  const cachedState = questionSetStateCache.get(String(currentQuestionSet.value?.id || ''))
  const cachedRunId = String(cachedState?.currentRun?.id || '')
  const cachedTimeline = Array.isArray(cachedState?.completionTimeline) ? cachedState.completionTimeline : []
  if (cachedRunId === String(newRunId) && cachedTimeline.length > 0) return
  completionTimeline.value = []
})

function splitSelectedAgents(payload = {}) {
  return splitSelectedAgentsUtil(payload, isEvaluatorAgentID)
}

const {
  startEvaluationNow,
  handleStartRun,
  processTaskCompleted,
  cancelBenchmark,
  rerunQuestion,
  retryQuestionForAllAgents,
  retryQuestionForEvaluators,
  retryFailedPrimaryResults,
  retryFailedEvaluatorResults,
  getFailedRetryTargets,
  getQuestionEvaluatorTargets,
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
  getAgentResults,
  onTaskCompleted: () => {
    const now = Date.now()
    completionTimeline.value = [...completionTimeline.value.slice(-11), now]
  }
})

const failedPrimaryRetryTargets = computed(() => getFailedRetryTargets('primary'))
const failedPrimaryRetryCount = computed(() => failedPrimaryRetryTargets.value.length)

const failedEvaluatorRetryTargets = computed(() => getFailedRetryTargets('evaluator'))
const failedEvaluatorRetryCount = computed(() => failedEvaluatorRetryTargets.value.length)

const canRetryFailedPrimary = computed(() => {
  return !!currentRun.value?.id && failedPrimaryRetryCount.value > 0 && !isRunning.value
})

const canRetryFailedEvaluators = computed(() => {
  return !!currentRun.value?.id && failedEvaluatorRetryCount.value > 0 && !isRunning.value
})

const retryFailedPrimaryTitle = computed(() => {
  if (!currentRun.value?.id) return 'No run available to retry'
  if (isRunning.value) return 'Wait for the current run or retry activity to finish'
  return `Retry ${failedPrimaryRetryCount.value} failed agent result${failedPrimaryRetryCount.value === 1 ? '' : 's'}`
})

const retryFailedEvaluatorTitle = computed(() => {
  if (!currentRun.value?.id) return 'No run available to retry'
  if (isRunning.value) return 'Wait for the current run or retry activity to finish'
  return `Retry ${failedEvaluatorRetryCount.value} failed evaluator result${failedEvaluatorRetryCount.value === 1 ? '' : 's'}`
})

function getQuestionEvaluatorRetryCount(questionId) {
  return getQuestionEvaluatorTargets(questionId).length
}

function getQuestionEvaluatorRetryTitle(questionId) {
  const count = getQuestionEvaluatorRetryCount(questionId)
  if (count === 0) return 'No evaluator available for this question'
  if (isQuestionLoading(questionId)) return 'Wait for the current question activity to finish'
  return `Retry evaluation for this question${count > 1 ? ` (${count} evaluator targets)` : ''}`
}

async function handleRunSave(payload) {
  const savedQuestionSet = payload?.questionSet
  if (!currentQuestionSet.value || !savedQuestionSet) return

  console.log('[Arena] handleRunSave: Received saved QS with', savedQuestionSet.agents?.length, 'agents')

  if (savedQuestionSet.id === currentQuestionSet.value.id) {
    const hasSavedAgents = Array.isArray(savedQuestionSet.agents)
    const hasPayloadAgents = Array.isArray(payload?.agents)
    const newAgents = hasSavedAgents
      ? savedQuestionSet.agents
      : (hasPayloadAgents ? payload.agents : currentQuestionSet.value.agents)
    
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
    const questionSetId = String(currentQuestionSet.value.id)
    const latestRunId = String(getRecentRunIdForQS(questionSetId) || '')
    const currentRunId = String(currentRun.value?.id || '')

    if (!currentRunId || (latestRunId && latestRunId !== currentRunId)) {
      latestRunCache.delete(questionSetId)
      await fetchLatestResultsForQS(questionSetId)
    }
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

function buildCompactQuestionCards(entries = []) {
  return entries.map(({ question, index }) => {
    const questionId = String(question?.id || '')
    return {
      id: questionId,
      questionNumber: index + 1,
      question: question?.question || question?.text || '',
      grounding: getQuestionExpectedAnswer(question),
      response: extractTextOnly(getQuestionResponse(questionId, false) || ''),
      evaluation: extractTextOnly(getQuestionEvaluation(questionId, false) || ''),
      evaluationScore: getQuestionEvaluationScore(questionId),
      evaluationSeverity: getQuestionEvaluationSeverity(questionId)
    }
  })
}

function resolvePdfScope(scope = pdfExportScope.value) {
  if (scope === 'auto') {
    if (canExportSelectedQuestionPdf.value) return 'selected'
    if (questionFilter.value !== 'all' && canExportFilteredQuestionsPdf.value) return 'filtered'
    return 'full'
  }
  if (scope === 'selected') return canExportSelectedQuestionPdf.value ? 'selected' : 'full'
  if (scope === 'filtered') return canExportFilteredQuestionsPdf.value ? 'filtered' : 'full'
  return 'full'
}

function getPdfQuestionEntries(scope) {
  if (scope === 'selected') {
    return selectedQuestionEntry.value ? [selectedQuestionEntry.value] : []
  }
  if (scope === 'filtered') {
    return filteredQuestionEntries.value
  }
  return []
}

function getCompactPdfHeader(scope, count) {
  if (scope === 'selected') {
    return {
      reportTitle: 'Selected Question Analysis',
      reportSubtitle: 'Compact export with question, grounding, response, and evaluation.'
    }
  }

  return {
    reportTitle: count === 1 ? 'Filtered Question Analysis' : 'Filtered Questions Analysis',
    reportSubtitle: `${count} question${count === 1 ? '' : 's'} from ${getQuestionFilterLabel()}.`
  }
}

function triggerCompactPdfPrint(entries = [], scope = 'selected') {
  const questionCards = buildCompactQuestionCards(entries)
  if (questionCards.length === 0) {
    startRunError.value = 'No questions available for this PDF export scope.'
    return false
  }

  const { reportTitle, reportSubtitle } = getCompactPdfHeader(scope, questionCards.length)
  emit('trigger-print', {
    workspaceName: currentQuestionSet.value?.name || 'Benchmark',
    summary: null,
    results: [],
    reportVariant: 'question_cards',
    reportTitle,
    reportSubtitle,
    questionCards
  })
  return true
}

function exportQuestionEntryToPdf(entry) {
  if (!entry || isExportingPdf.value) return
  selectQuestionForAnalysis(entry.question?.id || selectedQuestionId.value)
  triggerCompactPdfPrint([entry], 'selected')
}

function exportFilteredQuestionsPdf() {
  if (isExportingPdf.value) return
  triggerCompactPdfPrint(filteredQuestionEntries.value, 'filtered')
}

async function exportToPdf(scope = pdfExportScope.value) {
  if (!currentRun.value || isExportingPdf.value) return
  
  isExportingPdf.value = true
  
  try {
    // Ensure results are loaded before exporting
    if (isLoadingResults.value || hasLoadingResults()) {
      await waitForResultsToLoad()
    }

    const resolvedScope = resolvePdfScope(scope)
    if (resolvedScope !== 'full') {
      const entries = getPdfQuestionEntries(resolvedScope)
      triggerCompactPdfPrint(entries, resolvedScope)
      return
    }
    
    // Build agents array from displayAgents with their results
    const agentsArray = displayAgents.value.map(agent => ({
      id: agent.id,
      name: agent.name || agent.config?.name || 'Agent',
      provider: agent.provider_type,
      config: agent.config,
      results: getAgentResults(agent.id, true) // true = include all questions
    }))

    const pData = exportResultsReport({
      agentsRef: agentsArray,
      calculateStats: calculateStats,
      questionSetData: currentQuestionSet.value?.data || null
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
