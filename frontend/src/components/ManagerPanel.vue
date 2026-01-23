<template>
  <div class="manager-panel">
    <div class="manager-header">
      <div class="title-group">
        <h2>👔 Manager Panel</h2>
        <p class="subtitle">Manage your organization: <strong>{{ orgName }}</strong></p>
      </div>
      <div class="manager-tabs">
        <button :class="{ active: tab === 'overview' }" @click="tab = 'overview'">📊 Overview</button>
        <button :class="{ active: tab === 'users' }" @click="tab = 'users'">👥 Users</button>
        <button :class="{ active: tab === 'workspaces' }" @click="tab = 'workspaces'">📁 Workspaces</button>
        <button :class="{ active: tab === 'agents' }" @click="tab = 'agents'">🤖 Agents</button>
        <button :class="{ active: tab === 'runs' }" @click="tab = 'runs'">🏃 Runs</button>
      </div>
    </div>

    <div v-if="loading && !users.length" class="loading-state">
      <div class="spinner"></div>
      <p>Loading organization data...</p>
    </div>

    <div v-else-if="error" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-secondary" @click="loadAll">Retry</button>
    </div>

    <div v-else class="manager-content">
      <!-- Overview Tab -->
      <div v-if="tab === 'overview'" class="tab-content">
        <div class="stats-grid">
          <div class="stat-card">
            <span class="stat-value">{{ stats.user_count }}</span>
            <span class="stat-label">Users</span>
          </div>
          <div class="stat-card">
            <span class="stat-value">{{ stats.workspace_count }}</span>
            <span class="stat-label">Workspaces</span>
          </div>
          <div class="stat-card">
            <span class="stat-value">{{ stats.agent_count }}</span>
            <span class="stat-label">Agents</span>
          </div>
          <div class="stat-card">
            <span class="stat-value">{{ stats.run_count }}</span>
            <span class="stat-label">Runs</span>
          </div>
        </div>
      </div>

      <!-- Users Tab -->
      <div v-if="tab === 'users'" class="tab-content">
        <div class="tab-actions">
          <h3>Team Members</h3>
          <div class="header-buttons">
            <div class="invite-config">
              <label>Max Uses:</label>
              <input v-model.number="maxUsesInput" type="number" min="1" max="100" class="input-small" />
              <button class="btn btn-secondary" @click="generateInvite">🎟️ Generate Invite</button>
            </div>
            <button class="btn btn-primary" @click="showCreateUser = true">+ Add User</button>
          </div>
        </div>

        <!-- Invite Display -->
        <div v-if="inviteCode" class="invite-banner">
          <div class="invite-content">
            <span class="invite-label">New Invite Code:</span>
            <code class="invite-code">{{ inviteCode }}</code>
            <span class="invite-usage">(Uses: 0 / {{ maxUsesResult }})</span>
            <span class="invite-expiry">(Expires in 7 days)</span>
            <button class="btn-copy" @click="copyInvite">📋 Copy</button>
            <button class="btn-close-small" @click="inviteCode = ''">×</button>
          </div>
        </div>
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>User</th>
                <th>Workspaces</th>
                <th>Status</th>
                <th>Joined</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="user in users" :key="user.id">
                <td>
                  <div class="user-info">
                    <span class="user-name">{{ user.name }}</span>
                    <span class="user-email">{{ user.email }}</span>
                  </div>
                </td>
                <td>{{ user.workspace_count }}</td>
                <td>
                  <span :class="['status-badge', user.is_suspended ? 'suspended' : 'active']">
                    {{ user.is_suspended ? 'Suspended' : 'Active' }}
                  </span>
                </td>
                <td>{{ formatDate(user.created_at) }}</td>
                <td class="actions-cell">
                  <div class="actions-wrapper">
                    <button 
                      class="btn-icon" 
                      @click="loginAs(user)" 
                      title="Login As"
                      :disabled="user.id === currentUserId || user.is_suspended"
                    >
                      🕵️
                    </button>
                    <button class="btn-icon" @click="editUser(user)" title="Edit">✏️</button>
                    <button 
                      class="btn-icon" 
                      :class="user.is_suspended ? 'btn-success-icon' : 'btn-warning-icon'"
                      @click="toggleSuspend(user)"
                      :title="user.is_suspended ? 'Activate' : 'Suspend'"
                      :disabled="user.id === currentUserId"
                    >
                      {{ user.is_suspended ? '🔓' : '🔒' }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Workspaces Tab -->
      <div v-if="tab === 'workspaces'" class="tab-content">
        <h3>Organization Workspaces</h3>
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>Workspace</th>
                <th>Owner</th>
                <th>Agents</th>
                <th>Runs</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="ws in workspaces" :key="ws.id">
                <td>{{ ws.name }}</td>
                <td>{{ ws.user_name }}</td>
                <td>{{ ws.agent_count }}</td>
                <td>{{ ws.run_count }}</td>
                <td>{{ formatDate(ws.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Agents Tab -->
      <div v-if="tab === 'agents'" class="tab-content">
        <h3>All Agents</h3>
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>Agent</th>
                <th>Provider</th>
                <th>Workspace</th>
                <th>Owner</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="agent in agents" :key="agent.id">
                <td>{{ agent.name }}</td>
                <td><span class="provider-badge">{{ agent.provider_type }}</span></td>
                <td>{{ agent.workspace_name }}</td>
                <td>{{ agent.user_name }}</td>
                <td>
                  <span :class="['status-badge', agent.enabled ? 'active' : 'suspended']">
                    {{ agent.enabled ? 'Enabled' : 'Disabled' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Runs Tab -->
      <div v-if="tab === 'runs'" class="tab-content">
        <h3>Recent Runs</h3>
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>Run</th>
                <th>Status</th>
                <th>Question Set</th>
                <th>Workspace</th>
                <th>Results</th>
                <th>Date</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="run in runs" :key="run.id">
                <td>{{ run.question_set_name || 'Unknown Set' }}</td>
                <td><span :class="['status-badge', run.status]">{{ run.status }}</span></td>
                <td>{{ run.question_set_name || '-' }}</td>
                <td>{{ run.workspace_name }}</td>
                <td>{{ run.result_count }}</td>
                <td>{{ formatDate(run.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Create User Modal -->
    <div v-if="showCreateUser" class="modal-overlay" @click.self="showCreateUser = false">
      <div class="modal-container">
        <div class="modal-header">
          <h3>➕ Add Team Member</h3>
          <button class="btn-close" @click="showCreateUser = false">×</button>
        </div>
        <form @submit.prevent="createUser" class="modal-form">
          <div v-if="formError" class="error-message">{{ formError }}</div>
          <div class="form-group">
            <label>Name</label>
            <input v-model="userForm.name" type="text" required />
          </div>
          <div class="form-group">
            <label>Email</label>
            <input v-model="userForm.email" type="email" required />
          </div>
          <div class="form-group">
            <label>Password</label>
            <input v-model="userForm.password" type="password" required />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showCreateUser = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="formLoading">
              {{ formLoading ? 'Creating...' : 'Create User' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Edit User Modal -->
    <div v-if="showEditUser" class="modal-overlay" @click.self="showEditUser = false">
      <div class="modal-container">
        <div class="modal-header">
          <h3>✏️ Edit User</h3>
          <button class="btn-close" @click="showEditUser = false">×</button>
        </div>
        <form @submit.prevent="updateUser" class="modal-form">
          <div v-if="formError" class="error-message">{{ formError }}</div>
          <div class="form-group">
            <label>Name</label>
            <input v-model="editForm.name" type="text" required />
          </div>
          <div class="form-group">
            <label>Email</label>
            <input v-model="editForm.email" type="email" required />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showEditUser = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="formLoading">
              {{ formLoading ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { wsService } from '../services/websocket.js'

const props = defineProps({
  orgName: { type: String, default: 'My Organization' },
  currentUserId: String,
  workspaceId: String // Add workspaceId to props if needed for WS connection
})

const tab = ref('overview')
const loading = ref(true)
const error = ref('')

const stats = ref({ user_count: 0, workspace_count: 0, agent_count: 0, run_count: 0 })
const users = ref([])
const workspaces = ref([])
const agents = ref([])
const runs = ref([])

// Modals
const showCreateUser = ref(false)
const showEditUser = ref(false)
const formLoading = ref(false)
const formError = ref('')
const userForm = ref({ name: '', email: '', password: '' })
const editForm = ref({ id: '', name: '', email: '' })
const inviteCode = ref('')
const maxUsesInput = ref(1)
const maxUsesResult = ref(1)

function formatDate(dateStr) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

async function loadAll() {
  loading.value = true
  error.value = ''
  try {
    const [s, u, w, a, r] = await Promise.all([
      wsService.managerGetStats(),
      wsService.managerGetUsers(),
      wsService.managerGetWorkspaces(),
      wsService.managerGetAgents(),
      wsService.managerGetRuns()
    ])
    stats.value = s
    users.value = u
    workspaces.value = w
    agents.value = a
    runs.value = r
  } catch (e) {
    error.value = 'Failed to load manager data: ' + e.message
  } finally {
    loading.value = false
  }
}

async function createUser() {
  formLoading.value = true
  formError.value = ''
  try {
    await wsService.managerCreateUser(userForm.value)
    showCreateUser.value = false
    userForm.value = { name: '', email: '', password: '' }
    await loadAll()
  } catch (e) {
    formError.value = e.message
  } finally {
    formLoading.value = false
  }
}

async function generateInvite() {
  try {
    const result = await wsService.managerGenerateInvite(maxUsesInput.value)
    inviteCode.value = result.code
    maxUsesResult.value = maxUsesInput.value
  } catch (e) {
    alert('Failed to generate invite: ' + e.message)
  }
}

function copyInvite() {
  if (!inviteCode.value) return
  navigator.clipboard.writeText(inviteCode.value)
  alert('Invite code copied to clipboard!')
}

function editUser(user) {
  editForm.value = { id: user.id, name: user.name, email: user.email }
  showEditUser.value = true
}

async function updateUser() {
  formLoading.value = true
  formError.value = ''
  try {
    await wsService.managerUpdateUser({ 
      id: editForm.value.id, 
      name: editForm.value.name, 
      email: editForm.value.email 
    })
    showEditUser.value = false
    await loadAll()
  } catch (e) {
    formError.value = e.message
  } finally {
    formLoading.value = false
  }
}

async function toggleSuspend(user) {
  try {
    await wsService.managerToggleUserSuspension(user.id)
    await loadAll()
  } catch (e) {
    alert('Failed: ' + e.message)
  }
}

async function loginAs(user) {
  if (!confirm(`Are you sure you want to login as ${user.name}?`)) return
  
  try {
    const response = await wsService.managerImpersonateUser(user.id)
    if (response && response.token) {
      // Clear old state and set new context for reload
      localStorage.setItem('impersonation_token', response.token)
      localStorage.setItem('is_impersonating', '1')
      localStorage.removeItem('token')
      if (response.user) {
        const impersonatedUser = {
          ...response.user,
          impersonator_id: props.currentUserId || response.user.impersonator_id
        }
        localStorage.setItem('user', JSON.stringify(impersonatedUser))
      }
      if (response.workspace) {
        localStorage.setItem('workspace', JSON.stringify(response.workspace))
      }
      window.location.reload()
    }
  } catch (e) {
    alert('Failed to impersonate: ' + e.message)
  }
}

onMounted(loadAll)
</script>

<style scoped>
.manager-panel {
  padding: 1.5rem 2rem;
  background: #f8fafc;
  min-height: 100%;
}

.manager-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.title-group h2 {
  margin: 0 0 0.25rem;
  font-size: 1.5rem;
  color: #1e293b;
}

.subtitle {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
}

.manager-tabs {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.manager-tabs button {
  padding: 0.5rem 1rem;
  border: 1px solid #e2e8f0;
  background: white;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.15s;
}

.manager-tabs button:hover {
  background: #f1f5f9;
}

.manager-tabs button.active {
  background: #6366f1;
  color: white;
  border-color: #6366f1;
}

.manager-content {
  background: white;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  padding: 1.5rem;
}

.tab-content h3 {
  margin: 0 0 1rem;
  font-size: 1.1rem;
  color: #1e293b;
}

.tab-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.header-buttons {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}

.invite-config {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: #f1f5f9;
  padding: 0.25rem 0.5rem;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}

.invite-config label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
}

.input-small {
  width: 50px;
  padding: 0.25rem;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  font-size: 0.85rem;
  text-align: center;
}

.invite-banner {
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  margin-bottom: 1.5rem;
  animation: slideIn 0.3s ease-out;
}

.invite-content {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 0.9rem;
}

.invite-label {
  color: #1e40af;
  font-weight: 600;
}

.invite-code {
  background: white;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  border: 1px solid #bfdbfe;
  font-weight: 700;
  color: #2563eb;
  letter-spacing: 0.05em;
}

.invite-usage {
  font-weight: 600;
  color: #1e40af;
  font-size: 0.85rem;
}

.invite-expiry {
  color: #64748b;
  font-size: 0.8rem;
}

.btn-copy {
  background: white;
  border: 1px solid #cbd5e1;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  transition: all 0.2s;
}

.btn-copy:hover {
  background: #f1f5f9;
  border-color: #94a3b8;
}

.btn-close-small {
  margin-left: auto;
  background: none;
  border: none;
  font-size: 1.25rem;
  cursor: pointer;
  color: #94a3b8;
  line-height: 1;
}

@keyframes slideIn {
  from { opacity: 0; transform: translateY(-10px); }
  to { opacity: 1; transform: translateY(0); }
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 1rem;
}

.stat-card {
  background: linear-gradient(135deg, #f8fafc, #f1f5f9);
  padding: 1.5rem;
  border-radius: 12px;
  text-align: center;
  border: 1px solid #e2e8f0;
}

.stat-value {
  display: block;
  font-size: 2.5rem;
  font-weight: 700;
  color: #6366f1;
}

.stat-label {
  display: block;
  margin-top: 0.5rem;
  font-size: 0.8rem;
  color: #64748b;
  text-transform: uppercase;
}

.table-container {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th, td {
  padding: 0.75rem 1rem;
  text-align: left;
  border-bottom: 1px solid #e2e8f0;
}

th {
  font-size: 0.7rem;
  text-transform: uppercase;
  color: #94a3b8;
  font-weight: 600;
  background: #f8fafc;
}

td {
  color: #475569;
  font-size: 0.9rem;
}

.user-info {
  display: flex;
  flex-direction: column;
}

.user-name {
  font-weight: 600;
  color: #1e293b;
}

.user-email {
  font-size: 0.8rem;
  color: #64748b;
}

.status-badge {
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.75rem;
  font-weight: 500;
}

.status-badge.active, .status-badge.completed {
  background: #dcfce7;
  color: #16a34a;
}

.status-badge.suspended, .status-badge.failed {
  background: #fef2f2;
  color: #dc2626;
}

.status-badge.running {
  background: #fef3c7;
  color: #d97706;
}

.provider-badge {
  background: #f1f5f9;
  color: #475569;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  font-size: 0.75rem;
}

.actions-cell {
  vertical-align: middle;
}

.actions-wrapper {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: nowrap;
}

.btn-icon {
  background: none;
  border: 1px solid #e2e8f0;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.btn-icon:hover {
  background: #f1f5f9;
}

.btn-icon:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn {
  padding: 0.5rem 1rem;
  border-radius: 8px;
  font-size: 0.9rem;
  cursor: pointer;
  border: none;
  transition: all 0.15s;
}

.btn-primary {
  background: #6366f1;
  color: white;
}

.btn-primary:hover {
  background: #4f46e5;
}

.btn-secondary {
  background: #f1f5f9;
  color: #475569;
  border: 1px solid #e2e8f0;
}

.loading-state, .error-state {
  text-align: center;
  padding: 3rem;
  color: #64748b;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin: 0 auto 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Modals */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.5);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-container {
  background: white;
  border-radius: 12px;
  width: 100%;
  max-width: 400px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.2);
}

.modal-header {
  padding: 1rem 1.25rem;
  border-bottom: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal-header h3 {
  margin: 0;
  font-size: 1.1rem;
}

.btn-close {
  background: none;
  border: none;
  font-size: 1.5rem;
  cursor: pointer;
  color: #94a3b8;
}

.modal-form {
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.form-group label {
  font-size: 0.8rem;
  font-weight: 600;
  color: #475569;
}

.form-group input {
  padding: 0.6rem 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.9rem;
}

.form-group input:focus {
  outline: none;
  border-color: #6366f1;
}

.modal-actions {
  display: flex;
  gap: 0.75rem;
  justify-content: flex-end;
  margin-top: 0.5rem;
}

.error-message {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #dc2626;
  padding: 0.75rem;
  border-radius: 8px;
  font-size: 0.85rem;
}
</style>
