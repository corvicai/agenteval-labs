<template>
  <Transition name="fade">
    <div v-if="isVisible" class="maintenance-overlay">
      <div class="content">
        <div class="icon-pulse">
          <svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 24 24" fill="none"
            stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
            class="lucide lucide-wrench">
            <path
              d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
          </svg>
        </div>
        <h1>System Maintenance</h1>
        <p>The system is currently undergoing scheduled maintenance.</p>
        <p class="sub-text">We'll be back shortly. Stand by...</p>
        
        <div class="status-indicator">
          <div class="loader"></div>
          <span>Status: {{ statusText }}</span>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup>
import { ref, onMounted, watch, onUnmounted } from 'vue';

const props = defineProps({
  active: {
    type: Boolean,
    default: false
  }
});

const isVisible = ref(false);
const statusText = ref('Optimizing database...');
let pollInterval = null;

// Watch active prop to trigger visibility
watch(() => props.active, (newVal) => {
  if (newVal) {
    isVisible.value = true;
    startPolling();
  }
});

const startPolling = () => {
  if (pollInterval) return;
  
  statusText.value = 'Waiting for server...';
  
  pollInterval = setInterval(async () => {
    try {
      // Try to fetch the main page to see if we are back online
      // We look for a specific marker or just 200 OK and NO maintenance text
      const res = await fetch('/', { cache: 'no-store' });
      const text = await res.text();
      
      // If we see the maintenance page text, we represent "still in maintenance"
      if (text.includes('benchmarking-platform-maintenance-mode')) {
        statusText.value = 'System upgrading...';
        return; 
      }

      // If we get here, it means we likely got the real app back
      if (res.ok) {
        statusText.value = 'System online! Reloading...';
        clearInterval(pollInterval);
        setTimeout(() => {
          window.location.reload();
        }, 1000);
      }
    } catch (e) {
      statusText.value = 'Connection unavailable...';
    }
  }, 2000); // Poll every 2s
};

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
});
</script>

<style scoped>
.maintenance-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(15, 23, 42, 0.95);
  backdrop-filter: blur(10px);
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  flex-direction: column;
}

.content {
  text-align: center;
  max-width: 400px;
  padding: 2rem;
}

.icon-pulse {
  margin-bottom: 2rem;
  color: #3b82f6;
  animation: pulse-slow 3s infinite;
}

h1 {
  font-size: 2rem;
  font-weight: 700;
  margin-bottom: 1rem;
  background: linear-gradient(to right, #60a5fa, #a78bfa);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

p {
  color: #94a3b8;
  font-size: 1.1rem;
  margin-bottom: 0.5rem;
}

.sub-text {
  font-size: 0.9rem;
  opacity: 0.7;
  margin-bottom: 2rem;
}

.status-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  background: rgba(255, 255, 255, 0.1);
  padding: 0.75rem 1.5rem;
  border-radius: 99px;
  font-size: 0.9rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.loader {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-bottom-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@keyframes pulse-slow {
  0%, 100% { transform: scale(1); opacity: 0.8; }
  50% { transform: scale(1.1); opacity: 1; }
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.5s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
