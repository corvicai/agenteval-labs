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
        <div v-if="isDev" class="dev-badge">🚧 DEV MODE</div>
      </div>
          <div class="controls">
            <div class="buttons">
              <span class="user-badge" :class="{ admin: currentUser?.is_admin }" @click="openMyProfile">
                {{ currentUser?.is_admin ? '👑' : '👤' }} {{ currentUser?.name }}
              </span>
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
        @tab-change="val => adminTab = val"
      />
    </main>

    <!-- Admin Profile View -->
    <main v-if="viewMode === 'admin-profile'" class="main-content">
      <AdminProfileView 
        :entity-type="profileEntityType"
        :entity-id="profileEntityId"
        @back="() => { currentUser?.is_admin ? viewMode = 'admin' : viewMode = 'benchmarks'; profileEntityType = null; profileEntityId = null; }"
        @view-user="handleViewUserProfile"
        @view-org="handleViewOrgProfile"
        @updated="handleProfileUpdated"
      />
    </main>

    <!-- Manager Panel -->
    <main v-if="viewMode === 'manager' && isManager" class="main-content">
      <ManagerPanel 
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
                @view-history="goToHistory"
                @trigger-print="handleTriggerPrint"
                :is-zen-mode="isZenMode"
                @toggle-zen="val => isZenMode = val"
            />
          </KeepAlive>
          <!-- Workspace is automatically selected for each user -->
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
            <!-- Configure Agents button removed - use "Manage Agents" in header instead -->
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
            <!-- Workspace selection removed - each user has their own workspace -->
            <!-- Change Workspace button removed -->
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
import OnboardingScreen from './components/OnboardingScreen.vue'
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
const viewMode = ref(localStorage.getItem('viewMode') || 'benchmarks'); // 'benchmarks', 'admin', 'stats', 'admin-profile', 'manager'
const benchmarkMode = ref(localStorage.getItem('benchmarkMode') || 'runner') // 'history', 'runner'
const isLoggingIn = ref(false) // Flag to prevent concurrent initialization during login

// Manager state
const isManager = ref(false)
const appReady = ref(false)

// Admin Profile View State
const profileEntityType = ref(localStorage.getItem('profileEntityType')) // 'user' or 'organization'
const profileEntityId = ref(localStorage.getItem('profileEntityId'))
const adminTab = ref(localStorage.getItem('adminTab') || 'users') // Remember which tab was active

// Watch benchmarkMode to persist
// Watch benchmarkMode to persist
watch(benchmarkMode, (newVal) => {
  localStorage.setItem('benchmarkMode', newVal)
})

watch(viewMode, (newVal) => {
  localStorage.setItem('viewMode', newVal)
})

watch(adminTab, (newVal) => {
  localStorage.setItem('adminTab', newVal)
})

watch(profileEntityType, (newVal) => {
  if (newVal) localStorage.setItem('profileEntityType', newVal)
  else localStorage.removeItem('profileEntityType')
})

watch(profileEntityId, (newVal) => {
  if (newVal) localStorage.setItem('profileEntityId', newVal)
  else localStorage.removeItem('profileEntityId')
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
const showOnboarding = ref(false)

const workspaces = ref([])
const workspacesLoading = ref(false)
const workspacesError = ref('')
const currentWorkspace = ref(api.getStoredWorkspace())
const refreshInterval = ref(null)

const agents = computed(() => wsState.agents)
const questionSets = computed(() => wsState.questionSets)
const currentQuestionSet = ref(null)
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

  currentUser.value = mergedUser
  api.setStoredUser(mergedUser)
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

    // Onboarding check removed - users don't need organizations

  } catch (err) {
    console.error('[App] Login initialization failed:', err)
    } finally {
      appReady.value = true
      isLoggingIn.value = false
    }
}

function onOnboardingCompleted() {
  showOnboarding.value = false
  // Fully reload state to ensure everything is fresh
  window.location.reload()
}

// Admin Profile Navigation

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
        // Preference logic: exact match > last selected
        const lastQsId = localStorage.getItem('lastQuestionSetId')
        let targetSet = null
        
        if (lastQsId) {
          targetSet = uniqueSets.find(qs => qs.id === lastQsId)
        }

        if (!targetSet && preferredId) {
          targetSet = uniqueSets.find(s => s.id === preferredId)
        }
        
        // If still no target and we have a current selection, try to keep it if it exists in this workspace
        if (!targetSet && currentQuestionSet.value) {
           targetSet = uniqueSets.find(s => s.id === currentQuestionSet.value.id)
        }
        
        console.log('[App] loadQuestionSets: final targetSet:', targetSet?.name || 'none (defaulting to empty state)')
        
        // We REMOVED the "uniqueSets[0]" and "uniqueSets[uniqueSets.length - 1]" fallbacks 
        // to ensure we only show a set if it actually matches our context.
        currentQuestionSet.value = targetSet || null
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
      }
    } else {
       // Not authenticated, but we should be ready to show login
       appReady.value = true
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

 
