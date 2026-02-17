export function getRecentRunIdForQuestionSet(recentRuns, questionSetId) {
  const runs = recentRuns || []
  const latest = runs.find((r) => r.question_set_id === questionSetId && r.status !== 'running')
  return latest ? latest.id : null
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
