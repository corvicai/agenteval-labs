<template>
  <div class="chat-panel">
    <div class="panel-header">
      <div>
        <h2>
          {{ agentName }}
          <span 
            v-if="provider"
            class="agent-provider-badge"
          >
            {{ provider === 'openai' ? 'OpenAI' : provider }}
          </span>
        </h2>
        <div class="agent-url" v-if="provider !== 'custom'">
          <template v-if="provider === 'openai'">
            <div v-if="agentUrl">Prompt ID: {{ agentUrl }}</div>
            <div v-if="model">Model: {{ model }}</div>
          </template>
          <template v-else>
            {{ agentUrl }}
          </template>
        </div>
      </div>
      <div class="panel-header-actions">
        <button
          v-if="onRunAllForAgent && !readonly"
          class="btn-run-all-agent"
          @click="handleRunAllForAgent"
        >
          Run all
        </button>
        <span class="status connected" v-if="!readonly">Connected</span>
        <span class="status read-only" v-else>Read-only</span>
      </div>
    </div>
    <div class="messages" :ref="setMessagesRef" @scroll="handleScroll">
      <div
        v-for="(qa, index) in results"
        :key="qa.id || qa.question?.id || `q-${index}`"
        class="message-group"
        :ref="el => setMessageRef(index, el)"
        :data-question-index="index"
        tabindex="0"
      >
        <div class="question" :class="{ 'editing': editingIndex === index && editingField === 'question' }">
          <div class="question-header">
            <div class="question-text">
              <strong>Q{{ index + 1 }}:</strong> 
              <template v-if="editingIndex === index && editingField === 'question'">
                <textarea 
                  v-model="editValue" 
                  class="edit-textarea"
                  @blur="saveEdit(index)"
                  @keydown.esc="cancelEdit"
                  @keydown.enter.ctrl="saveEdit(index)"
                  v-focus
                ></textarea>
              </template>
              <template v-else>
                <span>{{ typeof qa.question === 'object' ? qa.question.question : qa.question }}</span>
              </template>
            </div>
            
            <!-- Expected Answer Display/Edit (only for OpenAI evaluator) -->
            <div class="expected-answer" v-if="provider === 'openai' && getExpectedAnswer(qa, index)">
              <div class="expected-header">
                <div class="expected-badge">Expected Answer</div>
                <button 
                  class="btn-edit-expected" 
                  @click="startEditExpected(index, getExpectedAnswer(qa, index))"
                  v-if="!readonly"
                >
                  Edit
                </button>
              </div>
              <template v-if="editingIndex === index && editingField === 'expected'">
                <textarea 
                  v-model="editValue" 
                  class="edit-textarea expected-edit"
                  placeholder="Enter the expected/gold standard answer..."
                  @blur="saveExpected(index)"
                  @keydown.esc="cancelEdit"
                  @keydown.enter.ctrl="saveExpected(index)"
                  v-focus
                ></textarea>
              </template>
              <template v-else>
                <div class="expected-content" v-html="getProcessedAnswer(getExpectedAnswer(qa, index))"></div>
              </template>
            </div>
            
            <!-- Add Expected Button (only for OpenAI evaluator, shown when no expected answer exists) -->
            <div class="add-expected-wrapper" v-else-if="provider === 'openai' && !readonly">
              <template v-if="editingIndex === index && editingField === 'expected'">
                <textarea 
                  v-model="editValue" 
                  class="edit-textarea expected-edit"
                  placeholder="Enter the expected/gold standard answer..."
                  @blur="saveExpected(index)"
                  @keydown.esc="cancelEdit"
                  @keydown.enter.ctrl="saveExpected(index)"
                  v-focus
                ></textarea>
              </template>
              <template v-else>
                <button 
                  class="btn-add-expected" 
                  @click="startEditExpected(index, '')"
                >
                  + Add Expected Answer
                </button>
              </template>
            </div>
            
            <div class="question-actions" v-if="!readonly">
              <button 
                class="btn-retry-question" 
                :class="{ 'is-running': rerunningIndex === index || qa.loading }"
                @click="handleRetry(index)" 
                v-if="provider !== 'custom'"
                :disabled="rerunningIndex === index || qa.loading"
              >
                {{ (rerunningIndex === index || qa.loading) ? '⏳ Running...' : '🔄 Re-run' }}
              </button>
              <button 
                class="btn-history" 
                @click="toggleHistory(index)"
                v-if="getQuestionHistory(qa).length > 1"
              >
                🕒 {{ historyVisible[index] ? 'Hide' : 'History' }}
              </button>
              <button
                v-if="provider === 'openai'"
                class="btn-spy-payload"
                @click="onSpyPayload(index, agentId)"
                title="Spy evaluation payload"
              >
                🕵️ Spy
              </button>
              <label class="btn-attach-image" v-if="provider !== 'custom'">
                Attach image
                <input
                  type="file"
                  accept="image/*"
                  class="upload-input"
                  @change="event => handleUploadImage(index, event)"
                />
              </label>
            </div>
          </div>
        </div>

        <!-- History Section -->
        <div v-if="historyVisible[index]" class="history-container">
          <div class="history-header">
            <h4>Run History</h4>
            <span class="history-count">{{ getQuestionHistory(qa).length }} runs</span>
          </div>
          <div class="history-list">
            <div 
              v-for="(run, rIdx) in getQuestionHistory(qa)" 
              :key="run.timestamp || rIdx" 
              class="history-item"
              :class="{ 'latest-entry': rIdx === 0 }"
            >
              <div class="history-item-meta">
                <span class="run-number">#{{ getQuestionHistory(qa).length - rIdx }}</span>
                <span class="run-time">{{ formatTimestamp(run.timestamp) }}</span>
                <span class="run-duration" v-if="getAgentAnswerFromRun(run)?.duration">
                  ({{ getAgentAnswerFromRun(run).duration }}s)
                </span>
                <span v-if="rIdx === 0" class="latest-badge">latest</span>
                <button 
                  v-if="!readonly"
                  class="btn-delete-history" 
                  @click.stop="handleDeleteHistory(run.id)"
                  title="Delete this run"
                >
                  <svg viewBox="0 0 24 24" width="14" height="14" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path><line x1="10" y1="11" x2="10" y2="17"></line><line x1="14" y1="11" x2="14" y2="17"></line></svg>
                </button>
              </div>
              <div class="history-item-content">
                <template v-if="getAgentAnswerFromRun(run)?.error">
                  <div class="history-error-full">
                    <span class="error-badge">Error</span>
                    <div class="error-text">{{ getAgentAnswerFromRun(run).error }}</div>
                  </div>
                </template>
                <template v-else>
                  <div 
                    class="history-text" 
                    v-html="getProcessedAnswer(getAgentAnswerFromRun(run)?.answerText || getAgentAnswerFromRun(run)?.answer)"
                  ></div>
                </template>
              </div>
            </div>
          </div>
        </div>
        <div class="answer" v-if="qa.loading">
          <div class="loading-spinner"></div>
          <span>Loading...</span>
        </div>
        <div class="answer" v-else-if="qa.error">
          <span class="error">Error: {{ qa.error }}</span>
          <button @click="handleRetry(index)" class="btn-retry">Retry</button>
        </div>
        <div class="answer" v-else-if="qa.answer || provider === 'custom'" :class="{ 'approved': qa.humanValidation === 'positive' }">
          <div class="answer-content">
            <template v-if="editingIndex === index && editingField === 'answer'">
              <textarea 
                v-model="editValue" 
                class="edit-textarea answer-edit"
                @blur="saveEdit(index)"
                @keydown.esc="cancelEdit"
                @keydown.enter.ctrl="saveEdit(index)"
                v-focus
              ></textarea>
            </template>
            <template v-else>
                <div 
                  class="answer-text" 
                  v-html="getProcessedAnswer(qa.answer) || (provider === 'custom' ? '<em style=\'color: #999;\'>Click to add response...</em>' : '')"
                  @click="provider === 'custom' ? startEdit(index, 'answer', getAnswerText(qa.answer)) : null"
                ></div>
              <button 
                v-if="provider === 'custom'"
                class="btn-edit-answer"
                @click="startEdit(index, 'answer', getAnswerText(qa.answer))"
              >
                Edit Answer
              </button>
            </template>
            <div class="answer-meta" v-if="getAnswerMeta(qa.answer)">
              <div class="meta-item" v-if="getAnswerMeta(qa.answer).title">
                <strong>Source:</strong> {{ getAnswerMeta(qa.answer).title }}
              </div>
              <div class="meta-item" v-if="getAnswerMeta(qa.answer).document">
                <strong>Document:</strong> {{ getAnswerMeta(qa.answer).document }}
              </div>
            </div>
          </div>
          <div class="answer-footer">
            <div class="answer-time" v-if="qa.duration || qa.timestamp">
              <span v-if="qa.duration">Response time: {{ formatDuration(qa.duration) }}</span>
              <span v-if="qa.duration && qa.timestamp" class="meta-separator">|</span>
              <span v-if="qa.timestamp" class="response-date">{{ formatTimestamp(qa.timestamp) }}</span>
            </div>
            <div class="human-validation">
              <button 
                @click="handleValidation(index, 'positive')" 
                class="btn-validation"
                :class="{ 'active-positive': qa.humanValidation === 'positive' }"
                title="Correct and complete answer"
              >
                👍
              </button>
              <button 
                @click="handleValidation(index, 'negative')" 
                class="btn-validation"
                :class="{ 'active-negative': qa.humanValidation === 'negative' }"
                title="Incorrect answer"
              >
                👎
              </button>
              <button 
                @click="handleValidation(index, 'alternative')" 
                class="btn-validation"
                :class="{ 'active-alternative': qa.humanValidation === 'alternative' }"
                title="Valid but different from expected answer"
              >
                🔄
              </button>
              <button 
                @click="handleValidation(index, 'partial')" 
                class="btn-validation"
                :class="{ 'active-partial': qa.humanValidation === 'partial' }"
                title="Partially correct"
              >
                ⚠️
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { processContent } from '../utils/markdown.js'
import { formatDuration } from '../utils/formatDuration.js'

export default {
  name: 'ChatPanel',
  props: {
    agentName: {
      type: String,
      required: true
    },
    agentId: {
      type: String,
      required: true
    },
    agentUrl: {
      type: String,
      required: true
    },
    model: {
      type: String,
      required: false,
      default: ''
    },
    results: {
      type: Array,
      required: true
    },
    messagesRef: {
      type: Object,
      required: true
    },
    messageRefs: {
      type: Object,
      required: true
    },
    readonly: {
      type: Boolean,
      default: false
    },
    onScroll: {
      type: Function,
      required: true
    },
    onRetry: {
      type: Function,
      required: true
    },
    onValidation: {
      type: Function,
      required: true
    },
    extractAnswerText: {
      type: Function,
      required: true
    },
    extractAnswerMeta: {
      type: Function,
      required: true
    },
    provider: {
      type: String,
      required: false,
      default: 'corvic'
    },
    onUploadImage: {
      type: Function,
      required: false
    },
    onRunAllForAgent: {
      type: Function,
      required: false
    },
    onUpdateResult: {
      type: Function,
      required: false
    },
    onUpdateExpected: {
      type: Function,
      required: false
    },
    expectedAnswers: {
      type: Object,
      default: () => ({})
    },
    historyByQuestion: {
      type: Object,
      default: () => ({})
    },
    getQuestionKey: {
      type: Function,
      required: true
    },
    onSpyPayload: {
      type: Function,
      required: false
    }
  },
  directives: {
    focus: {
      mounted(el) {
        el.focus()
      }
    }
  },
  data() {
    return {
      editingIndex: null,
      editingField: null,
      editValue: '',
      historyVisible: {}, // { [questionIndex]: boolean }
      rerunningIndex: null // Index of question currently being re-run
    }
  },
  methods: {
    setMessagesRef(el) {
      if (this.messagesRef) {
        this.messagesRef.value = el
      }
    },
    setMessageRef(index, el) {
      if (this.messageRefs && el) {
        this.messageRefs.value[index] = el
      }
    },
    handleScroll(event) {
      if (this.onScroll) {
        this.onScroll(event)
      }
    },
    handleRetry(index) {
      if (this.rerunningIndex !== null) return // Already running one
      if (this.onRetry) {
        this.rerunningIndex = index
        this.onRetry(index)
        // The loading state will be managed by the parent via qa.loading
        // Reset rerunningIndex after a short delay to allow UI feedback
        setTimeout(() => {
          this.rerunningIndex = null
        }, 500)
      }
    },
    handleRunAllForAgent() {
      if (this.onRunAllForAgent) {
        this.onRunAllForAgent()
      }
    },
    handleValidation(index, validation) {
      if (this.onValidation) {
        // Map UI validation values to API rating values
        const ratingMap = {
          'positive': 'like',
          'negative': 'dislike',
          'alternative': 'valid',
          'partial': 'wrong'
        }
        const rating = ratingMap[validation] || validation
        this.onValidation(index, rating)
      }
    },
    handleUploadImage(index, event) {
      if (!this.onUploadImage) return
      const file = event.target.files && event.target.files[0]
      if (!file) return
      this.onUploadImage(index, file)
      // Reset input so the same file can be selected again if needed
      event.target.value = ''
    },
    getAnswerText(answer) {
      return this.extractAnswerText ? this.extractAnswerText(answer) : ''
    },
    getProcessedAnswer(answer) {
      const rawText = this.getAnswerText(answer)
      
      if (!rawText || typeof rawText !== 'string') {
        return ''
      }
      
      const processed = processContent(rawText)
      return processed.html || rawText
    },
    getAnswerMeta(answer) {
      return this.extractAnswerMeta ? this.extractAnswerMeta(answer) : null
    },
    startEdit(index, field, value) {
      if (this.readonly) return
      this.editingIndex = index
      this.editingField = field
      this.editValue = value
    },
    cancelEdit() {
      this.editingIndex = null
      this.editingField = null
      this.editValue = ''
    },
    saveEdit(index) {
      if (this.editingIndex === null) return
      
      const field = this.editingField
      const value = this.editValue
      
      if (this.onUpdateResult) {
        this.onUpdateResult(index, field, value)
      }
      
      this.cancelEdit()
    },
    getExpectedAnswer(qa, index) {
      const id = String(index + 1).padStart(2, '0')
      // First check if question object has expected field
      if (typeof qa.question === 'object' && qa.question.expected) {
        return qa.question.expected
      }
      // Then check if there's a custom expected answer stored
      if (this.expectedAnswers && this.expectedAnswers[id]) {
        return this.expectedAnswers[id]
      }
      return null
    },
    startEditExpected(index, value) {
      if (this.readonly) return
      this.editingIndex = index
      this.editingField = 'expected'
      this.editValue = value || ''
    },
    saveExpected(index) {
      if (this.editingIndex === null || this.editingField !== 'expected') return
      
      const value = this.editValue.trim()
      
      if (this.onUpdateExpected) {
        this.onUpdateExpected(index, value)
      }
      
      this.cancelEdit()
    },
    toggleHistory(index) {
      this.historyVisible[index] = !this.historyVisible[index]
    },
    getQuestionHistory(qa) {
      if (!qa || !qa.question) return []
      const qKey = this.getQuestionKey(qa.question)
      const fullHistory = this.historyByQuestion[qKey] || []
      
      // Filter history to only include runs where THIS agent participated
      return fullHistory.filter(run => !!this.getAgentAnswerFromRun(run))
    },
    async handleDeleteHistory(messageId) {
      if (!messageId) return
      
      if (!confirm('Are you sure you want to delete this history entry? This will remove it for all agents.')) {
        return
      }

      try {
        // TODO: Implement delete API in frontend/backend
        // await deleteMessageApi(messageId)
        alert('Delete history functionality not yet implemented in backend V2')
        // Notify parent to reload history
        // this.$emit('delete-history', messageId)
      } catch (error) {
        console.error('Error deleting history entry:', error)
        alert('Failed to delete history entry: ' + error.message)
      }
    },
    getAgentAnswerFromRun(run) {
      if (!run || !run.agents) return null
      return run.agents.find(a => a.name === this.agentName || a.agentId === this.agentId)
    },
    formatTimestamp(ts) {
      if (!ts) return ''
      const date = new Date(ts)
      return date.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      })
    },
    formatDuration(seconds) {
      return formatDuration(seconds)
    }
  }
}
</script>

<style scoped>
.chat-panel {
  display: flex;
  flex-direction: column;
  background: #ffffff;
  overflow: visible;
  width: 100%;
  border-radius: 12px;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.08);
}

.panel-header {
  padding: 1rem 1.5rem;
  background: #f8f9fa;
  border-bottom: 1px solid #dee2e6;
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: sticky;
  top: 0;
  z-index: 10;
  border-radius: 12px 12px 0 0;
}

.panel-header-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.panel-header h2 {
  font-size: 1.125rem;
  color: #212529;
  margin: 0 0 0.25rem 0;
  font-weight: 600;
}

.agent-url {
  font-size: 0.75rem;
  color: #6c757d;
  font-family: 'Monaco', 'Menlo', monospace;
  word-break: break-all;
  max-width: fit-content;
}

.agent-provider-badge {
  display: inline-block;
  margin-left: 0.4rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: #e7f1ff;
  color: #0b5ed7;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
}

.status {
  padding: 0.25rem 0.75rem;
  border-radius: 12px;
  font-size: 0.75rem;
  font-weight: 600;
}

.status.connected {
  background: #d4edda;
  color: #155724;
}

.status.read-only {
  background: #ffe8cc;
  color: #d9480f;
}

.btn-run-all-agent {
  padding: 0.25rem 0.6rem;
  background: #ffffff;
  border: 1px solid #0d6efd;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #0d6efd;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.btn-run-all-agent:hover {
  background: #0d6efd;
  color: #ffffff;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
  position: relative;
}

.message-group {
  margin-bottom: 2rem;
  padding-bottom: 2rem;
  border-bottom: 1px solid #e9ecef;
}

.message-group:last-child {
  border-bottom: none;
}

.question {
  background: #f8f9fa;
  padding: 1rem;
  border-radius: 8px;
  margin-bottom: 1rem;
  color: #212529;
  border-left: 4px solid #49399d;
}

.question-header {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.expected-answer {
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 8px;
  padding: 1rem;
  margin-top: 0.75rem;
}

.expected-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.expected-badge {
  background: #10b981;
  color: white;
  font-size: 0.65rem;
  font-weight: 700;
  padding: 3px 10px;
  border-radius: 999px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.btn-edit-expected {
  padding: 0.2rem 0.5rem;
  background: transparent;
  border: 1px solid #10b981;
  border-radius: 4px;
  font-size: 0.7rem;
  color: #10b981;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-edit-expected:hover {
  background: #10b981;
  color: white;
}

.expected-content {
  font-size: 0.9rem;
  color: #166534;
  line-height: 1.6;
}

.expected-edit {
  min-height: 120px;
  border-color: #10b981;
}

.add-expected-wrapper {
  margin-top: 0.5rem;
}

.btn-add-expected {
  padding: 0.4rem 0.8rem;
  background: transparent;
  border: 1px dashed #10b981;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #10b981;
  cursor: pointer;
  transition: all 0.2s;
  width: 100%;
}

.btn-add-expected:hover {
  background: #f0fdf4;
  border-style: solid;
}

.question-text {
  flex: 1;
}

.question-actions {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
}

.btn-retry-question {
  padding: 0.25rem 0.6rem;
  background: #ffffff;
  border: 1px solid #49399d;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #49399d;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.btn-retry-question:hover:not(:disabled) {
  background: #49399d;
  color: #ffffff;
}

.btn-retry-question:disabled,
.btn-retry-question.is-running {
  background: #e9ecef;
  border-color: #adb5bd;
  color: #6c757d;
  cursor: not-allowed;
  opacity: 0.8;
}

.btn-retry-question.is-running {
  animation: pulse-subtle 1.5s ease-in-out infinite;
}

@keyframes pulse-subtle {
  0%, 100% { opacity: 0.7; }
  50% { opacity: 1; }
}

.btn-attach-image {
  padding: 0.25rem 0.6rem;
  background: #ffffff;
  border: 1px dashed #6c757d;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
  color: #495057;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.btn-attach-image:hover {
  background: #f1f3f5;
}

.btn-spy-payload {
  padding: 0.25rem 0.6rem;
  background: #fdf2f8;
  border: 1px solid #db2777;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #db2777;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.btn-spy-payload:hover {
  background: #db2777;
  color: #ffffff;
}

.upload-input {
  display: none;
}

.btn-delete-history {
  margin-left: auto;
  background: transparent;
  border: none;
  color: #dc3545;
  cursor: pointer;
  padding: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.2s;
  opacity: 0.6;
}

.btn-delete-history:hover {
  background: #fff5f5;
  opacity: 1;
}
.answer {
  background: #ffffff;
  padding: 1rem;
  border-radius: 8px;
  color: #212529;
  border: 1px solid #e9ecef;
  border-left: 4px solid #49399d;
}

.answer pre {
  margin: 0;
  white-space: pre-wrap;
  word-wrap: break-word;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.875rem;
  line-height: 1.6;
}

.answer .error {
  color: #ef4444;
  display: block;
  margin-bottom: 0.75rem;
}

.btn-retry {
  margin-top: 0.5rem;
  padding: 0.375rem 0.75rem;
  background: #49399d;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-retry:hover {
  background: #3d2f85;
}

.answer-content {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.answer-text {
  line-height: 1.7;
  color: #495057;
  word-wrap: break-word;
}

/* Markdown styling */
.answer-text :deep(h1) {
  font-size: 1.5rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
  color: #212529;
}

.answer-text :deep(h2) {
  font-size: 1.3rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
  color: #212529;
}

.answer-text :deep(h3) {
  font-size: 1.1rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
  color: #212529;
}

.answer-text :deep(p) {
  margin: 0 0 0.75rem 0;
}

.answer-text :deep(strong) {
  font-weight: 600;
  color: #212529;
}

.answer-text :deep(em) {
  font-style: italic;
}

.answer-text :deep(code) {
  background: #f5f5f5;
  padding: 0.125rem 0.35rem;
  border-radius: 3px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.875em;
  color: #d63384;
}

.answer-text :deep(pre) {
  background: #0f172a;
  color: #e2e8f0;
  padding: 0.75rem;
  border-radius: 8px;
  overflow-x: auto;
  margin: 0.5rem 0;
}

.answer-text :deep(pre code) {
  background: none;
  padding: 0;
  color: inherit;
  font-size: 0.875rem;
}

.answer-text :deep(ul),
.answer-text :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.5rem;
}

.answer-text :deep(li) {
  margin: 0.25rem 0;
}

.answer-text :deep(blockquote) {
  border-left: 3px solid #dee2e6;
  padding-left: 1rem;
  margin: 0.5rem 0;
  color: #6c757d;
  font-style: italic;
}

.answer-text :deep(a) {
  color: #49399d;
  text-decoration: none;
}

.answer-text :deep(a:hover) {
  text-decoration: underline;
}

.answer-text :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 8px;
  margin: 0.5rem 0;
  display: block;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.answer-text :deep(hr) {
  border: none;
  border-top: 1px solid #dee2e6;
  margin: 1rem 0;
}

.answer-text :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.5rem 0;
}

.answer-text :deep(th),
.answer-text :deep(td) {
  padding: 0.5rem;
  border: 1px solid #dee2e6;
  text-align: left;
}

.answer-text :deep(th) {
  background: #f8f9fa;
  font-weight: 600;
}

.answer-meta {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 1rem;
  border-top: 1px solid #e9ecef;
  font-size: 0.875rem;
}

.meta-item {
  color: #6c757d;
}

.meta-item strong {
  color: #495057;
  margin-right: 0.5rem;
}

.answer-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid #e9ecef;
}

.answer-time {
  font-size: 0.75rem;
  color: #6c757d;
  font-style: italic;
}

.human-validation {
  display: flex;
  gap: 0.5rem;
}

.btn-validation {
  padding: 0.25rem 0.5rem;
  background: #f8f9fa;
  border: 2px solid #dee2e6;
  border-radius: 6px;
  font-size: 1.25rem;
  cursor: pointer;
  transition: all 0.2s;
  line-height: 1;
}

.btn-validation:hover {
  background: #e9ecef;
  transform: scale(1.1);
}

.btn-validation.active-positive {
  background: #d4edda;
  border-color: #28a745;
  box-shadow: 0 0 0 3px rgba(40, 167, 69, 0.1);
}

.btn-validation.active-negative {
  background: #f8d7da;
  border-color: #dc3545;
  box-shadow: 0 0 0 3px rgba(220, 53, 69, 0.1);
}

.btn-validation.active-alternative {
  background: #d1ecf1;
  border-color: #17a2b8;
  box-shadow: 0 0 0 3px rgba(23, 162, 184, 0.1);
}

.btn-validation.active-partial {
  background: #fff3cd;
  border-color: #ffc107;
  box-shadow: 0 0 0 3px rgba(255, 193, 7, 0.1);
}

.answer.approved {
  border-left-color: #28a745;
  background: linear-gradient(to right, rgba(40, 167, 69, 0.03), #ffffff);
}

.loading-spinner {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid #e9ecef;
  border-top-color: #49399d;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
  margin-right: 0.5rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.edit-textarea {
  width: 100%;
  min-height: 100px;
  padding: 8px;
  border: 2px solid #49399d;
  border-radius: 4px;
  font-family: inherit;
  font-size: inherit;
  resize: vertical;
  background: #fff;
  color: #333;
  margin-top: 8px;
}

.answer-edit {
  min-height: 200px;
}

.btn-edit, .btn-edit-answer {
  padding: 0.2rem 0.5rem;
  background: #f8f9fa;
  border: 1px solid #dee2e6;
  border-radius: 4px;
  font-size: 0.7rem;
  color: #6c757d;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-edit:hover, .btn-edit-answer:hover {
  background: #e9ecef;
  color: #49399d;
  border-color: #49399d;
}

.question.editing {
  border-left-color: #ffc107;
}
.btn-history {
  padding: 0.25rem 0.6rem;
  background: #f1f5f9;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #475569;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-history:hover {
  background: #e2e8f0;
  color: #1e293b;
}

.history-container {
  margin-top: -1rem;
  margin-bottom: 1rem;
  padding: 1rem;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-left: 4px solid #cbd5e1;
  border-radius: 0 0 8px 8px;
  max-height: 400px;
  overflow-y: auto;
}

.history-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  border-bottom: 1px solid #cbd5e1;
  padding-bottom: 0.5rem;
}

.history-header h4 {
  margin: 0;
  font-size: 0.875rem;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.history-count {
  font-size: 0.75rem;
  color: #64748b;
  font-weight: 500;
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.history-item {
  padding: 0.75rem;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  transition: box-shadow 0.2s;
}

.history-item:hover {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

.history-item.latest-entry {
  border-color: #49399d33;
  background: #49399d05;
}

.history-item-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
  font-size: 0.75rem;
  color: #64748b;
}

.run-number {
  font-weight: 700;
  color: #475569;
}

.run-time {
  opacity: 0.8;
}

.run-duration {
  font-style: italic;
}

.latest-badge {
  background: #49399d;
  color: white;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  font-size: 0.65rem;
  font-weight: 600;
  text-transform: uppercase;
}

.history-item-content {
  font-size: 0.875rem;
  line-height: 1.6;
  color: #334155;
}

.history-text :deep(p) {
  margin: 0 0 0.5rem 0;
}

.history-text :deep(p:last-child) {
  margin-bottom: 0;
}

.history-error {
  color: #ef4444;
  font-weight: 500;
  margin-top: 0.5rem;
}

.history-error-full {
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 8px;
  padding: 0.75rem;
}

.error-badge {
  display: inline-block;
  background: #ef4444;
  color: white;
  font-size: 0.65rem;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 999px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
}

.error-text {
  color: #dc2626;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.8rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
