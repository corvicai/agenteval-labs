<template>
  <div class="stats-container">
    <div class="stats-header">
      <div class="title-group">
        <h2>📊 Performance Analytics</h2>
        <p class="subtitle">Your workspace and organization insights.</p>
      </div>
      <div class="stats-controls">
        <div class="stats-tabs">
          <button 
            :class="{ active: currentTab === 'workspace' }" 
            @click="currentTab = 'workspace'"
          >Workspace</button>
          <button 
            :class="{ active: currentTab === 'organization' }" 
            @click="currentTab = 'organization'"
          >Organization</button>
          <button 
            v-if="isAdmin"
            :class="{ active: currentTab === 'global' }" 
            @click="currentTab = 'global'"
          >Global Scope</button>
          <button 
            :class="{ active: currentTab === 'charts' }" 
            @click="currentTab = 'charts'"
          >📉 Quality Charts</button>
        </div>
        <button class="btn-refresh" @click="forceRefresh" :disabled="loading" style="margin-right: 8px;">
          🔄 {{ loading ? 'Loading...' : 'Refresh' }}
        </button>
        <button v-if="currentTab === 'workspace'" class="btn-danger-text" @click="clearHistory" :disabled="loading" style="border: 1px solid #e2e8f0; border-radius: 6px; padding: 0.5rem 1rem; cursor: pointer; background: white;">
          🗑️ Clear All History
        </button>
      </div>
    </div>

    <!-- Cache info -->
    <div v-if="stats && stats.computed_at" class="cache-info">
      <span v-if="stats.cache_hit" class="cache-badge cache-hit">⚡ Cached</span>
      <span v-else class="cache-badge cache-miss">🔥 Fresh</span>
      <span class="cache-time">Last updated: {{ formatDateRelative(stats.computed_at) }}</span>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>Aggregating performance data...</p>
    </div>

    <div v-else-if="stats" class="stats-content">
      <!-- Summary Cards -->
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-icon-wrapper runs">📊</div>
          <div class="stat-info">
            <div class="stat-label">Total Runs</div>
            <div class="stat-value">{{ stats.total_runs }}</div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon-wrapper tasks">✅</div>
          <div class="stat-info">
            <div class="stat-label">Processed Tasks</div>
            <div class="stat-value">{{ stats.total_results }}</div>
          </div>
        </div>
        <div class="stat-card" :class="getScoreClass(stats.success_rate)">
          <div class="stat-icon-wrapper success">🎯</div>
          <div class="stat-info">
            <div class="stat-label">Success Runs Rate</div>
            <div class="stat-value">{{ (stats.success_rate * 100).toFixed(1) }}%</div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon-wrapper speed">⏱️</div>
          <div class="stat-info">
            <div class="stat-label">Avg Duration</div>
            <div class="stat-value">{{ formatDuration(stats.avg_duration_ms / 1000) }}</div>
          </div>
        </div>
      </div>

      <!-- Evaluation Summary Cards -->
      <div class="section-header" style="margin-top: 24px;">
        <h3>Evaluation Metrics</h3>
      </div>
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-icon-wrapper evals">📝</div>
          <div class="stat-info">
            <div class="stat-label">Total Evaluations</div>
            <div class="stat-value">{{ stats.total_evaluations || 0 }}</div>
            <div class="stat-subtext" v-if="stats.total_evaluations > 0">
              {{ stats.likes_count }} likes, {{ stats.valids_count }} valids
            </div>
          </div>
        </div>
        <div class="stat-card" :class="getScoreClass(stats.positive_rate)">
          <div class="stat-icon-wrapper quality">💎</div>
          <div class="stat-info">
            <div class="stat-label">Positive Rate</div>
            <div class="stat-value">{{ (stats.positive_rate * 100).toFixed(1) }}%</div>
          </div>
        </div>
        <div class="stat-card" :class="getScoreClass(1 - stats.negative_rate)">
          <div class="stat-icon-wrapper errors">⚠️</div>
          <div class="stat-info">
            <div class="stat-label">Negative Rate</div>
            <div class="stat-value">{{ (stats.negative_rate * 100).toFixed(1) }}%</div>
            <div class="stat-subtext" v-if="stats.total_evaluations > 0">
              {{ stats.dislikes_count }} dislikes, {{ stats.wrongs_count }} wrongs
            </div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon-wrapper score">⭐</div>
          <div class="stat-info">
            <div class="stat-label">Avg Quality Score</div>
            <div class="stat-value">{{ stats.avg_score != null ? stats.avg_score.toFixed(1) : 0 }}</div>
          </div>
        </div>
      </div>

      <!-- Agent Performance Table -->
      <div class="agent-stats-section">
        <div class="section-header">
          <h3>Agent Performance Metrics</h3>
        </div>
        
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>Agent</th>
                <th v-if="activeScope === 'global'">Organization</th>
                <th>Owner</th>
                <th>Run Count</th>
                <th>Success Runs Rate</th>
                <th>Eval Count</th>
                <th>Pos Rate</th>
                <th>Avg Score</th>
                <th>Avg Latency</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="agent in paginatedAgents" :key="agent.agent_id">
                <td class="agent-name-cell">
                  <div class="agent-icon">🤖</div>
                  {{ agent.agent_name }}
                </td>
                <td v-if="activeScope === 'global'" class="owner-cell">
                  {{ agent.org_name || 'System' }}
                </td>
                <td class="owner-cell">
                  {{ agent.owner || 'System' }}
                </td>
                <td class="count-cell">{{ agent.count }}</td>
                <td>
                  <div class="score-badge" :class="getScoreBadgeClass(agent.success_rate)">
                    {{ (agent.success_rate * 100).toFixed(0) }}%
                  </div>
                </td>
                <td class="count-cell">{{ agent.total_evaluations || 0 }}</td>
                <td>
                  <div v-if="agent.total_evaluations > 0" class="score-badge" :class="getScoreBadgeClass(agent.positive_rate)">
                    {{ (agent.positive_rate * 100).toFixed(0) }}%
                  </div>
                  <span v-else>-</span>
                </td>
                <td class="count-cell">{{ agent.total_evaluations > 0 ? agent.avg_score.toFixed(1) : '-' }}</td>
                <td class="latency-cell">{{ formatDuration(agent.avg_duration_ms / 1000) }}</td>
                <td class="latency-cell">{{ formatDate(agent.created_at) }}</td>
              </tr>
            </tbody>
          </table>
          
          <div v-if="totalPages > 1" class="pagination-controls">
            <button class="page-btn" @click="prevPage" :disabled="currentPage === 1">Prev</button>
            <span class="page-info">Page {{ currentPage }} of {{ totalPages }}</span>
            <button class="page-btn" @click="nextPage" :disabled="currentPage === totalPages">Next</button>
          </div>
        </div>
      </div>

      <!-- Charts View -->
      <div v-if="currentTab === 'charts'" class="charts-view">
        <div class="chart-card">
          <h3>Response Quality Over Time (Avg Score)</h3>
          <div v-if="!stats.history || stats.history.length === 0" class="no-chart-data">
             No historical data available for this period. 
          </div>
          <div v-else class="chart-container">
            <svg viewBox="0 0 800 300" class="trend-chart">
               <line x1="50" y1="50" x2="750" y2="50" stroke="#e2e8f0" stroke-dasharray="4" />
               <line x1="50" y1="150" x2="750" y2="150" stroke="#e2e8f0" stroke-dasharray="4" />
               <line x1="50" y1="250" x2="750" y2="250" stroke="#cbd5e1" stroke-width="2" />
               <path :d="generatePath(stats.history)" fill="none" stroke="#6366f1" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
               <circle v-for="(p, i) in stats.history" :key="i" :cx="getX(i, stats.history.length)" :cy="getY(p.avg_score, chartMax)" r="5" fill="#6366f1">
                 <title>{{ p.date }}: Score {{ p.avg_score.toFixed(1) }} ({{ p.count }} evals)</title>
               </circle>
               <text v-for="(p, i) in getSparseHistory(stats.history)" :key="'label-'+i" :x="getX(p.index, stats.history.length)" y="275" text-anchor="middle" class="chart-label">
                 {{ formatDateDisplay(p.date) }}
               </text>
            </svg>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="empty-state">
      <div class="empty-icon">📈</div>
      <p>No benchmark data available yet for this scope.</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, computed } from 'vue';
import wsService from '../services/websocket.js';
import { useWSStore } from '../stores/wsStore';
import { formatDuration } from '../utils/formatDuration.js';

const { state: wsState } = useWSStore();

const props = defineProps({
  workspaceId: String,
  isAdmin: Boolean
});

const currentTab = ref('workspace');
const lastScope = ref('workspace');
const activeScope = computed(() => {
  return currentTab.value === 'charts' ? lastScope.value : currentTab.value;
});
const stats = ref(null);
const loading = ref(false);

const fetchStats = async (force = false) => {
  loading.value = true;
  try {
    let res;
    const scope = activeScope.value;
    if (scope === 'workspace') {
      res = await wsService.getWorkspaceStats(props.workspaceId, force);
    } else if (scope === 'organization') {
      res = await wsService.getOrganizationStats(force);
    } else if (scope === 'global') {
      res = await wsService.getGlobalStats(force);
    }
    stats.value = res;
  } catch (err) {
    console.error('Failed to fetch stats:', err);
    stats.value = null;
  } finally {
    loading.value = false;
  }
};

const forceRefresh = () => {
  fetchStats(true);
};

const clearHistory = async () => {
  if (!props.workspaceId) return;
  if (!confirm('Are you sure you want to PERMANENTLY delete all benchmark history for this workspace? This cannot be undone.')) return;

  loading.value = true;
  try {
    await wsService.deleteAllRuns();
    fetchStats(true);
    // Notify store if needed, but syncState will run on fresh connect or we can trigger it
    const { syncState } = useWSStore();
    syncState();
  } catch (err) {
    console.error('Failed to clear history:', err);
    alert('Failed to clear history: ' + err.message);
  } finally {
    loading.value = false;
  }
};

const currentPage = ref(1);
const itemsPerPage = 10;

const totalPages = computed(() => {
  if (!stats.value || !stats.value.agents) return 0;
  return Math.ceil(stats.value.agents.length / itemsPerPage);
});

const paginatedAgents = computed(() => {
  if (!stats.value || !stats.value.agents) return [];
  const start = (currentPage.value - 1) * itemsPerPage;
  const end = start + itemsPerPage;
  return stats.value.agents.slice(start, end);
});

const nextPage = () => {
  if (currentPage.value < totalPages.value) {
    currentPage.value++;
  }
};

const prevPage = () => {
  if (currentPage.value > 1) {
    currentPage.value--;
  }
};

const getScoreClass = (rate) => {
  if (rate >= 0.8) return 'score-high';
  if (rate >= 0.5) return 'score-med';
  return 'score-low';
};

const getScoreBadgeClass = (rate) => {
  if (rate >= 0.8) return 'badge-high';
  if (rate >= 0.5) return 'badge-med';
  return 'badge-low';
};

const formatDate = (dateStr) => {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  return date.toLocaleDateString('en-US', { 
    year: 'numeric', 
    month: 'short', 
    day: 'numeric' 
  });
};

const formatDateRelative = (dateStr) => {
  if (!dateStr) return 'Unknown';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now - date;
  const diffMins = Math.floor(diffMs / 60000);
  
  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return formatDate(dateStr);
};

// Chart helpers
const getX = (index, total) => {
  if (total <= 1) return 400;
  return 50 + (index * (700 / (total - 1)));
};

const getY = (score, max) => {
  const safeMax = max > 0 ? max : 10;
  const val = Math.max(0, Math.min(score, safeMax));
  return 250 - (val * (200 / safeMax));
};

const generatePath = (history) => {
  if (!history || history.length < 2) return '';
  const max = getChartMax(history);
  return history.map((p, i) => {
    const x = getX(i, history.length);
    const y = getY(p.avg_score, max);
    return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
  }).join(' ');
};

const getSparseHistory = (history) => {
  if (!history) return [];
  if (history.length <= 5) return history.map((h, i) => ({ ...h, index: i }));
  const result = [];
  const step = Math.floor(history.length / 5);
  for (let i = 0; i < history.length; i += step) {
    result.push({ ...history[i], index: i });
  }
  return result;
};

const formatDateDisplay = (dateStr) => {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
};

const getChartMax = (history) => {
  if (!history || history.length === 0) return 10;
  const maxValue = Math.max(...history.map((p) => p.avg_score || 0), 0);
  if (maxValue <= 10) return 10;
  if (maxValue <= 100) return 100;
  return maxValue;
};

const chartMax = computed(() => getChartMax(stats.value?.history));

onMounted(() => {
  if (wsState.isConnected) {
    fetchStats(false);
  }
});

watch(currentTab, (newTab) => {
  if (newTab !== 'charts') {
    lastScope.value = newTab;
  }
});

watch(() => wsState.isConnected, (connected) => {
  if (connected) {
    fetchStats(false);
  }
});

watch([() => activeScope.value, () => props.workspaceId], () => {
  if (wsState.isConnected) {
    fetchStats(false);
  }
});
</script>

<style scoped>
.stats-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
  color: var(--text-primary, #1e293b);
}

.stats-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 20px;
  flex-wrap: wrap;
  gap: 16px;
}

.stats-controls {
  display: flex;
  align-items: center;
  gap: 12px;
}

.title-group h2 {
  margin: 0;
  font-size: 1.5rem;
  color: #1e293b;
}

.subtitle {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 0.9rem;
}

.stats-tabs {
  display: flex;
  gap: 4px;
  background: #f1f5f9;
  padding: 4px;
  border-radius: 8px;
}

.stats-tabs button {
  padding: 8px 16px;
  border: none;
  background: transparent;
  color: #64748b;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.9rem;
  transition: all 0.2s;
}

.stats-tabs button:hover {
  background: #e2e8f0;
}

.stats-tabs button.active {
  background: white;
  color: #1e293b;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.btn-refresh {
  padding: 8px 16px;
  border: 1px solid #e2e8f0;
  background: white;
  color: #64748b;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.9rem;
  transition: all 0.2s;
}

.btn-refresh:hover:not(:disabled) {
  background: #f8fafc;
  border-color: #6366f1;
  color: #6366f1;
}

.btn-refresh:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.cache-info {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 0.85rem;
}

.cache-badge {
  padding: 4px 10px;
  border-radius: 12px;
  font-weight: 600;
  font-size: 0.75rem;
}

.cache-hit {
  background: #dbeafe;
  color: #2563eb;
}

.cache-miss {
  background: #fef3c7;
  color: #d97706;
}

.cache-time {
  color: #64748b;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 20px;
}

.stat-card {
  background: white;
  padding: 24px;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  display: flex;
  align-items: flex-start;
  gap: 20px;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -1px rgba(0, 0, 0, 0.03);
  transition: transform 0.2s, box-shadow 0.2s;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.08);
}

.stat-icon-wrapper {
  width: 54px;
  height: 54px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
  flex-shrink: 0;
}

.stat-icon-wrapper.runs { background: #eef2ff; color: #6366f1; }
.stat-icon-wrapper.tasks { background: #f0fdf4; color: #22c55e; }
.stat-icon-wrapper.success { background: #ecfdf5; color: #10b981; }
.stat-icon-wrapper.speed { background: #fff7ed; color: #f97316; }
.stat-icon-wrapper.evals { background: #fdf2f8; color: #ec4899; }
.stat-icon-wrapper.quality { background: #f0f9ff; color: #0ea5e9; }
.stat-icon-wrapper.errors { background: #fff1f2; color: #f43f5e; }
.stat-icon-wrapper.score { background: #fffbeb; color: #eab308; }

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-label {
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.075em;
  color: #64748b;
  font-weight: 600;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 800;
  color: #1e293b;
  line-height: 1;
}

.score-high .stat-value { color: #10b981; }
.score-med .stat-value { color: #f59e0b; }
.score-low .stat-value { color: #ef4444; }

.stat-subtext {
  font-size: 0.7rem;
  color: #94a3b8;
  margin-top: 6px;
  font-weight: 500;
}

.section-header {
  margin-bottom: 16px;
}

.section-header h3 {
  font-size: 1.1rem;
  font-weight: 500;
  margin: 0;
  color: #1e293b;
}

.table-container {
  background: white;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th {
  background: #f8fafc;
  padding: 12px 16px;
  text-align: left;
  font-size: 0.75rem;
  text-transform: uppercase;
  color: #64748b;
  letter-spacing: 0.05em;
}

td {
  padding: 16px;
  border-bottom: 1px solid #e2e8f0;
  color: #1e293b;
}

.agent-name-cell {
  display: flex;
  align-items: center;
  gap: 12px;
  font-weight: 600;
}

.agent-icon {
  width: 28px;
  height: 28px;
  background: #f1f5f9;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.9rem;
}

.score-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 12px;
  border-radius: 20px;
  font-weight: 700;
  font-size: 0.85rem;
  min-width: 60px;
}

.badge-high { background: #d1fae5; color: #059669; }
.badge-med { background: #fef3c7; color: #d97706; }
.badge-low { background: #fee2e2; color: #dc2626; }

.owner-cell {
  color: #64748b;
  font-size: 0.9rem;
}

.count-cell, .latency-cell {
  color: #64748b;
  font-weight: 500;
}

.loading-state, .empty-state {
  padding: 100px 0;
  text-align: center;
  color: #64748b;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 16px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-icon {
  font-size: 3rem;
  margin-bottom: 16px;
  opacity: 0.3;
}

.pagination-controls {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 16px;
  gap: 16px;
  border-top: 1px solid #e2e8f0;
}

.page-btn {
  padding: 8px 16px;
  border: 1px solid #e2e8f0;
  background: white;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
  color: #64748b;
  transition: all 0.2s;
}

.page-btn:hover:not(:disabled) {
  background: #f1f5f9;
  color: #1e293b;
  border-color: #cbd5e1;
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.page-info {
  font-size: 0.9rem;
  color: #64748b;
}
/* Charts styles */
.charts-view {
  display: flex;
  flex-direction: column;
  gap: 24px;
  margin-top: 24px;
}

.chart-card {
  background: white;
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
}

.chart-card h3 {
  margin-top: 0;
  margin-bottom: 24px;
  font-size: 1.1rem;
  color: #1e293b;
}

.chart-container {
  width: 100%;
  height: 300px;
}

.trend-chart {
  width: 100%;
  height: 100%;
  overflow: visible;
}

.chart-point:hover {
  r: 8;
  cursor: pointer;
}

.chart-label {
  font-size: 10px;
  fill: #64748b;
}

.no-chart-data {
  height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #94a3b8;
  font-style: italic;
  border: 1px dashed #e2e8f0;
  border-radius: 8px;
}

.chart-summary {
  background: #f8fafc;
  border-radius: 12px;
  padding: 20px;
  border-left: 4px solid #6366f1;
}

.summary-info h4 {
  margin-top: 0;
  color: #1e293b;
}

.summary-info p {
  color: #64748b;
  margin-bottom: 0;
  line-height: 1.5;
}
</style>
