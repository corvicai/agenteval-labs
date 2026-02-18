<template>
  <div class="run-history-panel">
    <div class="panel-header">
      <div class="header-top">
        <h3>History</h3>
        <button class="btn-refresh" @click="fetchRuns" title="Refresh">
          🔄
        </button>
      </div>
      <div class="filters">
        <div class="search-box">
          <input 
            v-model="searchQuery" 
            placeholder="Filter by name..." 
            class="search-input"
          />
          <button v-if="searchQuery || filterMissingEvals" class="btn-clear-inline" @click="clearFilters" title="Clear Filters">
            ✕
          </button>
        </div>
        <label class="checkbox-label">
          <input type="checkbox" v-model="filterMissingEvals">
          Pending Evals
        </label>
      </div>
    </div>
    
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>Loading runs...</span>
    </div>

    <div v-else-if="error" class="error-state">
      {{ error }}
      <button @click="fetchRuns">Retry</button>
    </div>

    <div v-else-if="filteredRuns.length === 0" class="empty-state">
      No runs match your filters.
    </div>

    <div v-else class="runs-list">
      <div 
        v-for="run in filteredRuns" 
        :key="run.id"
        class="run-item"
        :class="{ 'active': selectedRunId === run.id }"
        @click="$emit('select-run', run.id)"
      >
        <div class="run-info">
          <div class="run-title">
            {{ run.question_set_name || 'Benchmark Run' }}
          </div>
          <div class="run-meta">
            {{ formatTime(run.created_at) }}
          </div>
        </div>
        <div class="run-status">
          <span 
            class="status-badge"
            :class="getStatusClass(run.status)"
          >
            {{ run.status === 'completed_with_errors' ? 'Error' : run.status }}
          </span>
          <span class="result-count" v-if="run.result_count > 0">
            {{ run.result_count }} results
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { wsService } from '../services/websocket.js'

export default {
  name: 'RunHistoryPanel',
  props: {
    workspaceId: {
      type: String,
      required: true
    },
    selectedRunId: {
      type: String,
      default: null
    },
    preFilter: {
      type: String,
      default: ''
    }
  },
  data() {
    return {
      runs: [],
      loading: false,
      error: null,
      searchQuery: '',
      filterMissingEvals: false,
      pendingFetch: false,
      pendingFetchRetries: 0
    }
  },
  mounted() {
    wsService.on('connected', this.handleWSConnected)
  },
  unmounted() {
    wsService.off('connected', this.handleWSConnected)
  },
  computed: {
    filteredRuns() {
      if (!this.runs) return []
      
      return this.runs.filter(run => {
        // Name filter
        const name = run.question_set_name || 'Benchmark Run'
        const matchesName = name.toLowerCase().includes(this.searchQuery.toLowerCase())
        
        // Missing Evals filter (Not implemented fully in backend response yet, but placeholder logic)
        // Ideally we check if run.total_evaluations < run.total_questions * num_agents
        // For now, let's assume we don't have this field populated, so we rely on status.
        // If user wants to filter by "Pending Evals", maybe check if status is completed but has unrated items?
        // Or strictly strictly check if name matches. 
        // Let's implement name filter primarily.
        
        // If filterMissingEvals is true, we could check a mock property or metadata
        // For this task, we will just implement name filter robustly.
        if (this.filterMissingEvals) {
           // Placeholder: only show "completed" runs that might need review
           // In reality we need a "has_pending_evals" flag from backend
           if (run.status !== 'completed') return false
        }
        
        return matchesName
      })
    }
  },
  watch: {
    workspaceId: {
      immediate: true,
      handler(newVal) {
        if (newVal) this.fetchRuns()
      }
    },
    preFilter: {
      immediate: true,
      handler(newVal) {
        if (newVal) {
          this.searchQuery = newVal
        }
      }
    }
  },
  methods: {
    handleWSConnected() {
      if (this.pendingFetch && wsService.workspaceId === this.workspaceId) {
        this.pendingFetch = false
        this.fetchRuns()
      }
    },
    async fetchRuns() {
      if (!this.workspaceId) return
      
      if (!wsService.isConnected() || wsService.workspaceId !== this.workspaceId) {
        this.pendingFetch = true
        this.loading = true
        this.error = null
        return
      }

      this.loading = true
      this.error = null
      
      try {
        const data = await wsService.getWorkspaceRuns()
        this.runs = data || []
        this.pendingFetchRetries = 0
      } catch (err) {
        console.error('Failed to fetch runs:', err)
        const message = String(err?.message || '')
        // Retry if connection is still initializing or context is switching
        if (message.includes('no workspace') || message.includes('not authenticated') || message.includes('access denied')) {
          this.pendingFetch = true
          if (this.pendingFetchRetries < 5) {
            this.pendingFetchRetries += 1
            setTimeout(() => {
              if (!this.pendingFetch) return
              if (wsService.workspaceId === this.workspaceId) {
                this.pendingFetch = false
                this.fetchRuns()
              }
            }, 800) // Slightly longer wait
          }
          return
        }
        this.error = 'Failed to load history'
      } finally {
        if (!this.pendingFetch) {
          this.loading = false
        }
      }
    },
    formatTime(ts) {
      if (!ts) return ''
      return new Date(ts).toLocaleString(undefined, {
        month: 'short', 
        day: 'numeric',
        hour: '2-digit', 
        minute: '2-digit'
      })
    },
    getStatusClass(status) {
      switch (status) {
        case 'running': return 'status-running'
        case 'completed': return 'status-completed'
        case 'completed_with_errors': return 'status-failed' // Or a new class 'status-warning'
        case 'failed': return 'status-failed'
        case 'error': return 'status-failed' // Handle the raw 'error' status too
        default: return 'status-default'
      }
    },
    clearFilters() {
      this.searchQuery = ''
      this.filterMissingEvals = false
    }
  }
}
</script>

<style scoped>
.run-history-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #f8f9fa;
  border-right: 1px solid #dee2e6;
  width: 280px;
}

.panel-header {
  padding: 1rem;
  border-bottom: 1px solid #dee2e6;
  background: white;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.header-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filters {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.search-box {
  position: relative;
  display: flex;
  align-items: center;
}

.search-input {
  width: 100%;
  padding: 0.35rem 1.75rem 0.35rem 0.5rem;
  border: 1px solid #dee2e6;
  border-radius: 4px;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.search-input:focus {
  outline: none;
  border-color: #49399d;
  box-shadow: 0 0 0 2px rgba(73, 57, 157, 0.1);
}

.btn-clear-inline {
  position: absolute;
  right: 6px;
  background: #e9ecef;
  border: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  color: #adb5bd;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-clear-inline:hover {
  background: #dee2e6;
  color: #495057;
}

.checkbox-label {
  font-size: 0.75rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
  color: #495057;
  cursor: pointer;
}

.panel-header h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: #212529;
}

.btn-refresh {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 1rem;
  padding: 4px;
  border-radius: 4px;
}

.btn-refresh:hover {
  background: #f1f3f5;
}

.loading-state, .error-state, .empty-state {
  padding: 2rem;
  text-align: center;
  color: #6c757d;
  font-size: 0.875rem;
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid #dee2e6;
  border-top-color: #49399d;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.runs-list {
  flex: 1;
  overflow-y: auto;
}

.run-item {
  padding: 1rem;
  border-bottom: 1px solid #e9ecef;
  cursor: pointer;
  transition: background 0.2s;
  background: white;
}

.run-item:hover {
  background: #f8f9fa;
}

.run-item.active {
  background: #f3f0ff;
  border-left: 3px solid #49399d;
}

.run-info {
  display: flex;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.run-title {
  font-weight: 500;
  color: #343a40;
  font-size: 0.9rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}

.run-meta {
  font-size: 0.75rem;
  color: #868e96;
}

.run-status {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status-badge {
  font-size: 0.7rem;
  padding: 2px 6px;
  border-radius: 999px;
  text-transform: uppercase;
  font-weight: 600;
}

.status-running {
  background: #e7f5ff;
  color: #1c7ed6;
}

.status-completed {
  background: #d3f9d8;
  color: #2b8a3e;
}

.status-failed {
  background: #ffe3e3;
  color: #c92a2a;
}

.status-default {
  background: #f1f3f5;
  color: #495057;
}

.result-count {
  font-size: 0.75rem;
  color: #adb5bd;
}
</style>
