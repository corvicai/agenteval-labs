<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-container compare-runs-modal">
      <header class="modal-header">
        <h2>📊 Compare Runs</h2>
        <button class="btn-close" @click="$emit('close')">×</button>
      </header>

      <section class="modal-body">
        <!-- Templates dropdown (Optional for F2, just an empty array for now or list) -->
        <div class="template-picker" v-if="templates && templates.length">
          <label>Load template:</label>
          <select v-model="selectedTemplateId" @change="onTemplateChange">
            <option :value="null">— None —</option>
            <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </div>

        <!-- QS scope filter -->
        <div class="scope-picker" v-if="allRuns.length > 0">
          <label>
            <input type="checkbox" v-model="onlyCurrentQS" :disabled="!currentQuestionSetId">
            Only show runs of the current Question Set
          </label>
        </div>

        <!-- QS mismatch warning -->
        <div v-if="hasMultipleQS" class="warning-banner">
          ⚠️ Selected runs use different Question Sets.
          Per-question comparison will only show overlapping questions.
          For best results, compare runs of the same Question Set.
        </div>

        <div v-if="fetchingRuns" class="loading">Loading runs...</div>

        <!-- Runs list with selection + labels -->
        <div v-else>
          <div v-if="filteredRuns.length === 0" class="empty-hint">
            No runs available{{ onlyCurrentQS ? ' for the current Question Set' : '' }}.
          </div>
          <div class="runs-grid" v-else>
            <div v-for="run in filteredRuns" :key="run.id"
                 class="run-row" :class="{ selected: isSelected(run.id) }">
              <input type="checkbox" :checked="isSelected(run.id)"
                     @change="toggleSelect(run.id)">
              <div class="run-meta">
                <strong>{{ run.question_set_name || 'Benchmark Run' }}</strong>
                <span class="time">{{ formatTime(run.created_at) }}</span>
                <span class="badge" :class="`badge-${run.status}`">{{ run.status }}</span>
              </div>
              <input v-if="isSelected(run.id)"
                     class="label-input"
                     placeholder="Label (DEV, UAT, PROD…)"
                     v-model="labels[run.id]">
            </div>
          </div>
        </div>

        <!-- Metrics toggles -->
        <div class="metrics-toggles">
          <label v-for="(label, key) in METRIC_LABELS" :key="key">
            <input type="checkbox" v-model="metricsEnabled[key]">
            {{ label }}
          </label>
        </div>
      </section>

      <footer class="modal-footer">
        <button class="btn btn-ghost" @click="$emit('close')">Cancel</button>
        <button class="btn btn-secondary" @click="saveTemplate" :disabled="!canGenerate">
          💾 Save as template
        </button>
        <button class="btn btn-primary" @click="doGenerate" :disabled="!canGenerate || loading">
          {{ loading ? 'Generating…' : 'Generate report →' }}
        </button>
      </footer>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { wsService } from '../services/websocket.js'
import { useRunComparison } from '../composables/useRunComparison.js'

const props = defineProps({
  workspaceId: String,
  currentQuestionSetId: { type: String, default: '' },
  currentQuestionSetName: { type: String, default: '' }
})

const emit = defineEmits(['close', 'report-generated'])

const {
  selectedRunIds, labels, metricsEnabled,
  report, loading, error, canGenerate,
  generate
} = useRunComparison()

const METRIC_LABELS = {
  totals: 'Totals',
  agent_scores: 'Agent Scores',
  latency: 'Latency',
  success_quality: 'Success & Quality',
  per_question: 'Per-Question Breakdown',
  regressions: 'Regression Detection'
}

const allRuns = ref([])
const fetchingRuns = ref(true)
const templates = ref([])
const selectedTemplateId = ref(null)
const onlyCurrentQS = ref(!!props.currentQuestionSetId)

onMounted(async () => {
  try {
    const data = await wsService.getWorkspaceRuns()
    // Defensive sort by created_at DESC — backend already returns DESC, but
    // this guards against any caching/ordering drift so the "most recent"
    // auto-selection below is always correct.
    const sorted = [...(data || [])].sort((a, b) => {
      const ta = new Date(a?.created_at || 0).getTime()
      const tb = new Date(b?.created_at || 0).getTime()
      return tb - ta
    })
    allRuns.value = sorted
    autoSelectMostRecent()
  } catch (e) {
    console.error('Failed to load runs', e)
  } finally {
    fetchingRuns.value = false
  }
})

// Preselect the most recent usable run so the user lands in a ready-to-go
// state. Priority: most recent COMPLETED run of the current Question Set;
// falls back to the most recent COMPLETED run overall; finally, the most
// recent run of any status. The user can then just tick a second run and
// hit "Generate report".
function autoSelectMostRecent() {
  if (selectedRunIds.value.length > 0) return
  if (allRuns.value.length === 0) return

  const byQSCompleted = props.currentQuestionSetId
    ? allRuns.value.find(r => r.status === 'completed' && r.question_set_id === props.currentQuestionSetId)
    : null
  const anyCompleted = allRuns.value.find(r => r.status === 'completed')
  const firstAny = allRuns.value[0]
  const target = byQSCompleted || anyCompleted || firstAny
  if (target) {
    selectedRunIds.value.push(target.id)
    labels.value[target.id] = defaultLabelFor(target)
  }
}

function defaultLabelFor(run) {
  if (!run?.created_at) return 'Run 1'
  const d = new Date(run.created_at)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const filteredRuns = computed(() => {
  if (!onlyCurrentQS.value || !props.currentQuestionSetId) {
    return allRuns.value
  }
  return allRuns.value.filter(r => r.question_set_id === props.currentQuestionSetId)
})

const hasMultipleQS = computed(() => {
  if (selectedRunIds.value.length < 2) return false
  const selected = allRuns.value.filter(r => selectedRunIds.value.includes(r.id))
  if (selected.length === 0) return false
  const firstQS = selected[0].question_set_id
  return selected.some(r => r.question_set_id !== firstQS)
})

function isSelected(id) {
  return selectedRunIds.value.includes(id)
}

function toggleSelect(id) {
  const idx = selectedRunIds.value.indexOf(id)
  if (idx > -1) {
    selectedRunIds.value.splice(idx, 1)
    delete labels.value[id]
  } else {
    const run = allRuns.value.find(r => r.id === id)
    selectedRunIds.value.push(id)
    labels.value[id] = labels.value[id] || defaultLabelFor(run)
  }
}

function formatTime(ts) {
  if (!ts) return ''
  return new Date(ts).toLocaleString()
}

function onTemplateChange() {
  // Not implemented for F2 logic but placeholder
}

function saveTemplate() {
  // Placeholder
  alert("Save template not implemented in F2 MVP")
}

async function doGenerate() {
  await generate()
  if (!error.value && report.value) {
    emit('report-generated', {
      report: report.value,
      selection: {
        runIds: [...selectedRunIds.value],
        labels: { ...labels.value }
      }
    })
  } else if (error.value) {
    alert("Error generating report: " + error.value)
  }
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.modal-container {
  background: white;
  border-radius: 8px;
  width: 90%;
  max-width: 600px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
}
.modal-header {
  padding: 1rem;
  border-bottom: 1px solid #ddd;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.modal-body {
  padding: 1rem;
  overflow-y: auto;
  flex: 1;
}
.runs-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 1rem;
}
.run-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border: 1px solid #eee;
  border-radius: 4px;
}
.run-row.selected {
  background: #f0f7ff;
  border-color: #cce0ff;
}
.run-meta {
  flex: 1;
  display: flex;
  gap: 10px;
  font-size: 0.85rem;
}
.label-input {
  width: 120px;
  padding: 4px;
}
.metrics-toggles {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 1rem;
}
.warning-banner {
  background: #fff3cd;
  color: #856404;
  padding: 10px;
  border-radius: 4px;
  margin-bottom: 10px;
  font-size: 0.85rem;
}
.scope-picker {
  margin-bottom: 10px;
  font-size: 0.85rem;
}
.empty-hint {
  padding: 20px;
  text-align: center;
  color: #777;
  border: 1px dashed #ddd;
  border-radius: 4px;
  margin-bottom: 10px;
}
.badge {
  padding: 2px 8px;
  border-radius: 10px;
  background: #eee;
  font-size: 0.75rem;
  text-transform: uppercase;
}
.badge-completed { background: #d4edda; color: #155724; }
.badge-running { background: #cce5ff; color: #004085; }
.badge-pending { background: #fff3cd; color: #856404; }
.badge-cancelled, .badge-failed, .badge-error { background: #f8d7da; color: #721c24; }
.modal-footer {
  padding: 1rem;
  border-top: 1px solid #ddd;
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
.btn {
  padding: 0.5rem 1rem;
  border-radius: 4px;
  border: 1px solid #ccc;
  cursor: pointer;
  background: white;
}
.btn-primary {
  background: #007bff;
  color: white;
  border-color: #007bff;
}
.btn-primary:disabled {
  background: #aaa;
  border-color: #aaa;
  cursor: not-allowed;
}
</style>
