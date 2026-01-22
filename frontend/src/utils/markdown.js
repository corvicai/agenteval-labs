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
 * Regex pattern to detect base64 images in format: (data:image/TYPE;base64,DATA)
 * Supports: png, jpg, jpeg, gif, webp, svg+xml
 */
const BASE64_IMAGE_PATTERN = /\(data:image\/(png|jpe?g|gif|webp|svg\+xml);base64,([A-Za-z0-9+/=]+)\)/g

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
  return BASE64_IMAGE_PATTERN.test(text)
}

/**
 * Extracts all base64 images from text
 * Returns array of objects: [{ fullMatch, type, data, index }]
 */
export const extractBase64Images = (text) => {
  if (!text || typeof text !== 'string') return []
  
  const images = []
  const regex = new RegExp(BASE64_IMAGE_PATTERN.source, 'g')
  let match
  
  while ((match = regex.exec(text)) !== null) {
    images.push({
      fullMatch: match[0],
      type: match[1],
      data: match[2],
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
  
  return text.replace(
    BASE64_IMAGE_PATTERN,
    (match, type, data) => `![image](data:image/${type};base64,${data})`
  )
}

/**
 * Renders markdown text to HTML with sanitization
 * Automatically handles base64 images
 */
export const renderMarkdown = (text) => {
  if (!text || typeof text !== 'string') {
    return ''
  }
  
  // Convert base64 images to markdown format first
  const textWithImages = convertBase64ImagesToMarkdown(text)
  
  // Parse markdown to HTML
  const rawHtml = marked.parse(textWithImages)
  
  // Sanitize HTML allowing data URIs for images
  return purify.sanitize(rawHtml, {
    ADD_ATTR: ['target', 'rel'],
    ALLOWED_URI_REGEXP: ALLOWED_URIS
  })
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
    plainText = plainText.replace(BASE64_IMAGE_PATTERN, '[image]')
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
  
  return {
    raw: rawString,
    hasMarkdown: detectedMarkdown,
    hasImages: detectedImages,
    images: extractedImages,
    html,
    plainText: plainText.trim()
  }
}
