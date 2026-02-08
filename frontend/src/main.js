import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
console.log('AgentEval Version:', import.meta.env.VITE_GIT_COMMIT || 'development')

createApp(App).mount('#app')
