<template>
  <div class="qe-overlay" @click.self="$emit('close')">
    <div class="qe-modal">
      <div class="qe-header">
        <div class="qe-title-edit">
          <span class="qe-title-label">Question Set Name:</span>
          <input 
            v-model="localName" 
            class="qe-name-input" 
            placeholder="Enter Question Set Name" 
          />
        </div>
        <button class="btn-close" @click="$emit('close')">×</button>
      </div>

      <div class="qe-body">
        <div v-if="loading" class="qe-state">
          <span class="loading-spinner"></span> Loading questions...
        </div>
        
        <div v-else>
          <div v-if="error" class="qe-error-alert">
            {{ error }}
          </div>

          <div v-for="(cat, catIdx) in localCategories" :key="catIdx" class="category-block">
            <div class="category-header">
              <input v-model="cat.name" placeholder="Category Name" class="category-name-input" />
              <button @click="removeCategory(catIdx)" class="btn-remove-cat" title="Remove Category">Remove Category</button>
            </div>

            <div v-for="(q, qIdx) in cat.questions" :key="qIdx" class="question-item">
              <div class="question-item-header">
                <span class="q-label">Question {{ qIdx + 1 }}</span>
                <button @click="removeQuestion(catIdx, qIdx)" class="btn-remove-q" title="Remove Question">🗑️</button>
              </div>
              <div class="question-fields">
                <div class="field">
                  <label>Question Text</label>
                  <textarea v-model="q.question" rows="2" placeholder="Enter question..."></textarea>
                </div>
                <div class="field">
                  <label>Expected Answer (Optional)</label>
                  <textarea v-model="q.expected" rows="2" placeholder="Enter expected response..."></textarea>
                </div>
              </div>
            </div>

            <button @click="addQuestion(catIdx)" class="btn-add-q">+ Add Question to {{ cat.name || 'Category' }}</button>
          </div>

          <button @click="addCategory" class="btn-add-cat">+ Add New Category</button>
        </div>
      </div>

      <div class="qe-footer">
        <span class="qe-meta">{{ totalQuestions }} question(s) in {{ localCategories.length }} categories</span>
        <div class="qe-actions">
          <button @click="showImportModal = true" class="btn btn-import">
            📥 Import
          </button>
          <button @click="exportCurrentSet" class="btn btn-export">
            📤 Export
          </button>
          <button @click="$emit('close')" class="btn btn-secondary">Cancel</button>
          <button @click="saveChanges" :disabled="saving" class="btn btn-primary">
            {{ saving ? '⏳ Saving...' : '💾 Save Changes' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Import Modal (inside editor context) -->
    <ImportQuestionsModal
      v-if="showImportModal"
      :question-set="questionSet"
      :workspace-id="workspaceId"
      :in-editor-context="true"
      @close="showImportModal = false"
      @imported="handleEditorImport"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { wsService } from '../services/websocket.js'
import ImportQuestionsModal from './ImportQuestionsModal.vue'
import { generateQuestionSetName } from '../utils/nameGenerator.js'

const props = defineProps({
  questionSet: Object,
  workspaceId: String
})

const emit = defineEmits(['close', 'saved'])

const localCategories = ref([])
const localName = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref(null)
const showImportModal = ref(false)

const totalQuestions = computed(() => {
  return localCategories.value.reduce((acc, cat) => acc + (cat.questions?.length || 0), 0)
})

function resolveStableQuestionId(question, catIdx, qIdx) {
  const rawId = question?.id
  if (rawId !== null && rawId !== undefined && String(rawId) !== '') {
    return rawId
  }
  // Keep compatibility with runner fallback IDs to avoid orphaning historical results.
  return `${catIdx + 1}-${qIdx + 1}`
}

const loadQuestions = () => {
  loading.value = true
  try {
    if (props.questionSet) {
      localName.value = props.questionSet.name || 'New set'
      
      let data = props.questionSet.data
      if (typeof data === 'string') {
        try {
           data = JSON.parse(data)
        } catch(e) {
           console.error('Failed to parse data in editor', e)
        }
      }

      if (data && data.categories) {
        // Deep clone to avoid direct mutations
        const cloned = JSON.parse(JSON.stringify(data.categories))
        // Ensure every question has a stable ID.
        cloned.forEach((cat, cIdx) => {
          cat.questions?.forEach((q, qIdx) => {
            q.id = resolveStableQuestionId(q, cIdx, qIdx)
          })
        })
        localCategories.value = cloned
      } else {
        localCategories.value = [{ name: 'General', questions: [] }]
      }
    } else {
      localName.value = generateQuestionSetName()
      localCategories.value = [{ name: 'General', questions: [{ question: '', expected: '' }] }]
    }
  } catch (err) {
    error.value = 'Failed to load questions'
  } finally {
    loading.value = false
  }
}

const addCategory = () => {
  localCategories.value.push({ name: 'New Category', questions: [] })
}

const removeCategory = (idx) => {
  if (confirm('Are you sure you want to remove this category and all its questions?')) {
    localCategories.value.splice(idx, 1)
  }
}

const addQuestion = (catIdx) => {
  if (!localCategories.value[catIdx].questions) {
    localCategories.value[catIdx].questions = []
  }
  const newId = `q-${Date.now()}-${Math.floor(Math.random() * 1000)}`
  localCategories.value[catIdx].questions.push({ id: newId, question: '', expected: '' })
}

const removeQuestion = (catIdx, qIdx) => {
  localCategories.value[catIdx].questions.splice(qIdx, 1)
}

const handleEditorImport = ({ data, mode }) => {
  showImportModal.value = false
  
  if (mode === 'replace') {
    // Replace all with imported data
    localCategories.value = data.categories.map(cat => ({
      name: cat.name || 'Imported',
      questions: (cat.questions || []).map((q, idx) => ({
        id: `q-${Date.now()}-${idx}`,
        question: q.question || '',
        expected: q.expected || ''
      }))
    }))
  } else {
    // Append mode - merge by category name
    const importedCategories = data.categories || []
    importedCategories.forEach(importCat => {
      const existingCat = localCategories.value.find(c => c.name === importCat.name)
      if (existingCat) {
        // Add to existing category
        const newQuestions = (importCat.questions || []).map((q, idx) => ({
          id: `q-${Date.now()}-${idx}-${Math.random()}`,
          question: q.question || '',
          expected: q.expected || ''
        }))
        existingCat.questions = [...(existingCat.questions || []), ...newQuestions]
      } else {
        // Add new category
        localCategories.value.push({
          name: importCat.name || 'Imported',
          questions: (importCat.questions || []).map((q, idx) => ({
            id: `q-${Date.now()}-${idx}-${Math.random()}`,
            question: q.question || '',
            expected: q.expected || ''
          }))
        })
      }
    })
  }
}

const exportCurrentSet = () => {
  // Build export data from current local state
  const exportData = {
    title: localName.value || 'Exported Questions',
    categories: localCategories.value.map(cat => ({
      name: cat.name,
      questions: (cat.questions || []).map(q => ({
        question: q.question,
        expected: q.expected || ''
      }))
    }))
  }
  
  const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${localName.value || 'questions'}.json`
  a.click()
  URL.revokeObjectURL(url)
}

const saveChanges = async () => {
  if (!localName.value.trim()) {
    alert('Please enter a name for the question set.')
    return
  }

  saving.value = true
  error.value = null
  
  try {
    // Sanitize data: remove empty questions/categories if necessary, but here we keep them and let user decide
    const cleanedCategories = localCategories.value.map(cat => ({
      ...cat,
      name: cat.name || 'Unnamed Category',
      questions: (cat.questions || []).filter(q => q.question.trim() !== '')
    })).filter(cat => cat.questions.length > 0 || cat.name !== 'New Category')

    const payload = {
      name: localName.value,
      version: props.questionSet?.version || '1.0',
      data: {
        categories: cleanedCategories
      }
    }

    let updated
    if (props.questionSet?.id) {
       updated = await wsService.updateQuestionSet(props.questionSet.id, payload)
    } else {
       if (!props.workspaceId) throw new Error('Workspace ID missing')
       updated = await wsService.createQuestionSet(props.workspaceId, payload)
    }
    
    emit('saved', updated)
    emit('close')
  } catch (err) {
    console.error('Save failed:', err)
    error.value = 'Failed to save changes: ' + err.message
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadQuestions()
})
</script>

<style scoped>
.qe-overlay {
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

.qe-modal {
  background: #ffffff;
  border-radius: 12px;
  width: 100%;
  max-width: 900px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.qe-header {
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qe-title-edit {
  display: flex;
  align-items: center;
  gap: 1rem;
  flex: 1;
}

.qe-title-label {
  font-size: 0.9rem;
  font-weight: 600;
  color: #64748b;
  white-space: nowrap;
}

.qe-name-input {
  font-size: 1.25rem;
  font-weight: 700;
  color: #1e293b;
  border: 1px solid transparent;
  padding: 4px 8px;
  border-radius: 6px;
  width: 100%;
  max-width: 400px;
  background: transparent;
}

.qe-name-input:focus {
  background: #f1f5f9;
  border-color: #cbd5e1;
  outline: none;
}

.qe-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
}

.category-block {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 1.25rem;
  margin-bottom: 1.5rem;
}

.category-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.category-name-input {
  font-weight: 700;
  font-size: 1.1rem;
  border: 1px solid transparent;
  background: transparent;
  padding: 4px 8px;
  border-radius: 4px;
  color: #1e293b;
  width: 50%;
}

.category-name-input:focus {
  background: #ffffff;
  border-color: #cbd5e1;
  outline: none;
}

.btn-remove-cat {
  background: transparent;
  color: #ef4444;
  border: 1px solid #fecaca;
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 0.8rem;
  cursor: pointer;
}

.btn-remove-cat:hover {
  background: #fef2f2;
}

.question-item {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 1rem;
  margin-bottom: 1rem;
}

.question-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.q-label {
  font-size: 0.85rem;
  font-weight: 600;
  color: #64748b;
}

.btn-remove-q {
  background: transparent;
  border: none;
  cursor: pointer;
  font-size: 1.1rem;
  padding: 0;
  opacity: 0.6;
}

.btn-remove-q:hover {
  opacity: 1;
}

.question-fields {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.field label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
}

.field textarea {
  width: 100%;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 8px 12px;
  font-family: inherit;
  font-size: 0.9rem;
  resize: vertical;
}

.field textarea:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.btn-add-q {
  width: 100%;
  background: #ffffff;
  border: 1px dashed #cbd5e1;
  color: #64748b;
  padding: 8px;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-add-q:hover {
  background: #f1f5f9;
  border-color: #94a3b8;
  color: #1e293b;
}

.btn-add-cat {
  width: 100%;
  background: #f1f5f9;
  border: 2px dashed #cbd5e1;
  color: #475569;
  padding: 1rem;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
}

.btn-add-cat:hover {
  background: #e2e8f0;
  border-color: #94a3b8;
}

.qe-footer {
  padding: 1.25rem 1.5rem;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qe-meta {
  font-size: 0.85rem;
  color: #64748b;
}

.qe-actions {
  display: flex;
  gap: 0.75rem;
}

.qe-error-alert {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #991b1b;
  padding: 0.75rem 1rem;
  border-radius: 6px;
  margin-bottom: 1.5rem;
  font-size: 0.9rem;
}

.loading-spinner {
  display: inline-block;
  width: 1.5rem;
  height: 1.5rem;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-right: 0.5rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.btn-close {
  background: transparent;
  border: none;
  font-size: 2rem;
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

.btn-primary:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.btn-secondary {
  background: #f1f5f9;
  color: #475569;
}

.btn-secondary:hover {
  background: #e2e8f0;
}

.btn-import {
  background: #059669;
  color: white;
}

.btn-import:hover {
  background: #047857;
}

.btn-export {
  background: #0284c7;
  color: white;
}

.btn-export:hover {
  background: #0369a1;
}
</style>
