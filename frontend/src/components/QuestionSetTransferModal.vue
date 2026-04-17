<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container transfer-modal">
      <div class="modal-header">
        <h3>Share "{{ questionSet?.name || 'Question Set' }}"</h3>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <!-- Tabs -->
      <div class="share-tabs">
        <button
          class="share-tab"
          :class="{ active: activeTab === 'copy' }"
          @click="activeTab = 'copy'"
        >
          📄 Share a Copy
        </button>
        <button
          class="share-tab"
          :class="{ active: activeTab === 'collab' }"
          @click="activeTab = 'collab'"
        >
          👥 Collaborate
        </button>
      </div>

      <!-- Tab: Share copy -->
      <div v-if="activeTab === 'copy'" class="modal-body">
        <p class="tab-lead">
          Create a one-time link. The recipient gets an independent copy — questions, version, and name only.
        </p>

        <div v-if="copyError" class="error-message">{{ copyError }}</div>

        <div class="transfer-note">
          The link is single-use. The recipient must log in and open the link to import the question set.
        </div>

        <div v-if="shareData" class="share-result">
          <label class="form-label">Share Link</label>
          <div class="share-link-row">
            <input :value="shareData.url" type="text" readonly class="share-link-input" />
            <button class="btn btn-secondary" @click="copyShareLink" :disabled="copyLinkCopied">
              {{ copyLinkCopied ? 'Copied' : 'Copy' }}
            </button>
          </div>
          <p class="share-meta">
            Expires on {{ formatDateTime(shareData.expires_at) }}.
          </p>
        </div>

        <div class="modal-actions">
          <button class="btn btn-secondary" @click="$emit('close')" :disabled="isGenerating">Cancel</button>
          <button class="btn btn-primary" @click="generateShareLink" :disabled="isGenerating">
            {{ isGenerating ? 'Generating...' : shareData ? 'Generate New Link' : 'Generate Link' }}
          </button>
        </div>
      </div>

      <!-- Tab: Collaborate -->
      <div v-if="activeTab === 'collab'" class="modal-body">
        <p class="tab-lead">
          Invite someone to work on this question set together — they can run benchmarks and evaluate results in real time.
        </p>

        <!-- Invite form -->
        <div class="invite-form">
          <div class="form-row">
            <div class="form-group form-group-grow">
              <label class="form-label">Email (optional)</label>
              <input
                v-model="inviteEmail"
                type="email"
                class="form-input"
                placeholder="collaborator@example.com"
                :disabled="creating"
              />
            </div>
            <div class="form-group form-group-role">
              <label class="form-label">Role</label>
              <select v-model="inviteRole" class="form-select" :disabled="creating">
                <option value="editor">Editor</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>
          </div>

          <div v-if="inviteLink" class="invite-link-box">
            <label class="form-label">Invite Link</label>
            <div class="share-link-row">
              <input :value="inviteLink" readonly class="share-link-input link-mono" />
              <button class="btn btn-secondary" @click="copyInviteLink">
                {{ inviteLinkCopied ? 'Copied!' : 'Copy' }}
              </button>
            </div>
            <p class="share-meta">Expires on {{ formatDateTime(inviteExpires) }}. Share this link with your collaborator.</p>
          </div>

          <div v-if="createError" class="error-message">{{ createError }}</div>

          <button class="btn btn-primary" @click="createInvite" :disabled="creating">
            {{ creating ? 'Creating...' : 'Generate Invite Link' }}
          </button>
        </div>

        <!-- Collaborators list -->
        <div class="collaborators-section">
          <h4>Active Collaborators</h4>
          <div v-if="loadingCollaborators" class="muted-text">Loading...</div>
          <div v-else-if="collaborators.length === 0" class="muted-text">No collaborators yet.</div>
          <ul v-else class="collaborators-list">
            <li v-for="collab in collaborators" :key="collab.user_id" class="collaborator-item">
              <div class="collab-info">
                <span class="collab-name">{{ collab.user_name }}</span>
                <span class="collab-email">{{ collab.email }}</span>
                <span class="collab-role-badge" :class="collab.role">{{ collab.role }}</span>
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

        <div class="modal-actions">
          <button class="btn btn-secondary" @click="$emit('close')">Close</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  questionSet: {
    type: Object,
    required: true
  }
})

defineEmits(['close'])

// --- Tab state ---
const activeTab = ref('copy')

// --- Share copy tab ---
const isGenerating = ref(false)
const copyError = ref('')
const copyLinkCopied = ref(false)
const shareData = ref(null)

function formatDateTime(value) {
  if (!value) return 'Unknown'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return 'Unknown'
  return parsed.toLocaleString()
}

async function copyShareLink() {
  if (!shareData.value?.url) return
  copyError.value = ''
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(shareData.value.url)
    } else {
      const input = document.createElement('input')
      input.value = shareData.value.url
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
    }
    copyLinkCopied.value = true
    setTimeout(() => { copyLinkCopied.value = false }, 2000)
  } catch (err) {
    copyError.value = err?.message || 'Failed to copy link.'
  }
}

async function generateShareLink() {
  if (!props.questionSet?.id) return
  isGenerating.value = true
  copyError.value = ''
  copyLinkCopied.value = false
  try {
    const response = await wsService.createQuestionSetShareLink(props.questionSet.id)
    const url = new URL(window.location.origin + window.location.pathname)
    url.searchParams.set('share', response.token)
    shareData.value = { ...response, url: url.toString() }
  } catch (err) {
    copyError.value = err?.message || 'Failed to create share link.'
  } finally {
    isGenerating.value = false
  }
}

// --- Collaborate tab ---
const inviteEmail = ref('')
const inviteRole = ref('editor')
const inviteLink = ref('')
const inviteExpires = ref(null)
const inviteLinkCopied = ref(false)
const creating = ref(false)
const createError = ref('')

const collaborators = ref([])
const loadingCollaborators = ref(false)
const revoking = ref(null)
const revokeError = ref('')

async function createInvite() {
  if (!props.questionSet?.id) return
  creating.value = true
  createError.value = ''
  inviteLink.value = ''
  inviteExpires.value = null
  try {
    const result = await wsService.createCollabInvite(
      props.questionSet.id,
      inviteEmail.value || null,
      inviteRole.value
    )
    const baseUrl = window.location.origin + window.location.pathname
    inviteLink.value = `${baseUrl}?collab_invite=${result.token}`
    inviteExpires.value = result.expires_at
    await loadCollaborators()
  } catch (err) {
    createError.value = err?.message || 'Failed to create invite.'
  } finally {
    creating.value = false
  }
}

async function copyInviteLink() {
  if (!inviteLink.value) return
  try {
    await navigator.clipboard.writeText(inviteLink.value)
    inviteLinkCopied.value = true
    setTimeout(() => { inviteLinkCopied.value = false }, 2000)
  } catch {
    // fallback: user can copy manually from the input
  }
}

async function loadCollaborators() {
  if (!props.questionSet?.id) return
  loadingCollaborators.value = true
  try {
    const result = await wsService.listCollaborators(props.questionSet.id)
    collaborators.value = result.collaborators || []
  } catch {
    collaborators.value = []
  } finally {
    loadingCollaborators.value = false
  }
}

async function revokeCollaborator(userId) {
  revoking.value = userId
  revokeError.value = ''
  try {
    await wsService.revokeCollaborator(props.questionSet.id, userId)
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
.transfer-modal {
  width: min(580px, calc(100vw - 32px));
}

/* Tabs */
.share-tabs {
  display: flex;
  border-bottom: 1px solid #e5e7eb;
  background: #f9fafb;
}

.share-tab {
  flex: 1;
  padding: 10px 16px;
  font-size: 13px;
  font-weight: 500;
  color: #6b7280;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}

.share-tab:hover {
  color: #374151;
}

.share-tab.active {
  color: #2563eb;
  border-bottom-color: #2563eb;
  background: #fff;
}

/* Body */
.modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px 24px 0;
}

.tab-lead {
  margin: 0;
  color: #4b5563;
  font-size: 13.5px;
  line-height: 1.5;
}

.form-label {
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: #374151;
  margin-bottom: 4px;
}

.form-input,
.form-select {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 14px;
  box-sizing: border-box;
}

.transfer-note {
  background: #f3f4f6;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  color: #374151;
  font-size: 13px;
  line-height: 1.45;
  padding: 10px 12px;
}

.share-result,
.invite-link-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.invite-link-box {
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 10px;
  padding: 12px;
}

.share-link-row {
  display: flex;
  gap: 10px;
}

.share-link-input {
  flex: 1;
  min-width: 0;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  background: #f9fafb;
}

.link-mono {
  font-family: monospace;
  font-size: 12px;
  background: #fff;
}

.share-meta {
  margin: 0;
  color: #6b7280;
  font-size: 12px;
}

.invite-link-box .share-meta {
  color: #15803d;
}

/* Invite form */
.invite-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-row {
  display: flex;
  gap: 10px;
  align-items: flex-end;
}

.form-group {
  display: flex;
  flex-direction: column;
}

.form-group-grow {
  flex: 1;
}

.form-group-role {
  width: 130px;
  flex-shrink: 0;
}

/* Collaborators */
.collaborators-section {
  border-top: 1px solid #e5e7eb;
  padding-top: 14px;
}

.collaborators-section h4 {
  font-size: 13px;
  font-weight: 600;
  color: #374151;
  margin: 0 0 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.muted-text {
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
  gap: 6px;
}

.collaborator-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
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

.collab-role-badge.viewer {
  background: #f3f4f6;
  color: #374151;
}

.error-message {
  color: #dc2626;
  font-size: 13px;
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 8px;
  padding: 8px 12px;
}

/* Actions */
.modal-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  align-items: center;
  padding: 16px 0 20px;
  margin-top: 4px;
  border-top: 1px solid #f1f5f9;
}

.btn-xs {
  padding: 4px 8px;
  font-size: 12px;
}

@media (max-width: 600px) {
  .form-row {
    flex-direction: column;
  }
  .form-group-role {
    width: 100%;
  }
  .share-link-row {
    flex-direction: column;
  }
  .modal-actions {
    flex-direction: column-reverse;
    align-items: stretch;
  }
}
</style>
