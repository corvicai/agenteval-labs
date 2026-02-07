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
        <div class="add-validation-set-row">
          <button class="btn btn-secondary btn-sm btn-full-width" @click="handleEditQuestions" :disabled="!currentQuestionSet">
            ✏️ Edit Questions
          </button>
          <button class="btn btn-primary btn-sm btn-full-width" @click="handleCreateQuestionSet">
            <span class="icon">➕</span> Add Validation Set
          </button>
        </div>
      </div>

      <!-- Agents Tab -->
      <div v-show="activeTab === 'agents'" class="tab-panel">
        <div class="agents-header">
          <h3>🤖 Agents</h3>
        </div>
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
                {{ agent.provider_type === 'mcp' ? 'Corvic' : (agent.provider_type === 'evaluator' ? 'Evaluator' : agent.provider_type) }}
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
            <button class="btn btn-primary btn-sm" @click="$emit('manage-agents')">
              Manage Agents
            </button>
          </div>
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
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import QuestionEditorModal from './QuestionEditorModal.vue'

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
  'question-set-updated'
])

const activeTab = ref('questionSets')
const showQuestionEditor = ref(false)
const previousQuestionSet = ref(null)

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
  overflow-y: auto;
  padding: 16px;
}

.tab-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
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
}

/* spacing between pills */
.qs-list > li + li {
  margin-top: 14px;
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
  gap: 12px;

  width: 100%;
  padding: 18px 22px;
  border-radius: 28px;

  background: #F4F7FB;
  border: 2px solid #D7DEE8;

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
    0 0 0 6px rgba(47, 107, 255, 0.18),
    0 10px 22px rgba(47, 107, 255, 0.10);
}

/* left title */
.qs-name {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.15;
  letter-spacing: 0.1px;
  color: #1B2430;

  /* allow wrapping like the screenshot */
  white-space: normal;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* right meta text */
.qs-meta {
  font-size: 12px;
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

.add-validation-set-row {
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid #e0e0e0;
  flex-shrink: 0;
  background: #f8f9fa;
}

.btn-full-width {
  width: 100%;
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
  flex: 1;
  overflow-y: auto;
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
  padding: 6px 12px;
  font-size: 13px;
}
</style>
