<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container share-accept-modal">
      <div class="modal-header">
        <h3>Accept Shared Question Set</h3>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <div class="modal-body">
        <div v-if="isLoading" class="share-status">Loading share link...</div>
        <div v-else-if="loadError" class="error-message">{{ loadError }}</div>
        <template v-else-if="preview">
          <p class="share-description">
            <strong>{{ preview.question_set_name }}</strong>
            was shared by
            <strong>{{ preview.shared_by_name || 'another user' }}</strong>.
          </p>

          <div class="share-summary">
            <span>{{ preview.question_count || 0 }} questions</span>
            <span>{{ preview.category_count || 0 }} categories</span>
            <span>Expires on {{ formatDateTime(preview.expires_at) }}</span>
          </div>

          <div v-if="preview.status !== 'ready'" class="share-state-box">
            <template v-if="preview.status === 'used'">
              This link has already been used.
            </template>
            <template v-else-if="preview.status === 'expired'">
              This link has expired.
            </template>
            <template v-else>
              This share link is no longer valid.
            </template>
          </div>

          <template v-else>
            <label class="form-label" for="accept-share-workspace">Import into workspace</label>
            <select
              id="accept-share-workspace"
              v-model="targetWorkspaceId"
              class="share-workspace-select"
              :disabled="acceptLoading"
            >
              <option disabled value="">Select a workspace</option>
              <option v-for="workspace in workspaces" :key="workspace.id" :value="workspace.id">
                {{ workspace.name }}
              </option>
            </select>

            <p class="share-note">
              The imported copy contains only the question set metadata and questions. Agent bindings and run history are not included.
            </p>

            <div v-if="acceptError" class="error-message">{{ acceptError }}</div>
          </template>
        </template>
      </div>

      <div class="modal-actions">
        <button class="btn btn-secondary" @click="$emit('close')" :disabled="acceptLoading">Close</button>
        <button
          v-if="preview?.status === 'ready'"
          class="btn btn-primary"
          @click="acceptShare"
          :disabled="acceptLoading || !targetWorkspaceId"
        >
          {{ acceptLoading ? 'Importing...' : 'Import Question Set' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  token: {
    type: String,
    required: true
  },
  workspaces: {
    type: Array,
    default: () => []
  },
  currentWorkspaceId: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['close', 'accepted'])

const isLoading = ref(true)
const loadError = ref('')
const acceptError = ref('')
const acceptLoading = ref(false)
const preview = ref(null)
const targetWorkspaceId = ref('')

if (props.currentWorkspaceId) {
  targetWorkspaceId.value = props.currentWorkspaceId
} else if (props.workspaces?.length) {
  targetWorkspaceId.value = props.workspaces[0].id
}

function formatDateTime(value) {
  if (!value) return 'Unknown'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return 'Unknown'
  return parsed.toLocaleString()
}

async function loadPreview() {
  isLoading.value = true
  loadError.value = ''
  try {
    preview.value = await wsService.getQuestionSetShareLink(props.token)
  } catch (err) {
    loadError.value = err?.message || 'Failed to load share link.'
  } finally {
    isLoading.value = false
  }
}

async function acceptShare() {
  if (!targetWorkspaceId.value) return
  acceptLoading.value = true
  acceptError.value = ''
  try {
    const result = await wsService.acceptQuestionSetShareLink(props.token, targetWorkspaceId.value)
    emit('accepted', result)
  } catch (err) {
    acceptError.value = err?.message || 'Failed to import question set.'
  } finally {
    acceptLoading.value = false
  }
}

onMounted(() => {
  loadPreview()
})
</script>

<style scoped>
.share-accept-modal {
  width: min(560px, calc(100vw - 32px));
}

.modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.share-status,
.share-description,
.share-note {
  margin: 0;
}

.share-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  color: #4b5563;
  font-size: 13px;
}

.share-state-box,
.share-note {
  background: #f3f4f6;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  color: #374151;
  font-size: 13px;
  line-height: 1.45;
  padding: 10px 12px;
}

.form-label {
  display: block;
  font-weight: 600;
  color: #111827;
  margin-bottom: 6px;
}

.share-workspace-select {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 14px;
}
</style>
