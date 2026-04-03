<template>
  <div class="import-overlay" @click.self="$emit('close')">
    <div class="import-modal">
      <div class="import-header">
        <h3>📥 Import Questions</h3>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <div class="import-body">
        <!-- Target Selection (only show when not in editor context) -->
        <div v-if="!inEditorContext" class="import-target-section">
          <h4>Import to:</h4>
          <label class="mode-option">
            <input type="radio" v-model="importTarget" value="new" />
            <span class="mode-label success">✨ Create New Question Set</span>
          </label>
          <label v-if="questionSet" class="mode-option">
            <input type="radio" v-model="importTarget" value="current" />
            <span class="mode-label primary">📝 Add to "{{ questionSet?.name || 'Current Set' }}"</span>
          </label>
        </div>

        <!-- Mode Selection (only for current set) -->
        <div v-if="importTarget === 'current' || inEditorContext" class="import-mode-section">
          <label class="mode-option">
            <input type="radio" v-model="importMode" value="append" />
            <span class="mode-label primary">Append</span>
            <span class="mode-desc">Adds to existing questions</span>
          </label>
          <label class="mode-option">
            <input type="radio" v-model="importMode" value="replace" />
            <span class="mode-label danger">Replace All</span>
            <span class="mode-desc">Clears existing questions first</span>
          </label>
        </div>

        <div v-if="importMode === 'replace' && (importTarget === 'current' || inEditorContext)" class="warning-alert">
          ⚠️ <strong>Replace Mode</strong> will delete all existing questions in this set.
        </div>

        <!-- Format Tabs -->
        <div class="format-tabs">
          <button 
            class="format-tab" 
            :class="{ active: activeFormat === 'json' }"
            @click="activeFormat = 'json'"
          >JSON</button>
          <button 
            class="format-tab" 
            :class="{ active: activeFormat === 'csv' }"
            @click="activeFormat = 'csv'"
          >CSV</button>
        </div>

        <!-- JSON Format -->
        <div v-if="activeFormat === 'json'" class="format-section">
          <h4>JSON Format</h4>
          <p class="format-desc">Import questions organized by categories:</p>
          <pre class="format-example"><code>{
  "notes": "Optional context for the final PDF report summary.",
  "categories": [
    {
      "name": "Category Name",
      "questions": [
        {
          "question": "What is 2+2?",
          "expected": "4"
        },
        {
          "question": "Plain question without expected answer?"
        }
      ]
    }
  ]
}</code></pre>
          <label class="btn btn-primary import-btn">
            📂 Choose JSON File
            <input type="file" accept=".json" @change="handleFileSelect" />
          </label>
        </div>

        <!-- CSV Format -->
        <div v-if="activeFormat === 'csv'" class="format-section">
          <h4>CSV Format</h4>
          <p class="format-desc">Simple format with question and optional expected answer:</p>
          <pre class="format-example"><code>question,expected,category
"What is 2+2?","4","Math"
"What color is the sky?","Blue","General"
"Plain question without expected?","",""</code></pre>
          <p class="format-note">
            💡 The <code>category</code> column is optional. If omitted, questions go to "Imported".
          </p>
          <label class="btn btn-primary import-btn">
            📂 Choose CSV File
            <input type="file" accept=".csv" @change="handleFileSelect" />
          </label>
        </div>

        <!-- Error Display -->
        <div v-if="error" class="error-alert">
          {{ error }}
        </div>

        <!-- Loading State -->
        <div v-if="loading" class="loading-state">
          <span class="loading-spinner"></span> Importing...
        </div>
      </div>

      <div class="import-footer">
        <button @click="$emit('close')" class="btn btn-secondary">Cancel</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const props = defineProps({
  questionSet: Object,
  workspaceId: String,
  inEditorContext: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['close', 'imported'])

const importTarget = ref('new')  // 'new' or 'current'
const importMode = ref('append')
const activeFormat = ref('json')
const loading = ref(false)
const error = ref(null)

function parseCSV(text) {
  const lines = text.trim().split('\n')
  if (lines.length === 0) return { categories: [] }
  
  // Parse header
  const header = parseCSVLine(lines[0])
  const qIdx = header.findIndex(h => h.toLowerCase() === 'question')
  const eIdx = header.findIndex(h => h.toLowerCase() === 'expected')
  const cIdx = header.findIndex(h => h.toLowerCase() === 'category')
  
  if (qIdx === -1) {
    throw new Error('CSV must have a "question" column')
  }
  
  const categoriesMap = {}
  
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    
    const cols = parseCSVLine(line)
    const question = cols[qIdx] || ''
    const expected = eIdx >= 0 ? (cols[eIdx] || '') : ''
    const category = cIdx >= 0 ? (cols[cIdx] || 'Imported') : 'Imported'
    
    if (!question.trim()) continue
    
    if (!categoriesMap[category]) {
      categoriesMap[category] = []
    }
    
    categoriesMap[category].push({
      question: question.trim(),
      expected: expected.trim()
    })
  }
  
  const categories = Object.entries(categoriesMap).map(([name, questions]) => ({
    name,
    questions
  }))
  
  return { categories }
}

function parseCSVLine(line) {
  const result = []
  let current = ''
  let inQuotes = false
  
  for (let i = 0; i < line.length; i++) {
    const char = line[i]
    
    if (char === '"') {
      if (inQuotes && line[i + 1] === '"') {
        current += '"'
        i++
      } else {
        inQuotes = !inQuotes
      }
    } else if (char === ',' && !inQuotes) {
      result.push(current.trim())
      current = ''
    } else {
      current += char
    }
  }
  result.push(current.trim())
  return result
}

async function handleFileSelect(event) {
  const file = event.target.files[0]
  if (!file) return
  
  loading.value = true
  error.value = null
  
  try {
    const text = await file.text()
    let parsed
    let inferredTitle = null
    
    // Get filename without extension for fallback title
    const fileBaseName = file.name.replace(/\.(json|csv)$/i, '')
    
    if (file.name.endsWith('.csv')) {
      parsed = parseCSV(text)
      // For CSV, check if first line is a title comment
      const firstLine = text.trim().split('\n')[0]
      if (firstLine.startsWith('# Title:')) {
        inferredTitle = firstLine.replace('# Title:', '').trim()
      } else {
        inferredTitle = fileBaseName
      }
    } else {
      parsed = JSON.parse(text)
      // Use title from JSON, or fallback to filename
      inferredTitle = parsed.title || parsed.Title || fileBaseName
    }
    
    // Validate structure
    if (!parsed.categories || !Array.isArray(parsed.categories)) {
      throw new Error('Invalid format: expected { categories: [...] }')
    }
    
    // Normalize questions to objects
    parsed.categories = parsed.categories.map(cat => ({
      name: cat.name || 'Imported',
      questions: (cat.questions || []).map(q => {
        if (typeof q === 'string') {
          return { question: q, expected: '' }
        }
        return {
          question: q.question || q,
          expected: q.expected || ''
        }
      })
    }))
    
    emit('imported', {
      data: parsed,
      title: inferredTitle,
      mode: props.inEditorContext ? importMode.value : (importTarget.value === 'new' ? 'new' : importMode.value),
      target: props.inEditorContext ? 'current' : importTarget.value
    })
  } catch (e) {
    console.error('Import error:', e)
    error.value = 'Failed to import: ' + e.message
  } finally {
    loading.value = false
    event.target.value = ''
  }
}
</script>

<style scoped>
.import-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(15, 23, 42, 0.75);
  backdrop-filter: blur(4px);
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}

.import-modal {
  background: #ffffff;
  border-radius: 12px;
  width: 100%;
  max-width: 600px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);
}

.import-header {
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.import-header h3 {
  margin: 0;
  font-size: 1.25rem;
  color: #1e293b;
}

.import-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
}

.import-mode-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.mode-option {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.mode-option:hover {
  background: #f1f5f9;
}

.mode-option input {
  margin: 0;
}

.mode-label {
  font-weight: 600;
  font-size: 0.95rem;
}

.mode-label.danger {
  color: #dc2626;
}

.mode-label.primary {
  color: #4f46e5;
}

.mode-label.success {
  color: #059669;
}

.mode-desc {
  color: #64748b;
  font-size: 0.85rem;
}

.warning-alert {
  background: #fef3c7;
  border: 1px solid #fcd34d;
  color: #92400e;
  padding: 0.75rem 1rem;
  border-radius: 6px;
  margin-bottom: 1rem;
  font-size: 0.9rem;
}

.error-alert {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #991b1b;
  padding: 0.75rem 1rem;
  border-radius: 6px;
  margin-top: 1rem;
  font-size: 0.9rem;
}

.format-tabs {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.format-tab {
  padding: 0.5rem 1.25rem;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
  transition: all 0.2s;
}

.format-tab.active {
  background: #4f46e5;
  border-color: #4f46e5;
  color: white;
}

.format-section h4 {
  margin: 0 0 0.5rem 0;
  color: #1e293b;
  font-size: 1rem;
}

.format-desc {
  color: #64748b;
  font-size: 0.9rem;
  margin-bottom: 1rem;
}

.format-example {
  background: #1e293b;
  color: #e2e8f0;
  padding: 1rem;
  border-radius: 8px;
  overflow-x: auto;
  font-size: 0.85rem;
  margin-bottom: 1rem;
}

.format-example code {
  font-family: 'Fira Code', 'Monaco', monospace;
}

.format-note {
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  color: #1e40af;
  padding: 0.75rem 1rem;
  border-radius: 6px;
  margin-bottom: 1rem;
  font-size: 0.85rem;
}

.format-note code {
  background: rgba(59, 130, 246, 0.1);
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
}

.import-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.import-btn input {
  display: none;
}

.loading-state {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem;
  color: #64748b;
}

.loading-spinner {
  display: inline-block;
  width: 1.25rem;
  height: 1.25rem;
  border: 3px solid #e2e8f0;
  border-top-color: #4f46e5;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.import-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: flex-end;
}

.btn-close {
  background: transparent;
  border: none;
  font-size: 1.75rem;
  color: #94a3b8;
  cursor: pointer;
  line-height: 1;
}

.btn-close:hover {
  color: #1e293b;
}

.btn {
  padding: 0.6rem 1.25rem;
  border-radius: 6px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
  border: none;
}

.btn-primary {
  background: #4f46e5;
  color: white;
}

.btn-primary:hover {
  background: #4338ca;
}

.btn-secondary {
  background: #f1f5f9;
  color: #475569;
}

.btn-secondary:hover {
  background: #e2e8f0;
}
</style>
