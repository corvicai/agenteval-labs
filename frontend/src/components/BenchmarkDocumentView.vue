<template>
  <div class="benchmark-document-view">
    <!-- Sidebar: Run History -->
    <div class="sidebar" v-show="!showPrintView">
      <RunHistoryPanel 
        :workspace-id="workspaceId"
        :selected-run-id="selectedRunId"
        :pre-filter="preFilter"
        @select-run="handleSelectRun"
      />
    </div>

    <!-- Main Content: Benchmark Document -->
    <div class="main-content" v-show="!showPrintView">
      <div v-if="loading" class="loading-container">
        <div class="spinner"></div>
        <p>Loading benchmark document...</p>
      </div>

      <div v-else-if="!selectedRunId" class="empty-selection">
        <div class="empty-content">
          <div class="icon">📊</div>
          <h2>Select a Benchmark Run</h2>
          <p>Choose a run from the history sidebar to view its details, answers, and evaluations.</p>
        </div>
      </div>

      <div v-else-if="runData" class="document-container">
        <!-- Toolbar / Header -->
        <div class="document-header">
          <div class="header-left">
            <button class="btn-back-arena" @click="$emit('back')" title="Back to Arena">
              <span class="back-icon">←</span> Arena
            </button>
            <h1>{{ questionSetName || 'Benchmark Run' }}</h1>
            <div class="meta-badges">
              <span class="badge time">{{ formatTime(runData.run?.created_at) }}</span>
              <span class="badge status" :class="runData.run?.status">{{ runData.run?.status }}</span>
            </div>
            <div class="question-count" v-if="results.length">
              {{ results.length }} Questions
            </div>
          </div>
          <div class="header-actions">
            <!-- Navigation -->
            <div class="nav-controls">
              <button @click="scrollToPrev" class="btn-nav" title="Previous Question">⬆️</button>
              <button @click="scrollToNext" class="btn-nav" title="Next Question">⬇️</button>
            </div>
            
            <div class="divider"></div>

            <button class="btn-action" @click="exportToPdf">
              📄 Export PDF
            </button>
          </div>
        </div>

        <!-- Document Content: Columns for Agents -->
        <div class="document-body">
          <div class="agents-grid" :style="{ gridTemplateColumns: `repeat(${agentColumns.length}, minmax(400px, 1fr))` }">
            <div v-for="agent in agentColumns" :key="agent.id" class="agent-column">
              <ChatPanel
                :agent-name="agent.name"
                :agent-id="agent.id"
                :agent-url="getAgentUrl(agent)"
                :provider="agent.provider_type"
                :model="getAgentModel(agent)"
                :results="getAgentResults(agent.id)"
                :messages-ref="createRef()"
                :message-refs="createRef({})"
                :readonly="false" 
                :on-scroll="() => {}"
                :on-retry="(idx) => handleRetry(agent.id, idx)"
                :on-run-all-for-agent="() => handleRunAllAgent(agent.id)"
                :on-validation="() => {}"
                :extract-answer-text="extractAnswerText"
                :extract-answer-meta="extractAnswerMeta"
                :get-question-key="getQuestionKey"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { ref } from 'vue'
import { wsService } from '../services/websocket.js'
import RunHistoryPanel from './RunHistoryPanel.vue'
import ChatPanel from './ChatPanel.vue'
import PrintReport from './PrintReport.vue'
import { extractAnswerText, extractAnswerMeta } from '../utils/chatHelpers.js'
import { exportResultsReport } from '../utils/exporters.js'
import { downloadManager } from '../services/DownloadManager.js'
import { contentCache } from '../services/ContentCache.js'
import { formatDuration } from '../utils/formatDuration.js'

export default {
  name: 'BenchmarkDocumentView',
  components: {
    RunHistoryPanel,
    ChatPanel,
    PrintReport
  },
  props: {
    workspaceId: {
      type: String,
      required: true
    },
    preFilter: {
      type: String,
      default: ''
    }
  },
  data() {
    return {
      selectedRunId: null,
      loading: false,
      runData: null,
      questionSetName: '',
      questionSetName: '',
      results: [],
      agents: {},
      error: null,
      showPrintView: false,
      printData: {}
    }
  },
  mounted() {
    wsService.on('DATA_RESULT_DETAILS', this.handleResultDetails)
  },
  unmounted() {
    wsService.off('DATA_RESULT_DETAILS', this.handleResultDetails)
    downloadManager.cancelAll()
  },
  computed: {
    agentColumns() {
      // Return list of agents involved in this run
      if (!this.runData || !this.runData.agents) return []
      
      // Sort agents: Evaluator last, then by position or name
      return Object.values(this.runData.agents).sort((a, b) => {
        if (a.provider_type === 'evaluator') return 1
        if (b.provider_type === 'evaluator') return -1
        return (a.position || 0) - (b.position || 0)
      })
    }
  },
  methods: {
    extractAnswerText,
    extractAnswerMeta,
    
    async handleSelectRun(runId) {
      if (this.selectedRunId === runId) return
      this.selectedRunId = runId
      await this.loadRunDetails(runId)
    },

    async loadRunDetails(runId) {
      this.loading = true
      this.error = null
      this.runData = null
      downloadManager.cancelAll()
      
      try {
        const data = await wsService.getRunLite(runId)
        this.runData = data
        this.questionSetName = data.question_set?.name || 'Unknown Question Set'
        
        // Map Lite results to expected structure with placeholders
        const allIds = []
        this.results = (data.results || []).map(r => {
           const cached = contentCache.get(r.content_hash)
           
           if (!cached) {
               allIds.push(r.id)
           }
           
           return {
               ...r,
               question_id: String(r.question_id),
               answer: cached ? cached.answer : '',
               metadata: {},
               loading: !cached,
               // If cached, we might want to restore evaluations too if available
               // Note: evaluations might need structure adjustment depending on cache format
           }
        })
        
        this.agents = data.agents || {}
        
        // Enqueue only non-cached IDs
        if (allIds.length > 0) {
            downloadManager.enqueue(allIds)
        }

      } catch (err) {
        console.error('Error loading run details:', err)
        this.error = 'Failed to load details'
      } finally {
        this.loading = false
      }
    },

    handleResultDetails(payload) {
      if (!payload.results || !this.results.length) return
      
      payload.results.forEach(detail => {
         const idx = this.results.findIndex(r => r.id === detail.id)
         if (idx !== -1) {
            const existing = this.results[idx]
            
            // Save to cache if we have hash
            if (existing.content_hash) {
                contentCache.set(existing.content_hash, {
                    answer: detail.answer,
                    evaluations: detail.evaluations
                })
            }
            
            this.results[idx] = {
               ...existing,
               ...detail,
               question_id: String(detail.question_id),
               loading: false
            }
         }
      })
    },

    getAgentResults(agentId) {
      if (!this.runData || !this.runData.question_set) return []
      
      // We need to reconstruct the Q&A list for this agent based on the Question Set structure
      // and the results we have.
      
      // 1. Get ordered questions from Question Set
      const qsData = this.runData.question_set.data
      let questions = []
      if (qsData && qsData.categories) {
        qsData.categories.forEach((cat, catIdx) => {
          if (cat.questions) {
            cat.questions.forEach((q, qIdx) => {
              // Generate ID matching backend format if not present
              const qId = q.id != null && q.id !== '' ? String(q.id) : `${catIdx + 1}-${qIdx + 1}`
              questions.push({ ...q, id: qId })
            })
          }
        })
      }
      
      // 2. Map results for this agent
      return questions.map(q => {
        // Find result for this question and agent
        const qIdStr = String(q.id)
        
        const result = this.results.find(r => 
          r.agent_id === agentId && 
          String(r.question_id) === qIdStr
        )
        
        return {
          question: q, // Pass full question object
          answer: result ? result.answer : null,
          loading: false,
          error: result && result.status === 'error' ? 'Error in run' : null,
          duration: result ? result.duration_ms / 1000 : null,
          timestamp: result ? result.created_at : null,
          humanValidation: null // We could load this from evaluations if backend provided it
        }
      })
    },

    getAgentUrl(agent) {
      if (!agent.config) return ''
      return agent.config.url || agent.config.prompt_id || ''
    },
    
    getAgentModel(agent) {
      if (!agent.config) return ''
      return agent.config.model || ''
    },

    createRef(initialValue = null) {
      return ref(initialValue)
    },

    getQuestionKey(questionObj) {
      // Helper to identify questions for history linkage
      // Simple hash or ID usage
      return typeof questionObj === 'object' ? String(questionObj.id) : questionObj
    },

    formatTime(ts) {
      if (!ts) return ''
      return new Date(ts).toLocaleString()
    },

    scrollToPrev() {
      // TODO: Implement scroll logic
    },

    scrollToNext() {
      // TODO: Implement scroll logic
    },

    exportToPdf() {
      if (!this.runData) return
      
      const agentsMap = this.runData.agents || {}
      // Transform map to array with results populated
      const agentsArray = Object.values(agentsMap).map(agent => ({
        id: agent.id,
        name: agent.config?.name || agent.name,
        provider: agent.provider_type,
        config: agent.config,
        results: this.getAgentResults(agent.id)
      }))

      const pData = exportResultsReport({
        agentsRef: agentsArray,
        calculateStats: (results) => this.calculateStats(results)
      })
      
      this.$emit('trigger-print', {
        workspaceName: '', // Fallback to currentWorkspace.name in App.vue
        summary: pData.summary,
        results: pData.results
      })
    },

    async handleRetry(agentId, index) {
      if (!this.runData || !this.runData.run) return
      
      const results = this.getAgentResults(agentId)
      const item = results[index]
      if (!item || !item.question) return
      
      const questionId = String(item.question.id)
      
      // Optimistic update: set loading
      this.updateResultState(agentId, questionId, { loading: true })
      
      // Also clear evaluator results for this question if any
      this.clearEvaluatorResult(questionId)

      try {
        await wsService.rerunTask(this.runData.run.id, agentId, questionId)
      } catch (e) {
        console.error('Failed to rerun task:', e)
        this.updateResultState(agentId, questionId, { loading: false, error: 'Failed to start' })
      }
    },

    async handleRunAllAgent(agentId) {
       if (!this.runData || !this.runData.run) return
       if (!confirm('Are you sure you want to re-run ALL questions for this agent? Previous answers will be overwritten.')) return

       const results = this.getAgentResults(agentId)
       
       // Loop through all questions
       for (const item of results) {
           if (item.question) {
               const qId = String(item.question.id)
               // Optimistic update
               this.updateResultState(agentId, qId, { loading: true })
               // Clear evaluation
               this.clearEvaluatorResult(qId)
               
               // Fire and forget (or await if we want sequential, but parallel is faster if backend supports it)
               // Backend probably handles queueing.
               wsService.rerunTask(this.runData.run.id, agentId, qId).catch(console.error)
           }
       }
    },

    updateResultState(agentId, questionId, updates) {
       const idx = this.results.findIndex(r => r.agent_id === agentId && String(r.question_id) === String(questionId))
       if (idx !== -1) {
           this.results[idx] = { ...this.results[idx], ...updates }
       } else {
           // If result didn't exist (e.g. error before), we might need to add a placeholder
           // But getAgentResults constructs from questions, so effectively we just need to make sure 
           // getAgentResults sees the loading state.
           // Since getAgentResults pulls from `this.results`, we need to add an entry if missing.
           this.results.push({
               id: 'temp-' + Date.now(),
               agent_id: agentId,
               question_id: questionId,
               ...updates
           })
       }
    },
    
    clearEvaluatorResult(questionId) {
        // Find evaluator agent(s)
        const evaluators = Object.values(this.runData.agents || {}).filter(a => a.provider_type === 'evaluator')
        
        evaluators.forEach(evalAgent => {
            // Find result for this question
            const idx = this.results.findIndex(r => r.agent_id === evalAgent.id && String(r.question_id) === String(questionId))
            if (idx !== -1) {
                // Clear answer/validation to indicate it needs re-run
                this.results[idx] = {
                    ...this.results[idx],
                    answer: '',
                    humanValidation: null,
                    status: 'pending' // custom status
                }
            }
        })
    },

    triggerBrowserPrint() {
      window.print()
    },

    calculateStats(results) {
      if (!results || !Array.isArray(results)) return {}
      
      let answered = 0
      let errors = 0
      let total = results.length
      let totalDuration = 0
      let validations = { positive: 0, negative: 0, alternative: 0, partial: 0, notEvaluated: 0 }
      
      results.forEach(r => {
          if (r.answer || r.error) answered++
          if (r.error) errors++
          if (r.duration) totalDuration += parseFloat(r.duration) || 0
          
          if (r.humanValidation) {
              const v = r.humanValidation.toLowerCase()
              validations[v] = (validations[v] || 0) + 1
          } else {
              validations.notEvaluated++
          }
      })
      
      return {
          answered,
          totalQuestions: total,
          errors,
          avgDuration: answered ? (totalDuration / answered).toFixed(2) : 0,
          validations,
          percentages: {
              positive: total ? Math.round((validations.positive || 0) / total * 100) : 0,
              negative: total ? Math.round((validations.negative || 0) / total * 100) : 0,
              alternative: total ? Math.round((validations.alternative || 0) / total * 100) : 0,
              partial: total ? Math.round((validations.partial || 0) / total * 100) : 0,
          }
      }
    }
  }
}
</script>

<style scoped>
.print-preview-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: white;
  z-index: 2000;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.print-toolbar {
  padding: 1rem;
  background: #343a40;
  color: white;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-shrink: 0;
}

.print-content {
  flex: 1;
  overflow-y: auto;
  background: #525659; /* PDF viewer grey */
  padding: 2rem;
  display: flex;
  justify-content: center;
}


.benchmark-document-view {
  display: flex;
  height: 100%; /* Fill parent (App layout) */
  background: white;
  overflow: hidden;
}

.sidebar {
  flex: 0 0 280px;
  border-right: 1px solid #e9ecef;
  background: #f8f9fa;
  display: flex;
  flex-direction: column;
}

.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.loading-container, .empty-selection {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #adb5bd;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid #f1f3f5;
  border-top-color: #49399d;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 1rem;
}

.empty-content {
  text-align: center;
}

.empty-content .icon {
  font-size: 3rem;
  margin-bottom: 1rem;
  opacity: 0.5;
}

.document-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.document-header {
  padding: 1rem 2rem;
  background: white;
  border-bottom: 1px solid #dee2e6;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 2px 4px rgba(0,0,0,0.02);
  z-index: 10;
}

.header-left h1 {
  margin: 0;
  font-size: 1.25rem;
  color: #212529;
}

.meta-badges {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.25rem;
}

.badge {
  font-size: 0.75rem;
  padding: 0.1rem 0.5rem;
  border-radius: 4px;
  background: #f1f3f5;
  color: #495057;
}

.badge.status.running { background: #e7f5ff; color: #1c7ed6; }
.badge.status.completed { background: #d3f9d8; color: #2b8a3e; }
.badge.status.completed_with_errors { background: #ffe3e3; color: #c92a2a; }
.badge.status.error { background: #ffe3e3; color: #c92a2a; }

.document-body {
  flex: 1;
  overflow-y: auto;
  overflow-x: auto; /* Horizontal scroll if many agents */
  padding: 2rem;
  background: #f8f9fa;
}

.agents-grid {
  display: grid;
  gap: 1.5rem;
  /* Columns defined dynamically in style binding */
}

.agent-column {
  min-width: 0; /* Prevent overflow */
}

.btn-action {
  padding: 0.5rem 1rem;
  background: #49399d;
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 500;
  transition: background 0.2s;
}

.btn-action:hover {
  background: #3b2e7e;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.nav-controls {
  display: flex;
  gap: 0.25rem;
}

.btn-nav {
  background: white;
  border: 1px solid #dee2e6;
  border-radius: 4px;
  padding: 0.25rem 0.5rem;
  cursor: pointer;
}

.btn-nav:hover {
  background: #f1f3f5;
}

.divider {
  width: 1px;
  height: 24px;
  background: #dee2e6;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
