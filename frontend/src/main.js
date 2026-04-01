import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { config } from './config.js'

console.log('AgentEval Revision:', {
  commit: config.APP_REVISION || config.GIT_COMMIT || 'unknown',
  branch: config.APP_REVISION_BRANCH || '',
  dirty: config.APP_REVISION_DIRTY || '',
  updated_at: config.APP_REVISION_UPDATED_AT || ''
})

createApp(App).mount('#app')
