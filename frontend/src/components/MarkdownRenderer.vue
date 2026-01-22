<template>
  <div class="markdown-renderer">
    <!-- Show detection info (optional, for debugging) -->
    <div v-if="showMeta" class="meta-info">
      <span v-if="processed.hasMarkdown" class="badge">MD</span>
      <span v-if="processed.hasImages" class="badge">
        {{ processed.images.length }} Image{{ processed.images.length > 1 ? 's' : '' }}
      </span>
    </div>

    <!-- Render content -->
    <div 
      v-if="processed.hasMarkdown || processed.hasImages"
      class="markdown-content"
      v-html="processed.html"
    ></div>
    <div v-else class="plain-content">
      {{ processed.plainText }}
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { processContent } from '../utils/markdown.js'

const props = defineProps({
  // Raw string from DB (never modified)
  content: {
    type: String,
    required: true,
    default: ''
  },
  // Show metadata badges
  showMeta: {
    type: Boolean,
    default: false
  }
})

// Process content in runtime (reactive)
const processed = computed(() => processContent(props.content))
</script>

<style scoped>
.markdown-renderer {
  width: 100%;
}

.meta-info {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.badge {
  font-size: 0.75rem;
  padding: 0.25rem 0.5rem;
  background: #e0e0e0;
  border-radius: 4px;
  font-weight: 500;
}

.markdown-content {
  line-height: 1.6;
}

/* Markdown styling */
.markdown-content :deep(h1) {
  font-size: 1.5rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
}

.markdown-content :deep(h2) {
  font-size: 1.3rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
}

.markdown-content :deep(h3) {
  font-size: 1.1rem;
  margin: 1rem 0 0.5rem;
  font-weight: 600;
}

.markdown-content :deep(p) {
  margin: 0.5rem 0;
}

.markdown-content :deep(strong) {
  font-weight: 600;
}

.markdown-content :deep(em) {
  font-style: italic;
}

.markdown-content :deep(code) {
  background: #f5f5f5;
  padding: 0.125rem 0.25rem;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
  font-size: 0.9em;
}

.markdown-content :deep(pre) {
  background: #f5f5f5;
  padding: 1rem;
  border-radius: 4px;
  overflow-x: auto;
  margin: 0.5rem 0;
}

.markdown-content :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.5rem;
}

.markdown-content :deep(li) {
  margin: 0.25rem 0;
}

.markdown-content :deep(blockquote) {
  border-left: 3px solid #e0e0e0;
  padding-left: 1rem;
  margin: 0.5rem 0;
  color: #666;
}

.markdown-content :deep(a) {
  color: #0066cc;
  text-decoration: none;
}

.markdown-content :deep(a:hover) {
  text-decoration: underline;
}

.markdown-content :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
  margin: 0.5rem 0;
  display: block;
}

.markdown-content :deep(hr) {
  border: none;
  border-top: 1px solid #e0e0e0;
  margin: 1rem 0;
}

.plain-content {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
