export function getQuestionSetAgents(questionSet) {
  return Array.isArray(questionSet?.agents) ? questionSet.agents : []
}

function stringifyForSyncSignature(value) {
  try {
    return JSON.stringify(value)
  } catch (e) {
    return ''
  }
}

export function getQuestionSetSyncSignature(questionSet) {
  if (!questionSet) return ''

  const id = String(questionSet.id || '')
  const updatedAt = String(
    questionSet.updated_at ||
    questionSet.updatedAt ||
    questionSet.created_at ||
    questionSet.createdAt ||
    ''
  )

  if (updatedAt) {
    return `${id}:${updatedAt}`
  }

  return `${id}:${stringifyForSyncSignature(questionSet)}`
}

export function getQuestionSetListSyncSignature(questionSets = []) {
  if (!Array.isArray(questionSets)) return ''
  return questionSets.map((questionSet) => getQuestionSetSyncSignature(questionSet)).join('|')
}

export function resolveQuestionSetSelection({
  questionSets = [],
  preferredId = '',
  lastQuestionSetId = '',
  currentQuestionSet = null
} = {}) {
  const sets = Array.isArray(questionSets) ? questionSets : []
  const preferred = String(preferredId || '')
  const lastSelected = String(lastQuestionSetId || '')
  const currentId = String(currentQuestionSet?.id || '')

  if (preferred) {
    const preferredMatch = sets.find((questionSet) => String(questionSet?.id || '') === preferred)
    if (preferredMatch) return preferredMatch

    if (currentQuestionSet && currentId === preferred) {
      return currentQuestionSet
    }
  }

  if (lastSelected) {
    const lastSelectedMatch = sets.find((questionSet) => String(questionSet?.id || '') === lastSelected)
    if (lastSelectedMatch) return lastSelectedMatch
  }

  if (currentId) {
    const currentMatch = sets.find((questionSet) => String(questionSet?.id || '') === currentId)
    if (currentMatch) return currentMatch
  }

  return null
}

export function mergeQuestionSetForUI(nextSet, previousSet = null) {
  if (!nextSet) return null
  const nextAgents = getQuestionSetAgents(nextSet)
  const sameSet = previousSet && previousSet.id === nextSet.id ? previousSet : null
  const previousAgents = getQuestionSetAgents(sameSet)
  const hasExplicitAgents = Object.prototype.hasOwnProperty.call(nextSet, 'agents')

  // Keep local overrides only when incoming payload is partial (no "agents" field).
  if (!hasExplicitAgents && previousAgents.length > 0) {
    return { ...nextSet, agents: previousAgents }
  }

  // Preserve explicit empty array when backend confirms no active agents.
  if (hasExplicitAgents && nextAgents.length === 0) {
    return { ...nextSet, agents: [] }
  }

  return nextSet
}

export function getQuestionCount(set) {
  if (!set || !set.data) return 0
  let data = set.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch (e) {
      return 0
    }
  }
  if (!data.categories) return 0
  return data.categories.reduce((acc, cat) => acc + (cat.questions ? cat.questions.length : 0), 0)
}

export function getRunQuestionSetID(run) {
  if (!run) return ''
  return String(run.question_set_id || run.questionSetID || run.questionSetId || '')
}
