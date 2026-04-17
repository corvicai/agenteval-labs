<template>
  <div class="modal-overlay" @click.self="$emit('cancel')">
    <div class="modal-container force-delete-modal">
      <div class="modal-header force-delete-header">
        <span class="delete-icon">⚠️</span>
        <h3>Force delete agent</h3>
      </div>

      <div class="modal-body">
        <p class="agent-target">
          <strong>{{ agentName }}</strong>
        </p>

        <div class="warning-block">
          <p>
            This agent's configuration is <strong>unreadable</strong> due to an encryption key mismatch.
            A copy of the encrypted data will be preserved in the recovery archive before deletion.
          </p>
          <ul class="consequence-list">
            <li>The agent and all its credentials will be permanently removed.</li>
            <li v-if="collabCount > 0">
              <strong>{{ collabCount }} collaborator{{ collabCount === 1 ? '' : 's' }}</strong>
              will immediately lose access.
            </li>
            <li>Question set overrides that reference this agent will be removed.</li>
            <li>Existing run history is <em>not</em> deleted.</li>
          </ul>
        </div>

        <p class="recovery-note">
          💡 If you later find the original encryption key, the encrypted credentials can be recovered
          from the <code>agent_config_quarantines</code> table.
        </p>
      </div>

      <div class="modal-footer">
        <button class="btn btn-secondary" @click="$emit('cancel')">Cancel</button>
        <button class="btn btn-danger" @click="$emit('confirm')">Force delete</button>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  agentName: { type: String, required: true },
  collabCount: { type: Number, default: 0 },
})
defineEmits(['confirm', 'cancel'])
</script>

<style scoped>
.force-delete-modal {
  max-width: 480px;
  width: 100%;
}

.force-delete-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid #fee2e2;
}

.delete-icon {
  font-size: 1.4rem;
}

.force-delete-header h3 {
  margin: 0;
  font-size: 1.1rem;
  color: #991b1b;
}

.agent-target {
  font-size: 1rem;
  margin: 0 0 12px;
  color: #1e293b;
}

.warning-block {
  background: #fff1f2;
  border: 1px solid #fecdd3;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
}

.warning-block p {
  margin: 0 0 8px;
  color: #9f1239;
  font-size: 0.9rem;
  line-height: 1.5;
}

.consequence-list {
  margin: 0;
  padding-left: 20px;
  color: #7f1d1d;
  font-size: 0.875rem;
  line-height: 1.6;
}

.recovery-note {
  font-size: 0.825rem;
  color: #475569;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 10px 12px;
  margin: 0;
  line-height: 1.5;
}

.recovery-note code {
  font-family: monospace;
  background: rgba(0, 0, 0, 0.06);
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 0.8rem;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid #f1f5f9;
  margin-top: 16px;
}

.btn {
  padding: 8px 18px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid transparent;
  transition: background 0.15s;
}

.btn-secondary {
  background: #f1f5f9;
  color: #334155;
  border-color: #e2e8f0;
}

.btn-secondary:hover {
  background: #e2e8f0;
}

.btn-danger {
  background: #dc2626;
  color: #fff;
  border-color: #b91c1c;
}

.btn-danger:hover {
  background: #b91c1c;
}
</style>
