<template>
  <div class="profile-view">
    <header class="profile-header">
      <button class="btn-back" @click="$emit('back')">← Back</button>
      <h2>{{ entityType === 'user' ? '👤 User Profile' : '🏢 Organization Profile' }}</h2>
      <div v-if="canEdit" class="header-actions">
        <button v-if="!isEditing" class="btn btn-primary btn-edit" @click="startEditing">✏️ Edit Profile</button>
        <template v-else>
          <button class="btn btn-success" @click="saveProfile" :disabled="saving">
            {{ saving ? 'Saving...' : '💾 Save Changes' }}
          </button>
          <button class="btn btn-secondary" @click="cancelEditing" :disabled="saving">Cancel</button>
        </template>
      </div>
    </header>

    <div v-if="loading" class="loading-state">
      <span class="spinner"></span> Loading profile...
    </div>

    <div v-else-if="error" class="error-state">
      {{ error }}
    </div>

    <template v-else>
      <!-- USER PROFILE -->
      <div v-if="entityType === 'user' && userData" class="profile-content">
        <section class="profile-card identity-card">
          <h3>🆔 Identity</h3>
          <div class="info-grid">
            <div class="info-item" v-if="isAdmin">
              <label>ID</label>
              <span class="mono">{{ userData.id }}</span>
            </div>
            <div class="info-item">
              <label>Name</label>
              <input v-if="isEditing" v-model="editForm.name" class="profile-input" />
              <span v-else class="value-primary">{{ userData.name }}</span>
            </div>
            <div class="info-item">
              <label>Email</label>
              <input v-if="isEditing" v-model="editForm.email" class="profile-input" type="email" />
              <span v-else>{{ userData.email }}</span>
            </div>
            <div class="info-item">
              <label>Role</label>
              <span :class="userData.is_admin ? 'badge-admin' : 'badge-user'">
                {{ userData.is_admin ? '👑 Admin' : '👤 User' }}
              </span>
            </div>
            <div class="info-item">
              <label>Last Login</label>
              <span v-if="userData.last_login_at">{{ formatDate(userData.last_login_at) }}</span>
              <span v-else class="text-muted">Never</span>
            </div>
          </div>
        </section>

        <section v-if="userData.organizations?.length" class="profile-card org-card">
          <h3>🏢 Organizations ({{ userData.organizations.length }})</h3>
          <div class="table-container">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Joined</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="org in userData.organizations" :key="org.id">
                  <td class="value-primary" :class="{ clickable: isAdmin }" @click="isAdmin && viewOrg(org.id)">
                    {{ org.name }}
                  </td>
                  <td>
                    <span :class="org.is_manager ? 'badge-yes' : 'badge-no'">
                      {{ org.is_manager ? '✓ Manager' : '👤 Member' }}
                    </span>
                  </td>
                  <td>
                    <span :class="org.is_suspended ? 'badge-suspended' : 'badge-active'">
                      {{ org.is_suspended ? '⛔ Suspended' : '✓ Active' }}
                    </span>
                  </td>
                  <td>{{ formatDate(org.created_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="profile-card workspaces-card">
          <h3>📁 Workspaces ({{ userData.workspaces?.length || 0 }})</h3>
          <div v-if="userData.workspaces?.length" class="table-container">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Organization</th>
                  <th>Agents</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ws in userData.workspaces" :key="ws.id">
                  <td class="mono">{{ ws.id.slice(0, 8) }}...</td>
                  <td>{{ ws.name }}</td>
                  <td>
                    <span class="org-chip" v-if="ws.organization">{{ ws.organization.name }}</span>
                    <span v-else class="text-muted">-</span>
                  </td>
                  <td>{{ ws.agent_count || 0 }}</td>
                  <td>{{ formatDate(ws.created_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty-state">No workspaces found</p>
        </section>
        
        <!-- Password Change Section -->
        <section v-if="canChangePassword" class="profile-card password-card">
          <h3>🔐 Password Management</h3>
          <div v-if="!isChangingPassword" class="password-init">
            <p class="text-muted">Need to update your login credentials?</p>
            <button class="btn btn-secondary" @click="startChangingPassword">Change Password</button>
          </div>
          <form v-else @submit.prevent="submitPasswordChange" class="password-form">
            <div class="info-grid">
              <div v-if="!isAdmin" class="info-item">
                <label>Current Password</label>
                <input v-model="passForm.old" type="password" class="profile-input" required />
              </div>
              <div class="info-item">
                <label>New Password</label>
                <input v-model="passForm.new" type="password" class="profile-input" required />
                <div class="strength-meter">
                  <div class="strength-bar" :style="{ width: passwordStrength.percent + '%', background: passwordStrength.color }"></div>
                </div>
                <small :style="{ color: passwordStrength.color }">{{ passwordStrength.text }}</small>
              </div>
              <div class="info-item">
                <label>Confirm New Password</label>
                <input v-model="passForm.confirm" type="password" class="profile-input" required />
                <small v-if="passForm.confirm && passForm.new !== passForm.confirm" class="text-danger">Passwords do not match</small>
              </div>
            </div>
            
            <div class="password-rules">
              <p>Rules:</p>
              <ul>
                <li :class="{ met: passForm.new.length >= 8 }">At least 8 characters</li>
                <li :class="{ met: /[A-Z]/.test(passForm.new) }">At least one uppercase letter</li>
                <li :class="{ met: /[0-9!@#$%^&*]/.test(passForm.new) }">At least one number or special character</li>
              </ul>
            </div>

            <div class="form-actions mt-4">
              <button type="submit" class="btn btn-primary" :disabled="!isPasswordValid || saving">
                {{ saving ? 'Updating...' : 'Update Password' }}
              </button>
              <button type="button" class="btn btn-secondary" @click="isChangingPassword = false">Cancel</button>
            </div>
          </form>
        </section>

        <!-- Passkey Section -->
        <section v-if="entityType === 'user'" class="profile-card passkeys-card">
          <h3>🔑 Passkeys ({{ userData.passkeys?.length || 0 }})</h3>
          <div v-if="userData.passkeys?.length" class="table-container">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Created</th>
                  <th>Sign Count</th>
                  <th v-if="canEdit">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="pk in userData.passkeys" :key="pk.id">
                  <td class="mono font-xs">{{ pk.id }}</td>
                  <td>{{ formatDate(pk.created_at) }}</td>
                  <td>{{ pk.sign_count }}</td>
                  <td v-if="canEdit">
                    <button class="btn-text-danger" @click="deletePasskey(pk.id)" :disabled="registeringPasskey">
                      Delete
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty-state">No passkeys registered</p>
          <div class="mt-4" v-if="canEdit">
            <button class="btn btn-primary" @click="registerPasskey" :disabled="registeringPasskey || !isWebAuthnSupported">
              <span v-if="!isWebAuthnSupported">WebAuthn not supported</span>
              <span v-else>{{ registeringPasskey ? '⏳ Registering...' : '➕ Register New Passkey' }}</span>
            </button>
            <p v-if="!isWebAuthnSupported" class="text-xs text-muted mt-2">
              Passkeys require HTTPS or localhost to work.
            </p>
          </div>
        </section>
      </div>

      <!-- ORGANIZATION PROFILE -->
      <div v-if="entityType === 'organization' && orgData" class="profile-content">
        <section class="profile-card identity-card">
          <h3>🆔 Organization Identity</h3>
          <div class="info-grid">
            <div class="info-item">
              <label>ID</label>
              <span class="mono">{{ orgData.id }}</span>
            </div>
            <div class="info-item">
              <label>Name</label>
              <span class="value-primary">{{ orgData.name }}</span>
            </div>
            <div class="info-item">
              <label>Status</label>
              <span :class="orgData.is_suspended ? 'badge-suspended' : 'badge-active'">
                {{ orgData.is_suspended ? '⛔ Suspended' : '✓ Active' }}
              </span>
            </div>
            <div class="info-item">
              <label>Created At</label>
              <span>{{ formatDate(orgData.created_at) }}</span>
            </div>
          </div>
        </section>

        <section class="profile-card manager-card">
          <h3>👔 Manager</h3>
          <div v-if="orgData.manager" class="info-grid">
            <div class="info-item">
              <label>Manager ID</label>
              <span class="mono">{{ orgData.manager_id }}</span>
            </div>
            <div class="info-item">
              <label>Name</label>
              <span class="value-primary clickable" @click="viewUser(orgData.manager_id)">
                {{ orgData.manager.name }}
              </span>
            </div>
            <div class="info-item">
              <label>Email</label>
              <span>{{ orgData.manager.email }}</span>
            </div>
          </div>
          <p v-else class="empty-state">No manager assigned</p>
        </section>

        <section class="profile-card users-card">
          <h3>👥 Users ({{ orgData.users?.length || 0 }})</h3>
          <div v-if="orgData.users?.length" class="table-container">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Workspaces</th>
                  <th>Joined</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="user in orgData.users" :key="user.id" @click="viewUser(user.id)" class="clickable-row">
                  <td>
                    {{ user.name }}
                    <span v-if="user.id === orgData.manager_id" class="manager-badge">Manager</span>
                  </td>
                  <td>{{ user.email }}</td>
                  <td>
                    <span :class="user.is_admin ? 'badge-admin' : 'badge-user'">
                      {{ user.is_admin ? 'Admin' : 'User' }}
                    </span>
                  </td>
                  <td>{{ user.workspace_count || 0 }}</td>
                  <td>{{ formatDate(user.created_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty-state">No users in this organization</p>
        </section>

        <section class="profile-card workspaces-card">
          <h3>📁 Workspaces ({{ orgData.workspaces?.length || 0 }})</h3>
          <div v-if="orgData.workspaces?.length" class="table-container">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Owner</th>
                  <th>Agents</th>
                  <th>Runs</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ws in orgData.workspaces" :key="ws.id">
                  <td>{{ ws.name }}</td>
                  <td>{{ ws.user?.name || 'N/A' }}</td>
                  <td>{{ ws.agent_count || 0 }}</td>
                  <td>{{ ws.run_count || 0 }}</td>
                  <td>{{ formatDate(ws.created_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty-state">No workspaces found</p>
        </section>

        <section class="profile-card stats-card">
          <h3>📊 Statistics</h3>
          <div class="stats-grid">
            <div class="stat-box">
              <span class="stat-value">{{ orgData.users?.length || 0 }}</span>
              <span class="stat-label">Total Users</span>
            </div>
            <div class="stat-box">
              <span class="stat-value">{{ orgData.workspaces?.length || 0 }}</span>
              <span class="stat-label">Workspaces</span>
            </div>
            <div class="stat-box">
              <span class="stat-value">{{ totalAgents }}</span>
              <span class="stat-label">Agents</span>
            </div>
            <div class="stat-box">
              <span class="stat-value">{{ totalRuns }}</span>
              <span class="stat-label">Benchmark Runs</span>
            </div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<script>
import { wsService } from '../services/websocket.js'
import { webauthnService } from '../services/webauthn.js'

export default {
  name: 'AdminProfileView',
  props: {
    entityType: { type: String, required: true }, // 'user' or 'organization'
    entityId: { type: String, required: true }
  },
  emits: ['back', 'view-user', 'view-org', 'updated'],
  data() {
    return {
      loading: true,
      error: null,
      userData: null,
      orgData: null,
      isEditing: false,
      saving: false,
      editForm: {
        name: '',
        email: ''
      },
      isChangingPassword: false,
      passForm: {
        old: '',
        new: '',
        confirm: ''
      },
      registeringPasskey: false
    }
  },
  computed: {
    isAdmin() {
      const user = JSON.parse(localStorage.getItem('user') || '{}');
      return user.is_admin;
    },
    canEdit() {
      if (this.entityType !== 'user') return this.isAdmin;
      const user = JSON.parse(localStorage.getItem('user') || '{}');
      return user.is_admin || user.id === this.entityId;
    },
    canChangePassword() {
      if (this.entityType !== 'user') return false;
      const user = JSON.parse(localStorage.getItem('user') || '{}');
      // Self or SuperAdmin can change
      return user.id === this.entityId || user.is_admin;
    },
    isWebAuthnSupported() {
      return webauthnService.isSupported()
    },
    passwordStrength() {
      const p = this.passForm.new;
      if (!p) return { percent: 0, text: '', color: '#e2e8f0' };
      let score = 0;
      if (p.length >= 8) score += 34;
      if (/[A-Z]/.test(p)) score += 33;
      if (/[0-9!@#$%^&*]/.test(p)) score += 33;
      
      if (score < 60) return { percent: score, text: 'Weak', color: '#ef4444' };
      if (score < 100) return { percent: score, text: 'Medium', color: '#f59e0b' };
      return { percent: 100, text: 'Strong', color: '#22c55e' };
    },
    isPasswordValid() {
      const p = this.passForm.new;
      return p.length >= 8 && 
             /[A-Z]/.test(p) && 
             /[0-9!@#$%^&*]/.test(p) && 
             p === this.passForm.confirm;
    },
    totalAgents() {
      if (!this.userData?.workspaces) return 0
      return this.userData.workspaces.reduce((sum, ws) => sum + (ws.agent_count || 0), 0)
    },
    totalRuns() {
      // Not tracked per workspace generally, but if we had it
      return 0
    }
  },
  watch: {
    entityId: { immediate: true, handler: 'loadProfile' },
    entityType: { handler: 'loadProfile' }
  },
  methods: {
    async loadProfile() {
      this.loading = true
      this.error = null
      try {
        if (this.entityType === 'user') {
          this.userData = await wsService.adminGetUserProfile(this.entityId)
        } else {
          this.orgData = await wsService.adminGetOrgProfile(this.entityId)
        }
      } catch (e) {
        this.error = e.message || 'Failed to load profile'
      } finally {
        this.loading = false
      }
    },
    formatDate(dateStr) {
      if (!dateStr) return 'N/A'
      return new Date(dateStr).toLocaleDateString('en-US', {
        year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
      })
    },
    viewUser(userId) {
      this.$emit('view-user', userId)
    },
    viewOrg(orgId) {
      this.$emit('view-org', orgId)
    },
    startEditing() {
      this.editForm.name = this.userData.name
      this.editForm.email = this.userData.email
      this.isEditing = true
    },
    cancelEditing() {
      this.isEditing = false
    },
    async saveProfile() {
      this.saving = true
      try {
        const result = await wsService.adminUpdateUser({
          id: this.entityId,
          name: this.editForm.name,
          email: this.editForm.email
        })
        this.userData = { ...this.userData, ...result }
        this.isEditing = false
        
        // If updating self, update localStorage
        const localUser = JSON.parse(localStorage.getItem('user') || '{}')
        if (localUser.id === this.entityId) {
          localStorage.setItem('user', JSON.stringify({ ...localUser, name: result.name, email: result.email }))
          this.$emit('updated', result)
        }
        
        alert('Profile updated successfully')
      } catch (e) {
        alert('Failed to update profile: ' + e.message)
      } finally {
        this.saving = false
      }
    },
    startChangingPassword() {
      this.passForm = { old: '', new: '', confirm: '' };
      this.isChangingPassword = true;
    },
    async submitPasswordChange() {
      if (!this.isPasswordValid) return;
      this.saving = true;
      try {
        await wsService.changePassword(
          this.passForm.new,
          this.passForm.old,
          this.isAdmin && this.entityId !== JSON.parse(localStorage.getItem('user') || '{}').id ? this.entityId : ''
        );
        alert('Password updated successfully');
        this.isChangingPassword = false;
        this.passForm = { old: '', new: '', confirm: '' };
      } catch (e) {
        alert('Failed to update password: ' + e.message);
      } finally {
        this.saving = false;
      }
    },
    async registerPasskey() {
      this.registeringPasskey = true
      try {
        const options = await api.webAuthnRegisterBegin()
        const credential = await webauthnService.createCredential(options)
        await api.webAuthnRegisterFinish(credential)
        alert('Passkey registered successfully')
        await this.loadProfile() // Refresh
      } catch (e) {
        if (e.name === 'NotAllowedError') {
           // User cancelled
           return
        }
        alert('Failed to register passkey: ' + e.message)
      } finally {
        this.registeringPasskey = false
      }
    },
    async deletePasskey(keyId) {
      if (!confirm('Are you sure you want to delete this passkey?')) return
      try {
        await wsService.webAuthnDeleteKey(keyId)
        alert('Passkey deleted successfully')
        await this.loadProfile() // Refresh
      } catch (e) {
        alert('Failed to delete passkey: ' + e.message)
      }
    }
  }
}
</script>

<style scoped>
.profile-view {
  padding: 1.5rem 2rem;
  max-width: 1200px;
  margin: 0 auto;
  background: #f8fafc;
  min-height: 100%;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.profile-header h2 {
  margin: 0;
  font-size: 1.5rem;
  color: #1e293b;
}

.btn-back {
  background: white;
  border: 1px solid #e2e8f0;
  color: #64748b;
  padding: 0.5rem 1rem;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  font-size: 0.9rem;
}
.btn-back:hover {
  background: #f1f5f9;
  color: #1e293b;
  border-color: #cbd5e1;
}

.header-actions {
  margin-left: auto;
  display: flex;
  gap: 0.5rem;
}

.profile-input {
  padding: 0.5rem 0.75rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 0.95rem;
  width: 100%;
  max-width: 400px;
}

.profile-input:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.btn-success {
  background: #22c55e;
  color: white;
  border: none;
}
.btn-success:hover:not(:disabled) {
  background: #16a34a;
}
.btn-secondary {
  background: white;
  color: #64748b;
  border: 1px solid #e2e8f0;
}
.btn-secondary:hover:not(:disabled) {
  background: #f1f5f9;
}

.btn-text-danger {
  background: transparent;
  border: none;
  color: #ef4444;
  cursor: pointer;
  font-size: 0.85rem;
  padding: 0;
  text-decoration: underline;
}
.btn-text-danger:hover {
  color: #dc2626;
}

.profile-content {
  display: grid;
  gap: 1.5rem;
}

.profile-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 1.25rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
}

.profile-card h3 {
  margin: 0 0 1rem;
  font-size: 1rem;
  font-weight: 600;
  color: #475569;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 0.75rem;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1.25rem;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.info-item label {
  font-size: 0.7rem;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 500;
}

.info-item span {
  color: #1e293b;
  font-size: 0.95rem;
}

.mono {
  font-family: 'SF Mono', Monaco, 'Courier New', monospace;
  font-size: 0.8rem;
  background: #f1f5f9;
  padding: 0.35rem 0.6rem;
  border-radius: 6px;
  color: #475569;
  word-break: break-all;
}

.value-primary {
  font-weight: 600;
  font-size: 1.1rem;
  color: #6366f1 !important;
}

.clickable {
  cursor: pointer;
  text-decoration: underline;
  text-decoration-color: rgba(99, 102, 241, 0.3);
}
.clickable:hover {
  text-decoration-color: #6366f1;
}

.badge-admin, .badge-yes, .badge-active {
  background: linear-gradient(135deg, #22c55e, #16a34a);
  color: white !important;
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.8rem;
  display: inline-block;
  font-weight: 500;
}

.badge-user {
  background: #f1f5f9;
  color: #64748b;
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.8rem;
  border: 1px solid #e2e8f0;
}

.badge-no, .badge-suspended {
  background: linear-gradient(135deg, #ef4444, #dc2626);
  color: white;
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.8rem;
  display: inline-block;
  font-weight: 500;
}

.manager-badge {
  background: linear-gradient(135deg, #f59e0b, #d97706);
  color: white;
  padding: 0.15rem 0.5rem;
  border-radius: 6px;
  font-size: 0.65rem;
  margin-left: 0.5rem;
  font-weight: 600;
  text-transform: uppercase;
}

.table-container {
  overflow-x: auto;
  margin-top: 0.5rem;
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
  letter-spacing: 0.5px;
  background: #f8fafc;
}

td {
  color: #475569;
  font-size: 0.9rem;
}

.clickable-row {
  cursor: pointer;
  transition: background 0.15s;
}
.clickable-row:hover {
  background: #f1f5f9;
}

.empty-state {
  color: #94a3b8;
  text-align: center;
  padding: 1.5rem;
  font-style: italic;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 1rem;
}

.stat-box {
  background: linear-gradient(135deg, #f8fafc, #f1f5f9);
  padding: 1.25rem;
  border-radius: 10px;
  text-align: center;
  border: 1px solid #e2e8f0;
}

.stat-value {
  display: block;
  font-size: 2rem;
  font-weight: 700;
  color: #6366f1;
  line-height: 1;
}

.stat-label {
  display: block;
  margin-top: 0.5rem;
  font-size: 0.7rem;
  color: #94a3b8;
  text-transform: uppercase;
  font-weight: 500;
  letter-spacing: 0.5px;
}

.loading-state, .error-state {
  text-align: center;
  padding: 4rem 2rem;
  color: #64748b;
}

.spinner {
  display: inline-block;
  width: 24px;
  height: 24px;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: 0.75rem;
  vertical-align: middle;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-state {
  color: #dc2626;
  background: #fef2f2;
  border-radius: 12px;
  border: 1px solid #fecaca;
}

.password-form {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.strength-meter {
  height: 4px;
  background: #f1f5f9;
  border-radius: 2px;
  margin-top: 0.5rem;
  overflow: hidden;
}

.strength-bar {
  height: 100%;
  transition: all 0.3s ease;
}

.password-rules {
  background: #f8fafc;
  padding: 1rem;
  border-radius: 8px;
  font-size: 0.85rem;
}

.password-rules p {
  margin: 0 0 0.5rem;
  font-weight: 600;
  color: #64748b;
}

.password-rules ul {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.password-rules li {
  color: #94a3b8;
}

.password-rules li.met {
  color: #22c55e;
}

.password-rules li.met::before {
  content: '✓ ';
}

.password-rules li:not(.met)::before {
  content: '○ ';
}

.text-danger {
  color: #ef4444;
  font-size: 0.75rem;
}

.mt-4 {
  margin-top: 1rem;
}

.form-actions {
  display: flex;
  gap: 0.75rem;
}
</style>
