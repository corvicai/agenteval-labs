export function runProgressStorageKey(runId) {
  return `run_progress_${runId}`
}

export function saveRunProgress(runId, payload) {
  if (!runId) return
  localStorage.setItem(runProgressStorageKey(runId), JSON.stringify(payload))
}

export function loadRunProgress(runId) {
  if (!runId) return null
  const raw = localStorage.getItem(runProgressStorageKey(runId))
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch (e) {
    return null
  }
}

export function clearRunProgress(runId) {
  if (!runId) return
  localStorage.removeItem(runProgressStorageKey(runId))
}

export function hasLoadingResults(runResults, currentRun) {
  if (!runResults || !currentRun) return false
  for (const agentId in runResults) {
    const agentResults = runResults[agentId]
    for (const qId in agentResults) {
      if (agentResults[qId].loading) {
        return true
      }
    }
  }
  return false
}

export async function waitForResultsToLoad({
  isLoadingResults,
  hasLoadingResults,
  maxWaitMs = 5000
}) {
  const startTime = Date.now()
  while (isLoadingResults() && (Date.now() - startTime) < maxWaitMs) {
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  while (hasLoadingResults() && (Date.now() - startTime) < maxWaitMs) {
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
}
