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

// isQuestionSetShared centralizes "is this QS shared from another user?"
// Prefers the server-authoritative `is_shared` field (populated by enriched
// backend handlers) and falls back to the legacy client-side `_shared` flag
// that LeftSidebar sets when the user clicks a shared QS in the sidebar.
export function isQuestionSetShared(questionSet) {
  if (!questionSet) return false
  return Boolean(questionSet.is_shared || questionSet._shared)
}

// getSharedOwnerAgents returns the redacted owner agents attached to a shared
// QS, regardless of field casing/source (server vs. local sidebar injection).
export function getSharedOwnerAgents(questionSet) {
  if (!questionSet) return []
  if (Array.isArray(questionSet.owner_agents)) return questionSet.owner_agents
  if (Array.isArray(questionSet.ownerAgents)) return questionSet.ownerAgents
  return []
}

const SHARING_FIELDS = [
  'is_shared',
  'owner_user_id',
  'owner_name',
  'owner_workspace_id',
  'owner_agents',
  'role',
  // Legacy client-side marker kept for backwards compatibility.
  '_shared',
]

// preserveSharingMetadata copies sharing-related fields from `previous` onto
// `next` for fields that `next` does not explicitly provide. This prevents
// non-enriched broadcasts (e.g. generic data-changed payloads) from wiping
// out the authoritative sharing state the server already sent on a richer
// handler like GetRunDetails/GetRunLite.
function preserveSharingMetadata(next, previous) {
  if (!next || !previous) return next
  const merged = { ...next }
  for (const field of SHARING_FIELDS) {
    const nextHas = Object.prototype.hasOwnProperty.call(next, field) && next[field] !== undefined && next[field] !== null
    const prevHas = Object.prototype.hasOwnProperty.call(previous, field) && previous[field] !== undefined && previous[field] !== null
    if (!nextHas && prevHas) {
      merged[field] = previous[field]
    }
  }
  return merged
}

export function mergeQuestionSetForUI(nextSet, previousSet = null) {
  if (!nextSet) return null
  const nextAgents = getQuestionSetAgents(nextSet)
  const sameSet = previousSet && previousSet.id === nextSet.id ? previousSet : null
  const previousAgents = getQuestionSetAgents(sameSet)
  const hasExplicitAgents = Object.prototype.hasOwnProperty.call(nextSet, 'agents')

  // Keep local overrides only when incoming payload is partial (no "agents" field).
  if (!hasExplicitAgents && previousAgents.length > 0) {
    return preserveSharingMetadata({ ...nextSet, agents: previousAgents }, sameSet)
  }

  // Preserve explicit empty array when backend confirms no active agents.
  if (hasExplicitAgents && nextAgents.length === 0) {
    return preserveSharingMetadata({ ...nextSet, agents: [] }, sameSet)
  }

  return preserveSharingMetadata(nextSet, sameSet)
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
