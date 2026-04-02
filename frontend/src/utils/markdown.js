import { marked } from 'marked'
import DOMPurify from 'dompurify'

let purify
if (typeof window !== 'undefined') {
  if (typeof DOMPurify === 'function') {
    purify = DOMPurify(window)
  } else if (DOMPurify && DOMPurify.sanitize) {
    purify = DOMPurify
  }
}

if (!purify) {
  purify = {
    sanitize: (html) => html
  }
}

marked.setOptions({
  breaks: true,
  gfm: true
})

const ALLOWED_URIS = /^(?:(?:https?|mailto|ftp|tel|file|blob|data):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i

/**
 * Base64 image matcher source in format: (data:image/TYPE;base64,DATA)
 * Supports: png, jpg, jpeg, gif, webp, svg+xml
 */
const BASE64_IMAGE_SOURCE = '\\(data:image\\/(png|jpe?g|gif|webp|svg\\+xml);base64,([A-Za-z0-9+/=\\s]+)\\)'

const base64ImageRegex = (flags = 'g') => new RegExp(BASE64_IMAGE_SOURCE, flags)
const PROCESS_CONTENT_CACHE_LIMIT = 400
const CONTENT_PREVIEW_CACHE_LIMIT = 600
const processContentCache = new Map()
const contentPreviewCache = new Map()

function getCachedValue(cache, key) {
  if (!cache.has(key)) return null
  const value = cache.get(key)
  cache.delete(key)
  cache.set(key, value)
  return value
}

function setCachedValue(cache, key, value, maxEntries) {
  if (cache.has(key)) {
    cache.delete(key)
  }
  cache.set(key, value)
  if (cache.size <= maxEntries) return value

  const oldestKey = cache.keys().next().value
  if (oldestKey !== undefined) {
    cache.delete(oldestKey)
  }
  return value
}

/**
 * Detects if string contains markdown formatting
 */
export const hasMarkdown = (text) => {
  if (!text || typeof text !== 'string') return false

  const patterns = [
    /^#{1,6}\s/m,              // Headers
    /\*\*.*\*\*/,              // Bold
    /__.*__/,                  // Bold alternative
    /\*.*\*/,                  // Italic
    /_.*_/,                    // Italic alternative
    /\[.*\]\(.*\)/,            // Links
    /^[-*+]\s/m,               // Unordered lists
    /^\d+\.\s/m,               // Ordered lists
    /^>\s/m,                   // Blockquotes
    /`[^`]+`/,                 // Inline code
    /```[\s\S]*?```/,          // Code blocks
    /^---+$/m,                 // Horizontal rules
    /!\[.*\]\(.*\)/            // Images
  ]

  return patterns.some(pattern => pattern.test(text))
}

/**
 * Detects if string contains base64 images in format (data:image/...;base64,...)
 */
export const hasBase64Images = (text) => {
  if (!text || typeof text !== 'string') return false
  return base64ImageRegex('i').test(text)
}

/**
 * Extracts all base64 images from text
 * Returns array of objects: [{ fullMatch, type, data, index }]
 */
export const extractBase64Images = (text) => {
  if (!text || typeof text !== 'string') return []

  const images = []
  const regex = base64ImageRegex('gi')
  let match

  while ((match = regex.exec(text)) !== null) {
    const normalizedData = String(match[2] || '').replace(/\s+/g, '')
    images.push({
      fullMatch: match[0],
      type: match[1],
      data: normalizedData,
      index: match.index
    })
  }

  return images
}

/**
 * Converts base64 images in parentheses to markdown image syntax
 * (data:image/png;base64,xxx) => ![image](data:image/png;base64,xxx)
 */
export const convertBase64ImagesToMarkdown = (text) => {
  if (!text || typeof text !== 'string') return text

  // Use a replacer function to check context
  return text.replace(base64ImageRegex('gi'), (match, type, data, offset, string) => {
    // Check if preceded by ']' which implies it's already a markdown image: ![alt](data...)
    if (offset > 0 && string[offset - 1] === ']') {
      return match // Return original string unchanged
    }
    // Otherwise wrap it
    const normalizedData = String(data || '').replace(/\s+/g, '')
    return `![image](data:image/${type};base64,${normalizedData})`
  })
}

/**
 * Renders markdown text to HTML with sanitization
 * Automatically handles base64 images
 */
export const renderMarkdown = (text) => {
  if (!text || typeof text !== 'string') {
    return ''
  }

  // Remove unsupported citations like <<9>>
  // We do this before image processing to keep things clean
  const textCleaned = text.replace(/<<\d+>>/g, '')

  // Convert base64 images to markdown format first, but ONLY if not already in markdown format
  const textWithImages = convertBase64ImagesToMarkdown(textCleaned)

  // Parse markdown to HTML
  const rawHtml = marked.parse(textWithImages)

  // Sanitize HTML allowing data URIs for images
  return purify.sanitize(rawHtml, {
    ADD_ATTR: ['target', 'rel'],
    ALLOWED_URI_REGEXP: ALLOWED_URIS
  })
}

export const getContentPreviewText = (rawString, maxLen = 220) => {
  if (!rawString || typeof rawString !== 'string') {
    return ''
  }

  const cacheKey = `${maxLen}:${rawString}`
  const cached = getCachedValue(contentPreviewCache, cacheKey)
  if (cached != null) return cached

  let preview = rawString.replace(/<<\d+>>/g, '')
  preview = preview.replace(base64ImageRegex('gi'), ' [image] ')
  preview = preview
    .replace(/```[\s\S]*?```/g, ' [code block] ')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/!\[([^\]]*)\]\((.*?)\)/g, '$1 [image]')
    .replace(/\[([^\]]+)\]\((.*?)\)/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^\s*[-*+]\s+/gm, '')
    .replace(/^\s*\d+\.\s+/gm, '')
    .replace(/^>\s?/gm, '')
    .replace(/\*\*(.*?)\*\*/g, '$1')
    .replace(/__(.*?)__/g, '$1')
    .replace(/\*(.*?)\*/g, '$1')
    .replace(/_(.*?)_/g, '$1')
    .replace(/\r?\n+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()

  if (preview.length > maxLen) {
    preview = `${preview.slice(0, maxLen).trimEnd()}...`
  }

  return setCachedValue(contentPreviewCache, cacheKey, preview, CONTENT_PREVIEW_CACHE_LIMIT)
}

/**
 * Main processor: detects and converts markdown + images in runtime
 * Returns object with detected content types and rendered HTML
 */
export const processContent = (rawString) => {
  if (!rawString || typeof rawString !== 'string') {
    return {
      raw: rawString || '',
      hasMarkdown: false,
      hasImages: false,
      images: [],
      html: rawString || '',
      plainText: rawString || ''
    }
  }

  const cached = getCachedValue(processContentCache, rawString)
  if (cached) return cached

  // Try to detect if the whole content is JSON (object or array) and
  // pretty-print it inside a markdown code block for better readability.
  let working = rawString
  let forcedJsonMarkdown = false

  try {
    const trimmed = rawString.trim()
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
      (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      const parsed = JSON.parse(trimmed)
      const pretty = JSON.stringify(parsed, null, 2)
      working = '```json\n' + pretty + '\n```\n'
      forcedJsonMarkdown = true
    }
  } catch (e) {
    // Not valid JSON, ignore and keep original text
  }

  const detectedMarkdown = forcedJsonMarkdown ? true : hasMarkdown(working)
  const detectedImages = hasBase64Images(working)
  const extractedImages = detectedImages ? extractBase64Images(working) : []

  // If has markdown or images, render as HTML
  const shouldRender = detectedMarkdown || detectedImages
  const html = shouldRender ? renderMarkdown(working) : working

  // Plain text version (remove markdown syntax and images)
  let plainText = working
  if (detectedImages) {
    plainText = plainText.replace(base64ImageRegex('gi'), '[image]')
  }
  if (detectedMarkdown) {
    // Simple markdown removal (basic patterns)
    plainText = plainText
      .replace(/^#{1,6}\s/gm, '')
      .replace(/\*\*(.+?)\*\*/g, '$1')
      .replace(/__(.+?)__/g, '$1')
      .replace(/\*(.+?)\*/g, '$1')
      .replace(/_(.+?)_/g, '$1')
      .replace(/\[(.+?)\]\(.+?\)/g, '$1')
      .replace(/`(.+?)`/g, '$1')
      .replace(/```[\s\S]*?```/g, '')
  }

  const processed = {
    raw: rawString,
    hasMarkdown: detectedMarkdown,
    hasImages: detectedImages,
    images: extractedImages,
    html,
    plainText: plainText.trim()
  }

  return setCachedValue(processContentCache, rawString, processed, PROCESS_CONTENT_CACHE_LIMIT)
}
