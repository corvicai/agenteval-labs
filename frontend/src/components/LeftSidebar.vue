<template>
  <div class="left-sidebar">
    <div class="sidebar-tabs">
      <button 
        class="tab-btn" 
        :class="{ active: activeTab === 'questionSets' }"
        @click="activeTab = 'questionSets'"
      >
        📋 Question Sets
      </button>
      <button 
        class="tab-btn" 
        :class="{ active: activeTab === 'agents' }"
        @click="activeTab = 'agents'"
      >
        🤖 Agents
      </button>
    </div>

    <div class="sidebar-content">
      <!-- Question Sets Tab -->
      <div v-show="activeTab === 'questionSets'" class="tab-panel">
        <div class="tab-panel-body">
          <ul class="qs-list">
            <li v-if="questionSets.length === 0" class="empty-state">
              <p>No question sets yet</p>
            </li>
            <li 
              v-for="qs in questionSets" 
              :key="qs.id" 
              class="qs-item" 
              :class="{ active: currentQuestionSet?.id === qs.id }"
              @click="$emit('select-question-set', qs)"
            >
              <span class="qs-name">{{ qs.name }}</span>
              <span v-if="runningQuestionSetId === qs.id" class="running-indicator-dot"></span>
              <span class="qs-meta">{{ getQuestionCount(qs) }} questions</span>
            </li>
          </ul>
        </div>
        <div class="tab-panel-footer">
          <div class="sidebar-action-grid">
            <button class="btn btn-secondary btn-sm btn-full-width sidebar-action-btn" @click="handleEditQuestions" :disabled="!currentQuestionSet">
              ✏️ Edit
            </button>
            <button class="btn btn-secondary btn-sm btn-full-width sidebar-action-btn" @click="handleCloneQuestionSet" :disabled="!currentQuestionSet || !workspaceId">
              📄 Clone
            </button>
            <button class="btn btn-secondary btn-sm btn-full-width sidebar-action-btn" @click="openTransferModal" :disabled="!currentQuestionSet">
              🔗 Share
            </button>
            <button
              class="btn btn-danger btn-sm btn-full-width sidebar-action-btn"
              @click="openDeleteQuestionSetConfirm"
              :disabled="!currentQuestionSet || isDeletingQuestionSet || runningQuestionSetId === currentQuestionSet?.id"
            >
              🗑 Delete
            </button>
            <button class="btn btn-primary btn-sm btn-full-width sidebar-action-btn sidebar-action-btn-wide" @click="handleCreateQuestionSet">
              <span class="icon">➕</span> New Validation Set
            </button>
          </div>
        </div>
      </div>

      <!-- Agents Tab -->
      <div v-show="activeTab === 'agents'" class="tab-panel">
        <div class="tab-panel-body">
          <div class="agents-list-sidebar">
            <div 
              v-for="agent in agents" 
              :key="agent.id" 
              class="agent-item-sidebar"
              :class="{ 'disabled': !agent.enabled }"
            >
              <div class="agent-item-header">
                <span class="agent-name">{{ agent.name }}</span>
                <span class="agent-type-badge" :class="agent.provider_type">
                  {{ agent.provider_type === 'mcp' ? 'Corvic' : (agent.provider_type === 'evaluator' ? 'Evaluator' : (agent.provider_type === 'nvidia' ? 'NVIDIA NIM' : agent.provider_type)) }}
                </span>
              </div>
              <div class="agent-item-status">
                <span :class="agent.enabled ? 'status-enabled' : 'status-disabled'">
                  {{ agent.enabled ? '✅ Enabled' : '⏸️ Disabled' }}
                </span>
              </div>
            </div>
            <div v-if="agents.length === 0" class="empty-state">
              <p>No agents configured</p>
            </div>
          </div>
        </div>
        <div class="tab-panel-footer">
          <button class="btn btn-primary btn-sm btn-full-width" @click="$emit('manage-agents')">
            <span class="icon">⚙️</span> Manage Agents
          </button>
        </div>
      </div>
    </div>

    <!-- Question Editor Modal -->
    <QuestionEditorModal 
      v-if="showQuestionEditor"
      :question-set="currentQuestionSet"
      :workspace-id="workspaceId"
      @close="onQuestionEditorClose"
      @saved="onQuestionSetSaved"
    />

    <QuestionSetTransferModal
      v-if="showTransferModal && currentQuestionSet"
      :question-set="currentQuestionSet"
      @close="closeTransferModal"
    />

    <ConfirmDialog
      v-model:visible="showDeleteQuestionSetConfirm"
      title="Delete Question Set"
      :message="deleteQuestionSetMessage"
      confirm-text="Delete Set"
      cancel-text="Cancel"
      variant="danger"
      @confirm="confirmDeleteQuestionSet"
    />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import QuestionEditorModal from './QuestionEditorModal.vue'
import QuestionSetTransferModal from './QuestionSetTransferModal.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import wsService from '../services/websocket.js'

const props = defineProps({
  questionSets: {
    type: Array,
    default: () => []
  },
  currentQuestionSet: Object,
  agents: {
    type: Array,
    default: () => []
  },
  runningQuestionSetId: String,
  workspaceId: String
})

const emit = defineEmits([
  'create-question-set',
  'select-question-set',
  'view-history',
  'manage-agents',
  'question-set-updated',
  'question-set-deleted'
])

const activeTab = ref('questionSets')
const showQuestionEditor = ref(false)
const showTransferModal = ref(false)
const previousQuestionSet = ref(null)
const showDeleteQuestionSetConfirm = ref(false)
const isDeletingQuestionSet = ref(false)

const deleteQuestionSetMessage = computed(() => {
  const name = props.currentQuestionSet?.name || 'this question set'
  return `Delete "${name}" permanently? This also deletes its benchmark history, results, evaluations, agent bindings, and share links. This cannot be undone.`
})

function getQuestionCount(set) {
  if (!set || !set.data) return 0
  let data = set.data
  if (typeof data === 'string') {
    try { data = JSON.parse(data) } catch (e) { return 0 }
  }
  if (!data.categories) return 0
  return data.categories.reduce((acc, cat) => acc + (cat.questions ? cat.questions.length : 0), 0)
}

function handleCreateQuestionSet() {
  previousQuestionSet.value = props.currentQuestionSet
  emit('create-question-set')
  // Open editor immediately for new question set creation
  showQuestionEditor.value = true
}

function handleEditQuestions() {
  if (!props.currentQuestionSet) return
  previousQuestionSet.value = props.currentQuestionSet
  showQuestionEditor.value = true
}

function cloneQuestionSetData(questionSet) {
  if (!questionSet?.data) return null
  let data = questionSet.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch (error) {
      return null
    }
  }
  try {
    return JSON.parse(JSON.stringify(data))
  } catch (error) {
    return null
  }
}

function getClonedQuestionSetName(sourceName) {
  const baseName = String(sourceName || 'Validation Set').trim() || 'Validation Set'
  const existingNames = new Set(
    (props.questionSets || [])
      .map((set) => String(set?.name || '').trim().toLowerCase())
      .filter(Boolean)
  )

  const firstAttempt = `${baseName} (Copy)`
  if (!existingNames.has(firstAttempt.toLowerCase())) {
    return firstAttempt
  }

  let index = 2
  while (existingNames.has(`${baseName} (Copy ${index})`.toLowerCase())) {
    index++
  }
  return `${baseName} (Copy ${index})`
}

async function handleCloneQuestionSet() {
  if (!props.currentQuestionSet || !props.workspaceId) return

  const clonedData = cloneQuestionSetData(props.currentQuestionSet)
  if (!clonedData) {
    alert('Failed to clone set: invalid question set data.')
    return
  }

  const clonedName = getClonedQuestionSetName(props.currentQuestionSet.name)

  try {
    const created = await wsService.createQuestionSet(props.workspaceId, {
      name: clonedName,
      version: props.currentQuestionSet.version || '1.0',
      data: clonedData
    })

    // Clone includes only questions. Agent selection/config is intentionally not copied.
    emit('question-set-updated', created)
    emit('select-question-set', created)
  } catch (error) {
    console.error('Failed to clone question set:', error)
    alert('Failed to clone set: ' + (error?.message || 'Unknown error'))
  }
}

function openTransferModal() {
  if (!props.currentQuestionSet) return
  showTransferModal.value = true
}

function closeTransferModal() {
  showTransferModal.value = false
}

function openDeleteQuestionSetConfirm() {
  if (!props.currentQuestionSet || isDeletingQuestionSet.value) return
  showDeleteQuestionSetConfirm.value = true
}

async function confirmDeleteQuestionSet() {
  if (!props.currentQuestionSet?.id || isDeletingQuestionSet.value) return

  isDeletingQuestionSet.value = true
  try {
    const result = await wsService.deleteQuestionSet(props.currentQuestionSet.id)
    emit('question-set-deleted', result)
    emit('select-question-set', null)
  } catch (error) {
    console.error('Failed to delete question set:', error)
    alert('Failed to delete set: ' + (error?.message || 'Unknown error'))
  } finally {
    isDeletingQuestionSet.value = false
  }
}

function onQuestionEditorClose() {
  showQuestionEditor.value = false
  // If we were creating a new set and user closed without saving, restore previous
  if (!props.currentQuestionSet && previousQuestionSet.value) {
    emit('select-question-set', previousQuestionSet.value)
  }
  previousQuestionSet.value = null
}

function onQuestionSetSaved(updated) {
  showQuestionEditor.value = false
  previousQuestionSet.value = null
  emit('question-set-updated', updated)
}
</script>

<style scoped>
.left-sidebar {
  width: 300px;
  background: #f8f9fa;
  border-right: 1px solid #e0e0e0;
  display: flex;
  flex-direction: column;
  height: 100%;
}

.sidebar-tabs {
  display: flex;
  border-bottom: 1px solid #e0e0e0;
  background: white;
}

.tab-btn {
  flex: 1;
  padding: 12px 16px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: #666;
  transition: all 0.2s;
  border-bottom: 2px solid transparent;
}

.tab-btn:hover {
  background: #f5f5f5;
  color: #333;
}

.tab-btn.active {
  color: #007bff;
  border-bottom-color: #007bff;
  background: #f8f9fa;
}

.sidebar-content {
  flex: 1;
  min-height: 0;
  width: 100%;
  box-sizing: border-box;
  padding: 12px;
  display: flex;
  overflow: hidden;
}

.tab-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  flex: 1;
  width: 100%;
  min-height: 0;
  min-width: 0;
  box-sizing: border-box;
}

.tab-panel-body {
  flex: 1;
  min-height: 0;
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 2px;
}

.tab-panel-footer {
  flex-shrink: 0;
  width: 100%;
  box-sizing: border-box;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid #e0e0e0;
  background: #f8f9fa;
}

.questions-header-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.questions-header-top h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.questions-header-actions {
  display: flex;
  gap: 8px;
}

/* UL reset – no surrounding container box */
.qs-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  width: 100%;
  box-sizing: border-box;
}

/* spacing between pills */
.qs-list > li + li {
  margin-top: 10px;
}

/* empty state */
.qs-list .empty-state {
  padding: 16px 12px;
  color: #8A94A6;
  font-size: 14px;
}

/* main pill item */
.qs-item {
  display: flex;
  align-items: center;
  gap: 10px;

  width: 100%;
  padding: 14px 16px;
  border-radius: 18px;

  background: #F4F7FB;
  border: 1px solid #D7DEE8;

  cursor: pointer;
  user-select: none;

  box-shadow: 0 2px 10px rgba(16, 24, 40, 0.06);

  transition:
    background 160ms ease,
    border-color 160ms ease,
    box-shadow 160ms ease,
    transform 120ms ease;
}

.qs-item:hover {
  transform: translateY(-1px);
  border-color: #BFCBDA;
  box-shadow: 0 6px 18px rgba(16, 24, 40, 0.10);
}

/* selected / active state */
.qs-item.active {
  background: #EAF2FF;
  border-color: #2F6BFF;

  box-shadow:
    0 0 0 4px rgba(47, 107, 255, 0.16),
    0 10px 22px rgba(47, 107, 255, 0.10);
}

/* left title */
.qs-name {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.15;
  letter-spacing: 0.1px;
  color: #1B2430;

  /* allow wrapping like the screenshot */
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* right meta text */
.qs-meta {
  font-size: 11px;
  font-weight: 500;
  color: rgba(27, 36, 48, 0.50);
  white-space: nowrap;
}

.qs-item.active .qs-meta {
  color: #2F6BFF;        /* or #111827 if you want neutral */
  opacity: 1;
}

/* running indicator dot */
.running-indicator-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #2F6BFF;
  box-shadow: 0 0 0 4px rgba(47, 107, 255, 0.25);
}

/* keyboard accessibility */
.qs-item:focus-visible {
  outline: none;
  box-shadow:
    0 0 0 6px rgba(47, 107, 255, 0.22),
    0 10px 22px rgba(16, 24, 40, 0.10);
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.qs-action-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 4px 8px;
  margin-left: 8px;
  font-size: 14px;
  opacity: 0.6;
  transition: opacity 0.2s;
}

.qs-action-btn:hover {
  opacity: 1;
}

.sidebar-action-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  width: 100%;
  box-sizing: border-box;
}

.sidebar-action-grid > * {
  min-width: 0;
  box-sizing: border-box;
}

.sidebar-action-btn {
  min-height: 36px;
}

.sidebar-action-btn-wide {
  grid-column: 1 / -1;
}

.btn-full-width {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

/* Agents Tab Styles */
.agents-header {
  margin-bottom: 16px;
}

.agents-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.agents-list-sidebar {
  width: 100%;
  min-height: 0;
  box-sizing: border-box;
}

.agent-item-sidebar {
  padding: 12px;
  margin-bottom: 8px;
  background: white;
  border: 1px solid #e0e0e0;
  border-radius: 6px;
}

.agent-item-sidebar.disabled {
  opacity: 0.6;
}

.agent-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.agent-name {
  font-weight: 500;
  flex: 1;
}

.agent-type-badge {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 3px;
  background: #e9ecef;
  color: #495057;
}

.agent-type-badge.mcp {
  background: #d4edda;
  color: #155724;
}

.agent-type-badge.evaluator {
  background: #fff3cd;
  color: #856404;
}

.agent-type-badge.nvidia {
  background: #dcfce7;
  color: #14532d;
}

.agent-item-status {
  font-size: 12px;
}

.status-enabled {
  color: #28a745;
}

.status-disabled {
  color: #6c757d;
}

.empty-state {
  text-align: center;
  padding: 40px 20px;
  color: #666;
}

.empty-state p {
  margin-bottom: 16px;
}

.btn-sm {
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.2;
}

@media (max-width: 960px) {
  .sidebar-action-grid {
    grid-template-columns: 1fr;
  }

  .sidebar-action-btn-wide {
    grid-column: auto;
  }
}
</style>
