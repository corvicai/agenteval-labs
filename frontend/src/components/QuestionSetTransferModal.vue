<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container transfer-modal">
      <div class="modal-header">
        <h3>{{ modalTitle }}</h3>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <div class="modal-body">
        <p class="transfer-lead">{{ modalDescription }}</p>

        <div v-if="error" class="error-message">{{ error }}</div>

        <template v-if="mode === 'share'">
          <div class="transfer-note">
            The link is single-use. The recipient must log in, open the link, and choose one of their workspaces.
          </div>

          <div v-if="shareData" class="share-result">
            <label class="form-label">Share Link</label>
            <div class="share-link-row">
              <input :value="shareData.url" type="text" readonly class="share-link-input" />
              <button class="btn btn-secondary" @click="copyShareLink" :disabled="copied">
                {{ copied ? 'Copied' : 'Copy' }}
              </button>
            </div>
            <p class="share-meta">
              Expires on {{ formatDateTime(shareData.expires_at) }}. The imported copy contains only questions, version and name.
            </p>
          </div>
        </template>

        <template v-else>
          <div v-if="targetWorkspaces.length === 0" class="transfer-note">
            No other workspaces available. Create another workspace first to use this action.
          </div>

          <template v-else>
            <label class="form-label" for="transfer-target-workspace">Destination workspace</label>
            <select
              id="transfer-target-workspace"
              v-model="targetWorkspaceId"
              class="transfer-select"
            >
              <option disabled value="">Select a workspace</option>
              <option v-for="workspace in targetWorkspaces" :key="workspace.id" :value="workspace.id">
                {{ workspace.name }}
              </option>
            </select>

            <div v-if="mode === 'copy'" class="transfer-note">
              A new question set will be created in the destination workspace. Agents and run history are not copied.
            </div>

            <div v-if="mode === 'move'" class="transfer-note transfer-note-warning">
              The question set will leave the current workspace and move to the selected workspace. Question-set agent bindings are cleared, and past runs follow the move so history keeps working.
            </div>
          </template>
        </template>
      </div>

      <div class="modal-actions">
        <button class="btn btn-secondary" @click="$emit('close')" :disabled="isWorking">Cancel</button>
        <button
          class="btn btn-primary"
          @click="handlePrimaryAction"
          :disabled="primaryDisabled"
        >
          {{ primaryLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  mode: {
    type: String,
    required: true
  },
  questionSet: {
    type: Object,
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

const emit = defineEmits(['close', 'completed'])

const isWorking = ref(false)
const error = ref('')
const copied = ref(false)
const shareData = ref(null)
const targetWorkspaceId = ref('')

const targetWorkspaces = computed(() =>
  (props.workspaces || []).filter((workspace) => String(workspace?.id || '') !== String(props.currentWorkspaceId || ''))
)

if (targetWorkspaces.value.length > 0) {
  targetWorkspaceId.value = targetWorkspaces.value[0].id
}

const modalTitle = computed(() => {
  if (props.mode === 'share') return 'Share Question Set'
  if (props.mode === 'copy') return 'Copy To Workspace'
  return 'Move To Workspace'
})

const modalDescription = computed(() => {
  if (props.mode === 'share') {
    return `Create a one-time link for "${props.questionSet?.name || 'this question set'}".`
  }
  if (props.mode === 'copy') {
    return `Copy "${props.questionSet?.name || 'this question set'}" into another workspace.`
  }
  return `Move "${props.questionSet?.name || 'this question set'}" into another workspace.`
})

const primaryLabel = computed(() => {
  if (isWorking.value) return props.mode === 'share' ? 'Generating...' : 'Applying...'
  if (props.mode === 'share') return shareData.value ? 'Generate New Link' : 'Generate Link'
  if (props.mode === 'copy') return 'Copy Set'
  return 'Move Set'
})

const primaryDisabled = computed(() => {
  if (isWorking.value) return true
  if (props.mode === 'share') return false
  if (targetWorkspaces.value.length === 0) return true
  return !targetWorkspaceId.value
})

function formatDateTime(value) {
  if (!value) return 'Unknown'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return 'Unknown'
  return parsed.toLocaleString()
}

async function copyShareLink() {
  if (!shareData.value?.url) return
  error.value = ''
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
    copied.value = true
  } catch (err) {
    error.value = err?.message || 'Failed to copy share link.'
  }
}

async function handlePrimaryAction() {
  if (!props.questionSet?.id) return

  isWorking.value = true
  error.value = ''
  copied.value = false

  try {
    if (props.mode === 'share') {
      const response = await wsService.createQuestionSetShareLink(props.questionSet.id)
      const url = new URL(window.location.origin + window.location.pathname)
      url.searchParams.set('share', response.token)
      shareData.value = {
        ...response,
        url: url.toString()
      }
      return
    }

    const result = props.mode === 'copy'
      ? await wsService.copyQuestionSetToWorkspace(props.questionSet.id, targetWorkspaceId.value)
      : await wsService.moveQuestionSetToWorkspace(props.questionSet.id, targetWorkspaceId.value)

    emit('completed', {
      mode: props.mode,
      ...result
    })
  } catch (err) {
    error.value = err?.message || `Failed to ${props.mode} question set.`
  } finally {
    isWorking.value = false
  }
}
</script>

<style scoped>
.transfer-modal {
  width: min(560px, calc(100vw - 32px));
}

.modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.transfer-lead {
  margin: 0;
  color: #374151;
}

.form-label {
  display: block;
  font-weight: 600;
  color: #111827;
  margin-bottom: 6px;
}

.transfer-select,
.share-link-input {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 14px;
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

.transfer-note-warning {
  background: #fff7ed;
  border-color: #fdba74;
  color: #9a3412;
}

.share-result {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.share-link-row {
  display: flex;
  gap: 10px;
}

.share-link-input {
  background: #f9fafb;
}

.share-meta {
  margin: 0;
  color: #6b7280;
  font-size: 12px;
}

@media (max-width: 640px) {
  .share-link-row {
    flex-direction: column;
  }
}
</style>
