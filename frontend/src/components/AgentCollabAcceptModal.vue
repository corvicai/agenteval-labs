<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container collab-accept-modal">
      <div class="modal-header">
        <h3>Agent Share Invite</h3>
        <button class="btn-close" @click="$emit('close')">x</button>
      </div>

      <div class="modal-body">
        <div v-if="isLoading" class="status-text">Loading invite details...</div>
        <div v-else-if="loadError" class="error-message">{{ loadError }}</div>
        <template v-else-if="preview">
          <div v-if="preview.status !== 'ready'" class="invite-state-box">
            <template v-if="preview.status === 'used'">
              This agent share invite has already been accepted.
            </template>
            <template v-else-if="preview.status === 'expired'">
              This agent share invite has expired.
            </template>
            <template v-else-if="preview.status === 'already_owned'">
              You already own this agent.
            </template>
            <template v-else>
              This agent share invite is no longer valid.
            </template>
          </div>

          <template v-else>
            <div class="invite-description">
              <strong>{{ preview.shared_by_name || 'Another user' }}</strong>
              invited you to use the agent
              <strong>{{ preview.agent_name }}</strong>.
            </div>

            <div class="invite-summary">
              <span v-if="preview.provider_type" class="summary-badge">Type: {{ preview.provider_type }}</span>
              <span class="summary-badge role-badge">Role: {{ preview.role || 'user' }}</span>
            </div>

            <div class="invite-note">
              You will be able to run benchmarks with this agent in your own workspace.
              The owner's credentials stay encrypted server-side — you will never see the raw token.
              The owner can revoke access at any time.
            </div>

            <div v-if="acceptError" class="error-message">{{ acceptError }}</div>
          </template>
        </template>
      </div>

      <div class="modal-actions">
        <button class="btn btn-secondary" @click="$emit('close')" :disabled="accepting">
          {{ preview?.status !== 'ready' ? 'Close' : 'Cancel' }}
        </button>
        <button
          v-if="preview?.status === 'ready'"
          class="btn btn-primary"
          @click="acceptInvite"
          :disabled="accepting"
        >
          {{ accepting ? 'Accepting...' : 'Accept Invitation' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import wsService from '../services/websocket.js'
import { useWSStore } from '../stores/wsStore.js'

const props = defineProps({
  token: {
    type: String,
    required: true
  }
})

const emit = defineEmits(['close', 'accepted'])

const isLoading = ref(true)
const loadError = ref('')
const acceptError = ref('')
const accepting = ref(false)
const preview = ref(null)

const { syncState } = useWSStore()

async function loadPreview() {
  isLoading.value = true
  loadError.value = ''
  try {
    preview.value = await wsService.getAgentCollabInvite(props.token)
  } catch (err) {
    loadError.value = err?.message || 'Failed to load invitation details.'
  } finally {
    isLoading.value = false
  }
}

async function acceptInvite() {
  accepting.value = true
  acceptError.value = ''
  try {
    const result = await wsService.acceptAgentCollabInvite(props.token)
    await syncState()
    emit('accepted', result)
  } catch (err) {
    acceptError.value = err?.message || 'Failed to accept invitation.'
  } finally {
    accepting.value = false
  }
}

onMounted(() => {
  loadPreview()
})
</script>

<style scoped>
.collab-accept-modal {
  width: min(480px, calc(100vw - 32px));
}

.modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.status-text {
  color: #6b7280;
  font-size: 14px;
}

.invite-state-box {
  background: #fef3c7;
  border: 1px solid #fcd34d;
  border-radius: 10px;
  color: #92400e;
  padding: 12px 14px;
  font-size: 13px;
  line-height: 1.5;
}

.invite-description {
  font-size: 15px;
  line-height: 1.5;
  color: #111827;
}

.invite-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.summary-badge {
  background: #f3f4f6;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  color: #374151;
  font-weight: 500;
}

.role-badge {
  background: #dbeafe;
  border-color: #bfdbfe;
  color: #1d4ed8;
}

.invite-note {
  background: #f0f9ff;
  border: 1px solid #bae6fd;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 13px;
  color: #0c4a6e;
  line-height: 1.45;
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
  gap: 12px;
  justify-content: flex-end;
  padding: 1rem 1.5rem 1.5rem;
  border-top: 1px solid #f1f5f9;
}
</style>
