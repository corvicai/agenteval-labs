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
            <span class="qs-meta">{{ getQuestionCount(qs) }} qs</span>
            <button class="qs-action-btn" @click.stop="$emit('view-history', qs)" title="View History">📜</button>
          </li>
        </ul>
        <div class="add-validation-set-row">
          <button class="btn btn-primary btn-sm btn-full-width" @click="$emit('create-question-set')">
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
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

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
  runningQuestionSetId: String
})

const emit = defineEmits([
  'create-question-set',
  'select-question-set',
  'view-history',
  'manage-agents'
])

const activeTab = ref('questionSets')

function getQuestionCount(set) {
  if (!set || !set.data) return 0
  let data = set.data
  if (typeof data === 'string') {
    try { data = JSON.parse(data) } catch (e) { return 0 }
  }
  if (!data.categories) return 0
  return data.categories.reduce((acc, cat) => acc + (cat.questions ? cat.questions.length : 0), 0)
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

.qs-list {
  flex: 1;
  overflow-y: auto;
  margin-bottom: 16px;
  min-height: 0;
  list-style: none;
  padding: 0;
  margin: 0 0 16px 0;
}

.qs-list .empty-state {
  text-align: center;
  padding: 40px 20px;
  color: #666;
  font-size: 14px;
  list-style: none;
}

.qs-item {
  padding: 10px 12px;
  cursor: pointer;
  transition: background-color 0.15s ease;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #f0f0f0;
}

.qs-item:hover {
  background-color: #f5f5f5;
}

.qs-item.active {
  background-color: #e7f3ff;
  border-left: 3px solid #007bff;
  padding-left: 9px; /* Adjust for border */
}

.qs-name {
  font-weight: 500;
  flex: 1;
  color: #333;
}

.qs-meta {
  font-size: 11px;
  color: #666;
  margin-left: 8px;
}

.running-indicator-dot {
  width: 8px;
  height: 8px;
  background: #28a745;
  border-radius: 50%;
  margin-left: 8px;
  animation: pulse 1.5s infinite;
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
