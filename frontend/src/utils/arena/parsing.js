const SCORE_OUT_OF_TEN_REGEX = /(^|[^0-9.])((?:10(?:\.0+)?)|(?:\d(?:\.\d+)?))\s*\/\s*10($|[^0-9])/g
const BASE64_IMAGE_PATTERN = /data:image\/[^;]+;base64,[A-Za-z0-9+/=\s]+/g

export function parseEvaluatorTaskQuestionID(questionId) {
  if (!questionId || typeof questionId !== 'string') return null
  if (!questionId.startsWith('eval-')) return null

  const rest = questionId.slice(5)
  if (rest.length < 38) return null
  if (rest[36] !== '-') return null

  const targetAgentId = rest.slice(0, 36)
  const originalQuestionId = rest.slice(37)
  if (!targetAgentId || !originalQuestionId) return null

  return {
    targetAgentId,
    questionId: originalQuestionId
  }
}

export function extractScoreOutOfTen(text) {
  if (!text || typeof text !== 'string') return null
  SCORE_OUT_OF_TEN_REGEX.lastIndex = 0
  const matches = [...text.matchAll(SCORE_OUT_OF_TEN_REGEX)]
  for (let i = matches.length - 1; i >= 0; i--) {
    const raw = Number.parseFloat(matches[i][2])
    if (Number.isNaN(raw)) continue
    if (raw < 0 || raw > 10) continue
    return raw
  }
  return null
}

export function truncatePreviewText(text, maxLen = 150) {
  if (!text || typeof text !== 'string') return ''
  if (text.length <= maxLen) return text

  const truncatedText = text.substring(0, maxLen)

  let lastImageEnd = -1
  let match
  const pattern = new RegExp(BASE64_IMAGE_PATTERN)
  while ((match = pattern.exec(text)) !== null) {
    if (match.index + match[0].length <= maxLen) {
      lastImageEnd = match.index + match[0].length
    } else {
      break
    }
  }

  if (lastImageEnd > 0 && lastImageEnd > maxLen) {
    return text.substring(0, lastImageEnd) + '...'
  }

  let truncateAt = maxLen
  if (truncatedText.includes('data:image')) {
    const imageStart = truncatedText.lastIndexOf('data:image')
    if (imageStart >= 0) {
      const afterImage = truncatedText.indexOf(' ', imageStart + 100)
      truncateAt = afterImage > 0 && afterImage <= maxLen ? afterImage : imageStart
    }
  }

  return text.substring(0, truncateAt) + '...'
}

export function extractQuestionIdsFromQuestionSet(questionSet) {
  const ids = []
  if (!questionSet?.data) return ids
  let data = questionSet.data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch (e) {
      return ids
    }
  }

  const categories = data.categories || []
  for (let catIdx = 0; catIdx < categories.length; catIdx++) {
    const cat = categories[catIdx]
    const catQuestions = cat.questions || []
    for (let qIdx = 0; qIdx < catQuestions.length; qIdx++) {
      const q = catQuestions[qIdx]
      const qId = q.id != null && q.id !== '' ? String(q.id) : `${catIdx + 1}-${qIdx + 1}`
      ids.push(qId)
    }
  }
  return ids
}
