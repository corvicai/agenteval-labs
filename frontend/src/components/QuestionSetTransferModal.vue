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

        <div class="transfer-note">
          The link is single-use. The recipient must log in and open the link to import the question set.
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
      </div>

      <div class="modal-actions">
        <button class="btn btn-secondary" @click="$emit('close')" :disabled="isWorking">Cancel</button>
        <button
          class="btn btn-primary"
          @click="handlePrimaryAction"
          :disabled="isWorking"
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
  questionSet: {
    type: Object,
    required: true
  }
})

defineEmits(['close'])

const isWorking = ref(false)
const error = ref('')
const copied = ref(false)
const shareData = ref(null)

const modalTitle = computed(() => 'Share Question Set')

const modalDescription = computed(() => {
  return `Create a one-time link for "${props.questionSet?.name || 'this question set'}".`
})

const primaryLabel = computed(() => {
  if (isWorking.value) return 'Generating...'
  return shareData.value ? 'Generate New Link' : 'Generate Link'
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
    const response = await wsService.createQuestionSetShareLink(props.questionSet.id)
    const url = new URL(window.location.origin + window.location.pathname)
    url.searchParams.set('share', response.token)
    shareData.value = {
      ...response,
      url: url.toString()
    }
  } catch (err) {
    error.value = err?.message || 'Failed to create share link.'
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

.share-link-input {
  width: 100%;
  border: 1px solid #d1d5db;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 14px;
  background: #f9fafb;
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

.share-result {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.share-link-row {
  display: flex;
  gap: 10px;
}

.share-meta {
  margin: 0;
  color: #6b7280;
  font-size: 12px;
}

.modal-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  align-items: center;
  padding: 1rem 1.5rem 1.5rem;
  border-top: 1px solid #f1f5f9;
  background: #fff;
}

@media (max-width: 640px) {
  .share-link-row {
    flex-direction: column;
  }

  .modal-actions {
    flex-direction: column-reverse;
    align-items: stretch;
  }
}
</style>
