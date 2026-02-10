import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { config } from './config.js'
console.log('AgentEval Version:', config.GIT_COMMIT || 'development')

createApp(App).mount('#app')
