import { ref } from 'vue'

const DEFAULT_RETRY_TRACK_TTL_MS = 20 * 60 * 1000

function resolveValue(valueOrFn, fallback = '') {
  if (typeof valueOrFn === 'function') {
    try {
      const resolved = valueOrFn()
      return resolved ?? fallback
    } catch (e) {
      return fallback
    }
  }
  return valueOrFn ?? fallback
}

export function useArenaRetryTracking(options = {}) {
  const {
    workspaceId,
    getRunId,
    getQuestionSetId,
    ttlMs = DEFAULT_RETRY_TRACK_TTL_MS
  } = options

  const retryingQuestions = ref({})
  const retryRegistry = ref({})

  function retryStorageKey() {
    const wsId = resolveValue(workspaceId, 'global')
    return `retry_tracking_${wsId || 'global'}`
  }

  function hasActiveRetryEntries() {
    return Object.values(retryRegistry.value || {}).some((item) => item?.status === 'queued' || item?.status === 'running')
  }

  function persistRetryRegistry() {
    try {
      localStorage.setItem(retryStorageKey(), JSON.stringify(retryRegistry.value))
    } catch (e) {
      console.warn('[Arena] Failed to persist retry registry:', e)
    }
  }

  function pruneRetryRegistry() {
    const now = Date.now()
    const next = {}
    for (const retryId in retryRegistry.value) {
      const item = retryRegistry.value[retryId]
      const expiresAt = item?.expires_at ? new Date(item.expires_at).getTime() : 0
      if (expiresAt > now) {
        next[retryId] = item
      }
    }
    retryRegistry.value = next
  }

  function rebuildRetryingQuestionsFromRegistry() {
    const active = {}
    for (const retryId in retryRegistry.value) {
      const item = retryRegistry.value[retryId]
      if (!item?.question_id) continue
      if (item.status !== 'queued' && item.status !== 'running') continue
      const qIdStr = String(item.question_id)
      if (!active[qIdStr]) active[qIdStr] = {}
      active[qIdStr][retryId] = true
    }
    retryingQuestions.value = active
  }

  function loadRetryRegistry() {
    try {
      const raw = localStorage.getItem(retryStorageKey())
      if (!raw) {
        retryRegistry.value = {}
        retryingQuestions.value = {}
        return
      }
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object') {
        retryRegistry.value = parsed
        pruneRetryRegistry()
        rebuildRetryingQuestionsFromRegistry()
        persistRetryRegistry()
      }
    } catch (e) {
      console.warn('[Arena] Failed to load retry registry:', e)
      retryRegistry.value = {}
      retryingQuestions.value = {}
    }
  }

  function markRetryStarted(questionId, retryId, meta = {}) {
    if (!questionId || !retryId) return
    const qIdStr = String(questionId)
    if (!retryingQuestions.value[qIdStr]) {
      retryingQuestions.value[qIdStr] = {}
    }
    retryingQuestions.value[qIdStr][retryId] = true

    if (String(retryId).startsWith('local-')) {
      return
    }

    const now = Date.now()
    const existing = retryRegistry.value[retryId] || {}
    retryRegistry.value[retryId] = {
      retry_id: retryId,
      run_id: meta.runId || existing.run_id || resolveValue(getRunId, ''),
      agent_id: meta.agentId || existing.agent_id || '',
      question_id: qIdStr,
      question_set_id: meta.questionSetId || existing.question_set_id || resolveValue(getQuestionSetId, ''),
      status: meta.status || existing.status || 'queued',
      updated_at: new Date(now).toISOString(),
      expires_at: new Date(now + ttlMs).toISOString()
    }

    persistRetryRegistry()
  }

  function markRetryFinished(questionId, retryId, status = 'completed') {
    if (!questionId || !retryId) {
      return { questionCleared: false }
    }
    const qIdStr = String(questionId)
    const retries = retryingQuestions.value[qIdStr]
    if (retries) {
      delete retries[retryId]
      if (Object.keys(retries).length === 0) {
        delete retryingQuestions.value[qIdStr]
      }
    }

    const existing = retryRegistry.value[retryId]
    if (existing) {
      if (status === 'queued' || status === 'running') {
        existing.status = status
        existing.updated_at = new Date().toISOString()
        existing.expires_at = new Date(Date.now() + ttlMs).toISOString()
        retryRegistry.value[retryId] = existing
      } else {
        delete retryRegistry.value[retryId]
      }
      persistRetryRegistry()
    }

    return { questionCleared: !retryingQuestions.value[qIdStr] }
  }

  function clearRetryTrackingForRun(runId) {
    if (!runId) return
    let changed = false
    for (const retryId in retryRegistry.value) {
      if (retryRegistry.value[retryId]?.run_id === runId) {
        delete retryRegistry.value[retryId]
        changed = true
      }
    }
    if (changed) {
      persistRetryRegistry()
    }
  }

  function isQuestionRetrying(questionId) {
    if (!questionId) return false
    const qIdStr = String(questionId)
    const retries = retryingQuestions.value[qIdStr]
    return !!(retries && Object.keys(retries).length > 0)
  }

  return {
    retryingQuestions,
    retryRegistry,
    hasActiveRetryEntries,
    persistRetryRegistry,
    loadRetryRegistry,
    markRetryStarted,
    markRetryFinished,
    clearRetryTrackingForRun,
    isQuestionRetrying
  }
}
