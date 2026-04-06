<template>
  <div class="app-root" :class="{ 'has-banner': isImpersonating }">
    <!-- Impersonation Banner -->
    <div v-if="isImpersonating" class="impersonation-banner no-print">
      <div class="banner-content">
        <span class="banner-icon">🕵️</span>
        <span class="banner-text">
          You are currently logged in as <strong>{{ currentUserName }}</strong>
        </span>
        <button class="btn btn-primary btn-stop-impersonation" @click="handleLogout">
          Stop Impersonating
        </button>
      </div>
    </div>

    <!-- Global Print Preview Overlay -->
    <Teleport to="body">
      <div v-if="showPrintView" class="global-print-preview-overlay">
        <div class="print-toolbar no-print">
          <button class="btn-back-print" @click="showPrintView = false">
            ← Back to App
          </button>
          <div class="print-actions">
            <span class="print-hint">Preview your report, then use Print (Cmd+P / Ctrl+P) to save as PDF</span>
            <button class="btn-print-now" @click="triggerBrowserPrint">🖨️ Print / Save PDF</button>
          </div>
        </div>
        <div class="print-content">
          <PrintReport 
            :workspace-name="printData.workspaceName || 'Benchmark'"
            :summary-stats="printData.summary"
            :results="printData.results"
            :report-variant="printData.reportVariant || 'full'"
            :report-title="printData.reportTitle"
            :report-subtitle="printData.reportSubtitle"
            :question-cards="printData.questionCards || []"
          />
        </div>
      </div>
    </Teleport>

    <!-- Login Screen -->
    <LoginScreen v-if="!isAuthenticated" @login="onLogin" class="no-print" />

    <!-- Onboarding Screen -->
    <OnboardingScreen v-else-if="showOnboarding" @completed="onOnboardingCompleted" />

    <!-- Loading State while connection is establishing -->
    <div v-else-if="!appReady" class="app-init-loader">
      <div class="spinner"></div>
      <p>Initializing connection...</p>
    </div>

    <!-- Main App (when authenticated and ready) -->
    <template v-else>
      <!-- Workspace Selector Modal - Hidden: Each user has their own workspace -->

      <!-- Main App Container -->
      <div class="app-container">
        <!-- Header -->
        <header class="header">
          <div class="header-left">
        <CorvicLogo width="100px" height="28px" class="header-logo" @click="viewMode = 'benchmarks'" />
        <h1 @click="viewMode = 'benchmarks'">Benchmarking</h1>
      </div>
          <div class="controls">
            <div v-if="workspaces.length > 1" class="workspace-switcher">
              <label for="workspace-switcher-select">Workspace</label>
              <select
                id="workspace-switcher-select"
                :value="currentWorkspace?.id || ''"
                @change="handleWorkspaceSelectionChange"
              >
                <option v-for="workspace in workspaces" :key="workspace.id" :value="workspace.id">
                  {{ workspace.name }}
                </option>
              </select>
            </div>
            <div class="buttons">
              <span class="user-badge" :class="{ admin: isAdmin }" @click="openMyProfile">
                {{ isAdmin ? '👑' : '👤' }} {{ currentUserName }}
              </span>
              <button class="btn btn-danger btn-logout-compact icon-only has-tooltip"
                      title="Logout"
                      aria-label="Logout"
                      @click="handleLogout">
                <span class="logout-icon">→</span>
              </button>

            </div>
          </div>
        </header>

        <!-- Navigation for views -->
        <nav class="app-nav">
          <button 
            class="nav-btn" 
            :class="{ active: viewMode === 'benchmarks' }"
            @click="handleArenaHistoryClick"
          >{{ getArenaHistoryLabel }}</button>
          <button 
            class="nav-btn" 
            :class="{ active: viewMode === 'stats' }"
            @click="viewMode = 'stats'"
          >📊 Stats</button>
          <button
            v-if="isAdmin"
            class="nav-btn"
            :class="{ active: viewMode === 'admin' || viewMode === 'admin-profile' }"
            @click="openAdminPanel"
          >⚙️ Admin</button>
          <button 
            class="nav-btn" 
            :class="{ active: viewMode === 'docs' }"
            @click="viewMode = 'docs'"
          >📂 Docs</button>
        </nav>

        <!-- Stats View -->
    <main v-if="viewMode === 'stats' && currentWorkspace" class="main-content">
      <StatsView 
        :workspaceId="currentWorkspace.id" 
        :isAdmin="isAdmin"
      />
    </main>

    <!-- Admin Panel -->
    <main v-if="viewMode === 'admin' && isAdmin" class="main-content no-padding">
      <AdminPanel
        :current-user-id="currentUserId"
        :initial-tab="adminTab"
        @close="viewMode = 'benchmarks'"
        @view-user-profile="handleViewUserProfile"
        @view-org-profile="handleViewOrgProfile"
        @tab-change="val => adminTab = val"
      />
    </main>

    <!-- Admin Profile View -->
    <main
      v-if="viewMode === 'admin-profile' && isAdmin && profileEntityType && profileEntityId"
      class="main-content"
    >
      <AdminProfileView
        :entity-type="profileEntityType"
        :entity-id="profileEntityId"
        @back="closeAdminProfile"
        @view-user="handleViewUserProfile"
        @view-org="handleViewOrgProfile"
        @updated="handleProfileUpdated"
      />
    </main>


    <!-- Manager Panel -->
    <main v-if="viewMode === 'manager' && isManager" class="main-content">
      <ManagerPanel 
        :current-user-id="currentUserId"
        :workspace-id="currentWorkspace?.id"
      />
    </main>
    
    <!-- Docs View -->
    <main v-if="viewMode === 'docs'" class="main-content">
      <DocsView />
    </main>

        <!-- Main Benchmarking Content -->
        <!-- History View -->
        <!-- <main v-if="viewMode === 'benchmarks' && benchmarkMode === 'history'" class="main-content no-padding">
        
          <BenchmarkDocumentView 
             :workspace-id="currentWorkspace?.id"
             :pre-filter="historyFilter"
             @back="benchmarkMode = 'runner'"
             @trigger-print="handleTriggerPrint"
          />
        </main> -->

        <!-- Arena View -->
        <!-- Arena View - Always show when in benchmarks mode -->
        <main v-if="viewMode === 'benchmarks'" class="main-content no-padding">
          <KeepAlive>
            <BenchmarkArena 
                v-if="viewMode === 'benchmarks'"
                :key="currentWorkspace?.id || 'no-workspace'"
                :workspace-id="currentWorkspace?.id"
                :workspaces="workspaces"
                :agents="agents || []"
                :question-sets="questionSets || []"
                :initial-question-set-id="currentQuestionSet?.id"
                @update:currentQuestionSet="val => currentQuestionSet = val"
                @view-history="goToHistory"
                @trigger-print="handleTriggerPrint"
                @manage-agents="showConfig = true"
            />
          </KeepAlive>
          <!-- Workspace is automatically selected for each user -->
        </main>

      </div>


      <!-- Question Editor Modal (Global access) -->
      <QuestionEditorModal 
        v-if="showQuestionEditor"
        :question-set="currentQuestionSet"
        :workspace-id="currentWorkspace?.id"
        @close="onQuestionEditorClose"
        @saved="onQuestionSetSaved"
      />

      <!-- Import Questions Modal -->
      <ImportQuestionsModal
        v-if="showImportModal"
        :question-set="currentQuestionSet"
        :workspace-id="currentWorkspace?.id"
        @close="showImportModal = false"
        @imported="handleImported"
      />

      <!-- Agent Manager Modal (Global) -->
      <AgentManagerModal
        v-if="showConfig"
        :agents="agents"
        :workspace-id="currentWorkspace?.id"
        :question-set="currentQuestionSet"
        @update="loadAgents"
        @close="showConfig = false"
      />

      <QuestionSetShareAcceptModal
        v-if="showShareAcceptModal && pendingShareToken"
        :token="pendingShareToken"
        :workspaces="workspaces"
        :current-workspace-id="currentWorkspace?.id"
        @close="dismissPendingShareLink"
        @accepted="handleQuestionSetShareAccepted"
      />
    </template>
    <MaintenanceOverlay :active="wsState.isMaintenance" />
    <AfkReconnectOverlay
      :active="afkOverlayVisible"
      :reconnecting="isReconnectingFromAfk"
      :mode="reconnectOverlayMode"
      @reconnect="reconnectFromAfk"
    />
    <div
      v-if="afkDebug.enabled"
      style="position: fixed; left: 12px; bottom: 12px; z-index: 12000; width: min(420px, calc(100vw - 24px)); background: rgba(2, 6, 23, 0.92); color: #e2e8f0; border: 1px solid rgba(148, 163, 184, 0.35); border-radius: 10px; padding: 10px 12px; font: 12px/1.35 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; box-shadow: 0 12px 24px rgba(2, 6, 23, 0.45);"
    >
      <div style="font-weight: 700; color: #93c5fd; margin-bottom: 6px;">AFK DEBUG</div>
      <div>timeout={{ Math.round(effectiveAfkTimeoutMs / 1000) }}s{{ wsState.runningQuestionSetId ? ' (2x run)' : '' }} | remaining={{ afkDebugRemainingLabel }}</div>
      <div>last={{ afkDebug.lastActivitySource }} | {{ afkDebugLastActivityLabel }}</div>
      <div>canTrack={{ afkDebug.canTrack }} ws={{ wsState.isConnected }} vis={{ afkDebug.visibility }} focus={{ afkDebug.hasFocus }}</div>
      <div>overlay={{ afkOverlayVisible }} ({{ reconnectOverlayMode }}) reconnecting={{ isReconnectingFromAfk }}</div>
      <div>events={{ afkDebug.activityCount }} ignored={{ afkDebug.ignoredCount }} heartbeats={{ afkDebug.heartbeatCount }}</div>
      <div v-if="afkDebug.lastDisconnectReason">lastDisconnect={{ afkDebug.lastDisconnectReason }}</div>
      <div style="display: flex; gap: 8px; margin-top: 8px;">
        <button
          style="border: 1px solid #475569; background: #0f172a; color: #e2e8f0; border-radius: 6px; padding: 4px 8px; cursor: pointer;"
          @click="markUserActivity(true, 'debug-button')"
        >
          Ping activity
        </button>
        <button
          style="border: 1px solid #7f1d1d; background: #450a0a; color: #fecaca; border-radius: 6px; padding: 4px 8px; cursor: pointer;"
          @click="forceAfkDebug"
        >
          Force AFK
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import LoginScreen from './components/LoginScreen.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import CorvicLogo from './components/CorvicLogo.vue'
import BenchmarkArena from './components/BenchmarkArena.vue'
import BenchmarkDocumentView from './components/BenchmarkDocumentView.vue'
import AdminPanel from './components/AdminPanel.vue'
import AdminProfileView from './components/AdminProfileView.vue'
import ManagerPanel from './components/ManagerPanel.vue';
import StatsView from './components/StatsView.vue';
import QuestionEditorModal from './components/QuestionEditorModal.vue';
import ImportQuestionsModal from './components/ImportQuestionsModal.vue';
import MaintenanceOverlay from './components/MaintenanceOverlay.vue'
import AfkReconnectOverlay from './components/AfkReconnectOverlay.vue'
import AgentManagerModal from './components/AgentManagerModal.vue'
import DocsView from './components/DocsView.vue'
import PrintReport from './components/PrintReport.vue'
import QuestionSetShareAcceptModal from './components/QuestionSetShareAcceptModal.vue'
import * as api from './services/api.js'
import wsService from './services/websocket.js'
import { useWSStore } from './stores/wsStore'
import { downloadManager } from './services/DownloadManager.js'
import { contentCache } from './services/ContentCache.js'
import { generateQuestionSetName } from './utils/nameGenerator.js'
import { getQuestionSetListSyncSignature, getQuestionSetSyncSignature, resolveQuestionSetSelection } from './utils/arena/questionSet.js'
import { capturePostHogEvent, identifyPostHogUser, resetPostHogUser } from './services/posthog.js'
import { config } from './config'
import './App.css'

const { state: wsState, syncState, connect: wsConnect, disconnect: wsDisconnect } = useWSStore()

// Auth State
const isAuthenticated = ref(api.isLoggedIn())
const currentUser = ref(api.getStoredUser())
const initialViewMode = localStorage.getItem('viewMode') || 'benchmarks'
const viewMode = ref(initialViewMode === 'admin-profile' ? 'admin' : initialViewMode); // 'benchmarks', 'stats', 'manager', 'admin', 'admin-profile'
const benchmarkMode = ref(localStorage.getItem('benchmarkMode') || 'runner') // 'history', 'runner'
const isLoggingIn = ref(false) // Flag to prevent concurrent initialization during login

// Manager state
const isManager = ref(false)
const appReady = ref(false)
const currentUserRecord = computed(() => currentUser.value?.user || currentUser.value || null)
const currentUserId = computed(() => currentUserRecord.value?.id || currentUserRecord.value?.ID || '')
const currentUserName = computed(() => currentUserRecord.value?.name || '')
const currentUserEmail = computed(() => currentUserRecord.value?.email || '')
const isAdmin = computed(() => Boolean(currentUserRecord.value?.is_admin))
const profileEntityType = ref(null)
const profileEntityId = ref(null)
const adminTab = ref('users')


// Watch benchmarkMode to persist
// Watch benchmarkMode to persist
watch(benchmarkMode, (newVal) => {
  localStorage.setItem('benchmarkMode', newVal)
})

watch(viewMode, (newVal) => {
  localStorage.setItem('viewMode', newVal)
})

watch(isAdmin, (adminEnabled) => {
  if (adminEnabled) return
  if (viewMode.value === 'admin' || viewMode.value === 'admin-profile') {
    closeAdminProfile()
    viewMode.value = 'benchmarks'
  }
}, { immediate: true })



// State
const showWorkspaceModal = ref(false)
const showActionsModal = ref(false)
const showImportModal = ref(false)
const showConfig = ref(false)
const showSummary = ref(false)
const showQuestionEditor = ref(false)
const previousQuestionSet = ref(null) // Used to restore when canceling new set creation
const showRunSetup = ref(false)
const showOnboarding = ref(false)

const workspaces = ref([])
const workspacesLoading = ref(false)
const workspacesError = ref('')
const currentWorkspace = ref(api.getStoredWorkspace())
const pendingShareToken = ref('')
const showShareAcceptModal = ref(false)
const refreshInterval = ref(null)
const afkOverlayVisible = ref(false)
const isReconnectingFromAfk = ref(false)
const reconnectOverlayMode = ref('afk')
const reconnectOverlayGraceMs = 7000
let reconnectOverlayTimer = null

watch(
  [
    currentUserId,
    currentUserEmail,
    currentUserName,
    isAdmin,
    () => currentWorkspace.value?.id || '',
    () => currentWorkspace.value?.name || ''
  ],
  () => {
    if (!isAuthenticated.value || !currentUserId.value) return
    identifyPostHogUser({
      userId: currentUserId.value,
      email: currentUserEmail.value,
      name: currentUserName.value,
      workspaceId: currentWorkspace.value?.id || '',
      workspaceName: currentWorkspace.value?.name || '',
      isAdmin: isAdmin.value
    })
  },
  { immediate: true }
)

watch(
  [isAuthenticated, appReady, () => workspaces.value.length],
  () => {
    maybeOpenPendingShareLink()
  }
)

const DEFAULT_AFK_TIMEOUT_MS = 300000
const MIN_AFK_TIMEOUT_MS = 60000
const afkForegroundHeartbeatMs = 15000
let afkForegroundHeartbeat = null
const afkDebugStorageKey = 'afk_debug'
const afkDebugTickMs = 1000
let afkDebugTicker = null

function parseAfkTimeoutMs(rawValue) {
  if (rawValue == null) return null
  const value = String(rawValue).trim().toLowerCase()
  if (!value) return null

  if (/^\d+$/.test(value)) {
    const timeout = Number.parseInt(value, 10)
    return Number.isFinite(timeout) ? timeout : null
  }

  const match = value.match(/^(\d+(?:\.\d+)?)\s*(ms|s|m|h)$/)
  if (!match) return null

  const amount = Number.parseFloat(match[1])
  const unit = match[2]
  if (!Number.isFinite(amount) || amount <= 0) return null

  if (unit === 'ms') return Math.round(amount)
  if (unit === 's') return Math.round(amount * 1000)
  if (unit === 'm') return Math.round(amount * 60000)
  if (unit === 'h') return Math.round(amount * 3600000)
  return null
}

const parsedAfkTimeout = parseAfkTimeoutMs(config.AFK_TIMEOUT_MS)
const afkTimeoutMs = Number.isFinite(parsedAfkTimeout) && parsedAfkTimeout >= MIN_AFK_TIMEOUT_MS
  ? parsedAfkTimeout
  : DEFAULT_AFK_TIMEOUT_MS

// During an active benchmark run, double the AFK timeout so long runs don't
// unexpectedly disconnect the user who may be watching progress.
const effectiveAfkTimeoutMs = computed(() =>
  wsState.runningQuestionSetId ? afkTimeoutMs * 3 : afkTimeoutMs
)
if (parsedAfkTimeout != null && parsedAfkTimeout < MIN_AFK_TIMEOUT_MS) {
  console.warn('[AFK] Ignoring too-low AFK timeout configuration', {
    configured: config.AFK_TIMEOUT_MS,
    parsedMs: parsedAfkTimeout,
    minMs: MIN_AFK_TIMEOUT_MS,
    usingMs: afkTimeoutMs
  })
}

const afkActivityEvents = [
  'pointerdown',
  'pointerup',
  'pointermove',
  'mousemove',
  'mousedown',
  'mouseup',
  'touchstart',
  'touchmove',
  'scroll',
  'wheel',
  'keydown',
  'keyup',
  'click',
  'dblclick',
  'input',
  'focus'
]
let afkTimer = null
let lastAfkResetAt = 0
const afkListenerOptions = { capture: true, passive: true }
const afkDebug = ref({
  enabled: false,
  nowMs: Date.now(),
  remainingMs: null,
  lastActivityAt: 0,
  lastActivitySource: '-',
  visibility: typeof document !== 'undefined' ? document.visibilityState : 'unknown',
  hasFocus: typeof document !== 'undefined' ? document.hasFocus() : false,
  canTrack: false,
  wsConnected: false,
  overlayVisible: false,
  overlayMode: 'afk',
  activityCount: 0,
  ignoredCount: 0,
  heartbeatCount: 0,
  lastDisconnectReason: '',
  lastDisconnectAt: 0
})

const afkDebugRemainingLabel = computed(() => {
  const remainingMs = afkDebug.value.remainingMs
  if (remainingMs == null) return '-'
  return `${Math.max(0, Math.ceil(remainingMs / 1000))}s`
})

const afkDebugLastActivityLabel = computed(() => {
  if (!afkDebug.value.lastActivityAt) return '-'
  const ageSec = Math.max(0, Math.floor((afkDebug.value.nowMs - afkDebug.value.lastActivityAt) / 1000))
  const time = new Date(afkDebug.value.lastActivityAt).toLocaleTimeString()
  return `${ageSec}s ago @ ${time}`
})

const agents = computed(() => wsState.agents)
const questionSets = computed(() => wsState.questionSets)
const currentQuestionSet = ref(null)
const loadingResults = ref(false)


const historyFilter = ref('')

// Print state
const showPrintView = ref(false)
const printData = ref({
  workspaceName: '',
  summary: {},
  results: [],
  reportVariant: 'full',
  reportTitle: '',
  reportSubtitle: '',
  questionCards: []
})

watch(currentWorkspace, (newVal) => {
  if (newVal) {
    localStorage.setItem('workspace', JSON.stringify(newVal))
  } else {
    localStorage.removeItem('workspace')
  }
}, { deep: true })

watch(currentUser, (newUser) => {
  if (newUser) {
    localStorage.setItem('user', JSON.stringify(newUser))
  } else {
    localStorage.removeItem('user')
  }
}, { deep: true })


// Restore Logic
watch(currentQuestionSet, async (newSet) => {
  if (newSet) {
    localStorage.setItem('lastQuestionSetId', newSet.id)
    await loadAgents()
  } else {
    localStorage.removeItem('lastQuestionSetId')
    await loadAgents() // Revert to global agents
  }
})

const questionSetListSyncKey = computed(() => getQuestionSetListSyncSignature(wsState.questionSets))

watch(questionSetListSyncKey, () => {
  const sets = Array.isArray(wsState.questionSets) ? wsState.questionSets : []

  if (!currentQuestionSet.value) {
    if (sets.length > 0) {
      console.log('[App] Question sets arrived via WebSocket, initializing...')
    }
    loadQuestionSets()
    return
  }

  // Sync current selection with the new object in store (it might have been updated via broadcast)
  const updated = sets.find(s => s.id === currentQuestionSet.value.id)
  if (updated && getQuestionSetSyncSignature(updated) !== getQuestionSetSyncSignature(currentQuestionSet.value)) {
    console.log('[App] Syncing currentQuestionSet with store update')
    currentQuestionSet.value = updated
    return
  }

  if (!updated) {
    console.log('[App] Current question set no longer exists, clearing selection')
    loadQuestionSets()
  }
}, { immediate: true })

watch(() => wsState.runningQuestionSetId, () => {
  // Reschedule the AFK timer whenever run state changes so it picks up the
  // correct effective timeout (3x during a run, 1x when idle).
  scheduleAfkTimer()
})

const isRunning = ref(false)
const isLoadingResults = ref(false)
const completedTasks = ref(0)
const totalTasks = ref(0)

const isDev = import.meta.env.DEV

// Workspace Creation State
const isCreatingWorkspace = ref(false)
const newWorkspaceName = ref('')
const newWsInput = ref(null)

// Workspace Cloning State
const isCloningWorkspace = ref(false)
const cloneSourceWorkspace = ref(null)
const cloneNewName = ref('')
const cloningLoading = ref(false)
const cloneWsInput = ref(null)


// Computed
// Smart label for Arena/History navigation button
const getArenaHistoryLabel = computed(() => {
  return '⚔️ Arena'
})

function countQuestionsFromQuestionSetData(data) {
  if (!data) return 0
  const categories = Array.isArray(data?.categories) ? data.categories : []
  return categories.reduce((total, category) => total + (Array.isArray(category?.questions) ? category.questions.length : 0), 0)
}

function applyMeResponse(me) {
  if (!me) return
  const existingUser = currentUser.value || {}
  const incomingUser = me.user || {}
  const mergedUser = { ...existingUser, ...incomingUser }

  currentUser.value = mergedUser
  api.setStoredUser(mergedUser)
}

function getPendingShareTokenFromUrl() {
  const params = new URLSearchParams(window.location.search)
  return String(params.get('share') || '').trim()
}

function clearPendingShareTokenFromUrl() {
  const url = new URL(window.location.href)
  url.searchParams.delete('share')
  const nextUrl = `${url.pathname}${url.search}${url.hash}`
  window.history.replaceState({}, '', nextUrl)
}

function maybeOpenPendingShareLink() {
  if (!pendingShareToken.value) {
    pendingShareToken.value = getPendingShareTokenFromUrl()
  }
  if (!pendingShareToken.value) return
  if (!isAuthenticated.value || !appReady.value) return
  if (!workspaces.value.length) return
  showShareAcceptModal.value = true
}

function dismissPendingShareLink() {
  showShareAcceptModal.value = false
  pendingShareToken.value = ''
  clearPendingShareTokenFromUrl()
}

function handleArenaHistoryClick() {
  viewMode.value = 'benchmarks'
  benchmarkMode.value = 'runner'
}

function openMyProfile() {
  if (!isAdmin.value || !currentUserId.value) return
  adminTab.value = 'users'
  profileEntityType.value = 'user'
  profileEntityId.value = currentUserId.value
  viewMode.value = 'admin-profile'
}

function openAdminPanel() {
  if (!isAdmin.value) return
  closeAdminProfile()
  viewMode.value = 'admin'
  capturePostHogEvent('admin_panel_opened', {
    admin_tab: adminTab.value,
    workspace_id: currentWorkspace.value?.id || ''
  })
}

function closeAdminProfile() {
  profileEntityType.value = null
  profileEntityId.value = null
  viewMode.value = isAdmin.value ? 'admin' : 'benchmarks'
}

function handleViewUserProfile(userId) {
  if (!isAdmin.value || !userId) return
  adminTab.value = 'users'
  profileEntityType.value = 'user'
  profileEntityId.value = userId
  viewMode.value = 'admin-profile'
}

function handleViewOrgProfile(orgId) {
  if (!isAdmin.value || !orgId) return
  adminTab.value = 'organizations'
  profileEntityType.value = 'organization'
  profileEntityId.value = orgId
  viewMode.value = 'admin-profile'
}

function handleProfileUpdated(updatedUser) {
  if (!updatedUser?.id || updatedUser.id !== currentUserId.value) return
  currentUser.value = { ...currentUser.value, ...updatedUser }
}

const enabledAgents = computed(() => agents.value.filter(a => a.enabled))

// When viewing historical run results, show only agents that have results
// When running a new benchmark, show all enabled agents
const displayAgents = computed(() => {
  let list = []
  // If we have runResults, show only agents that have data
  const resultAgentIds = Object.keys(runResults.value || {})
  if (resultAgentIds.length > 0 && !isRunning.value) {
    // Filter enabledAgents to only those in results, preserving order
    const agentsWithResults = enabledAgents.value.filter(a => resultAgentIds.includes(a.id))
    // Also include any agents from results that might not be in enabledAgents (old agents)
    const oldAgentIds = resultAgentIds.filter(id => !enabledAgents.value.some(a => a.id === id))
    // For old agents, try to find them in all agents or create placeholder
    const oldAgents = oldAgentIds.map(id => {
      const found = agents.value.find(a => a.id === id)
      return found || { id, name: 'Agent (historical)', provider_type: 'unknown' }
    })
    list = [...agentsWithResults, ...oldAgents]
  } else {
    list = [...enabledAgents.value]
  }

  // Sort: 1. Evaluators last, 2. Position
  const isEvaluator = (a) => a.provider_type === 'openai' || a.provider_type === 'evaluator'
  return list.sort((a, b) => {
    // Evaluators always at the bottom
    if (isEvaluator(a) && !isEvaluator(b)) return 1
    if (!isEvaluator(a) && isEvaluator(b)) return -1
    
    // Sort by position
    return (a.position || 0) - (b.position || 0)
  })
})


// Auth Methods
async function onLogin() {
  if (isLoggingIn.value) return
  isLoggingIn.value = true
  
  try {
    isAuthenticated.value = true
    currentUser.value = api.getStoredUser()
    
    // 0. Do NOT force disconnect blindly.
    // If we are connected anonymously but now have a user/workspace context, connect() handles the switch gracefully.

    // 1. Determine local workspace preference first
    currentWorkspace.value = api.getStoredWorkspace()

    // 2. Ensure we are connected with the correct scope (Authenticated)
    if (currentWorkspace.value) {
      await wsConnect(currentWorkspace.value.id)
    } else {
      // If no workspace selected yet, connect with just the token (User scope)
      // Pass null as workspace ID, but ensure we trigger a reconnect to send the token
      await wsConnect(null)
    }

    // 3. Load Workspaces (auto-selects user's workspace)
    await loadWorkspaces()
    
    if (currentWorkspace.value) {
      await loadQuestionSets()
    }
    
    // 4. Check status and fetch full profile (workspaces + orgs)
    try {
      const me = await wsService.getMe()
      applyMeResponse(me)
      
      const result = await wsService.checkManagerStatus()
      isManager.value = result?.is_manager || false
    } catch (e) {
      console.log('Could not fetch user profile:', e)
    }

    capturePostHogEvent('login_succeeded', {
      workspace_id: currentWorkspace.value?.id || '',
      is_admin: isAdmin.value
    })

    // Onboarding check removed - users don't need organizations

    } catch (err) {
      console.error('[App] Login initialization failed:', err)
    } finally {
      appReady.value = true
      maybeOpenPendingShareLink()
      scheduleAfkTimer()
      isLoggingIn.value = false
    }
}

function onOnboardingCompleted() {
  showOnboarding.value = false
  // Fully reload state to ensure everything is fresh
  window.location.reload()
}


async function handleLogout() {
  capturePostHogEvent('logout_clicked', {
    workspace_id: currentWorkspace.value?.id || '',
    is_admin: isAdmin.value,
    is_impersonating: isImpersonating.value
  })
  await api.logout()
  isAuthenticated.value = false
  currentUser.value = null
  currentWorkspace.value = null
  profileEntityType.value = null
  profileEntityId.value = null
  adminTab.value = 'users'
  afkOverlayVisible.value = false
  isReconnectingFromAfk.value = false
  reconnectOverlayMode.value = 'afk'
  clearReconnectOverlayTimer()
  clearAfkForegroundHeartbeat()
  clearAfkTimer()
  // runResults, currentRun, tasks, selectedQuestionId are now in BenchmarkArena and will be unmounted.
  // We just need to clear global state.
  isManager.value = false
  wsService.disconnect('logout')
  resetPostHogUser()
  // Redirect to login page
  viewMode.value = 'benchmarks'
}

// Methods


async function loadWorkspaces() {
  workspacesLoading.value = true
  workspacesError.value = ''
  try {
    // Ensure we are authenticated first
    if (!isAuthenticated.value) {
      workspacesError.value = 'Not authenticated'
      workspacesLoading.value = false
      return
    }

    // Ensure we have a connection before fetching
    if (!wsService.isConnected()) {
      const targetWsId = currentWorkspace.value ? currentWorkspace.value.id : null
      console.log(`[App] Connecting for workspaces (ID: ${targetWsId})...`)
      await wsService.connect(targetWsId)
    }

    workspaces.value = (await wsService.getWorkspaces()) || []

    if ((workspaces.value?.length || 0) === 0) {
      try {
        const fallbackName = 'main'
        console.warn('[App] No workspaces found; creating a default workspace:', fallbackName)
        const created = await wsService.createWorkspace(fallbackName)
        workspaces.value = created ? [created] : []
      } catch (err) {
        console.error('[App] Failed to auto-create workspace:', err)
      }
    }
    
    // Validate current workspace - check if we have one (logic continues...)
    const savedWs = localStorage.getItem('workspace')
    if (!currentWorkspace.value && savedWs) {
      try {
        currentWorkspace.value = JSON.parse(savedWs)
      } catch (e) {
        console.error('Failed to parse saved workspace', e)
      }
    }

    // Auto-select user's workspace (each user has their own workspace)
    if (currentWorkspace.value) {
      const exists = workspaces.value.find(w => w.id === currentWorkspace.value.id)
      if (!exists) {
        console.warn('Current workspace no longer exists, selecting first available.')
        // Select first workspace if current one doesn't exist
        if (workspaces.value.length > 0) {
          currentWorkspace.value = workspaces.value[0]
          await selectWorkspace(workspaces.value[0])
        } else {
          currentWorkspace.value = null
          localStorage.removeItem('workspace')
        }
      }
    } else if ((workspaces.value?.length || 0) > 0) {
      // Auto-select first workspace if none selected
      currentWorkspace.value = workspaces.value[0]
      await selectWorkspace(workspaces.value[0])
    }
  } catch (e) {
    const message = String(e?.message || e || '')
    workspacesError.value = message || 'Failed to load workspaces'
    if (message.toLowerCase().includes('not authenticated') || message.includes('401')) {
      handleLogout()
    }
  } finally {
    workspacesLoading.value = false
  }
}


async function selectWorkspace(ws) {
  try {
    const result = await wsService.switchWorkspace(ws.id)
    // Token is now handled via cookie (set by backend on switch)
    
    // Clear last question set selection to ensure "Select a Question Set" screen is shown in new workspace
    currentQuestionSet.value = null
    localStorage.removeItem('lastQuestionSetId')
    
    currentWorkspace.value = result.workspace || ws
    showWorkspaceModal.value = false
    await wsConnect(ws.id)
    // syncState is called automatically on 'connected' in wsStore
    await loadQuestionSets()
    capturePostHogEvent('workspace_switched', {
      workspace_id: currentWorkspace.value?.id || '',
      workspace_name: currentWorkspace.value?.name || ''
    })
  } catch (e) {
    console.error('Failed to switch workspace:', e)
  }
}

function handleWorkspaceSelectionChange(event) {
  const nextWorkspaceId = String(event?.target?.value || '')
  if (!nextWorkspaceId) return
  const targetWorkspace = workspaces.value.find((workspace) => workspace.id === nextWorkspaceId)
  if (targetWorkspace) {
    selectWorkspace(targetWorkspace)
  }
}

async function loadQuestionSets(preferredId = null) {
  if (!isAuthenticated.value || !currentWorkspace.value) return
  try {
    // Data is now managed by wsStore, but we need to handle selection logic
    const uniqueSets = Array.isArray(wsState.questionSets) ? wsState.questionSets : []
    const targetSet = resolveQuestionSetSelection({
      questionSets: uniqueSets,
      preferredId,
      lastQuestionSetId: localStorage.getItem('lastQuestionSetId'),
      currentQuestionSet: currentQuestionSet.value
    })
    
    if (uniqueSets.length > 0) {
        console.log('[App] loadQuestionSets: final targetSet:', targetSet?.name || 'none (defaulting to empty state)')
        
        const nextSelection = targetSet || null
        if (getQuestionSetSyncSignature(nextSelection) !== getQuestionSetSyncSignature(currentQuestionSet.value)) {
          currentQuestionSet.value = nextSelection
        }
    } else {
      if (targetSet && getQuestionSetSyncSignature(targetSet) === getQuestionSetSyncSignature(currentQuestionSet.value)) {
        console.log('[App] loadQuestionSets: preserving preferred selection while store sync catches up')
        return
      }
      if (currentQuestionSet.value !== null) {
        currentQuestionSet.value = null
      }
    }
  } catch (e) {
    console.error('Failed to load question sets:', e)
  }
}

async function handleQuestionSetShareAccepted(result) {
  const targetWorkspaceId = String(result?.workspace_id || '')
  const importedQuestionSet = result?.question_set || null

  dismissPendingShareLink()

  if (importedQuestionSet?.id) {
    capturePostHogEvent('question_set_share_imported', {
      question_set_id: importedQuestionSet.id,
      workspace_id: targetWorkspaceId,
      question_count: countQuestionsFromQuestionSetData(importedQuestionSet.data)
    })
  }

  if (targetWorkspaceId && currentWorkspace.value?.id === targetWorkspaceId && importedQuestionSet?.id) {
    currentQuestionSet.value = importedQuestionSet
    await loadQuestionSets(importedQuestionSet.id)
    return
  }

  const targetWorkspace = workspaces.value.find((workspace) => workspace.id === targetWorkspaceId)
  if (targetWorkspace && window.confirm(`Question set imported to "${targetWorkspace.name}". Switch to that workspace now?`)) {
    await selectWorkspace(targetWorkspace)
    if (importedQuestionSet?.id) {
      currentQuestionSet.value = importedQuestionSet
      await loadQuestionSets(importedQuestionSet.id)
    }
  }
}

function switchQuestionSet(id) {
  const set = questionSets.value.find(s => s.id === id)
  if (set) {
    currentQuestionSet.value = set
  }
}

function getFlatQuestions(questionSet) {
  if (!questionSet?.data) return []
  
  let data = questionSet.data
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
      const qId = q.id != null && q.id !== '' ? String(q.id) : `${catIdx + 1}-${qIdx + 1}`
      questions.push({ ...q, id: qId, category: cat.name, question: questionText })
    }
  }
  return questions
}

function goToHistory(qs) {
  // Set filter
  historyFilter.value = qs.name || ''
  // Switch view
  viewMode.value = 'benchmarks'
  benchmarkMode.value = 'history'
}



function onQuestionSetSaved(updated) {
  currentQuestionSet.value = updated
  previousQuestionSet.value = null // Clear since we saved successfully
  showQuestionEditor.value = false
  // Reload list to ensure dropdown is updated
  loadQuestionSets(updated.id)
}

function onQuestionEditorClose() {
  // If we were creating a new set (currentQuestionSet is null) and user closed without saving,
  // restore the previous question set
  if (currentQuestionSet.value === null && previousQuestionSet.value) {
    currentQuestionSet.value = previousQuestionSet.value
  }
  previousQuestionSet.value = null
  showQuestionEditor.value = false
}

function startCreatingWorkspace() {
  isCreatingWorkspace.value = true
  newWorkspaceName.value = ''
  // Focus input next tick
  setTimeout(() => newWsInput.value?.focus(), 100)
}



async function createNewWorkspace() {
  // Legacy method, replaced by inline
}

async function createWorkspaceInline() {
  if (!currentUser.value?.organization) {
    alert('Organization context lost. Please select an organization again.')
    return
  }
  const name = newWorkspaceName.value.trim()
  if (!name) return
  
  try {
    const newWs = await wsService.createWorkspace(name)
    workspaces.value.push(newWs)
    isCreatingWorkspace.value = false
    await selectWorkspace(newWs)
    capturePostHogEvent('workspace_created', {
      workspace_id: newWs?.id || '',
      workspace_name: newWs?.name || name
    })
  } catch (e) {
    console.error('Failed to create workspace:', e)
    alert('Failed to create workspace: ' + (e.message || 'Unknown error'))
  }
}

function startCloningWorkspace(ws) {
  if (!currentUser.value?.organization) {
    alert('Please select an organization before cloning a workspace.')
    return
  }
  isCreatingWorkspace.value = false
  isCloningWorkspace.value = true
  cloneSourceWorkspace.value = ws
  cloneNewName.value = `${ws.name} (Copy)`
  setTimeout(() => cloneWsInput.value?.focus(), 100)
}

function cancelCloning() {
  isCloningWorkspace.value = false
  cloneSourceWorkspace.value = null
  cloneNewName.value = ''
}

async function cloneWorkspaceInline() {
  if (!cloneSourceWorkspace.value || !cloneNewName.value.trim()) return
  
  cloningLoading.value = true
  try {
    const sourceWorkspaceId = cloneSourceWorkspace.value.id
    const clonedWs = await wsService.cloneWorkspace(
      sourceWorkspaceId, 
      cloneNewName.value.trim()
    )
    workspaces.value.push(clonedWs)
    cancelCloning()
    await selectWorkspace(clonedWs)
    capturePostHogEvent('workspace_cloned', {
      workspace_id: clonedWs?.id || '',
      source_workspace_id: sourceWorkspaceId,
      workspace_name: clonedWs?.name || ''
    })
  } catch (e) {
    console.error('Failed to clone workspace:', e)
    alert('Failed to clone workspace: ' + (e.message || 'Unknown error'))
  } finally {
    cloningLoading.value = false
  }
}

async function loadAgents() {
  // Agents are now managed by wsStore.
  // We might still need to fetch agents filtered by question set if applicable,
  // but for the WebSocket-First approach, we'll rely on the full agents list
  // and filter in computed as needed.
}





async function handleImported({ data, mode, target, title }) {
  try {
    // Use title from import, or generate one
    const setName = title || `Imported - ${generateQuestionSetName()}`
    const categoryCount = Array.isArray(data?.categories) ? data.categories.length : 0
    const questionCount = countQuestionsFromQuestionSetData(data)
    
    // If target is 'new' OR no current question set, create a new one
    if (target === 'new' || !currentQuestionSet.value) {
      const result = await wsService.createQuestionSet(currentWorkspace.value.id, {
        name: setName,
        version: '1.0',
        data: data
      })
      currentQuestionSet.value = result
      capturePostHogEvent('question_set_imported', {
        action: 'created_new',
        question_set_id: result?.id || '',
        workspace_id: currentWorkspace.value?.id || '',
        category_count: categoryCount,
        question_count: questionCount
      })
    } else {
      // Target is 'current' - update existing question set
      let finalData = data
      
      if (mode === 'append') {
        // Merge with existing data
        let existingData = currentQuestionSet.value.data
        if (typeof existingData === 'string') {
          try {
            existingData = JSON.parse(existingData)
          } catch (e) {
            existingData = { notes: '', categories: [] }
          }
        }
        
        const existingCategories = existingData?.categories || []
        const importedCategories = data.categories || []
        const existingNotes = typeof existingData?.notes === 'string' ? existingData.notes.trim() : ''
        const importedNotes = typeof data?.notes === 'string' ? data.notes.trim() : ''
        
        // Merge categories by name
        const mergedCategories = [...existingCategories]
        importedCategories.forEach(importCat => {
          const existingCat = mergedCategories.find(c => c.name === importCat.name)
          if (existingCat) {
            existingCat.questions = [...(existingCat.questions || []), ...(importCat.questions || [])]
          } else {
            mergedCategories.push(importCat)
          }
        })
        
        finalData = {
          ...existingData,
          ...data,
          notes: existingNotes || importedNotes,
          categories: mergedCategories
        }
      }
      
      const updated = await wsService.updateQuestionSet(currentQuestionSet.value.id, {
        name: currentQuestionSet.value.name,
        version: currentQuestionSet.value.version || '1.0',
        data: finalData
      })
      currentQuestionSet.value = updated
      capturePostHogEvent('question_set_imported', {
        action: mode === 'append' ? 'appended_current' : 'replaced_current',
        question_set_id: updated?.id || '',
        workspace_id: currentWorkspace.value?.id || '',
        category_count: categoryCount,
        question_count: questionCount
      })
    }
    
    showImportModal.value = false
    await loadQuestionSets(currentQuestionSet.value?.id)
  } catch (e) {
    console.error('Import failed:', e)
    alert('Failed to import: ' + e.message)
  }
}


async function exportQuestions() {
  if (!currentQuestionSet.value) return
  
  try {
    const rawData = await wsService.exportQuestionSet(currentQuestionSet.value.id)
    
    // Add title to the export
    const exportData = {
      title: currentQuestionSet.value.name,
      ...rawData
    }
    
    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${currentQuestionSet.value.name}.json`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    console.error('Export failed:', e)
  }
}

function handleTriggerPrint(data) {
  // If workspaceName is missing or looks like a UUID, fallback to currentWorkspace.name
  const isUUID = (str) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(str);
  
  let wsName = data.workspaceName;
  if (!wsName || isUUID(wsName)) {
    wsName = currentWorkspace.value?.name || 'Benchmark';
  }

  printData.value = {
    workspaceName: wsName,
    summary: data.summary || null,
    results: data.results || [],
    reportVariant: data.reportVariant || 'full',
    reportTitle: data.reportTitle || '',
    reportSubtitle: data.reportSubtitle || '',
    questionCards: data.questionCards || []
  }
  showPrintView.value = true
}

function triggerBrowserPrint() {
  window.print()
}



// Lifecycle
// Persistence Logic




// Lifecycle
const isImpersonating = computed(() => {
  const user = currentUser.value?.user || currentUser.value
  return !!user?.impersonator_id
})

function clearAfkTimer() {
  if (!afkTimer) return
  clearTimeout(afkTimer)
  afkTimer = null
  refreshAfkDebugState()
}

function afkDebugLog(eventName, details = {}) {
  if (!afkDebug.value.enabled) return
  console.debug('[AFK][debug]', eventName, details)
}

function clearAfkDebugTicker() {
  if (!afkDebugTicker) return
  clearInterval(afkDebugTicker)
  afkDebugTicker = null
}

function refreshAfkDebugState(extra = {}) {
  if (!afkDebug.value.enabled) return
  const remainingMs = canTrackAfk() && lastAfkResetAt
    ? Math.max(0, effectiveAfkTimeoutMs.value - (Date.now() - lastAfkResetAt))
    : null
  afkDebug.value = {
    ...afkDebug.value,
    nowMs: Date.now(),
    remainingMs,
    visibility: document.visibilityState,
    hasFocus: document.hasFocus(),
    canTrack: canTrackAfk(),
    wsConnected: wsState.isConnected,
    overlayVisible: afkOverlayVisible.value,
    overlayMode: reconnectOverlayMode.value,
    ...extra
  }
}

function setAfkDebugEnabled(enabled) {
  afkDebug.value = { ...afkDebug.value, enabled }
  if (!enabled) {
    clearAfkDebugTicker()
    try {
      delete window.__AFK_DEBUG__
    } catch (_) {}
    return
  }

  refreshAfkDebugState()
  if (!afkDebugTicker) {
    afkDebugTicker = setInterval(() => refreshAfkDebugState(), afkDebugTickMs)
  }

  window.__AFK_DEBUG__ = {
    status: () => ({ ...afkDebug.value }),
    ping: () => markUserActivity(true, 'debug-api'),
    forceAfk: () => activateAfkMode(),
    enable: () => {
      localStorage.setItem(afkDebugStorageKey, '1')
      setAfkDebugEnabled(true)
    },
    disable: () => {
      localStorage.removeItem(afkDebugStorageKey)
      setAfkDebugEnabled(false)
    }
  }

  afkDebugLog('enabled', {
    timeoutMs: effectiveAfkTimeoutMs.value,
    configuredTimeout: config.AFK_TIMEOUT_MS || null
  })
}

function setupAfkDebugMode() {
  const params = new URLSearchParams(window.location.search)
  const queryFlag = params.get('afkDebug')
  if (queryFlag === '1') localStorage.setItem(afkDebugStorageKey, '1')
  if (queryFlag === '0') localStorage.removeItem(afkDebugStorageKey)
  const enabled = localStorage.getItem(afkDebugStorageKey) === '1'
  setAfkDebugEnabled(enabled)
}

function teardownAfkDebugMode() {
  clearAfkDebugTicker()
  try {
    delete window.__AFK_DEBUG__
  } catch (_) {}
}

function clearReconnectOverlayTimer() {
  if (!reconnectOverlayTimer) return
  clearTimeout(reconnectOverlayTimer)
  reconnectOverlayTimer = null
  refreshAfkDebugState()
}

function clearAfkForegroundHeartbeat() {
  if (!afkForegroundHeartbeat) return
  clearInterval(afkForegroundHeartbeat)
  afkForegroundHeartbeat = null
  refreshAfkDebugState()
}

function startAfkForegroundHeartbeat() {
  if (afkForegroundHeartbeat) return
  afkForegroundHeartbeat = setInterval(() => {
    if (!canTrackAfk()) return
    if (document.visibilityState !== 'visible') return
    if (!document.hasFocus()) return
    afkDebug.value = {
      ...afkDebug.value,
      heartbeatCount: afkDebug.value.heartbeatCount + 1
    }
    markUserActivity(true, 'heartbeat')
  }, afkForegroundHeartbeatMs)
  refreshAfkDebugState()
}

function showReconnectOverlay(mode = 'afk') {
  reconnectOverlayMode.value = mode
  afkOverlayVisible.value = true
  isReconnectingFromAfk.value = false
  afkDebugLog('overlay-visible', { mode })
  refreshAfkDebugState()
}

function markUserActivity(force = false, source = 'activity') {
  if (afkOverlayVisible.value) return
  const now = Date.now()
  if (!force && now - lastAfkResetAt < 500) {
    afkDebug.value = {
      ...afkDebug.value,
      ignoredCount: afkDebug.value.ignoredCount + 1
    }
    afkDebugLog('activity-ignored', { source, deltaMs: now - lastAfkResetAt })
    return
  }
  lastAfkResetAt = now
  afkDebug.value = {
    ...afkDebug.value,
    activityCount: afkDebug.value.activityCount + 1,
    lastActivityAt: now,
    lastActivitySource: source
  }
  afkDebugLog('activity', { source, force })
  if (!canTrackAfk()) {
    clearAfkTimer()
    refreshAfkDebugState()
    return
  }
  scheduleAfkTimer()
  refreshAfkDebugState()
}

function canTrackAfk() {
  return isAuthenticated.value && appReady.value && wsState.isConnected && !wsState.isMaintenance
}

async function activateAfkMode() {
  if (!canTrackAfk() || afkOverlayVisible.value) return
  const timeoutMs = effectiveAfkTimeoutMs.value
  console.log('[AFK] Idle timeout reached; pausing session', {
    timeoutMs,
    workspaceId: currentWorkspace.value?.id || null
  })
  afkDebugLog('activate-afk', {
    timeoutMs,
    workspaceId: currentWorkspace.value?.id || null
  })
  afkDebug.value = {
    ...afkDebug.value,
    lastDisconnectReason: 'afk-timeout',
    lastDisconnectAt: Date.now()
  }
  clearReconnectOverlayTimer()
  showReconnectOverlay('afk')
  wsDisconnect('afk-timeout')
  refreshAfkDebugState()
}

function scheduleAfkTimer() {
  clearAfkTimer()
  if (!canTrackAfk() || afkOverlayVisible.value) return
  if (!lastAfkResetAt) {
    lastAfkResetAt = Date.now()
  }
  const timeoutMs = effectiveAfkTimeoutMs.value
  const elapsed = Date.now() - lastAfkResetAt
  const remaining = timeoutMs - elapsed
  if (remaining <= 0) {
    afkDebugLog('schedule-expired', { elapsed, remaining, timeoutMs })
    activateAfkMode()
    return
  }
  afkTimer = setTimeout(() => activateAfkMode(), remaining)
  afkDebugLog('schedule', { remainingMs: remaining, elapsedMs: elapsed, timeoutMs })
  refreshAfkDebugState()
}

function handleUserActivity(event) {
  markUserActivity(false, event?.type || 'event')
}

function handleVisibilityChange() {
  if (document.visibilityState !== 'visible') {
    clearAfkTimer()
    afkDebugLog('hidden', {})
    refreshAfkDebugState()
    return
  }
  afkDebugLog('visible', {})
  markUserActivity(true, 'visibilitychange')
}

function bindAfkActivityListeners() {
  afkActivityEvents.forEach((eventName) => {
    window.addEventListener(eventName, handleUserActivity, afkListenerOptions)
    document.addEventListener(eventName, handleUserActivity, afkListenerOptions)
  })
  window.addEventListener('focus', handleUserActivity)
  document.addEventListener('selectionchange', handleUserActivity)
}

function unbindAfkActivityListeners() {
  afkActivityEvents.forEach((eventName) => {
    window.removeEventListener(eventName, handleUserActivity, afkListenerOptions)
    document.removeEventListener(eventName, handleUserActivity, afkListenerOptions)
  })
  window.removeEventListener('focus', handleUserActivity)
  document.removeEventListener('selectionchange', handleUserActivity)
}

function forceAfkDebug() {
  activateAfkMode()
}

async function reconnectFromAfk() {
  if (isReconnectingFromAfk.value) return
  clearReconnectOverlayTimer()
  afkDebugLog('manual-reconnect-click', { overlayMode: reconnectOverlayMode.value })

  if (wsState.isConnected) {
    afkOverlayVisible.value = false
    reconnectOverlayMode.value = 'afk'
    lastAfkResetAt = Date.now()
    scheduleAfkTimer()
    refreshAfkDebugState()
    return
  }

  console.log('[AFK] Reconnect requested by user', { mode: reconnectOverlayMode.value })
  isReconnectingFromAfk.value = true
  try {
    await wsConnect(currentWorkspace.value?.id || null)
    await syncState()
    afkOverlayVisible.value = false
    reconnectOverlayMode.value = 'afk'
    lastAfkResetAt = Date.now()
    scheduleAfkTimer()
    console.log('[AFK] Reconnected and sync completed')
    afkDebugLog('manual-reconnect-success', {})
  } catch (e) {
    console.error('[App] Reconnect failed:', e)
    afkDebugLog('manual-reconnect-failed', { error: e?.message || String(e) })
  } finally {
    isReconnectingFromAfk.value = false
    refreshAfkDebugState()
  }
}

onMounted(async () => {
  setupAfkDebugMode()
  bindAfkActivityListeners()
  startAfkForegroundHeartbeat()
  document.addEventListener('visibilitychange', handleVisibilityChange)
  // If we think we are authenticated, verify with the backend
  if (isAuthenticated.value) {
    try {
      // Establish WebSocket connection first (uses token/cookie automatically)
      const savedWsString = localStorage.getItem('workspace')
      let initialWsId = null
      if (savedWsString) {
        try {
          initialWsId = JSON.parse(savedWsString).id
        } catch (e) {}
      }
      
      await wsConnect(initialWsId)

      // Fetch current user via WebSocket
      const me = await wsService.getMe()
      applyMeResponse(me)
      isAuthenticated.value = true
      
      await loadWorkspaces()
      
      if (currentWorkspace.value) {
        await loadQuestionSets()
        // Check for active run to restore
      }
      // Workspace is auto-selected for each user, no need to show modal
      
      // Check if user is a manager (via WebSocket)
      try {
        const result = await wsService.checkManagerStatus()
        console.log('[App] Manager status check:', result)
        isManager.value = result?.is_manager || false
      } catch (e) {
        console.log('Could not check manager status:', e)
      }
      } catch (e) {
        console.warn('[App] Session verification failed:', e)
        handleLogout()
      } finally {
        appReady.value = true
        maybeOpenPendingShareLink()
        lastAfkResetAt = Date.now()
        scheduleAfkTimer()
      }
    } else {
       // Not authenticated, but we should be ready to show login
       appReady.value = true
       lastAfkResetAt = Date.now()
       clearAfkTimer()
    }
  
  // Custom handlers for UI-specific reactivity in App.vue

  
  wsService.on('EVT_ERROR', (payload) => {
    const err = (typeof payload === 'string' ? payload : payload?.error || '').toLowerCase()
    if (err.includes('not authenticated') || err.includes('user not found') || err.includes('invalid token')) {
      console.warn('[App] Authentication error received via WS:', err)
      handleLogout()
    }
  })

  wsService.on('session_expired', (payload) => {
    if (!isAuthenticated.value) return
    console.warn('[App] Session expired while recovering WS connection; logging out', payload)
    handleLogout()
  })

  wsService.on('connected', () => {
    clearReconnectOverlayTimer()
    afkDebugLog('ws-connected', {})
    if (afkOverlayVisible.value && reconnectOverlayMode.value === 'connection') {
      console.log('[App] WS reconnected automatically; closing reconnect overlay')
      afkOverlayVisible.value = false
      reconnectOverlayMode.value = 'afk'
    }
    // WS reconnect is not user activity. Keep the existing AFK clock so that
    // transient disconnects don't "refresh" the remaining time.
    if (!afkOverlayVisible.value) scheduleAfkTimer()
    refreshAfkDebugState()
  })

  wsService.on('disconnected', (payload) => {
    if (!isAuthenticated.value || !appReady.value) return
    if (afkOverlayVisible.value) return
    if (wsState.isMaintenance) return
    const reason = payload?.disconnectReason || 'unknown'
    afkDebug.value = {
      ...afkDebug.value,
      lastDisconnectReason: reason,
      lastDisconnectAt: Date.now()
    }
    afkDebugLog('ws-disconnected', {
      reason,
      reconnectPlanned: payload?.reconnectPlanned !== false,
      code: payload?.code,
      wsReason: payload?.reason || ''
    })
    if (['afk-timeout', 'logout', 'app-unmount'].includes(reason)) return
    clearAfkTimer()
    const reconnectPlanned = payload?.reconnectPlanned !== false
    if (reconnectPlanned) {
      clearReconnectOverlayTimer()
      reconnectOverlayTimer = setTimeout(() => {
        if (!isAuthenticated.value || !appReady.value) return
        if (wsState.isMaintenance || wsState.isConnected || afkOverlayVisible.value) return
        console.warn('[App] WS still disconnected after grace period; showing reconnect overlay', payload)
        showReconnectOverlay('connection')
      }, reconnectOverlayGraceMs)
      return
    }
    console.warn('[App] WS disconnected; showing reconnect overlay', payload)
    showReconnectOverlay('connection')
    refreshAfkDebugState()
  })

  wsService.on('EVT_FORCE_LOGOUT', (payload) => {
    console.warn('[App] Force Logout received:', payload)
    handleLogout()
  })

  // Silent token refresh every 15 minutes (if authenticated)
  refreshInterval.value = setInterval(async () => {
    if (isAuthenticated.value) {
      await api.refreshToken()
    }
  }, 15 * 60 * 1000) // 15 minutes
})

onUnmounted(() => {
  unbindAfkActivityListeners()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  clearAfkTimer()
  clearAfkForegroundHeartbeat()
  clearReconnectOverlayTimer()
  teardownAfkDebugMode()
  wsDisconnect('app-unmount')
  if (refreshInterval.value) {
    clearInterval(refreshInterval.value)
  }
  downloadManager.cancelAll()
})
</script>

 
/* Compact, modern logout button */
.btn-logout-compact {
  display: inline-flex;
  align-items: center;
  gap: 6px;

  padding: 6px 10px;
  border-radius: 10px;

  font-size: 12.5px;
  font-weight: 500;
  line-height: 1;

  /* soften the danger look */
  background: rgba(239, 68, 68, 0.10);
  border: 1px solid rgba(239, 68, 68, 0.25);
  color: rgb(185, 28, 28);

  box-shadow: none;

  transition:
    background 140ms ease,
    border-color 140ms ease,
    box-shadow 140ms ease,
    transform 100ms ease;
}

.btn-logout-compact:hover {
  background: rgba(239, 68, 68, 0.16);
  border-color: rgba(239, 68, 68, 0.40);
  box-shadow: 0 4px 10px rgba(239, 68, 68, 0.15);
  transform: translateY(-1px);
}

.btn-logout-compact:active {
  transform: translateY(0);
  box-shadow: 0 2px 6px rgba(239, 68, 68, 0.12);
}

.btn-logout-compact:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.25);
}
.btn-logout-compact.icon-only {
  padding: 6px 8px;
}

.btn-logout-compact.icon-only .logout-icon {
  font-size: 14px;
}

/* Tooltip container */
.btn-logout-compact.has-tooltip {
  position: relative;
}

/* Hidden label */
.btn-logout-compact .tooltip-text {
  position: absolute;
  top: 50%;
  right: calc(100% + 8px);
  transform: translateY(-50%);

  padding: 6px 10px;
  border-radius: 8px;

  background: #111827;
  color: #fff;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;

  opacity: 0;
  pointer-events: none;
  transform-origin: right center;
  transition: opacity 120ms ease, transform 120ms ease;
}

/* Show on hover / focus */
.btn-logout-compact:hover .tooltip-text,
.btn-logout-compact:focus-visible .tooltip-text {
  opacity: 1;
  transform: translateY(-50%) translateX(-2px);
}
