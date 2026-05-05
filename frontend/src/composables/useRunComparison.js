import { ref, computed } from 'vue'
import { comparisonService } from '../services/comparison.js'

export function useRunComparison() {
  const selectedRunIds = ref([])
  const labels = ref({})           // { [runId]: "DEV" }
  const metricsEnabled = ref({
    totals: true, agent_scores: true, latency: true,
    success_quality: true, per_question: true, regressions: true
  })
  const report = ref(null)
  const loading = ref(false)
  const error = ref(null)

  const canGenerate = computed(() => selectedRunIds.value.length >= 2)

  async function generate() {
    if (!canGenerate.value) return
    loading.value = true; error.value = null
    try {
      report.value = await comparisonService.compareRuns({
        runIds: selectedRunIds.value,
        labels: labels.value,
        metricsEnabled: metricsEnabled.value
      })
    } catch (e) { 
      error.value = e.message || 'Unknown error during report generation'
    } finally { 
      loading.value = false 
    }
  }

  async function saveSnapshot({ name, saveAsTemplate, templateName } = {}) {
    return comparisonService.createComparison({
      name: name || defaultName(),
      run_ids: selectedRunIds.value,
      labels: labels.value,
      metrics_enabled: metricsEnabled.value,
      save_as_template: !!saveAsTemplate,
      template_name: templateName
    })
  }

  function loadTemplate(template) {
    labels.value = template.config.default_labels || {}
    metricsEnabled.value = template.config.metrics_enabled || metricsEnabled.value
  }

  function defaultName() {
    const d = new Date().toISOString().slice(0, 16).replace('T', ' ')
    return `Comparison ${d}`
  }

  return {
    selectedRunIds, labels, metricsEnabled,
    report, loading, error, canGenerate,
    generate, saveSnapshot, loadTemplate
  }
}
