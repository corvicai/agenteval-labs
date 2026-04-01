export function getRecentRunIdForQuestionSet(recentRuns, questionSetId) {
  const runs = recentRuns || []
  const latest = runs.find((r) => r.question_set_id === questionSetId && r.status !== 'running')
  return latest ? latest.id : null
}

function getRunSyncSignature(run) {
  if (!run) return ''

  const id = String(run.id || '')
  const status = String(run.status || '')
  const updatedAt = String(run.updated_at || run.updatedAt || run.created_at || run.createdAt || '')
  const totalTasks = String(run.total_tasks || run.totalTasks || '')
  const completedTasks = String(run.completed_tasks || run.completedTasks || run.completed || '')

  return `${id}:${status}:${updatedAt}:${totalTasks}:${completedTasks}`
}

export function getRecentRunsSyncSignature(recentRuns = []) {
  if (!Array.isArray(recentRuns)) return ''
  return recentRuns.map((run) => getRunSyncSignature(run)).join('|')
}

export function getCachedRunForQuestionSet(cacheMap, recentRuns, questionSetId) {
  const cached = cacheMap.get(questionSetId)
  if (!cached) return null

  const recentRunId = getRecentRunIdForQuestionSet(recentRuns, questionSetId)
  if (recentRunId && cached.runId !== recentRunId) return null

  return cached.data
}

export function setCachedRunForQuestionSet(cacheMap, questionSetId, data) {
  cacheMap.set(questionSetId, { data, runId: data?.run?.id || null })
}
