<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container collab-invite-modal">
      <div class="modal-header">
        <h3>Share Agent</h3>
        <button class="btn-close" @click="$emit('close')">x</button>
      </div>

      <div class="modal-body">
        <p class="modal-subtitle">
          Share <strong>{{ agentName }}</strong> with a teammate.
          They can run benchmarks using this agent, but the credentials (API key / token)
          stay encrypted server-side and never leave the backend.
        </p>

        <div class="invite-form">
          <div class="form-group">
            <label class="form-label">Email (optional)</label>
            <input
              v-model="inviteEmail"
              type="email"
              class="form-input"
              placeholder="teammate@example.com"
              :disabled="creating"
            />
          </div>

          <div v-if="inviteLink" class="invite-link-box">
            <label class="form-label">Invite Link</label>
            <div class="link-row">
              <input
                :value="inviteLink"
                readonly
                class="form-input link-input"
              />
              <button class="btn btn-secondary btn-sm" @click="copyLink">
                {{ copied ? 'Copied!' : 'Copy' }}
              </button>
            </div>
            <p class="invite-link-note">
              Link expires on {{ formatDate(inviteExpires) }}. Share it with your teammate.
            </p>
          </div>

          <div v-if="createError" class="error-message">{{ createError }}</div>

          <button
            class="btn btn-primary"
            @click="createInvite"
            :disabled="creating"
          >
            {{ creating ? 'Creating...' : 'Generate Invite Link' }}
          </button>
        </div>

        <div class="collaborators-section" v-if="collaborators !== null">
          <h4>Active Collaborators</h4>

          <div v-if="loadingCollaborators" class="loading-text">Loading...</div>
          <div v-else-if="collaborators.length === 0" class="empty-collaborators">
            No collaborators yet.
          </div>
          <ul v-else class="collaborators-list">
            <li v-for="collab in collaborators" :key="collab.user_id" class="collaborator-item">
              <div class="collab-info">
                <span class="collab-name">{{ collab.user_name }}</span>
                <span class="collab-email">{{ collab.email }}</span>
                <span class="collab-role-badge">{{ collab.role || 'user' }}</span>
              </div>
              <button
                class="btn btn-danger btn-xs"
                @click="revokeCollaborator(collab.user_id)"
                :disabled="revoking === collab.user_id"
              >
                {{ revoking === collab.user_id ? 'Revoking...' : 'Revoke' }}
              </button>
            </li>
          </ul>
          <div v-if="revokeError" class="error-message">{{ revokeError }}</div>
        </div>

        <button
          v-if="collaborators === null"
          class="btn btn-secondary btn-sm"
          @click="loadCollaborators"
        >
          View Active Collaborators
        </button>
      </div>

      <div class="modal-actions">
        <button class="btn btn-secondary" @click="$emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  agentId: {
    type: String,
    required: true
  },
  agentName: {
    type: String,
    default: 'this agent'
  }
})

const emit = defineEmits(['close'])

const inviteEmail = ref('')
const inviteLink = ref('')
const inviteExpires = ref(null)
const creating = ref(false)
const createError = ref('')
const copied = ref(false)

const collaborators = ref(null)
const loadingCollaborators = ref(false)
const revoking = ref(null)
const revokeError = ref('')

function formatDate(value) {
  if (!value) return ''
  const d = new Date(value)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleString()
}

async function createInvite() {
  creating.value = true
  createError.value = ''
  inviteLink.value = ''
  inviteExpires.value = null

  try {
    const result = await wsService.createAgentCollabInvite(
      props.agentId,
      inviteEmail.value || null,
      'user'
    )
    const token = result.token
    const baseUrl = window.location.origin + window.location.pathname
    inviteLink.value = `${baseUrl}?agent_collab_invite=${token}`
    inviteExpires.value = result.expires_at

    if (collaborators.value !== null) {
      await loadCollaborators()
    }
  } catch (err) {
    createError.value = err?.message || 'Failed to create invite.'
  } finally {
    creating.value = false
  }
}

async function copyLink() {
  if (!inviteLink.value) return
  try {
    await navigator.clipboard.writeText(inviteLink.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    // noop — user can still select the text manually
  }
}

async function loadCollaborators() {
  loadingCollaborators.value = true
  try {
    const result = await wsService.listAgentCollaborators(props.agentId)
    collaborators.value = result.collaborators || []
  } catch (err) {
    console.error('[ShareAgentModal] Failed to load collaborators:', err)
    collaborators.value = []
  } finally {
    loadingCollaborators.value = false
  }
}

async function revokeCollaborator(userId) {
  revoking.value = userId
  revokeError.value = ''
  try {
    await wsService.revokeAgentCollaborator(props.agentId, userId)
    await loadCollaborators()
  } catch (err) {
    revokeError.value = err?.message || 'Failed to revoke collaborator.'
  } finally {
    revoking.value = null
  }
}

onMounted(() => {
  loadCollaborators()
})
</script>

<style scoped>
.collab-invite-modal {
  width: min(560px, calc(100vw - 32px));
}

.modal-subtitle {
  color: #4b5563;
  font-size: 14px;
  margin-bottom: 16px;
}

.invite-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-label {
  font-size: 13px;
  font-weight: 600;
  color: #374151;
}

.form-input {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 14px;
  box-sizing: border-box;
}

.invite-link-box {
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 10px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.link-row {
  display: flex;
  gap: 8px;
}

.link-input {
  flex: 1;
  background: #fff;
  font-size: 12px;
  font-family: monospace;
}

.invite-link-note {
  font-size: 12px;
  color: #15803d;
  margin: 0;
}

.collaborators-section {
  border-top: 1px solid #e5e7eb;
  padding-top: 16px;
  margin-top: 4px;
}

.collaborators-section h4 {
  font-size: 14px;
  font-weight: 600;
  color: #111827;
  margin: 0 0 12px;
}

.loading-text {
  color: #6b7280;
  font-size: 13px;
}

.empty-collaborators {
  color: #9ca3af;
  font-size: 13px;
  font-style: italic;
}

.collaborators-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.collaborator-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.collab-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  flex-wrap: wrap;
}

.collab-name {
  font-weight: 600;
  font-size: 13px;
  color: #111827;
}

.collab-email {
  font-size: 12px;
  color: #6b7280;
}

.collab-role-badge {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  background: #dbeafe;
  color: #1d4ed8;
  font-weight: 500;
}

.error-message {
  color: #dc2626;
  font-size: 13px;
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 8px;
  padding: 8px 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  padding: 1rem 1.5rem 1.5rem;
  border-top: 1px solid #f1f5f9;
}

.btn-xs {
  padding: 4px 8px;
  font-size: 12px;
}
</style>
