export function flattenQuestionSetQuestions(questionSetData) {
  if (!questionSetData) return []

  let data = questionSetData
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch (e) {
      console.error('Failed to parse question set data:', e)
      return []
    }
  }

  const questions = []
  const categories = data.categories || []
  for (let catIdx = 0; catIdx < categories.length; catIdx++) {
    const cat = categories[catIdx]
    const catQuestions = cat.questions || []
    for (let qIdx = 0; qIdx < catQuestions.length; qIdx++) {
      const q = catQuestions[qIdx]
      const questionText = q.question || q.text || ''
      const qId = q.id != null && q.id !== '' ? String(q.id) : `${catIdx + 1}-${qIdx + 1}`
      questions.push({ ...q, id: qId, category: cat.name, question: questionText })
    }
  }
  return questions
}

export function hasQuestionBeenRun(runResults, questionId) {
  if (!runResults || !questionId) return false
  const qIdStr = String(questionId)

  for (const agentId in runResults) {
    const agentResults = runResults[agentId]
    if (agentResults && agentResults[qIdStr]) {
      const result = agentResults[qIdStr]
      if (result.answer || result.error || result.timestamp) {
        return true
      }
    }
  }
  return false
}

export function getQuestionStatus(runResults, questionId, isQuestionRetrying = () => false) {
  if (!runResults || !questionId) return 'status-not-run'
  const qIdStr = String(questionId)

  if (isQuestionRetrying(qIdStr)) return 'status-loading'

  let hasError = false
  let hasAnswer = false
  let hasSuccess = false
  let isLoading = false

  for (const agentId in runResults) {
    const agentResults = runResults[agentId]
    if (agentResults && agentResults[qIdStr]) {
      const result = agentResults[qIdStr]
      if (result.loading) isLoading = true
      if (result.error) hasError = true
      if (result.answer) hasAnswer = true
      if (result.success === true) hasSuccess = true
    }
  }

  if (isLoading) return 'status-loading'
  if (hasError && !hasAnswer && !hasSuccess) return 'status-error'
  if (hasAnswer || hasSuccess) return 'status-completed'
  return 'status-not-run'
}

export function isQuestionLoading(runResults, questionId, isQuestionRetrying = () => false) {
  if (!runResults || !questionId) return false
  const qIdStr = String(questionId)

  if (isQuestionRetrying(qIdStr)) return true

  for (const agentId in runResults) {
    const agentResults = runResults[agentId]
    if (agentResults && agentResults[qIdStr]?.loading) {
      return true
    }
  }
  return false
}

export function getQuestionStatusText(status) {
  switch (status) {
    case 'status-loading':
      return '⏳ Running'
    case 'status-error':
      return '❌ Error'
    case 'status-completed':
      return '✅ Completed'
    default:
      return '⭕ Not Run'
  }
}

export function getQuestionStatusTooltip(status, questionId, taskProgress, isQuestionRetrying = () => false) {
  if (status !== 'status-loading') return ''

  const qIdStr = String(questionId)
  let best = null

  for (const agentId in taskProgress || {}) {
    const entry = taskProgress[agentId]?.[qIdStr]
    if (!entry) continue
    if (!best) {
      best = entry
      continue
    }

    const entryElapsed = typeof entry.elapsed_ms === 'number' ? entry.elapsed_ms : -1
    const bestElapsed = typeof best.elapsed_ms === 'number' ? best.elapsed_ms : -1
    if (entryElapsed > bestElapsed) {
      best = entry
      continue
    }

    if (entryElapsed === bestElapsed) {
      const entryTs = Date.parse(entry.timestamp || '')
      const bestTs = Date.parse(best.timestamp || '')
      if (!Number.isNaN(entryTs) && (Number.isNaN(bestTs) || entryTs > bestTs)) {
        best = entry
      }
    }
  }

  if (best?.message) return best.message
  if (isQuestionRetrying(qIdStr)) return 'Retry is still running...'
  return 'Task is running...'
}
