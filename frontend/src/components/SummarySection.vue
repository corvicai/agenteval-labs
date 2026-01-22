<template>
  <div class="summary-section">
    <div class="summary-header">
      <h3>📊 Run Summary</h3>
      <button class="btn-close-summary" @click="$emit('close')">×</button>
    </div>
    <div class="summary-content">
      <div class="summary-grid">
        <!-- Stats Card -->
        <div class="summary-card">
          <h4>📈 Statistics</h4>
          <div class="stats-grid">
            <div class="stat">
              <span class="stat-label">Total</span>
              <span class="stat-value">{{ totalQuestions }}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Completed</span>
              <span class="stat-value">{{ completedCount }}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Errors</span>
              <span class="stat-value">{{ errorCount }}</span>
            </div>
          </div>
        </div>

        <!-- Agent Performance Card -->
        <div class="summary-card">
          <h4>🤖 Agent Performance</h4>
          <div class="agent-stats">
            <div v-for="agent in agents" :key="agent.id" class="agent-stat-row">
              <span class="agent-name">{{ agent.name }}</span>
              <div class="agent-metrics">
                <div class="metric-item" title="Success Runs Rate">
                  <span class="metric-label">Success</span>
                  <span class="metric-value">{{ getAgentSuccessRate(agent.id) }}%</span>
                </div>
                <div class="metric-item" :class="getScoreClass(getAgentPosRate(agent.id)/100)" title="User Positive Rating Rate">
                  <span class="metric-label">Quality</span>
                  <span class="metric-value">{{ getAgentPosRate(agent.id) }}%</span>
                </div>
                <div v-if="getAgentAvgScore(agent.id) > 0" class="metric-item score" title="Average Quality Score">
                  <span class="metric-label">Score</span>
                  <span class="metric-value">{{ getAgentAvgScore(agent.id) }}</span>
                </div>
                <div class="metric-item duration" title="Average Response Time">
                  <span class="metric-label">Latency</span>
                  <span class="metric-value">{{ getAgentAvgDuration(agent.id) }}ms</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Comparison Card -->
        <div class="summary-card comparison-card">
          <h4>⚡ Comparison</h4>
          <div class="comparison-items">
            <div class="comparison-item">
              <span class="comparison-label">Fastest Agent</span>
              <span class="comparison-value">{{ fastestAgent }}</span>
            </div>
            <div class="comparison-item">
              <span class="comparison-label">Most Accurate</span>
              <span class="comparison-value">{{ mostAccurateAgent }}</span>
            </div>
            <div class="comparison-item">
              <span class="comparison-label">Run Duration</span>
              <span class="comparison-value">{{ totalDuration }}s</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  run: Object,
  agents: Array,
  results: Object
})

defineEmits(['close'])

const totalQuestions = computed(() => {
  let count = 0
  for (const agentId in props.results) {
    count = Math.max(count, Object.keys(props.results[agentId]).length)
  }
  return count
})

const completedCount = computed(() => {
  let count = 0
  for (const agentId in props.results) {
    for (const qId in props.results[agentId]) {
      if (props.results[agentId][qId].success) count++
    }
  }
  return count
})

const errorCount = computed(() => {
  let count = 0
  for (const agentId in props.results) {
    for (const qId in props.results[agentId]) {
      if (!props.results[agentId][qId].success) count++
    }
  }
  return count
})

function getAgentSuccessRate(agentId) {
  const agentResults = props.results[agentId] || {}
  const total = Object.keys(agentResults).length
  if (total === 0) return 0
  const success = Object.values(agentResults).filter(r => r.success).length
  return Math.round((success / total) * 100)
}

function getAgentAvgDuration(agentId) {
  const agentResults = props.results[agentId] || {}
  const durations = Object.values(agentResults).map(r => r.duration_ms || 0)
  if (durations.length === 0) return 0
  return Math.round(durations.reduce((a, b) => a + b, 0) / durations.length)
}

function getAgentPosRate(agentId) {
  const agentResults = props.results[agentId] || {}
  const items = Object.values(agentResults)
  const total = items.length
  if (total === 0) return 0
  
  const positive = items.filter(r => {
    if (r.evaluations && r.evaluations.length > 0) {
      const userEval = r.evaluations.find(e => e.rater_type === 'user')
      if (userEval) {
        if (userEval.rating_code === 1 || userEval.rating_code === 2) return true
        return userEval.rating === 'like' || userEval.rating === 'valid'
      }
    }
    return r.humanValidation === 'positive' || r.humanValidation === 'like' || r.humanValidation === 'valid'
  }).length
  
  return Math.round((positive / total) * 100)
}

function getScoreClass(rate) {
  if (rate >= 0.8) return 'score-high'
  if (rate >= 0.5) return 'score-med'
  return 'score-low'
}

function getAgentAvgScore(agentId) {
  const agentResults = props.results[agentId] || {}
  let totalScore = 0
  let count = 0
  for (const qId in agentResults) {
    const res = agentResults[qId]
    if (res.evaluations && res.evaluations.length > 0) {
      const userEval = res.evaluations.find(e => e.rater_type === 'user')
      if (userEval && userEval.score !== undefined && userEval.score !== null) {
        totalScore += userEval.score
        count++
      }
    }
  }
  return count > 0 ? Math.round(totalScore / count) : 0
}

const fastestAgent = computed(() => {
  let fastest = null
  let minDuration = Infinity
  for (const agent of props.agents || []) {
    const avg = getAgentAvgDuration(agent.id)
    if (avg > 0 && avg < minDuration) {
      minDuration = avg
      fastest = agent.name
    }
  }
  return fastest || 'N/A'
})

const mostAccurateAgent = computed(() => {
  let best = null
  let maxRate = -1
  for (const agent of props.agents || []) {
    const rate = getAgentSuccessRate(agent.id)
    if (rate > maxRate) {
      maxRate = rate
      best = agent.name
    }
  }
  return best || 'N/A'
})

const totalDuration = computed(() => {
  let max = 0
  for (const agentId in props.results) {
    for (const qId in props.results[agentId]) {
      max += props.results[agentId][qId].duration_ms || 0
    }
  }
  return (max / 1000).toFixed(1)
})
</script>

<style scoped>
.summary-header h3 {
  margin: 0;
  color: #6366f1;
  font-size: 1.25rem;
}

.agent-stats {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.agent-stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.75rem;
  background: #f8fafc;
  border-radius: 6px;
}

.agent-name {
  font-weight: 600;
  color: #1e293b;
}

.agent-metric {
  font-size: 0.8rem;
  color: #64748b;
}

.agent-metrics {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.metric-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 50px;
  padding: 4px 8px;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  gap: 2px;
}

.metric-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  color: #94a3b8;
  font-weight: 600;
}

.metric-value {
  font-size: 0.85rem;
  font-weight: 700;
  color: #1e293b;
}

.score-high .metric-value { color: #10b981; }
.score-med .metric-value { color: #f59e0b; }
.score-low .metric-value { color: #ef4444; }

.metric-item.score {
  background: #f0f9ff;
  border-color: #bae6fd;
}
.metric-item.score .metric-value { color: #0284c7; }

.metric-item.duration .metric-value { color: #64748b; font-weight: 500; }

.summary-card h4 {
  color: #6366f1;
  border-bottom: 2px solid #e0e7ff;
}

.stat-value {
  color: #1e293b;
}

.stat-label {
  color: #64748b;
}
</style>
