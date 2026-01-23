<template>
  <div class="app-root" :class="{ 'has-banner': isImpersonating, 'zen-mode-active': isZenMode }">
    <!-- Impersonation Banner -->
    <div v-if="isImpersonating" class="impersonation-banner no-print">
      <div class="banner-content">
        <span class="banner-icon">🕵️</span>
        <span class="banner-text">
          You are currently logged in as <strong>{{ currentUser?.name || currentUser?.user?.name }}</strong>
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
          />
        </div>
      </div>
    </Teleport>

    <!-- Login Screen -->
    <LoginScreen v-if="!isAuthenticated" @login="onLogin" class="no-print" />

    <!-- Main App (when authenticated) -->
    <template v-else>
      <!-- Workspace Selector Modal -->
      <div v-if="showWorkspaceModal" 
           class="actions-modal-overlay workspace-overlay" 
           @click.self="currentUser?.organization ? showWorkspaceModal = false : null">
        <div class="actions-modal workspace-modal">
          <div class="actions-modal-header">
            <h3>🏢 Select Workspace</h3>
            <button v-if="currentUser?.organization" class="btn-close" @click="showWorkspaceModal = false">×</button>
          </div>
          <p class="workspace-modal-subtitle">Choose a workspace to benchmark agents</p>
          
          <div v-if="workspacesLoading" class="workspace-state">
            <span class="loading-spinner"></span> Loading workspaces...
          </div>
          <div v-else-if="workspacesError" class="workspace-error">{{ workspacesError }}</div>
          <div v-else class="workspace-grid">
            <div 
              v-for="ws in (workspaces || [])" 
              :key="ws.id"
              class="workspace-card"
              :class="{ active: currentWorkspace?.id === ws.id }"
              @click="selectWorkspace(ws)"
            >
              <h4 class="workspace-name">{{ ws.name }}</h4>
              <div class="workspace-meta">
                <span>{{ ws.agent_count || 0 }} agents</span>
              </div>
              <button 
                class="btn-clone-ws" 
                @click.stop="startCloningWorkspace(ws)"
                title="Clone this workspace"
              >📋 Clone</button>
            </div>
            <div v-if="!isCreatingWorkspace && !isCloningWorkspace" 
                 class="workspace-card new-workspace" 
                 :class="{ disabled: !currentUser?.organization }"
                 @click="startCreatingWorkspace"
            >
              <h4 class="workspace-name">+ New Workspace</h4>
              <p v-if="!currentUser?.organization" class="workspace-meta">Select an organization first</p>
            </div>
            <div v-else-if="isCreatingWorkspace" class="workspace-card create-workspace-form">
              <input 
                v-model="newWorkspaceName" 
                ref="newWsInput"
                placeholder="Workspace Name" 
                class="ws-name-input"
                @keyup.enter="createWorkspaceInline"
                @keyup.esc="isCreatingWorkspace = false"
              />
              <div class="ws-create-actions">
                <button class="btn-create-ws" @click="createWorkspaceInline" :disabled="!newWorkspaceName.trim()">Create</button>
                <button class="btn-cancel-ws" @click="isCreatingWorkspace = false">Cancel</button>
              </div>
            </div>
            <div v-else-if="isCloningWorkspace" class="workspace-card clone-workspace-form">
              <p class="clone-source-label">📋 Clone from: <strong>{{ cloneSourceWorkspace?.name }}</strong></p>
              <input 
                v-model="cloneNewName" 
                ref="cloneWsInput"
                placeholder="New Workspace Name" 
                class="ws-name-input"
                @keyup.enter="cloneWorkspaceInline"
                @keyup.esc="cancelCloning"
              />
              <div class="ws-create-actions">
                <button class="btn-create-ws" @click="cloneWorkspaceInline" :disabled="!cloneNewName.trim() || cloningLoading">
                  {{ cloningLoading ? '⏳ Cloning...' : '📋 Clone' }}
                </button>
                <button class="btn-cancel-ws" @click="cancelCloning">Cancel</button>
              </div>
            </div>
          </div>

          <!-- Organization Selection -->
          <div class="org-selection-section">
            <div class="actions-modal-header border-top">
              <h3>🏢 Organizations</h3>
            </div>
            
            <div v-if="(userOrganizations?.length || 0) > 1" class="org-list-wrapper">
              <p class="workspace-modal-subtitle">Switch your active organization context</p>
              <div class="org-grid">
                <div 
                  v-for="org in (userOrganizations || [])" 
                  :key="org.id"
                  class="org-item-card"
                  :class="{ active: currentUser?.organization?.id === org.id, 'is-active': currentUser?.organization?.id === org.id }"
                  @click="switchToOrg(org)"
                >
                  <div class="org-item-icon">🏢</div>
                  <div class="org-item-info">
                    <span class="org-item-name">{{ org.name }}</span>
                    <span class="org-item-role">{{ org.role }}</span>
                  </div>
                </div>
              </div>
            </div>

            <div class="join-org-container">
              <div v-if="!isJoiningOrg" class="join-org-trigger" @click="isJoiningOrg = true">
                <span class="join-icon">➕</span> Join new organization with invite code
              </div>
              <div v-else class="join-org-form">
                <input 
                  v-model="joinOrgInviteCode" 
                  placeholder="Enter Invite Code" 
                  class="join-org-input"
                  @keyup.enter="handleJoinOrg"
                />
                <div class="join-org-actions">
                  <button class="btn-join-org" @click="handleJoinOrg" :disabled="!joinOrgInviteCode.trim() || joinLoading">
                    {{ joinLoading ? 'Joining...' : 'Join' }}
                  </button>
                  <button class="btn-cancel-join" @click="isJoiningOrg = false">Cancel</button>
                </div>
              </div>
            </div>
          </div>
          
          <!-- Emergency Logout in Modal (if stuck) -->
          <div v-if="!currentUser?.organization" class="modal-footer-logout">
             <button class="btn-link logout-link" @click="handleLogout">🚪 Logout of account</button>
          </div>
        </div>
      </div>

      <!-- Main App Container -->
      <div class="app-container">
        <!-- Header -->
        <header class="header">
          <div class="header-left">
        <CorvicLogo width="100px" height="28px" class="header-logo" @click="viewMode = 'benchmarks'" />
        <h1 @click="viewMode = 'benchmarks'">Benchmarking</h1>
        <div v-if="currentWorkspace" class="workspace-chip" @click="showWorkspaceModal = true">
          <span class="org-label">{{ currentOrgName }} / </span>
          <span>{{ currentWorkspace.name }}</span>
          <span class="chip-arrow">▾</span>
        </div>
        <div v-if="isDev" class="dev-badge">🚧 DEV MODE</div>
      </div>
          <div class="controls">
            <div class="buttons">
              <span class="user-badge" :class="{ admin: currentUser?.is_admin }" @click="openMyProfile">
                {{ currentUser?.is_admin ? '👑' : '👤' }} {{ currentUser?.name }}
              </span>
              <button class="btn btn-secondary workspace-trigger" @click="showWorkspaceModal = true">
                🏢 {{ currentWorkspace?.name || 'Select Workspace' }}
              </button>
              <button class="btn btn-primary" @click="showConfig = !showConfig">
                🤖 {{ showConfig ? 'Hide Agents' : 'Manage Agents' }}
              </button>
              <button class="btn btn-primary" @click="showActionsModal = true" :disabled="!currentWorkspace">
                ⚙️ Actions
              </button>
              <button class="btn btn-danger" @click="handleLogout">
                🚪 Logout
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
            v-if="isManager" 
            class="nav-btn" 
            :class="{ active: viewMode === 'manager' }"
            @click="viewMode = 'manager'"
          >👔 Manager</button>
          <button 
            v-if="currentUser?.is_admin" 
            class="nav-btn" 
            :class="{ active: viewMode === 'admin' }"
            @click="viewMode = 'admin'"
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
        :isAdmin="currentUser?.is_admin"
      />
    </main>

    <!-- Admin Panel (for admins only) -->
    <main v-if="viewMode === 'admin' && currentUser?.is_admin" class="main-content no-padding">
      <AdminPanel 
        :current-user-id="currentUser?.id"
        :initial-tab="adminTab"
        @close="viewMode = 'benchmarks'"
        @view-user-profile="handleViewUserProfile"
        @view-org-profile="handleViewOrgProfile"
      />
    </main>

    <!-- Admin Profile View -->
    <main v-if="viewMode === 'admin-profile'" class="main-content">
      <AdminProfileView 
        :entity-type="profileEntityType"
        :entity-id="profileEntityId"
        @back="currentUser?.is_admin ? viewMode = 'admin' : viewMode = 'benchmarks'"
        @view-user="handleViewUserProfile"
        @view-org="handleViewOrgProfile"
        @updated="handleProfileUpdated"
      />
    </main>

    <!-- Manager Panel (for org managers) -->
    <main v-if="viewMode === 'manager' && isManager" class="main-content">
      <ManagerPanel 
        :org-name="currentOrgName"
        :current-user-id="currentUser?.id"
        :workspace-id="currentWorkspace?.id"
      />
    </main>
    
    <!-- Docs View -->
    <main v-if="viewMode === 'docs'" class="main-content">
      <DocsView />
    </main>

        <!-- Main Benchmarking Content -->
        <!-- History View -->
        <main v-if="viewMode === 'benchmarks' && benchmarkMode === 'history'" class="main-content no-padding">
          <BenchmarkDocumentView 
             :workspace-id="currentWorkspace?.id"
             :pre-filter="historyFilter"
             @back="benchmarkMode = 'runner'"
             @trigger-print="handleTriggerPrint"
          />
        </main>

        <!-- Arena View -->
        <main v-show="viewMode === 'benchmarks' && benchmarkMode === 'runner'" class="main-content no-padding">
          <KeepAlive>
            <BenchmarkArena 
                v-if="currentWorkspace"
                :key="currentWorkspace.id"
                :workspace-id="currentWorkspace.id"
                :agents="agents || []"
                :question-sets="questionSets || []"
                :initial-question-set-id="currentQuestionSet?.id"
                @update:currentQuestionSet="val => currentQuestionSet = val"
                @configure-agents="showConfig = true"
                @view-history="goToHistory"
                @trigger-print="handleTriggerPrint"
                :is-zen-mode="isZenMode"
                @toggle-zen="val => isZenMode = val"
            />
          </KeepAlive>
          <div v-if="!currentWorkspace" class="benchmarks-empty-state">
             <div class="empty-icon">🏢</div>
             <h3>Select a Workspace</h3>
             <p>You need to select a workspace to start benchmarking.</p>
             <button class="btn btn-primary" @click="showWorkspaceModal = true">Select Workspace</button>
          </div>
        </main>

      </div>

      <!-- Actions Modal -->
      <div v-if="showActionsModal" class="actions-modal-overlay" @click.self="showActionsModal = false">
        <div class="actions-modal">
          <div class="actions-modal-header">
            <h3>⚙️ Actions</h3>
            <button class="btn-close" @click="showActionsModal = false">×</button>
          </div>
          <div class="actions-grid">
            <button class="btn btn-primary" @click="showQuestionEditor = true; showActionsModal = false" :disabled="!currentQuestionSet">
              ✏️ Edit Questions
            </button>
            <button class="btn btn-primary" @click="showConfig = !showConfig; showActionsModal = false">
              🤖 {{ showConfig ? 'Hide' : 'Configure' }} Agents
            </button>
            <!-- Summary toggle removed as it lives in Arena now, or we can message it via refs if needed, but easier to just let Arena handle it internally -->
            <button class="btn btn-export" @click="showImportModal = true; showActionsModal = false">
              📥 Import Questions
            </button>
            <button class="btn btn-export" @click="exportQuestions" :disabled="!currentQuestionSet">
              📤 Export Questions
            </button>
            <!-- Run Evaluators also likely belongs in Arena, but keeping global actions maybe? NO, it acts on current active run. -->
            <hr style="border: 0; border-top: 1px solid #e2e8f0; margin: 0.5rem 0; width: 100%;">
            <button class="btn btn-secondary" @click="openMyProfile(); showActionsModal = false">
              👤 View my profile
            </button>
            <button class="btn btn-secondary" @click="showWorkspaceModal = true; showActionsModal = false">
              🏢 Change Workspace
            </button>
          </div>
        </div>
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
    </template>
    <MaintenanceOverlay :active="wsState.isMaintenance" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import LoginScreen from './components/LoginScreen.vue'
import CorvicLogo from './components/CorvicLogo.vue'
import BenchmarkArena from './components/BenchmarkArena.vue'
import BenchmarkDocumentView from './components/BenchmarkDocumentView.vue'
import AdminPanel from './components/AdminPanel.vue';
import AdminProfileView from './components/AdminProfileView.vue';
import ManagerPanel from './components/ManagerPanel.vue';
import StatsView from './components/StatsView.vue';
import QuestionEditorModal from './components/QuestionEditorModal.vue';
import ImportQuestionsModal from './components/ImportQuestionsModal.vue';
import MaintenanceOverlay from './components/MaintenanceOverlay.vue'
import AgentManagerModal from './components/AgentManagerModal.vue'
import DocsView from './components/DocsView.vue'
import PrintReport from './components/PrintReport.vue'
import * as api from './services/api.js'
import wsService from './services/websocket.js'
import { useWSStore } from './stores/wsStore'
import { downloadManager } from './services/DownloadManager.js'
import { contentCache } from './services/ContentCache.js'
import { generateQuestionSetName } from './utils/nameGenerator.js'
import './App.css'

const { state: wsState, syncState, connect: wsConnect, disconnect: wsDisconnect } = useWSStore()

// Auth State
const isAuthenticated = ref(api.isLoggedIn())
const currentUser = ref(api.getStoredUser())
const currentOrgName = computed(() => {
  return currentUser.value?.organization?.name || currentUser.value?.organization_name || 'My Organization'
})
const viewMode = ref('benchmarks'); // 'benchmarks', 'admin', 'stats', 'admin-profile', 'manager'
const benchmarkMode = ref(localStorage.getItem('benchmarkMode') || 'runner') // 'history', 'runner'
const isLoggingIn = ref(false) // Flag to prevent concurrent initialization during login

// Manager state
const isManager = ref(false)

// Admin Profile View State
const profileEntityType = ref(null) // 'user' or 'organization'
const profileEntityId = ref(null)

// Watch benchmarkMode to persist
// Watch benchmarkMode to persist
watch(benchmarkMode, (newVal) => {
  localStorage.setItem('benchmarkMode', newVal)
})

// State
const showWorkspaceModal = ref(false)
const showActionsModal = ref(false)
const showImportModal = ref(false)
const showConfig = ref(false)
const showSummary = ref(false)
const showQuestionEditor = ref(false)
const isZenMode = ref(false)
const previousQuestionSet = ref(null) // Used to restore when canceling new set creation
const showRunSetup = ref(false)

const workspaces = ref([])
const workspacesLoading = ref(false)
const workspacesError = ref('')
const currentWorkspace = ref(api.getStoredWorkspace())
const refreshInterval = ref(null)

const agents = computed(() => wsState.agents)
const questionSets = computed(() => wsState.questionSets)
const currentQuestionSet = ref(null)
const userOrganizations = ref([])
const isJoiningOrg = ref(false)
const joinOrgInviteCode = ref('')
const joinLoading = ref(false)
const loadingResults = ref(false)


const historyFilter = ref('')

// Print state
const showPrintView = ref(false)
const printData = ref({ workspaceName: '', summary: {}, results: [] })

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



watch(() => wsState.questionSets, (newSets) => {
  if (!newSets || newSets.length === 0) return

  if (!currentQuestionSet.value) {
    console.log('[App] Question sets arrived via WebSocket, initializing...')
    loadQuestionSets()
  } else {
    // Sync current selection with the new object in store (it might have been updated via broadcast)
    const updated = newSets.find(s => s.id === currentQuestionSet.value.id)
    if (updated && updated !== currentQuestionSet.value) {
      console.log('[App] Syncing currentQuestionSet with store update')
      currentQuestionSet.value = updated
    }
  }
}, { immediate: true, deep: true })

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

function applyMeResponse(me) {
  if (!me) return
  const existingUser = currentUser.value || {}
  const incomingUser = me.user || {}
  const mergedUser = { ...existingUser, ...incomingUser }

  const orgFromMe = me.organization
  if (orgFromMe?.name && (!mergedUser.organization || mergedUser.organization?.id !== orgFromMe.id)) {
    mergedUser.organization = orgFromMe
  }

  const orgList = Array.isArray(incomingUser.organizations) ? incomingUser.organizations : []
  if (!mergedUser.organization && orgList.length === 1) {
    mergedUser.organization = { id: orgList[0].id, name: orgList[0].name }
  }

  if (mergedUser.organization?.name) {
    mergedUser.organization_name = mergedUser.organization.name
  } else if (!mergedUser.organization_name && existingUser.organization_name) {
    mergedUser.organization_name = existingUser.organization_name
  }

  currentUser.value = mergedUser

  if (Array.isArray(incomingUser.organizations)) {
    userOrganizations.value = incomingUser.organizations
  }
}

function handleArenaHistoryClick() {
  viewMode.value = 'benchmarks'
  benchmarkMode.value = 'runner'
}

function openMyProfile() {
  console.log('[App] Opening my profile...', currentUser.value)
  if (!currentUser.value) return
  profileEntityType.value = 'user'
  profileEntityId.value = currentUser.value.id || currentUser.value.ID
  viewMode.value = 'admin-profile'
  console.log('[App] State set:', { profileEntityType: profileEntityType.value, profileEntityId: profileEntityId.value, viewMode: viewMode.value })
}

function handleProfileUpdated(updatedUser) {
  if (currentUser.value && currentUser.value.id === updatedUser.id) {
    currentUser.value = { ...currentUser.value, ...updatedUser }
  }
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
    
    // 0. Force disconnect any anonymous connection to ensure we reconnect with new auth cookie
    wsService.disconnect()

    // 1. Load Workspaces first to get valid context
    await loadWorkspaces()
    
    // 2. Determine and set current workspace
    currentWorkspace.value = api.getStoredWorkspace()
    
    if (!currentWorkspace.value) {
      showWorkspaceModal.value = true
    } else {
      // 3. Connect and sync via WebSocket
      await wsConnect(currentWorkspace.value.id)
      // loadQuestionSets is still needed for initial selection logic, but data comes from WS
      await loadQuestionSets()
      
      // Check for active run
      // usage removed
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
  } catch (err) {
    console.error('[App] Login initialization failed:', err)
  } finally {
    isLoggingIn.value = false
  }
}

// Admin Profile Navigation
const adminTab = ref('users') // Remember which tab was active

function handleViewUserProfile(userId) {
  adminTab.value = 'users'
  profileEntityType.value = 'user'
  profileEntityId.value = userId
  viewMode.value = 'admin-profile'
}

function handleViewOrgProfile(orgId) {
  adminTab.value = 'organizations'
  profileEntityType.value = 'organization'
  profileEntityId.value = orgId
  viewMode.value = 'admin-profile'
}

async function handleLogout() {
  await api.logout()
  isAuthenticated.value = false
  currentUser.value = null
  currentWorkspace.value = null
  // runResults, currentRun, tasks, selectedQuestionId are now in BenchmarkArena and will be unmounted.
  // We just need to clear global state.
  isManager.value = false
  wsService.disconnect()
  // Also force reload to ensure no in-memory state lingers
  window.location.reload()
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

    // Use WebSocket only for workspaces fetching
    // If we already have a workspace selected, skip this anonymous connect 
    // because we will connect with the workspace ID immediately after.
    if (!wsService.isConnected() && !currentWorkspace.value) {
      console.log('[App] Connecting anonymously for workspace discovery...')
      await wsService.connect(null)
    }
    workspaces.value = (await wsService.getWorkspaces()) || []
    
    // Validate current workspace - check if we have one saved in localStorage first
    const savedWs = localStorage.getItem('workspace')
    if (!currentWorkspace.value && savedWs) {
      try {
        currentWorkspace.value = JSON.parse(savedWs)
      } catch (e) {
        console.error('Failed to parse saved workspace', e)
      }
    }

    if (currentWorkspace.value) {
      const exists = workspaces.value.find(w => w.id === currentWorkspace.value.id)
      if (!exists) {
        console.warn('Current workspace no longer exists, clearing.')
        currentWorkspace.value = null
        localStorage.removeItem('workspace')
        showWorkspaceModal.value = true
      }
    } else if ((workspaces.value?.length || 0) > 0) {
      // Fallback: if no workspace selected but list is not empty, maybe show modal
      showWorkspaceModal.value = true
    }
  } catch (e) {
    const message = String(e?.message || e || '')
    workspacesError.value = message || 'Failed to load workspaces'
    if (message.includes('Not authenticated') || message.includes('401')) {
      handleLogout()
    }
  } finally {
    workspacesLoading.value = false
  }
}

async function switchToOrg(org) {
  if (currentUser.value?.organization?.id === org.id) return
  
  try {
    showWorkspaceModal.value = false
    loadingResults.value = true // Show loading overlay
    
    // Call REST login with orgId to get new credentials
    const result = await api.selectOrganization(org.id)
    
    // Re-initialize app state with new org
    await onLogin()
    
    // Explicitly refresh page to ensure all components reset correctly
    window.location.reload()
  } catch (e) {
    console.error('Failed to switch organization:', e)
    alert('Failed to switch organization: ' + (e.message || 'Unknown error'))
  } finally {
    loadingResults.value = false
  }
}

async function handleJoinOrg() {
  if (!joinOrgInviteCode.value.trim()) return
  
  joinLoading.value = true
  try {
    const result = await wsService.joinOrganization(joinOrgInviteCode.value)
    alert('Successfully joined organization!')
    isJoiningOrg.value = false
    joinOrgInviteCode.value = ''
    
    // Store new workspace if provided
    if (result.workspace) {
      localStorage.setItem('workspace', JSON.stringify(result.workspace))
      currentWorkspace.value = result.workspace
    }

    // Refresh user profile and organizations
    const me = await wsService.getMe()
    currentUser.value = me.user
    userOrganizations.value = me.user.organizations || []
    
    showWorkspaceModal.value = false
    
    // If successfully joined, it's often better to reload to ensure all stores sync up
    window.location.reload()
  } catch (e) {
    alert('Failed to join organization: ' + e.message)
  } finally {
    joinLoading.value = false
  }
}

async function selectWorkspace(ws) {
  try {
    const result = await wsService.switchWorkspace(ws.id)
    // Token is now handled via cookie (set by backend on switch)
    // Do NOT write to localStorage
    
    currentWorkspace.value = result.workspace || ws
    showWorkspaceModal.value = false
    await wsConnect(ws.id)
    // syncState is called automatically on 'connected' in wsStore
    await loadQuestionSets()
  } catch (e) {
    console.error('Failed to switch workspace:', e)
  }
}

async function loadQuestionSets(preferredId = null) {
  if (!isAuthenticated.value || !currentWorkspace.value) return
  try {
    // Data is now managed by wsStore, but we need to handle selection logic
    const uniqueSets = wsState.questionSets
    
    if (uniqueSets && uniqueSets.length > 0) {
        // Preference logic: exact match > last selected > first available
        // Attempt to restore last selected question set
        const lastQsId = localStorage.getItem('lastQuestionSetId')
        let found = false
        
        if (lastQsId) {
          const saved = uniqueSets.find(qs => qs.id === lastQsId)
          if (saved) {
            currentQuestionSet.value = saved
            found = true
          }
        }

        if (!found) {
           if (preferredId) {
             const pref = uniqueSets.find(s => s.id === preferredId)
             if (pref) currentQuestionSet.value = pref
           }
           // Fallback to first if still null
           if (!currentQuestionSet.value) currentQuestionSet.value = uniqueSets[0]
        }
        let targetSet = null
        
        if (preferredId) {
          targetSet = uniqueSets.find(s => s.id === preferredId)
        }
        
        if (!targetSet && currentQuestionSet.value) {
           targetSet = uniqueSets.find(s => s.id === currentQuestionSet.value.id)
        }
        
        // If still no target, use the most recent one (assuming sets are not strictly ordered, we might want to sort)
        // For now, default to first or last? Let's default to the *last* one if we assume append-only creation, 
        // but sorting by CreatedAt would be better. Let's just pick index 0 for now as default.
        if (!targetSet) {
           targetSet = uniqueSets[uniqueSets.length - 1] // Picking last one might be better for "newly created"
        }
        
        currentQuestionSet.value = targetSet
        
        if (currentQuestionSet.value && targetSet) {
           // We might want to notify BenchmarkArena to select first question, but it handles its own state
        }
    } else {
      currentQuestionSet.value = null
    }
  } catch (e) {
    console.error('Failed to load question sets:', e)
  }
}

function switchQuestionSet(id) {
  const set = questionSets.value.find(s => s.id === id)
  if (set) {
    currentQuestionSet.value = set
  }
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
  if (!currentUser.value?.organization) {
    alert('Please select an organization before creating a workspace.')
    return
  }
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
    const clonedWs = await wsService.cloneWorkspace(
      cloneSourceWorkspace.value.id, 
      cloneNewName.value.trim()
    )
    workspaces.value.push(clonedWs)
    cancelCloning()
    await selectWorkspace(clonedWs)
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
    
    // If target is 'new' OR no current question set, create a new one
    if (target === 'new' || !currentQuestionSet.value) {
      const result = await wsService.createQuestionSet(currentWorkspace.value.id, {
        name: setName,
        version: '1.0',
        data: data
      })
      currentQuestionSet.value = result
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
            existingData = { categories: [] }
          }
        }
        
        const existingCategories = existingData?.categories || []
        const importedCategories = data.categories || []
        
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
        
        finalData = { categories: mergedCategories }
      }
      
      const updated = await wsService.updateQuestionSet(currentQuestionSet.value.id, {
        name: currentQuestionSet.value.name,
        version: currentQuestionSet.value.version || '1.0',
        data: finalData
      })
      currentQuestionSet.value = updated
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
    ...data,
    workspaceName: wsName
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

const handleKeydown = (e) => {
  if (e.key === 'Escape' && isZenMode.value) {
    isZenMode.value = false
  }
}

onMounted(async () => {
  window.addEventListener('keydown', handleKeydown)
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

      } else {
        showWorkspaceModal.value = true
      }
      
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
    }
  }
  
  // Custom handlers for UI-specific reactivity in App.vue

  
  wsService.on('EVT_ERROR', (payload) => {
    const err = (typeof payload === 'string' ? payload : payload?.error || '').toLowerCase()
    if (err.includes('not authenticated') || err.includes('user not found') || err.includes('invalid token')) {
      console.warn('[App] Authentication error received via WS:', err)
      handleLogout()
    }
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
  window.removeEventListener('keydown', handleKeydown)
  wsDisconnect()
  if (refreshInterval.value) {
    clearInterval(refreshInterval.value)
  }
  downloadManager.cancelAll()
})
</script>

 
