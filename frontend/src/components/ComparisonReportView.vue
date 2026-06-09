<template>
  <div class="comparison-report-view">
    <header class="view-header">
      <h1>{{ report.name || 'Comparison Report' }}</h1>
      <div class="actions">
        <button class="btn" @click="saveSnapshot">💾 Save snapshot</button>
        <button class="btn" @click="exportPDF">📄 Export PDF</button>
        <button class="btn btn-close" @click="$emit('close')">×</button>
      </div>
    </header>

    <div class="view-body">
      <div v-if="!report.same_question_set" class="warning-banner">
        ⚠️ These runs use different Question Sets. Per-Question and Regressions
        are limited to overlapping questions ({{ report.common_question_ids?.length || 0 }} common).
      </div>

      <section v-if="metricsEnabled.totals" class="section totals">
        <h2>Overview</h2>
        <div class="totals-grid">
          <div v-for="r in report.runs" :key="r.id" class="total-card">
            <div class="label-title">{{ r.label || r.name }}</div>
            <div v-if="r.question_set?.name" class="qs-name">{{ r.question_set.name }}</div>
            <div class="stat">Questions: <strong>{{ r.totals.questions }}</strong></div>
            <div class="stat">Completed: <strong>{{ r.totals.completed }}</strong></div>
            <div class="stat">Errors: <strong>{{ r.totals.errors }}</strong></div>
            <div class="stat">Duration: <strong>{{ formatDuration(r.totals.duration_ms) }}</strong></div>
          </div>
        </div>
      </section>

      <section v-if="metricsEnabled.agent_scores" class="section">
        <h2>Agent Scores</h2>
        <v-chart v-if="hasAgentData" class="chart-container" :option="agentScoresRadarOption"
                 autoresize style="height:400px; width: 100%;" />
        <div v-else class="empty-chart">No evaluated agents to display.</div>
      </section>

      <section v-if="metricsEnabled.latency" class="section">
        <h2>Latency</h2>
        <v-chart v-if="hasAgentData" class="chart-container" :option="latencyBarOption"
                 autoresize style="height:320px; width: 100%;" />
        <div v-else class="empty-chart">No latency data to display.</div>
      </section>

      <section v-if="metricsEnabled.success_quality" class="section">
        <h2>Success & Quality</h2>
        <div v-if="hasAgentData" class="twin-charts">
          <v-chart class="chart-container" :option="successRateOption"
                   autoresize style="height:320px; width: 100%;" />
          <v-chart class="chart-container" :option="qualityRateOption"
                   autoresize style="height:320px; width: 100%;" />
        </div>
        <div v-else class="empty-chart">No agent data to display.</div>
      </section>

      <section v-if="metricsEnabled.per_question" class="section">
        <h2>Per-Question Breakdown</h2>
        <div v-if="heatmapData.values.length" class="heatmap-wrap">
          <v-chart class="chart-container" :option="heatmapOption"
                   autoresize :style="heatmapStyle" />
          <p class="hint">
            Shows average score (1–5) per question across runs. Only the
            {{ report.common_question_ids?.length || 0 }} question(s) present in every
            selected run are included.
          </p>
        </div>
        <div v-else class="empty-chart">
          No overlapping evaluated questions between the selected runs.
        </div>
      </section>

      <section v-if="metricsEnabled.regressions" class="section">
        <h2>Regressions</h2>
        <div v-if="regressions.length" class="regressions-wrap">
          <p class="hint">
            {{ regressions.length }} regression(s) detected where the score dropped by 1 point or more
            compared to the previous run.
          </p>
          <table class="regressions-table">
            <thead>
              <tr>
                <th>Question</th>
                <th>Agent</th>
                <th>From</th>
                <th>To</th>
                <th>Δ</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(r, idx) in regressions" :key="idx">
                <td><code>{{ shortQuestion(r.question_id) }}</code></td>
                <td>{{ agentName(r.agent_id) }}</td>
                <td>{{ r.from_label }} ({{ formatScore(r.from_score) }})</td>
                <td>{{ r.to_label }} ({{ formatScore(r.to_score) }})</td>
                <td :class="deltaClass(r.delta)">{{ r.delta.toFixed(2) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-chart">No regressions detected.</div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import VChart from 'vue-echarts'

const props = defineProps({
  report: {
    type: Object,
    required: true
  },
  selection: {
    type: Object,
    required: true
  }
})

defineEmits(['close'])

const metricsEnabled = computed(() => props.report.metrics_enabled || {})

const uniqueAgents = computed(() => {
  const agentsMap = new Map()
  ;(props.report.runs || []).forEach((run) => {
    ;(run.agents || []).forEach((agent) => {
      if (!agentsMap.has(agent.id)) {
        agentsMap.set(agent.id, agent)
      }
    })
  })
  return Array.from(agentsMap.values())
})

const hasAgentData = computed(() => uniqueAgents.value.length > 0)

const regressions = computed(() => props.report.regressions || [])

function formatDuration(ms) {
  if (!ms) return '0s'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return (ms / 1000).toFixed(1) + 's'
}

function formatScore(v) {
  if (v === null || v === undefined) return '—'
  return Number(v).toFixed(2)
}

function shortQuestion(q) {
  if (!q) return ''
  return q.length > 36 ? q.slice(0, 33) + '…' : q
}

function agentName(id) {
  const a = uniqueAgents.value.find((x) => x.id === id)
  return a?.name || id?.slice(0, 8) || '—'
}

function deltaClass(d) {
  if (d <= -2) return 'delta-critical'
  if (d <= -1) return 'delta-warn'
  return ''
}

function saveSnapshot() {
  alert('Save snapshot not fully implemented yet in MVP')
}

function exportPDF() {
  window.print()
}

const agentScoresRadarOption = computed(() => {
  const agents = uniqueAgents.value
  const indicators = agents.map((a) => ({ name: a.name, max: 5 }))
  const series = (props.report.runs || []).map((run) => ({
    name: run.label || run.name,
    value: agents.map((a) => {
      const block = (run.agents || []).find((x) => x.id === a.id)
      return block?.avg_score ?? 0
    })
  }))
  return {
    tooltip: {},
    legend: { data: series.map((s) => s.name), bottom: 0 },
    radar: {
      indicator: indicators.length ? indicators : [{ name: '—', max: 5 }],
      shape: 'polygon'
    },
    series: [{ type: 'radar', data: series }]
  }
})

const latencyBarOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: (props.report.runs || []).map((r) => r.label || r.name), bottom: 0 },
  xAxis: { type: 'category', data: uniqueAgents.value.map((a) => a.name) },
  yAxis: { type: 'value', name: 'Latency (ms)' },
  series: (props.report.runs || []).map((run) => ({
    name: run.label || run.name,
    type: 'bar',
    data: uniqueAgents.value.map((a) =>
      Math.round((run.agents || []).find((x) => x.id === a.id)?.avg_latency_ms || 0)
    )
  }))
}))

function buildPercentBar(title, pick) {
  return {
    title: { text: title, left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis', valueFormatter: (v) => `${Number(v).toFixed(1)}%` },
    legend: { data: (props.report.runs || []).map((r) => r.label || r.name), bottom: 0 },
    xAxis: { type: 'category', data: uniqueAgents.value.map((a) => a.name) },
    yAxis: { type: 'value', name: '%', min: 0, max: 100 },
    series: (props.report.runs || []).map((run) => ({
      name: run.label || run.name,
      type: 'bar',
      data: uniqueAgents.value.map((a) => {
        const block = (run.agents || []).find((x) => x.id === a.id)
        return Number(pick(block) || 0).toFixed(1)
      })
    }))
  }
}

const successRateOption = computed(() => buildPercentBar('Success Rate', (b) => b?.success_rate))
const qualityRateOption = computed(() => buildPercentBar('Quality Rate (likes + valid)', (b) => b?.quality_rate))

// Per-question heatmap.
// y-axis = runs (one row per run), x-axis = question_id (only the common ones).
// Value = average score across all agents for that (run, question). Cells with
// no score are rendered blank so "no data" is visually distinct from "0".
const heatmapData = computed(() => {
  const common = props.report.common_question_ids || []
  const runs = props.report.runs || []
  const values = []
  runs.forEach((run, rIdx) => {
    common.forEach((qid, qIdx) => {
      const agentScores = (run.per_question || []).filter((pq) => pq.question_id === qid && pq.score != null)
      if (!agentScores.length) return
      const avg = agentScores.reduce((s, pq) => s + pq.score, 0) / agentScores.length
      values.push([qIdx, rIdx, Number(avg.toFixed(2))])
    })
  })
  return {
    xLabels: common.map(shortQuestion),
    yLabels: runs.map((r) => r.label || r.name),
    values
  }
})

const heatmapStyle = computed(() => {
  const cols = heatmapData.value.xLabels.length
  const rows = heatmapData.value.yLabels.length
  const h = Math.max(180, rows * 48 + 120)
  const w = Math.max(600, cols * 32 + 160)
  return { height: `${h}px`, width: '100%', minWidth: `${w}px` }
})

const heatmapOption = computed(() => ({
  tooltip: {
    position: 'top',
    formatter: (p) =>
      `${heatmapData.value.yLabels[p.value[1]]}<br/>${heatmapData.value.xLabels[p.value[0]]}<br/><b>${p.value[2]}</b> / 5`
  },
  grid: { left: 120, right: 40, top: 40, bottom: 80, containLabel: true },
  xAxis: {
    type: 'category',
    data: heatmapData.value.xLabels,
    splitArea: { show: true },
    axisLabel: { rotate: 45, interval: 0, fontSize: 10 }
  },
  yAxis: {
    type: 'category',
    data: heatmapData.value.yLabels,
    splitArea: { show: true }
  },
  visualMap: {
    min: 1,
    max: 5,
    calculable: true,
    orient: 'horizontal',
    left: 'center',
    bottom: 10,
    inRange: { color: ['#f8d7da', '#fff3cd', '#d4edda'] }
  },
  series: [
    {
      name: 'Avg score',
      type: 'heatmap',
      data: heatmapData.value.values,
      label: { show: heatmapData.value.xLabels.length <= 30, fontSize: 10 },
      emphasis: { itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0, 0, 0, 0.5)' } }
    }
  ]
}))
</script>

<style scoped>
.comparison-report-view {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: #f8f9fa;
  z-index: 1050;
  display: flex;
  flex-direction: column;
}
.view-header {
  padding: 1rem 2rem;
  background: white;
  border-bottom: 1px solid #dee2e6;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.view-header h1 {
  margin: 0;
  font-size: 1.5rem;
}
.actions {
  display: flex;
  gap: 1rem;
}
.btn {
  padding: 0.5rem 1rem;
  border: 1px solid #ced4da;
  background: white;
  border-radius: 4px;
  cursor: pointer;
}
.btn:hover {
  background: #e9ecef;
}
.view-body {
  flex: 1;
  padding: 2rem;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2rem;
}
.section {
  background: white;
  padding: 1.5rem;
  border-radius: 8px;
  border: 1px solid #dee2e6;
}
.section h2 {
  margin-top: 0;
  margin-bottom: 1rem;
  font-size: 1.25rem;
  border-bottom: 1px solid #eee;
  padding-bottom: 0.5rem;
}
.totals-grid {
  display: flex;
  gap: 1.5rem;
  flex-wrap: wrap;
}
.total-card {
  flex: 1;
  min-width: 200px;
  background: #f8f9fa;
  padding: 1rem;
  border-radius: 6px;
  border: 1px solid #e9ecef;
}
.label-title {
  font-weight: bold;
  margin-bottom: 0.25rem;
  color: #49399d;
  font-size: 1.1rem;
}
.qs-name {
  font-size: 0.75rem;
  color: #888;
  margin-bottom: 0.5rem;
}
.stat {
  font-size: 0.9rem;
  margin-bottom: 0.25rem;
}
.chart-container {
  min-height: 300px;
}
.twin-charts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 1rem;
}
.empty-chart {
  padding: 20px;
  text-align: center;
  color: #888;
  background: #fafbfc;
  border: 1px dashed #ddd;
  border-radius: 4px;
}
.heatmap-wrap { overflow-x: auto; }
.hint {
  font-size: 0.8rem;
  color: #666;
  margin-top: 0.5rem;
}
.warning-banner {
  background: #fff3cd;
  color: #856404;
  padding: 10px 14px;
  border-radius: 6px;
  border: 1px solid #ffeeba;
}
.regressions-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
}
.regressions-table th, .regressions-table td {
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #eee;
  text-align: left;
}
.regressions-table th { background: #f8f9fa; font-weight: 600; }
.regressions-table code {
  background: #f1f3f5;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 0.85rem;
}
.delta-warn { color: #c99500; font-weight: 600; }
.delta-critical { color: #dc3545; font-weight: 700; }
</style>
