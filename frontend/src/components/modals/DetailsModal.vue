<template>
  <div v-if="isOpen" class="details-overlay" @click.self="close">
    <div class="details-modal">
      <!-- Header -->
      <div class="modal-header">
        <h3>🔍 Developer Details</h3>
        <button class="btn-close" @click="close">×</button>
      </div>

      <!-- Content -->
      <div class="modal-body">
        <!-- IDs Section -->
        <section class="section">
          <h4 class="section-title">📋 Identifiers</h4>
          <div class="ids-grid">
            <div v-for="(val, key) in identifiers" :key="key" class="id-row">
              <span class="id-label">{{ key }}:</span>
              <div class="id-value-wrap">
                <code class="id-value">{{ val }}</code>
                <button class="btn-copy" @click="copy(val)" title="Copy">📋</button>
              </div>
            </div>
          </div>
        </section>

        <!-- Raw Data Sections -->
        <section v-for="(data, label) in rawBlocks" :key="label" class="section">
          <h4 class="section-title">📦 {{ label }}</h4>
          <div class="code-block-wrap">
            <pre class="code-block">{{ formatJSON(data) }}</pre>
            <button class="btn-copy-block" @click="copy(formatJSON(data))" title="Copy JSON">📋</button>
          </div>
        </section>
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <button class="btn btn-secondary" @click="close">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  isOpen: Boolean,
  details: Object
});

const emit = defineEmits(['close']);

const identifiers = computed(() => {
  if (!props.details) return {};
  const ids = {};
  if (props.details.run_id) ids['Run ID'] = props.details.run_id;
  if (props.details.agent_id) ids['Agent ID'] = props.details.agent_id;
  if (props.details.question_id) ids['Question ID'] = props.details.question_id;
  if (props.details.result_id) ids['Result ID'] = props.details.result_id;
  if (props.details.question_set_id) ids['Question Set ID'] = props.details.question_set_id;
  return ids;
});

const rawBlocks = computed(() => {
  if (!props.details) return {};
  const blocks = {};
  if (props.details.question) blocks['Original Question'] = props.details.question;
  if (props.details.result) blocks['Raw Result'] = props.details.result;
  if (props.details.metadata) blocks['Metadata'] = props.details.metadata;
  return blocks;
});

const close = () => {
  emit('close');
};

const formatJSON = (val) => {
  try {
    return JSON.stringify(val, null, 2);
  } catch (e) {
    return String(val);
  }
};

const copy = (text) => {
  navigator.clipboard.writeText(text);
};
</script>

<style scoped>
.details-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.7);
  backdrop-filter: blur(4px);
}

.details-modal {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 12px;
  width: 90%;
  max-width: 700px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #334155;
  background: #0f172a;
  border-radius: 12px 12px 0 0;
}

.modal-header h3 {
  margin: 0;
  font-size: 1.25rem;
  color: #f1f5f9;
}

.btn-close {
  background: transparent;
  border: none;
  color: #94a3b8;
  font-size: 1.5rem;
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  line-height: 1;
}

.btn-close:hover {
  color: #f1f5f9;
}

.modal-body {
  padding: 1.5rem;
  overflow-y: auto;
  flex: 1;
}

.section {
  margin-bottom: 1.5rem;
}

.section-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  margin-bottom: 0.75rem;
}

.ids-grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.id-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #0f172a;
  padding: 0.75rem 1rem;
  border-radius: 8px;
  border: 1px solid #334155;
}

.id-label {
  color: #cbd5e1;
  font-weight: 500;
  font-size: 0.875rem;
}

.id-value-wrap {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.id-value {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.75rem;
  color: #818cf8;
  background: #1e1b4b;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  border: 1px solid #4338ca;
}

.btn-copy {
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 0.25rem;
  opacity: 0.6;
  transition: opacity 0.2s;
}

.btn-copy:hover {
  opacity: 1;
}

.code-block-wrap {
  position: relative;
}

.code-block {
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 1rem;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.75rem;
  color: #e2e8f0;
  overflow-x: auto;
  max-height: 200px;
  white-space: pre-wrap;
  word-break: break-all;
}

.btn-copy-block {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  background: #334155;
  border: 1px solid #475569;
  border-radius: 4px;
  padding: 0.25rem 0.5rem;
  cursor: pointer;
  opacity: 0.7;
  transition: opacity 0.2s;
}

.btn-copy-block:hover {
  opacity: 1;
}

.modal-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid #334155;
  display: flex;
  justify-content: flex-end;
  background: #0f172a;
  border-radius: 0 0 12px 12px;
}
</style>
