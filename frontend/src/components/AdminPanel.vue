<template>
  <div class="admin-panel-container">
    <div class="admin-header">
      <div class="title-group">
        <h2>⚙️ Admin Panel</h2>
        <div class="subtitle-row">
          <p class="subtitle">Manage users and organizations across the platform.</p>
          <span class="online-badge">🟢 {{ wsState.onlineCount }} Users Online</span>
        </div>
      </div>
      <div class="admin-tabs">
        <button 
          :class="{ active: currentTab === 'users' }" 
          @click="currentTab = 'users'"
        >👥 Users</button>
        <button 
          :class="{ active: currentTab === 'organizations' }" 
          @click="currentTab = 'organizations'"
        >🏢 Organizations</button>
        <button 
          :class="{ active: currentTab === 'logs' }" 
          @click="currentTab = 'logs'"
        >📜 Login Logs</button>
      </div>
    </div>

    <div v-if="loading && !users.length && !organizations.length" class="loading-state">
      <div class="spinner"></div>
      <p>Loading administrative data...</p>
    </div>

    <div v-else-if="error" class="error-state">
      <div class="error-icon">⚠️</div>
      <p>{{ error }}</p>
      <button class="btn btn-secondary" @click="reloadData">Retry</button>
    </div>

    <div v-else class="admin-content">
      <!-- Users Tab -->
      <div v-if="currentTab === 'users'" class="tab-view">
        <div class="tab-actions">
          <h3>User Directory</h3>
          <button class="btn btn-primary" @click="showCreateModal = true">
            + Create User
          </button>
        </div>
        
        <!-- Filter Bar -->
        <div class="filter-bar">
          <div class="search-box">
            <span class="search-icon">🔍</span>
            <input 
              v-model="userSearch" 
              type="text" 
              placeholder="Search by name or email..." 
              class="search-input"
            />
            <button v-if="userSearch" class="clear-btn" @click="userSearch = ''">&times;</button>
          </div>
          <div class="filter-group">
            <select v-model="userOrgFilter" class="filter-select">
              <option value="">All Organizations</option>
              <option v-for="org in organizations" :key="org.id" :value="org.name">
                {{ org.name }}
              </option>
            </select>
            <select v-model="userRoleFilter" class="filter-select">
              <option value="">All Roles</option>
              <option value="admin">Admins Only</option>
              <option value="user">Users Only</option>
            </select>
          </div>
          <span v-if="filteredUsers.length !== users.length" class="filter-count">
            Showing {{ filteredUsers.length }} of {{ users.length }}
          </span>
        </div>
        
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>User</th>
                <th>Organization</th>
                <th>Workspaces</th>
                <th>Role</th>
                <th>Created</th>
                <th>Last Login</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="user in filteredUsers" :key="user.id">
                <td>
                  <div class="user-info">
                    <span class="user-name clickable" @click="viewUserProfile(user.id)">
                      {{ user.name }}
                      <span v-if="wsState.onlineUsers.includes(user.id)" class="online-dot" title="Online">●</span>
                    </span>
                    <span class="user-email">{{ user.email }}</span>
                  </div>
                </td>
                <td>
                  <span class="org-badge">{{ user.organization_name || 'No Org' }}</span>
                </td>
                <td>
                  <div class="workspace-pills">
                    <span v-for="ws in user.workspaces" :key="ws.id" class="ws-pill">
                      {{ ws.name }}
                    </span>
                    <span v-if="!user.workspaces?.length" class="no-ws">No workspaces</span>
                  </div>
                </td>
                <td>
                  <span class="role-badge" :class="{ admin: user.is_admin }">
                    {{ user.is_admin ? 'Admin' : 'User' }}
                  </span>
                </td>
                <td>
                  <span class="date-badge">{{ formatDate(user.created_at) }}</span>
                </td>
                <td>
                  <span v-if="user.last_login_at" class="date-badge">{{ formatDateTime(user.last_login_at) }}</span>
                  <span v-else class="text-muted">Never</span>
                </td>
                <td class="actions-cell">
                  <div class="actions-wrapper">
                    <button class="btn-icon" @click="viewUserProfile(user.id)" title="View Profile">👁️</button>
                    <button class="btn-icon" @click="editUser(user)" title="Edit">✏️</button>
                    <button 
                      class="btn-icon" 
                      @click="toggleAdmin(user)" 
                      :title="user.is_admin ? 'Demote from Admin' : 'Promote to Admin'"
                      :disabled="user.id === currentUserId"
                    >🛡️</button>
                    <button 
                      class="btn-icon" 
                      :class="user.is_suspended ? 'btn-success-icon' : 'btn-warning-icon'"
                      @click="toggleUserSuspension(user)" 
                      :title="user.is_suspended ? 'Activate' : 'Suspend'"
                      :disabled="user.id === currentUserId"
                    >
                      {{ user.is_suspended ? '🔓' : '🔒' }}
                    </button>
                    <button 
                      class="btn-icon btn-danger-icon" 
                      @click="deleteUser(user)" 
                      title="Delete"
                      :disabled="user.id === currentUserId"
                    >🗑️</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Organizations Tab -->
      <div v-else-if="currentTab === 'organizations'" class="tab-view">
        <div class="tab-actions">
          <h3>Organization Directory</h3>
          <button class="btn btn-primary" @click="openOrgModal()">+ Create Organization</button>
        </div>
        
        <!-- Filter Bar -->
        <div class="filter-bar">
          <div class="search-box">
            <span class="search-icon">🔍</span>
            <input 
              v-model="orgSearch" 
              type="text" 
              placeholder="Search organizations..." 
              class="search-input"
            />
            <button v-if="orgSearch" class="clear-btn" @click="orgSearch = ''">&times;</button>
          </div>
          <div class="filter-group">
            <select v-model="orgStatusFilter" class="filter-select">
              <option value="">All Status</option>
              <option value="active">Active Only</option>
              <option value="suspended">Suspended Only</option>
            </select>
          </div>
          <span v-if="filteredOrganizations.length !== organizations.length" class="filter-count">
            Showing {{ filteredOrganizations.length }} of {{ organizations.length }}
          </span>
        </div>
        
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>ORGANIZATION</th>
                <th>MANAGER</th>
                <th>USERS</th>
                <th>STATUS</th>
                <th>CREATED</th>
                <th>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="org in filteredOrganizations" :key="org.id">
                <td>
                  <div class="user-info">
                    <span class="user-name clickable" @click="viewOrgProfile(org.id)">{{ org.name }}</span>
                    <span class="user-email">ID: {{ org.id.slice(0, 8) }}...</span>
                  </div>
                </td>
                <td>
                  <span class="org-badge">{{ org.manager_name || 'No Manager' }}</span>
                </td>
                <td>
                  <span class="workspace-badge">{{ org.user_count || 0 }} Users</span>
                </td>
                <td>
                  <span :class="['role-badge', org.is_suspended ? 'role-user' : 'role-admin']">
                    {{ org.is_suspended ? 'SUSPENDED' : 'ACTIVE' }}
                  </span>
                </td>
                <td>
                  <span class="date-badge">{{ formatDate(org.created_at) }}</span>
                </td>
                <td class="actions-cell">
                  <div class="actions-wrapper">
                    <button class="btn-icon" @click="viewOrgProfile(org.id)" title="View Profile">👁️</button>
                    <button class="btn-icon" @click="openOrgModal(org)" title="Edit">✏️</button>
                    <button 
                      class="btn-icon" 
                      :class="org.is_suspended ? 'btn-success-icon' : 'btn-warning-icon'"
                      @click="toggleOrgSuspension(org)" 
                      :title="org.is_suspended ? 'Activate' : 'Suspend'"
                    >
                      {{ org.is_suspended ? '🔓' : '🔒' }}
                    </button>
                    <button class="btn-icon btn-danger-icon" @click="confirmDeleteOrg(org)" title="Delete">🗑️</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Login Logs Tab -->
      <div v-else-if="currentTab === 'logs'" class="tab-view">
        <div class="tab-actions">
          <h3>Recent Login Activity</h3>
          <button class="btn btn-secondary" @click="loadLoginLogs">🔄 Refresh</button>
        </div>

        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Status</th>
                <th>User / Email</th>
                <th>IP Address</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="log in loginLogs" :key="log.id">
                <td>
                  <span class="date-badge">{{ formatDateTime(log.created_at) }}</span>
                </td>
                <td>
                  <span 
                    class="role-badge" 
                    :class="log.status === 'success' ? 'role-admin' : 'role-user'"
                    :style="log.status === 'success' ? 'background: #dcfce7; color: #166534;' : 'background: #fee2e2; color: #991b1b;'"
                  >
                    {{ log.status.toUpperCase() }}
                  </span>
                </td>
                <td>
                  <div class="user-info">
                    <span class="user-email">{{ log.user_email }}</span>
                    <span v-if="log.user_id" class="user-name-small">ID: {{ log.user_id.slice(0,8) }}...</span>
                  </div>
                </td>
                <td>
                  <span class="ip-address">{{ log.ip_address }}</span>
                </td>
                 <td class="details-cell">
                   <div class="log-details">
                     <span v-if="log.failure_reason" class="error-text">Reason: {{ log.failure_reason }}</span>
                     <span class="user-agent" :title="log.user_agent">{{ formatUserAgent(log.user_agent) }}</span>
                   </div>
                </td>
              </tr>
              <tr v-if="loginLogs.length === 0">
                <td colspan="5" class="empty-cell">No logs found</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

    </div>

    <!-- Create/Edit User Modal -->
    <div v-if="showCreateModal || showEditModal" class="modal-overlay" @click.self="closeModals">
      <div class="modal-container">
        <div class="modal-header">
          <h3>{{ showEditModal ? '✏️ Edit User' : '➕ Create User' }}</h3>
          <button class="btn-close" @click="closeModals">×</button>
        </div>
        <form @submit.prevent="saveUser" class="modal-form">
          <div class="form-group">
            <label>Full Name</label>
            <input v-model="formData.name" type="text" required placeholder="John Doe" />
          </div>
          <div class="form-group">
            <label>Email Address</label>
            <input v-model="formData.email" type="email" required placeholder="john@example.com" />
          </div>
          <div class="form-group">
            <label>Password {{ showEditModal ? '(Optional)' : '' }}</label>
            <input v-model="formData.password" type="password" :required="!showEditModal" :placeholder="showEditModal ? 'Keep current' : '••••••••'" />
          </div>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" v-model="formData.is_admin" />
              <span>Grant Administrator Privileges</span>
            </label>
          </div>
          <div v-if="formError" class="modal-error">{{ formError }}</div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="closeModals">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="formLoading">
              <span v-if="formLoading" class="spinner-small"></span>
              {{ showEditModal ? 'Save Changes' : 'Create User' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Organization Modal -->
    <div v-if="showOrgModal" class="modal-overlay" @click.self="showOrgModal = false">
      <div class="modal-container">
        <div class="modal-header">
          <h3>{{ editingOrg ? '✏️ Edit Organization' : '➕ Create Organization' }}</h3>
          <button class="btn-close" @click="showOrgModal = false">×</button>
        </div>
        <form @submit.prevent="saveOrg" class="modal-form">
          <div class="form-group">
            <label>Organization Name</label>
            <input v-model="orgForm.name" type="text" required placeholder="Corvic AI" />
          </div>
          <div class="form-group">
            <label>Manager (Organization Admin)</label>
            <select v-model="orgForm.manager_id" class="form-select">
              <option :value="null">No Manager</option>
              <option v-for="user in users" :key="user.id" :value="user.id">
                {{ user.name }} ({{ user.email }})
              </option>
            </select>
          </div>
          <div v-if="formError" class="modal-error">{{ formError }}</div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showOrgModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="formLoading">
              <span v-if="formLoading" class="spinner-small"></span>
              {{ editingOrg ? 'Save Changes' : 'Create Organization' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Confirmation Dialog -->
    <ConfirmDialog
      v-model:visible="showConfirmDialog"
      :title="confirmDialogConfig.title"
      :message="confirmDialogConfig.message"
      :confirm-text="confirmDialogConfig.confirmText"
      :cancel-text="confirmDialogConfig.cancelText"
      :variant="confirmDialogConfig.variant"
      @confirm="executeConfirmedAction"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { wsService } from '../services/websocket.js'
import { useWSStore } from '../stores/wsStore.js'
import ConfirmDialog from './ConfirmDialog.vue'

const props = defineProps({
  currentUserId: String,
  initialTab: { type: String, default: 'users' }
})

const { state: wsState } = useWSStore()

const emit = defineEmits(['close', 'view-user-profile', 'view-org-profile'])

const currentTab = ref(props.initialTab);
const users = ref([]);
const organizations = ref([]);
const loginLogs = ref([]);
const loading = ref(true); // Changed to true initially
const error = ref('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showOrgModal = ref(false) // New
const formData = ref({ name: '', email: '', password: '', is_admin: false })
const orgForm = ref({ name: '', manager_id: null }) // Updated
const formLoading = ref(false)
const formError = ref('')
const editingUserId = ref(null)
const editingOrg = ref(null)

// Filter State
const userSearch = ref('')
const userOrgFilter = ref('')
const userRoleFilter = ref('')
const orgSearch = ref('')
const orgStatusFilter = ref('')

// Computed: Filtered Users
const filteredUsers = computed(() => {
  return users.value.filter(user => {
    // Search filter
    const searchLower = userSearch.value.toLowerCase()
    const matchesSearch = !userSearch.value || 
      user.name?.toLowerCase().includes(searchLower) ||
      user.email?.toLowerCase().includes(searchLower)
    
    // Organization filter
    const matchesOrg = !userOrgFilter.value || 
      user.organization_name === userOrgFilter.value
    
    // Role filter
    const matchesRole = !userRoleFilter.value ||
      (userRoleFilter.value === 'admin' && user.is_admin) ||
      (userRoleFilter.value === 'user' && !user.is_admin)
    
    return matchesSearch && matchesOrg && matchesRole
  })
})

// Computed: Filtered Organizations
const filteredOrganizations = computed(() => {
  return organizations.value.filter(org => {
    // Search filter
    const searchLower = orgSearch.value.toLowerCase()
    const matchesSearch = !orgSearch.value || 
      org.name?.toLowerCase().includes(searchLower) ||
      org.manager_name?.toLowerCase().includes(searchLower)
    
    // Status filter
    const matchesStatus = !orgStatusFilter.value ||
      (orgStatusFilter.value === 'active' && !org.is_suspended) ||
      (orgStatusFilter.value === 'suspended' && org.is_suspended)
    
    return matchesSearch && matchesStatus
  })
})

// Confirmation Dialog State
const showConfirmDialog = ref(false)
const confirmDialogConfig = ref({
  title: 'Confirm',
  message: 'Are you sure?',
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  variant: 'default'
})
const pendingAction = ref(null)

function formatDate(dateStr) {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', { 
    year: 'numeric', 
    month: 'short', 
    day: 'numeric' 
  })
}

function formatDateTime(dateStr) {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleString('en-US', { 
    month: 'short', 
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit'
  })
}

function formatUserAgent(ua) {
  if (!ua) return '-'
  if (ua.length > 50) return ua.substring(0, 47) + '...'
  return ua
}

function viewUserProfile(userId) {
  emit('view-user-profile', userId)
}

function viewOrgProfile(orgId) {
  emit('view-org-profile', orgId)
}


async function loadData() {
  loading.value = true
  error.value = ''
  try {
    // Use WebSocket for admin data
    const [usersList, orgsList] = await Promise.all([
      wsService.adminGetUsers(),
      wsService.adminGetOrganizations()
    ])
    users.value = usersList
    organizations.value = orgsList
    
    // Load logs if tab is active
    if (currentTab.value === 'logs') {
      await loadLoginLogs()
    }
  } catch (e) {
    error.value = 'Failed to load administrative data: ' + e.message
  } finally {
    loading.value = false
  }
}

async function loadLoginLogs() {
  try {
    const logs = await wsService.adminGetLoginLogs(100)
    loginLogs.value = logs || []
  } catch (e) {
    console.error('Failed to load logs:', e)
  }
}

function editUser(user) {
  editingUserId.value = user.id
  formData.value = { name: user.name, email: user.email, is_admin: user.is_admin }
  showEditModal.value = true
}

function closeModals() {
  showCreateModal.value = false
  showEditModal.value = false
  formData.value = { name: '', email: '', password: '', is_admin: false }
  formError.value = ''
  editingUserId.value = null
}

async function saveUser() {
  formLoading.value = true
  formError.value = ''
  
  try {
    if (showEditModal.value) {
      await wsService.adminUpdateUser({ 
        id: editingUserId.value, 
        ...formData.value 
      })
    } else {
      await wsService.adminCreateUser(formData.value)
    }
    closeModals()
    loadData()
  } catch (e) {
    formError.value = e.message
  } finally {
    formLoading.value = false
  }
}

async function toggleUserSuspension(user) {
  try {
    await wsService.adminUpdateUser({ 
      id: user.id, 
      is_suspended: !user.is_suspended 
    })
    await loadData()
  } catch (e) {
    alert('Failed to update user status: ' + e.message)
  }
}

function toggleAdmin(user) {
  const action = user.is_admin ? 'demote' : 'promote'
  confirmDialogConfig.value = {
    title: user.is_admin ? 'Demote Admin' : 'Promote to Admin',
    message: `Are you sure you want to ${action} "${user.name}" ${user.is_admin ? 'to a regular user' : 'to an administrator'}?`,
    confirmText: user.is_admin ? 'Demote' : 'Promote',
    cancelText: 'Cancel',
    variant: user.is_admin ? 'warning' : 'primary'
  }
  pendingAction.value = async () => {
    try {
      await wsService.adminUpdateUser({ 
        id: user.id, 
        is_admin: !user.is_admin 
      })
      await loadData()
    } catch (e) {
      alert(`Failed to ${action}: ` + e.message)
    }
  }
  showConfirmDialog.value = true
}


function deleteUser(user) {
  confirmDialogConfig.value = {
    title: 'Delete User',
    message: `Are you sure you want to delete "${user.name}"? This action cannot be undone.`,
    confirmText: 'Delete',
    cancelText: 'Cancel',
    variant: 'danger'
  }
  pendingAction.value = async () => {
    try {
      await wsService.adminDeleteUser(user.id)
      loadData()
    } catch (e) {
      alert('Failed to delete: ' + e.message)
    }
  }
  showConfirmDialog.value = true
}

// Organization Methods
function openOrgModal(org = null) {
  editingOrg.value = org
  if (org) {
    orgForm.value = { 
      name: org.name,
      manager_id: org.manager_id
    }
  } else {
    orgForm.value = { 
      name: '',
      manager_id: null
    }
  }
  showOrgModal.value = true
}

async function saveOrg() {
  formLoading.value = true
  formError.value = ''
  try {
    if (editingOrg.value) {
      await wsService.adminUpdateOrg({
        id: editingOrg.value.id,
        ...orgForm.value
      })
    } else {
      await wsService.adminCreateOrg(orgForm.value)
    }
    await loadData()
    showOrgModal.value = false
  } catch (e) {
    formError.value = 'Failed to save organization: ' + e.message
  } finally {
    formLoading.value = false
  }
}

async function toggleOrgSuspension(org) {
  try {
    await wsService.adminUpdateOrg({ 
      id: org.id, 
      is_suspended: !org.is_suspended 
    })
    await loadData()
  } catch (e) {
    alert('Failed to update organization status: ' + e.message)
  }
}

function confirmDeleteOrg(org) {
  confirmDialogConfig.value = {
    title: 'Delete Organization',
    message: `Are you sure you want to delete "${org.name}"? This will permanently delete all users, workspaces, and data associated with this organization.`,
    confirmText: 'Delete Organization',
    cancelText: 'Cancel',
    variant: 'danger'
  }
  pendingAction.value = async () => {
    try {
      await wsService.adminDeleteOrg(org.id)
      await loadData()
    } catch (e) {
      alert('Failed to delete organization: ' + e.message)
    }
  }
  showConfirmDialog.value = true
}

async function executeConfirmedAction() {
  if (pendingAction.value) {
    await pendingAction.value()
    pendingAction.value = null
  }
}

onMounted(() => {
  loadData();
})

// Watch tab change to load data
import { watch } from 'vue'
watch(currentTab, (newTab) => {
  if (newTab === 'logs' && loginLogs.value.length === 0) {
    loadLoginLogs()
  }
})
</script>

<style scoped>
.admin-panel-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 1.5rem 2rem;
  box-sizing: border-box;
  overflow: hidden;
  background: var(--app-bg);
  color: var(--text-primary);
}

.admin-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 20px;
  flex-shrink: 0;
  margin-bottom: 24px;
}

.title-group h2 {
  margin: 0;
  font-size: 1.75rem;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.subtitle-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
}

.subtitle {
  margin: 0;
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.online-badge {
  background: #f0fdf4;
  color: #16a34a;
  border: 1px solid #bbf7d0;
  padding: 2px 8px;
  border-radius: 99px;
  font-size: 0.75rem;
  font-weight: 600;
}

.online-dot {
  color: #22c55e;
  margin-left: 6px;
  font-size: 0.8rem;
  vertical-align: middle;
}

.admin-tabs {
  display: flex;
  gap: 4px;
  background: #f1f5f9;
  padding: 4px;
  border-radius: 10px;
}

.admin-tabs button {
  background: transparent;
  border: none;
  padding: 8px 16px;
  color: var(--text-secondary);
  font-weight: 600;
  font-size: 0.9rem;
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.2s;
}

.admin-tabs button:hover {
  background: #e2e8f0;
}

.admin-tabs button.active {
  background: white;
  color: var(--text-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.admin-content {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.tab-view {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.tab-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.tab-actions h3 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 500;
  color: var(--text-primary);
}

/* Filter Bar Styles */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  flex-wrap: wrap;
}

.search-box {
  position: relative;
  flex: 1;
  min-width: 200px;
  max-width: 400px;
}

.search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 0.9rem;
  opacity: 0.6;
}

.search-input {
  width: 100%;
  padding: 10px 36px 10px 36px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 0.9rem;
  background: white;
  color: #1e293b;
  transition: all 0.2s;
}

.search-input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.search-input::placeholder {
  color: #94a3b8;
}

.clear-btn {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  background: white;
  border: none;
  font-size: 1.2rem;
  color: #94a3b8;
  cursor: pointer;
  padding: 4px;
  line-height: 1;
}

.clear-btn:hover {
  background: #cbd5e1;
  color: #1e293b;
}

.filter-group {
  display: flex;
  gap: 12px;
}

.filter-select {
  padding: 10px 32px 10px 14px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 0.85rem;
  background: white;
  color: #1e293b;
  cursor: pointer;
  appearance: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 20 20'%3E%3Cpath stroke='%2364748b' stroke-linecap='round' stroke-linejoin='round' stroke-width='1.5' d='m6 8 4 4 4-4'/%3E%3C/svg%3E");
  background-position: right 8px center;
  background-repeat: no-repeat;
  background-size: 16px;
  transition: all 0.2s;
}

.filter-select:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.filter-select:hover {
  border-color: #cbd5e1;
}

.filter-count {
  font-size: 0.8rem;
  color: #6366f1;
  background: #e0e7ff;
  padding: 6px 12px;
  border-radius: 999px;
  font-weight: 500;
  white-space: nowrap;
}

.date-badge {
  font-size: 0.85rem;
  color: var(--text-secondary);
  white-space: nowrap;
}

.table-container {
  background: white;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th {
  background: #f8fafc;
  padding: 12px 16px;
  text-align: left;
  font-size: 0.75rem;
  text-transform: uppercase;
  color: #64748b;
  letter-spacing: 0.05em;
}

td {
  padding: 12px 16px;
  border-bottom: 1px solid #e2e8f0;
  vertical-align: middle;
  color: #1e293b;
}

.user-info {
  display: flex;
  flex-direction: column;
}

.user-name {
  font-weight: 600;
  color: #1e293b;
}

.user-name.clickable {
  cursor: pointer;
  color: var(--accent);
}
.user-name.clickable:hover {
  text-decoration: underline;
}

.btn-success-icon {
  color: #16a34a;
  border-color: #bbf7d0;
  background: #f0fdf4;
}
.btn-success-icon:hover {
  background: #dcfce7;
}

.btn-warning-icon {
  color: #d97706;
  border-color: #fde68a;
  background: #fffbeb;
}
.btn-warning-icon:hover {
  background: #fef3c7;
}

.user-email {
  font-size: 0.8rem;
  color: var(--text-secondary);
}

.workspace-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.ws-pill {
  font-size: 0.75rem;
  background: #e0e7ff;
  color: #4f46e5;
  border: 1px solid #c7d2fe;
  padding: 2px 10px;
  border-radius: 999px;
}

.no-ws {
  font-style: italic;
  font-size: 0.75rem;
  color: #94a3b8;
}

.role-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 999px;
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  background: #f1f5f9;
  color: #64748b;
}

.role-badge.admin {
  background: #d1fae5;
  color: #059669;
}

.org-badge {
  background: #e0e7ff;
  color: #4338ca;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 500;
}

.actions-cell {
  vertical-align: middle;
}

.actions-wrapper {
  display: flex;
  gap: 12px;
  justify-content: flex-start;
  align-items: center;
}

.btn-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  background: white;
  border: 1px solid #e2e8f0;
  color: #64748b;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  font-size: 1rem;
}

.btn-icon:hover {
  background: #f8fafc;
  color: #6366f1;
  border-color: #c7d2fe;
  transform: translateY(-1px);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

.btn-danger-icon:hover {
  color: #ef4444 !important;
  border-color: #fecaca !important;
  background: #fef2f2 !important;
}

.btn-icon:disabled {
  opacity: 0.3;
  cursor: not-allowed;
  transform: none;
  box-shadow: none;
}

.org-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #1e293b;
}

.id-code {
  font-family: monospace;
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.8rem;
  color: #64748b;
}

.loading-state, .error-state {
  padding: 100px 0;
  text-align: center;
  color: #64748b;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 16px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

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
  border-radius: 16px;
  border: 1px solid #e2e8f0;
  padding: 24px;
  width: 100%;
  max-width: 440px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.15);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.modal-header h3 {
  margin: 0;
  font-size: 1.2rem;
  color: #1e293b;
}

.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  font-size: 0.85rem;
  font-weight: 500;
  color: #64748b;
  margin-bottom: 8px;
}

.form-group input, .form-group select {
  width: 100%;
  padding: 10px 14px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  color: #1e293b;
  font-size: 0.95rem;
}

.form-group input:focus, .form-group select:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.checkbox-group label {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  color: #1e293b;
}

.modal-error {
  background: #fef2f2;
  color: #dc2626;
  padding: 12px;
  border-radius: 8px;
  font-size: 0.85rem;
  margin-bottom: 20px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

/* Login Logs Styles */
.user-name-small {
  display: block;
  font-size: 0.75rem;
  color: #94a3b8;
}

.ip-address {
  font-family: monospace;
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.85rem;
}

.log-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.error-text {
  color: #ef4444;
  font-size: 0.8rem;
  font-weight: 500;
}

.user-agent {
  color: #64748b;
  font-size: 0.75rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
}

</style>
