<template>
  <Transition name="afk-fade">
    <div v-if="active" class="afk-overlay">
      <div class="afk-card">
        <h2>{{ titleText }}</h2>
        <p>{{ descriptionText }}</p>
        <button class="btn-reconnect" :disabled="reconnecting" @click="$emit('reconnect')">
          {{ reconnecting ? 'Reconnecting...' : buttonText }}
        </button>
      </div>
    </div>
  </Transition>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  active: {
    type: Boolean,
    default: false
  },
  reconnecting: {
    type: Boolean,
    default: false
  },
  mode: {
    type: String,
    default: 'afk',
    validator: (value) => ['afk', 'connection'].includes(value)
  }
})

defineEmits(['reconnect'])

const titleText = computed(() => (
  props.mode === 'connection'
    ? 'Connection lost. Trying to recover.'
    : "We missed you. Glad you're back."
))

const descriptionText = computed(() => (
  props.mode === 'connection'
    ? 'The real-time connection dropped. Click below if it does not reconnect automatically.'
    : 'Your session is currently paused. Click below to reconnect.'
))

const buttonText = computed(() => (
  props.mode === 'connection' ? 'Reconnect' : 'Reconnect now'
))
</script>

<style scoped>
.afk-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(12, 20, 36, 0.82);
  backdrop-filter: blur(4px);
  padding: 1rem;
}

.afk-card {
  width: min(520px, 100%);
  background: #ffffff;
  border: 1px solid #d6deeb;
  border-radius: 16px;
  padding: 1.5rem;
  box-shadow: 0 20px 48px rgba(15, 23, 42, 0.28);
  text-align: center;
}

.afk-card h2 {
  margin: 0 0 0.65rem;
  color: #0f172a;
  font-size: 1.3rem;
}

.afk-card p {
  margin: 0 0 1.15rem;
  color: #334155;
  line-height: 1.45;
}

.btn-reconnect {
  border: 0;
  background: #1d4ed8;
  color: #ffffff;
  border-radius: 10px;
  padding: 0.72rem 1.1rem;
  font-size: 0.95rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 120ms ease;
}

.btn-reconnect:hover:not(:disabled) {
  background: #1e40af;
}

.btn-reconnect:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.afk-fade-enter-active,
.afk-fade-leave-active {
  transition: opacity 0.2s ease;
}

.afk-fade-enter-from,
.afk-fade-leave-to {
  opacity: 0;
}
</style>
