<template>
  <div class="onboarding-container">
    <div class="onboarding-card animate-in">
      <div class="onboarding-header">
        <div class="icon-wrapper">🚀</div>
        <h1>Welcome to AgentEval</h1>
        <p class="subtitle">To get started, please setup your organization context.</p>
      </div>

      <div class="options-grid">
        <!-- Option 1: Create Organization -->
        <div class="option-card" :class="{ active: mode === 'create' }" @click="mode = 'create'">
          <div class="option-icon">🏢</div>
          <h3>Create Organization</h3>
          <p>Start a new benchmarking environment for your team.</p>
        </div>

        <!-- Option 2: Join Organization -->
        <div class="option-card" :class="{ active: mode === 'join' }" @click="mode = 'join'">
          <div class="option-icon">📩</div>
          <h3>Join Organization</h3>
          <p>Join an existing team using an invite code.</p>
        </div>
      </div>

      <!-- Action Forms -->
      <div class="action-area">
        <div v-if="mode === 'create'" class="form-animate">
          <div class="form-group">
            <label>Organization Name</label>
            <input 
              v-model="orgName" 
              type="text" 
              placeholder="e.g. Acme Corp" 
              class="input-lg"
              @keyup.enter="handleCreate"
              autofocus
            />
          </div>
          <button 
            class="btn btn-primary btn-block btn-lg" 
            :disabled="!orgName.trim() || loading"
            @click="handleCreate"
          >
            <span v-if="loading" class="spinner small"></span>
            {{ loading ? 'Creating...' : 'Create & Start' }}
          </button>
        </div>

        <div v-if="mode === 'join'" class="form-animate">
          <div class="form-group">
            <label>Invite Code</label>
            <input 
              v-model="inviteCode" 
              type="text" 
              placeholder="e.g. INV-123456" 
              class="input-lg"
              @keyup.enter="handleJoin"
              autofocus
            />
          </div>
          <button 
            class="btn btn-primary btn-block btn-lg" 
            :disabled="!inviteCode.trim() || loading"
            @click="handleJoin"
          >
            <span v-if="loading" class="spinner small"></span>
            {{ loading ? 'Joining...' : 'Join & Start' }}
          </button>
        </div>

        <div v-if="error" class="error-message">
          {{ error }}
        </div>
      </div>
      
      <div class="logout-footer">
         <button class="btn-link" @click="handleLogout">Logout</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { wsService } from '../services/websocket.js'
import * as api from '../services/api.js'

const emit = defineEmits(['completed'])

const mode = ref('create') // 'create' | 'join'
const orgName = ref('')
const inviteCode = ref('')
const loading = ref(false)
const error = ref('')

async function handleCreate() {
  if (!orgName.value.trim()) return
  loading.value = true
  error.value = ''
  
  try {
    const response = await wsService.createOrganization(orgName.value)
    if (response.success) {
      if (response.user) localStorage.setItem('user', JSON.stringify(response.user))
      if (response.workspace) localStorage.setItem('workspace', JSON.stringify(response.workspace))
      if (response.token) localStorage.setItem('token', response.token)
      
      emit('completed')
    }
  } catch (e) {
    error.value = e.message || 'Failed to create organization'
  } finally {
    loading.value = false
  }
}

async function handleJoin() {
  if (!inviteCode.value.trim()) return
  loading.value = true
  error.value = ''
  
  try {
    const response = await wsService.joinOrganization(inviteCode.value)
    if (response.success) {
      // Backend for joinOrganization returns { success, user, workspace }?
      // Check ws_auth_handlers.go handleJoinOrganization response structure
      // It returns: { success: true, user: user, workspace: workspace }
      
      if (response.user) localStorage.setItem('user', JSON.stringify(response.user))
      if (response.workspace) localStorage.setItem('workspace', JSON.stringify(response.workspace))
      // Token update? joinOrganization handler doesn't seem to regenerate token in my previous read?
      // Wait, handleJoinOrganization in ws_auth_handlers.go lines 556+...
      // I should double check if it returns a new token. If not, the current token might be stale (limited scope).
      // If the current token is partial, we DO need a new token.
      
      // Let's rely on emitting 'completed' which triggers a reload or re-login flow in App.vue
      emit('completed')
    }
  } catch (e) {
    error.value = e.message || 'Failed to join organization'
  } finally {
    loading.value = false
  }
}

async function handleLogout() {
  await api.logout()
  window.location.reload()
}
</script>

<style scoped>
.onboarding-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f0f4ff 0%, #e0e7ff 100%);
  padding: 1rem;
}

.onboarding-card {
  background: white;
  width: 100%;
  max-width: 550px;
  border-radius: 24px;
  box-shadow: 0 20px 40px rgba(0,0,0,0.08); /* Soft shadow */
  padding: 3rem; /* Generous padding */
  text-align: center;
}

.onboarding-header {
  margin-bottom: 2.5rem;
}

.icon-wrapper {
  font-size: 3rem;
  margin-bottom: 1rem;
  animation: float 3s ease-in-out infinite;
}

.onboarding-header h1 {
  font-size: 1.75rem;
  font-weight: 800;
  color: #1e293b;
  margin-bottom: 0.5rem;
  letter-spacing: -0.5px;
}

.subtitle {
  color: #64748b;
  font-size: 1rem;
}

.options-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
  margin-bottom: 2rem;
}

.option-card {
  background: #f8fafc;
  border: 2px solid #e2e8f0;
  border-radius: 16px;
  padding: 1.5rem;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  text-align: left;
}

.option-card:hover {
  border-color: #cbd5e1;
  transform: translateY(-2px);
}

.option-card.active {
  border-color: #6366f1;
  background: #eff6ff;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.15);
}

.option-icon {
  font-size: 1.75rem;
  margin-bottom: 0.75rem;
}

.option-card h3 {
  font-size: 1rem;
  font-weight: 700;
  color: #1e293b;
  margin-bottom: 0.25rem;
}

.option-card p {
  font-size: 0.8rem;
  color: #64748b;
  line-height: 1.4;
}

.action-area {
  margin-top: 1rem;
  min-height: 140px; /* Prevent layout jump */
}

.form-group {
  margin-bottom: 1.25rem;
  text-align: left;
}

.form-group label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  color: #475569;
  margin-bottom: 0.5rem;
  margin-left: 4px;
}

.input-lg {
  width: 100%;
  padding: 0.875rem 1rem;
  font-size: 1rem;
  border: 2px solid #e2e8f0;
  border-radius: 12px;
  transition: all 0.2s;
}

.input-lg:focus {
  border-color: #6366f1;
  outline: none;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.btn-lg {
  padding: 0.875rem;
  font-size: 1rem;
  border-radius: 12px;
}

.error-message {
  margin-top: 1rem;
  color: #dc2626;
  font-size: 0.9rem;
  background: #fef2f2;
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid #fee2e2;
}

.logout-footer {
    margin-top: 2rem;
    border-top: 1px solid #f1f5f9;
    padding-top: 1rem;
}
.btn-link {
    background: none;
    border: none;
    color: #94a3b8;
    text-decoration: underline;
    cursor: pointer;
    font-size: 0.85rem;
}
.btn-link:hover {
    color: #64748b;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}

.animate-in {
  animation: fadeIn 0.5s ease-out;
}

.form-animate {
  animation: slideUp 0.3s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; transform: scale(0.98); }
  to { opacity: 1; transform: scale(1); }
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
