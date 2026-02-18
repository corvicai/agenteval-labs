<template>
  <div class="docs-view">
    <div class="docs-header">
      <h1>📂 Documentation & Guides</h1>
      <p>Master your benchmarking environment and agent evaluations.</p>
    </div>

    <div class="docs-content">
      <!-- Section: Agents -->
      <section class="docs-section">
        <h2>🤖 Agent Configuration</h2>
        <div class="card">
          <h3>Corvic Agents (MCP)</h3>
          <p>These are agents connected via the <strong>Model Context Protocol</strong>. They leverage specialized tools and context to answer questions.</p>
          <ul>
            <li><strong>Endpoint URL:</strong> The HTTP/SSE endpoint where your agent listener is running.</li>
            <li><strong>Token:</strong> Security token for your agent. Use <code>MOCK</code> for dry-runs.</li>
            <li><strong>Max Parallel Requests:</strong> Limits how many concurrent questions this agent processes. Defaults to 5.</li>
          </ul>
        </div>

        <div class="card">
          <h3>Supported Agent Providers</h3>
          <p>The platform supports multiple providers for execution and evaluation workflows.</p>
          <ul>
            <li><strong>mcp:</strong> Corvic agents via MCP endpoint + token.</li>
            <li><strong>openai:</strong> OpenAI API.</li>
            <li><strong>nvidia:</strong> NVIDIA NIM API. Browse models at <a href="https://build.nvidia.com/models" target="_blank" rel="noopener noreferrer">build.nvidia.com/models</a>.</li>
            <li><strong>openrouter:</strong> OpenRouter API.</li>
            <li><strong>anthropic:</strong> Anthropic/Claude API.</li>
            <li><strong>openai_compatible:</strong> Any provider exposing an OpenAI-compatible endpoint (<code>base_url</code> + API key).</li>
            <li><strong>evaluator:</strong> Multi-provider evaluator agent (recommended for benchmark scoring).</li>
          </ul>
        </div>
      </section>

      <!-- Section: Evaluators -->
      <section class="docs-section">
        <h2>⚖️ Evaluator Agents</h2>
        <p>Evaluators are specialized agents that score responses from other agents. They compare each answer against the user question and, when present, the expected answer.</p>
        
        <div class="card">
          <h3>1. Evaluator Provider Options</h3>
          <p>You can pick one evaluator provider explicitly in <strong>Manage Agents</strong>:</p>
          <ul>
            <li><strong>OpenAI:</strong> <code>openai_api_key</code> (or <code>api_key</code>), model, and optional managed mode (<code>prompt_id</code>).</li>
            <li><strong>NVIDIA NIM:</strong> <code>nvidia_api_key</code>, model, and optional base URL/system prompt.</li>
            <li><strong>OpenRouter:</strong> <code>openrouter_api_key</code>, model, and optional base URL/system prompt.</li>
            <li><strong>Anthropic:</strong> <code>anthropic_api_key</code>, model, and optional system prompt.</li>
            <li><strong>OpenAI-Compatible:</strong> <code>compatible_api_key</code> + <code>compatible_base_url</code> for custom providers.</li>
          </ul>
          <p><strong>Tip:</strong> prefer explicit provider selection instead of auto mode for predictable behavior.</p>
        </div>

        <div class="card">
          <h3>2. Credentials Checklist</h3>
          <p>Before running benchmark/evaluation, ensure:</p>
          <ul>
            <li>Your selected evaluator provider has valid credentials.</li>
            <li>The evaluator has a target primary agent in Run Setup.</li>
            <li>Your account has quota/credits in the provider dashboard.</li>
          </ul>
        </div>

        <div class="card highlight">
          <h3>3. Recommended System Prompt</h3>
          <p>Configure your <strong>Evaluator Agent</strong> with this system prompt to ensure consistent and strict benchmarking results. This is the same default prompt used by the API when Standard mode has no custom system prompt.</p>
          
          <div class="code-block-container">
            <button class="btn-copy" @click="copyToClipboard">
              {{ copied ? '✅ Copied!' : '📋 Copy Prompt' }}
            </button>
            <div class="code-block">
            <pre>
<code v-if="isPromptLoading">Loading prompt from API...</code>
<code v-else-if="evaluatorSystemPrompt">{{ evaluatorSystemPrompt }}</code>
<code v-else>Unable to load prompt from API.</code>
            </pre>
            </div>
            <p v-if="promptLoadError" class="prompt-error">{{ promptLoadError }}</p>
          </div>
        </div>
      </section>

      <!-- Section: General Tips -->
      <section class="docs-section">
        <h2>💡 Tips & Best Practices</h2>
        <div class="tips-grid">
          <div class="tip-card">
            <h4>Expected Answer Quality</h4>
            <p>Write expected answers logically and concisely. State exactly what must be present in a correct response, with objective facts and format constraints when needed.</p>
          </div>
          <div class="tip-card">
            <h4>Expected Answer Structure</h4>
            <p>Prefer clear acceptance criteria: required entities, key facts, units/dates, and boundaries for correctness. Avoid vague goals like “good explanation”.</p>
          </div>
          <div class="tip-card">
            <h4>Evaluator Precision</h4>
            <p>The clearer your expected answer, the more precise and stable the evaluator score becomes across runs and providers.</p>
          </div>
          <div class="tip-card">
            <h4>Rate Limiting</h4>
            <p>If you see <code>429 Too Many Requests</code>, reduce the "Max Parallel Requests" in Agent settings or check your API quotas.</p>
          </div>
          <div class="tip-card">
            <h4>Caching</h4>
            <p>The system caches results to improve performance. Use "Re-run" on a specific question if you want to bypass the cache and force a new execution.</p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const copied = ref(false)
const evaluatorSystemPrompt = ref('')
const isPromptLoading = ref(true)
const promptLoadError = ref('')

async function loadPromptFromAPI() {
  isPromptLoading.value = true
  promptLoadError.value = ''
  try {
    const response = await fetch('/api/prompts/evaluator-system', {
      method: 'GET',
      credentials: 'include'
    })
    if (!response.ok) {
      throw new Error(`Request failed: ${response.status}`)
    }
    const data = await response.json()
    const prompt = typeof data?.prompt === 'string' ? data.prompt.trim() : ''
    if (!prompt) {
      throw new Error('Prompt is empty')
    }
    evaluatorSystemPrompt.value = prompt
  } catch (err) {
    console.error('Failed to load evaluator prompt from API:', err)
    evaluatorSystemPrompt.value = ''
    promptLoadError.value = 'Prompt is unavailable right now. Check API health and try again.'
  } finally {
    isPromptLoading.value = false
  }
}

async function copyToClipboard() {
  if (!evaluatorSystemPrompt.value) return
  try {
    await navigator.clipboard.writeText(evaluatorSystemPrompt.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch (err) {
    console.error('Failed to copy text: ', err)
  }
}

onMounted(() => {
  loadPromptFromAPI()
})
</script>

<style scoped>
.docs-view {
  max-width: 1000px;
  margin: 0 auto;
  padding: 2rem;
}

.docs-header {
  margin-bottom: 3rem;
  text-align: center;
}

.docs-header h1 {
  font-size: 2.5rem;
  color: #1a1f36;
  margin-bottom: 0.5rem;
}

.docs-header p {
  font-size: 1.125rem;
  color: #64748b;
}

.docs-section {
  margin-bottom: 4rem;
}

.docs-section h2 {
  font-size: 1.75rem;
  color: #1a1f36;
  border-bottom: 2px solid #e2e8f0;
  padding-bottom: 0.5rem;
  margin-bottom: 1.5rem;
}

.card {
  background: white;
  border-radius: 12px;
  padding: 2rem;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  margin-bottom: 1.5rem;
}

.card.highlight {
  border-left: 6px solid #6366f1;
}

.card h3 {
  margin-top: 0;
  margin-bottom: 1rem;
  color: #1a1f36;
}

.card p {
  color: #475569;
  line-height: 1.6;
}

.card ul {
  color: #475569;
  padding-left: 1.5rem;
}

.card li {
  margin-bottom: 0.5rem;
}

.code-block-container {
  position: relative;
  margin-top: 1.5rem;
}

.btn-copy {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  z-index: 10;
  padding: 0.5rem 1rem;
  font-size: 0.75rem;
  font-weight: 600;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  box-shadow: 0 1px 2px rgba(0,0,0,0.05);
}

.btn-copy:hover {
  background: #f8fafc;
  border-color: #cbd5e1;
}

.prompt-error {
  margin-top: 0.75rem;
  font-size: 0.85rem;
  color: #b91c1c;
}

.code-block {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 1.5rem;
  padding-top: 3rem; /* Space for copy button */
  overflow-x: auto;
  max-height: 300px;
  overflow-y: auto;
}

.code-block::-webkit-scrollbar {
  width: 8px;
}

.code-block::-webkit-scrollbar-track {
  background: #f1f5f9;
  border-radius: 0 8px 8px 0;
}

.code-block::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 4px;
}

.code-block::-webkit-scrollbar-thumb:hover {
  background: #94a3b8;
}

.code-block pre {
  margin: 0;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
  font-size: 0.875rem;
  line-height: 1.7;
  color: #1e293b;
  white-space: pre-wrap;
}

.docs-actions {
  display: flex;
  gap: 1rem;
  margin-top: 1.5rem;
}

.docs-actions .btn {
  text-decoration: none;
  font-size: 0.875rem;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

.tips-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 1.5rem;
}

.tip-card {
  background: #fdf2f8;
  border: 1px solid #fbcfe8;
  border-radius: 12px;
  padding: 1.5rem;
}

.tip-card h4 {
  margin-top: 0;
  color: #9d174d;
  margin-bottom: 0.5rem;
}

.tip-card p {
  color: #be185d;
  font-size: 0.9375rem;
  margin: 0;
}
</style>
