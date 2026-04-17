<template>
  <div class="modal-overlay" @click.self="handleClose">
    <div class="modal-container agent-manager-modal">
      <div class="modal-header">
        <h3>🤖 Manage Agents</h3>
        <button class="btn-close" @click="handleClose">×</button>
      </div>
      
      <div class="modal-body">
        <div v-if="!hasEnabledAgents && localAgents.length > 0" class="alert alert-warning">
          ⚠️ <strong>Warning:</strong> No primary agents are enabled. You won't be able to run benchmarks.
        </div>

        <div v-if="hasUndecryptableAgents" class="alert alert-danger">
          🔐 <strong>Credential issue detected:</strong> One or more agents lost their credentials due to an encryption key change.
          Open each agent marked with <strong>🔐 Needs credentials</strong> and re-enter its API key/token to restore it.
        </div>

        <div v-if="!editingAgentId" class="agent-tabs">
          <button :class="['tab-btn', { active: activeTab === 'agents' }]" @click="activeTab = 'agents'">
            Agents <span class="tab-count">{{ agentsTabCount }}</span>
          </button>
          <button :class="['tab-btn', { active: activeTab === 'evaluators' }]" @click="activeTab = 'evaluators'">
            Evaluators <span class="tab-count">{{ evaluatorsTabCount }}</span>
          </button>
        </div>

        <div v-if="editingAgentId" class="agent-breadcrumb">
          <button class="btn-back" @click="closeAgentConfig">← Back</button>
          <span class="breadcrumb-name">{{ editingAgent?.name }}</span>
          <span class="agent-type-badge" :class="editingAgent?.provider_type">{{ getAgentTypeLabel(editingAgent) }}</span>
        </div>

        <div class="agents-list">
          <div
            v-for="(agent, index) in visibleAgents"
            :key="agent.id"
            class="agent-card"
            :class="{
              'disabled-card': !agent.enabled,
              'clickable': !editingAgentId && !agent.is_shared,
              'shared-card': agent.is_shared
            }"
            @click="!editingAgentId && !agent.is_shared && openAgentConfig(agent)"
          >
            <div class="agent-header" :class="{ 'no-mb': editingAgentId !== agent.id }">
              <span class="agent-name-text">{{ agent.name }}</span>
              <span v-if="agent.config_status === 'needs_recredentials'" class="creds-lost-badge" title="Credentials lost — re-enter to restore">
                🔐 Needs credentials
              </span>
              <span class="agent-type-badge" :class="agent.provider_type">
                {{ getAgentTypeLabel(agent) }}
              </span>
              <span v-if="agent.is_shared" class="shared-badge" :title="'Shared by ' + (agent.owner_name || 'another user')">
                shared · @{{ agent.owner_name || 'owner' }}
              </span>
              <div class="agent-actions">
                <button
                  v-if="!agent.is_shared"
                  class="btn-icon"
                  @click.stop="toggleAgent(agent)"
                  :title="agent.enabled ? 'Disable' : 'Enable'"
                >
                  {{ agent.enabled ? '✅' : '⏸️' }}
                </button>
                <button
                  v-if="!agent.is_shared"
                  class="btn-icon"
                  @click.stop="openShareModal(agent)"
                  title="Share agent"
                >🔗</button>
                <button class="btn-icon" @click.stop="showSpyModal(agent)" title="Spy Payload">🔍</button>
                <button
                  v-if="!agent.is_shared"
                  class="btn-icon btn-danger-icon"
                  @click.stop="deleteAgent(agent)"
                  title="Delete"
                >🗑️</button>
              </div>
            </div>
            
            <div v-if="editingAgentId === agent.id" class="agent-config">
              <div class="config-fields">
                <div class="field full-width">
                  <label>Agent Name</label>
                  <input v-model="agent.name" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" placeholder="Agent name..." />
                </div>
              </div>
              <div v-if="agent.provider_type === 'mcp'" class="config-fields-mcp">
                <!-- Mode selection removed, defaulting to HTTP -->
                <div class="field full-width">
                  <span class="mode-badge">Remote (HTTP / SSE)</span>
                </div>
                
                <div class="field">
                  <label>Endpoint URL</label>
                  <input v-model="agent.config.endpoint" placeholder="https://..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Token</label>
                  <input v-model="agent.config.token" type="password" placeholder="Token..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                
                <div v-if="isDev" class="alert alert-info mt-2 full-width">
                  <strong>💡 Dev Tip:</strong> Set Token to <code>MOCK</code> to simulate responses.
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'openai'" class="config-fields">
                <div class="field">
                  <label>API Key</label>
                  <input v-model="agent.config.api_key" type="password" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Prompt ID</label>
                  <input v-model="agent.config.prompt_id" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Prompt Version</label>
                  <input v-model="agent.config.prompt_version" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Project ID (Optional)</label>
                  <input v-model="agent.config.project_id" placeholder="proj_..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'nvidia'" class="config-fields">
                <div class="field">
                  <label>NVIDIA API Key</label>
                  <input v-model="agent.config.api_key" type="password" placeholder="nvapi-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Model</label>
                  <input v-model="agent.config.model" placeholder="meta/llama-3.1-8b-instruct" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  <small class="field-hint">
                    Find NVIDIA model IDs at
                    <a href="https://build.nvidia.com/models" target="_blank" rel="noopener noreferrer">build.nvidia.com/models</a>.
                  </small>
                </div>
                <div class="field full-width">
                  <label>Base URL (Optional)</label>
                  <input v-model="agent.config.base_url" placeholder="https://integrate.api.nvidia.com/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  <small class="field-hint">Leave empty to use the default NVIDIA NIM endpoint.</small>
                </div>
                <div class="field full-width">
                  <label>System Prompt (Optional)</label>
                  <textarea
                    v-model="agent.config.system_prompt"
                    rows="3"
                    placeholder="Optional system instructions"
                    @focus="startEditing"
                    @blur="saveAgent(agent); stopEditing()"
                    @input="markPendingChanges(agent)"
                  />
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'openrouter'" class="config-fields">
                <div class="field">
                  <label>OpenRouter API Key</label>
                  <input v-model="agent.config.openrouter_api_key" type="password" placeholder="sk-or-v1-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Model</label>
                  <input v-model="agent.config.openrouter_model" placeholder="openai/gpt-4o-mini" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>Base URL (Optional)</label>
                  <input v-model="agent.config.openrouter_base_url" placeholder="https://openrouter.ai/api/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>HTTP-Referer (Optional)</label>
                  <input v-model="agent.config.openrouter_http_referer" placeholder="https://your-app.example" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>X-Title (Optional)</label>
                  <input v-model="agent.config.openrouter_x_title" placeholder="Agenteval Labs" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>System Prompt (Optional)</label>
                  <textarea
                    v-model="agent.config.openrouter_system_prompt"
                    rows="3"
                    placeholder="Optional system instructions"
                    @focus="startEditing"
                    @blur="saveAgent(agent); stopEditing()"
                    @input="markPendingChanges(agent)"
                  />
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'openai_compatible'" class="config-fields">
                <div class="field">
                  <label>API Key</label>
                  <input v-model="agent.config.compatible_api_key" type="password" placeholder="token..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Model</label>
                  <input v-model="agent.config.compatible_model" placeholder="gpt-4o-mini" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>Base URL (Required)</label>
                  <input v-model="agent.config.compatible_base_url" placeholder="https://your-provider.example/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>System Prompt (Optional)</label>
                  <textarea
                    v-model="agent.config.compatible_system_prompt"
                    rows="3"
                    placeholder="Optional system instructions"
                    @focus="startEditing"
                    @blur="saveAgent(agent); stopEditing()"
                    @input="markPendingChanges(agent)"
                  />
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'anthropic'" class="config-fields">
                <div class="field">
                  <label>Anthropic API Key</label>
                  <input v-model="agent.config.anthropic_api_key" type="password" placeholder="sk-ant-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Model</label>
                  <input v-model="agent.config.anthropic_model" placeholder="claude-3-5-sonnet-latest" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>Base URL (Optional)</label>
                  <input v-model="agent.config.anthropic_base_url" placeholder="https://api.anthropic.com/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field">
                  <label>Anthropic Version (Optional)</label>
                  <input v-model="agent.config.anthropic_version" placeholder="2023-06-01" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                </div>
                <div class="field full-width">
                  <label>System Prompt (Optional)</label>
                  <textarea
                    v-model="agent.config.anthropic_system_prompt"
                    rows="3"
                    placeholder="Optional system instructions"
                    @focus="startEditing"
                    @blur="saveAgent(agent); stopEditing()"
                    @input="markPendingChanges(agent)"
                  />
                </div>
              </div>
              <div v-else-if="agent.provider_type === 'evaluator'" class="config-fields">
                <div class="field full-width">
                  <label>Evaluator Provider</label>
                  <select
                    v-model="agent.config.llm_provider"
                    @focus="startEditing"
                    @blur="saveAgent(agent); stopEditing()"
                    @change="onEvaluatorProviderChange(agent)"
                  >
                    <option value="openai">OpenAI</option>
                    <option value="nvidia">NVIDIA NIM</option>
                    <option value="anthropic">Claude (Anthropic)</option>
                    <option value="openrouter">OpenRouter</option>
                    <option value="openai_compatible">OpenAI-Compatible</option>
                    <option value="auto">Auto (Legacy Fallback)</option>
                  </select>
                  <small class="field-hint">
                    Pick the provider explicitly. Use OpenAI-Compatible for custom endpoints not listed here.
                  </small>
                </div>

                <template v-if="showOpenAIEvaluatorFields(agent)">
                  <div class="field full-width" v-if="getEvaluatorProvider(agent) === 'auto'">
                    <label>OpenAI Settings</label>
                  </div>
                  <div class="field full-width">
                    <label>OpenAI Mode</label>
                    <select
                      v-model="agent.config.openai_mode"
                      @focus="startEditing"
                      @blur="saveAgent(agent); stopEditing()"
                      @change="markPendingChanges(agent)"
                    >
                      <option value="managed">Managed Prompt</option>
                      <option value="standard">Standard API</option>
                    </select>
                    <small class="field-hint">
                      Managed uses Prompt ID on OpenAI. Standard injects a system prompt on every call.
                    </small>
                  </div>
                  <div class="field">
                    <label>OpenAI API Key</label>
                    <input v-model="agent.config.openai_api_key" type="password" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" placeholder="sk-..." />
                  </div>
                  <template v-if="getEvaluatorMode(agent) === 'managed'">
                    <div class="field">
                      <label>Prompt ID (Required)</label>
                      <input v-model="agent.config.openai_prompt_id" placeholder="prompt_..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    </div>
                    <div class="field">
                      <label>Prompt Version (Optional)</label>
                      <input v-model="agent.config.openai_prompt_version" placeholder="e.g. v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    </div>
                    <div class="field">
                      <label>Project ID (Optional)</label>
                      <input v-model="agent.config.openai_project_id" placeholder="proj_..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    </div>
                  </template>
                  <template v-else>
                    <div class="field">
                      <label>Model</label>
                      <input v-model="agent.config.openai_model" placeholder="gpt-4o-mini" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    </div>
                    <div class="field">
                      <label>Project ID (Optional)</label>
                      <input v-model="agent.config.openai_project_id" placeholder="proj_..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    </div>
                    <div class="field full-width">
                      <label>System Prompt (Injected on every request)</label>
                      <textarea
                        v-model="agent.config.openai_system_prompt"
                        rows="4"
                        placeholder="Optional. Leave blank to use the platform default evaluator prompt."
                        @focus="startEditing"
                        @blur="saveAgent(agent); stopEditing()"
                        @input="markPendingChanges(agent)"
                      />
                      <small class="field-hint">If empty, the API uses the backend default evaluator prompt.</small>
                    </div>
                  </template>
                </template>

                <template v-if="showNVIDIAEvaluatorFields(agent)">
                  <div class="field full-width" v-if="getEvaluatorProvider(agent) === 'auto'">
                    <label>NVIDIA Settings</label>
                  </div>
                  <div class="field">
                    <label>NVIDIA API Key</label>
                    <input v-model="agent.config.nvidia_api_key" type="password" placeholder="nvapi-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>Model</label>
                    <input v-model="agent.config.nvidia_model" placeholder="meta/llama-3.1-8b-instruct" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                    <small class="field-hint">
                      Find NVIDIA model IDs at
                      <a href="https://build.nvidia.com/models" target="_blank" rel="noopener noreferrer">build.nvidia.com/models</a>.
                    </small>
                  </div>
                  <div class="field full-width">
                    <label>Base URL (Optional)</label>
                    <input v-model="agent.config.nvidia_base_url" placeholder="https://integrate.api.nvidia.com/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>System Prompt (Optional)</label>
                    <textarea
                      v-model="agent.config.nvidia_system_prompt"
                      rows="4"
                      placeholder="Optional NVIDIA evaluator system instructions."
                      @focus="startEditing"
                      @blur="saveAgent(agent); stopEditing()"
                      @input="markPendingChanges(agent)"
                    />
                  </div>
                </template>

                <template v-if="showOpenRouterEvaluatorFields(agent)">
                  <div class="field full-width" v-if="getEvaluatorProvider(agent) === 'auto'">
                    <label>OpenRouter Settings</label>
                  </div>
                  <div class="field">
                    <label>OpenRouter API Key</label>
                    <input v-model="agent.config.openrouter_api_key" type="password" placeholder="sk-or-v1-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>Model</label>
                    <input v-model="agent.config.openrouter_model" placeholder="openai/gpt-4o-mini" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>Base URL (Optional)</label>
                    <input v-model="agent.config.openrouter_base_url" placeholder="https://openrouter.ai/api/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>HTTP-Referer (Optional)</label>
                    <input v-model="agent.config.openrouter_http_referer" placeholder="https://your-app.example" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>X-Title (Optional)</label>
                    <input v-model="agent.config.openrouter_x_title" placeholder="Agenteval Labs" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>System Prompt (Optional)</label>
                    <textarea
                      v-model="agent.config.openrouter_system_prompt"
                      rows="4"
                      placeholder="Optional OpenRouter evaluator system instructions."
                      @focus="startEditing"
                      @blur="saveAgent(agent); stopEditing()"
                      @input="markPendingChanges(agent)"
                    />
                  </div>
                </template>

                <template v-if="showAnthropicEvaluatorFields(agent)">
                  <div class="field full-width" v-if="getEvaluatorProvider(agent) === 'auto'">
                    <label>Claude / Anthropic Settings</label>
                  </div>
                  <div class="field">
                    <label>Anthropic API Key</label>
                    <input v-model="agent.config.anthropic_api_key" type="password" placeholder="sk-ant-..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>Model</label>
                    <input v-model="agent.config.anthropic_model" placeholder="claude-3-5-sonnet-latest" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>Base URL (Optional)</label>
                    <input v-model="agent.config.anthropic_base_url" placeholder="https://api.anthropic.com/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>Anthropic Version (Optional)</label>
                    <input v-model="agent.config.anthropic_version" placeholder="2023-06-01" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>System Prompt (Optional)</label>
                    <textarea
                      v-model="agent.config.anthropic_system_prompt"
                      rows="4"
                      placeholder="Optional Anthropic evaluator system instructions."
                      @focus="startEditing"
                      @blur="saveAgent(agent); stopEditing()"
                      @input="markPendingChanges(agent)"
                    />
                  </div>
                </template>

                <template v-if="showCompatibleEvaluatorFields(agent)">
                  <div class="field full-width" v-if="getEvaluatorProvider(agent) === 'auto'">
                    <label>OpenAI-Compatible Settings</label>
                  </div>
                  <div class="field">
                    <label>API Key</label>
                    <input v-model="agent.config.compatible_api_key" type="password" placeholder="token..." @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field">
                    <label>Model</label>
                    <input v-model="agent.config.compatible_model" placeholder="gpt-4o-mini" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>Base URL (Required)</label>
                    <input v-model="agent.config.compatible_base_url" placeholder="https://your-provider.example/v1" @focus="startEditing" @blur="saveAgent(agent); stopEditing()" @input="markPendingChanges(agent)" />
                  </div>
                  <div class="field full-width">
                    <label>System Prompt (Optional)</label>
                    <textarea
                      v-model="agent.config.compatible_system_prompt"
                      rows="4"
                      placeholder="Optional OpenAI-compatible evaluator system instructions."
                      @focus="startEditing"
                      @blur="saveAgent(agent); stopEditing()"
                      @input="markPendingChanges(agent)"
                    />
                  </div>
                </template>
              </div>
              
              <!-- Common Rate Limiting Field for all agent types -->
              <div class="config-fields rate-limit-field">
                <div class="field">
                  <label title="Maximum number of parallel requests to this agent (prevents rate limiting errors)">
                    ⚡ Max Parallel Requests
                  </label>
                  <input 
                    type="number" 
                    v-model.number="agent.max_concurrency" 
                    min="1" 
                    max="20" 
                    placeholder="5"
                    @focus="startEditing" 
                    @blur="saveAgent(agent); stopEditing()" 
                    @input="markPendingChanges(agent)" 
                  />
                  <small class="field-hint">Lower = safer for rate-limited APIs</small>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <div class="agent-config-actions">
          <template v-if="!editingAgentId">
            <button v-if="activeTab === 'agents'" class="btn btn-primary" @click="addAgent('mcp')">+ Corvic Agent</button>
            <button v-if="activeTab === 'evaluators'" class="btn btn-secondary" @click="openEvaluatorModeModal">+ Eval Agent</button>
          </template>
          <button v-else class="btn btn-secondary" @click="closeAgentConfig">← Back to list</button>
        </div>
        <div class="footer-right">
          <span v-if="saveStatus" class="save-status" :class="saveStatus">{{ saveStatusText }}</span>
          <button class="btn btn-primary" @click="saveAllAgents" :disabled="!pendingChanges || saveStatus === 'saving'">Save Changes</button>
          <button class="btn btn-secondary" @click="handleClose">Close</button>
        </div>
      </div>

      <!-- Evaluator Mode Modal (Nested) -->
      <div v-if="showEvaluatorModeModal" class="modal-overlay" style="z-index: 1001;" @click.self="closeEvaluatorModeModal">
        <div class="modal-container evaluator-mode-modal">
          <div class="modal-header">
            <h3>🧪 Create Evaluator</h3>
            <button class="btn-close" @click="closeEvaluatorModeModal">×</button>
          </div>
          <div class="modal-body">
            <p class="evaluator-mode-intro">
              <strong>Choose one evaluator provider.</strong>
              Known providers are pre-configured; for others use OpenAI-Compatible with custom base URL.
            </p>
            <div class="evaluator-mode-grid">
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('managed')">
                <span class="mode-title">Eval OpenAI Managed</span>
                <span class="mode-subtitle">Prompt ID / Prompt Version</span>
                <span class="mode-description">
                  Uses a managed prompt stored in OpenAI. Good when prompt governance is centralized.
                </span>
              </button>
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('standard')">
                <span class="mode-title">Eval OpenAI Standard</span>
                <span class="mode-subtitle">Model + System Prompt</span>
                <span class="mode-description">
                  Sends model input with system prompt injected on every request.
                </span>
              </button>
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('nvidia')">
                <span class="mode-title">Eval NVIDIA</span>
                <span class="mode-subtitle">NIM API</span>
                <span class="mode-description">
                  Uses NVIDIA NIM models with your NVIDIA API key.
                </span>
              </button>
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('openrouter')">
                <span class="mode-title">Eval OpenRouter</span>
                <span class="mode-subtitle">OpenRouter API</span>
                <span class="mode-description">
                  Uses OpenRouter endpoint and model catalog.
                </span>
              </button>
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('anthropic')">
                <span class="mode-title">Eval Claude</span>
                <span class="mode-subtitle">Anthropic Messages API</span>
                <span class="mode-description">
                  Uses Anthropic Claude models with native Messages API.
                </span>
              </button>
              <button class="evaluator-mode-card" @click="selectEvaluatorMode('openai_compatible')">
                <span class="mode-title">Eval Compatible</span>
                <span class="mode-subtitle">Custom OpenAI-like API</span>
                <span class="mode-description">
                  For providers with OpenAI-compatible chat/completions endpoints.
                </span>
              </button>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="closeEvaluatorModeModal">Cancel</button>
          </div>
        </div>
      </div>

      <!-- Share Agent Modal (Nested, owner only) -->
      <ShareAgentModal
        v-if="shareAgentTarget"
        :agent-id="shareAgentTarget.id"
        :agent-name="shareAgentTarget.name"
        @close="shareAgentTarget = null"
      />

      <!-- Force Delete Confirmation Modal (Nested) -->
      <AgentForceDeleteModal
        v-if="forceDeleteAgent"
        :agent-name="forceDeleteAgent.name"
        @confirm="confirmForceDelete"
        @cancel="forceDeleteAgent = null"
      />

      <!-- Spy Modal (Nested) -->
      <div v-if="spyAgent" class="modal-overlay" style="z-index: 1001;" @click.self="spyAgent = null">
        <div class="modal-container payload-modal">
          <div class="modal-header">
            <h3>🔍 Spy Payload: {{ spyAgent.name }}</h3>
            <button class="btn-close" @click="spyAgent = null">×</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">
              This shows what will be sent to the Go runner (secrets are redacted):
            </p>
            <pre class="payload-content">{{ JSON.stringify(spyPayload, null, 2) }}</pre>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="spyAgent = null">Close</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, computed, onMounted } from 'vue'

import { capturePostHogEvent } from '../services/posthog.js'
import { wsService } from '../services/websocket.js'
import { generateAgentName } from '../utils/nameGenerator.js'
import ShareAgentModal from './ShareAgentModal.vue'
import AgentForceDeleteModal from './AgentForceDeleteModal.vue'

const isDev = import.meta.env.DEV

const props = defineProps({
  agents: Array,
  workspaceId: String,
  questionSet: Object,
  initialAgentId: String
})

const emit = defineEmits(['update', 'close'])

const localAgents = ref([])
const hasEnabledAgents = computed(() => localAgents.value.some(a => a.enabled && a.provider_type !== 'evaluator'))
const hasUndecryptableAgents = computed(() => localAgents.value.some(a => a.config_status === 'needs_recredentials'))

const spyAgent = ref(null)
const spyPayload = ref(null)
const shareAgentTarget = ref(null)

function openShareModal(agent) {
  if (agent?.is_shared) return
  shareAgentTarget.value = agent
}
const saveStatus = ref(null) // 'saving', 'saved', 'error'
const saveStatusText = ref('')
const pendingChanges = ref(false)
const dirtyAgentIds = ref(new Set())

// Force-delete confirmation modal state
const forceDeleteModal = ref(null) // { agent, resolve }
const forceDeleteAgent = ref(null)
const pendingCreateIds = ref(new Set())
const isEditing = ref(false) // Block watcher updates when editing
const showEvaluatorModeModal = ref(false)

const activeTab = ref('agents') // 'agents' | 'evaluators'
const editingAgentId = ref(null)

const editingAgent = computed(() =>
  editingAgentId.value
    ? localAgents.value.find(a => a.id === editingAgentId.value) ?? null
    : null
)

const agentsTabCount = computed(() => localAgents.value.filter(a => a.provider_type !== 'evaluator').length)
const evaluatorsTabCount = computed(() => localAgents.value.filter(a => a.provider_type === 'evaluator').length)

const visibleAgents = computed(() => {
  if (editingAgentId.value) {
    return localAgents.value.filter(a => a.id === editingAgentId.value)
  }
  return localAgents.value.filter(a =>
    activeTab.value === 'evaluators'
      ? a.provider_type === 'evaluator'
      : a.provider_type !== 'evaluator'
  )
})

function normalizeConfig(rawConfig, providerType) {
  let config = rawConfig
  if (typeof config === 'string') {
    try {
      config = JSON.parse(config)
    } catch (e) {
      config = {}
    }
  }
  if (!config || typeof config !== 'object' || Array.isArray(config)) {
    config = {}
  }

  if (providerType === 'mcp') {
    config = {
      mode: 'http',
      endpoint: '',
      token: '',
      ...config
    }
  } else if (providerType === 'nvidia') {
    config = {
      api_key: '',
      model: 'meta/llama-3.1-8b-instruct',
      base_url: '',
      system_prompt: '',
      ...config
    }
  } else if (providerType === 'openai') {
    config = {
      api_key: '',
      prompt_id: '',
      prompt_version: '',
      project_id: '',
      ...config
    }
  } else if (providerType === 'openrouter') {
    const apiKey = typeof config.openrouter_api_key === 'string'
      ? config.openrouter_api_key
      : (typeof config.api_key === 'string' ? config.api_key : '')
    const model = typeof config.openrouter_model === 'string'
      ? config.openrouter_model
      : (typeof config.model === 'string' && config.model.trim() !== '' ? config.model : 'openai/gpt-4o-mini')
    const baseURL = typeof config.openrouter_base_url === 'string'
      ? config.openrouter_base_url
      : (typeof config.base_url === 'string' && config.base_url.trim() !== '' ? config.base_url : 'https://openrouter.ai/api/v1')
    const systemPrompt = typeof config.openrouter_system_prompt === 'string'
      ? config.openrouter_system_prompt
      : (typeof config.system_prompt === 'string' ? config.system_prompt : '')

    config = {
      openrouter_api_key: apiKey,
      openrouter_model: model,
      openrouter_base_url: baseURL,
      openrouter_system_prompt: systemPrompt,
      openrouter_http_referer: typeof config.openrouter_http_referer === 'string'
        ? config.openrouter_http_referer
        : (typeof config.http_referer === 'string' ? config.http_referer : ''),
      openrouter_x_title: typeof config.openrouter_x_title === 'string'
        ? config.openrouter_x_title
        : (typeof config.x_title === 'string' ? config.x_title : ''),
      api_key: apiKey,
      model,
      base_url: baseURL,
      system_prompt: systemPrompt,
      ...config
    }
  } else if (providerType === 'openai_compatible') {
    const apiKey = typeof config.compatible_api_key === 'string'
      ? config.compatible_api_key
      : (typeof config.api_key === 'string' ? config.api_key : '')
    const model = typeof config.compatible_model === 'string'
      ? config.compatible_model
      : (typeof config.model === 'string' && config.model.trim() !== '' ? config.model : 'gpt-4o-mini')
    const baseURL = typeof config.compatible_base_url === 'string'
      ? config.compatible_base_url
      : (typeof config.base_url === 'string' ? config.base_url : '')
    const systemPrompt = typeof config.compatible_system_prompt === 'string'
      ? config.compatible_system_prompt
      : (typeof config.system_prompt === 'string' ? config.system_prompt : '')

    config = {
      compatible_api_key: apiKey,
      compatible_model: model,
      compatible_base_url: baseURL,
      compatible_system_prompt: systemPrompt,
      api_key: apiKey,
      model,
      base_url: baseURL,
      system_prompt: systemPrompt,
      ...config
    }
  } else if (providerType === 'anthropic') {
    const apiKey = typeof config.anthropic_api_key === 'string'
      ? config.anthropic_api_key
      : (typeof config.api_key === 'string' ? config.api_key : '')
    const model = typeof config.anthropic_model === 'string'
      ? config.anthropic_model
      : (typeof config.model === 'string' && config.model.trim() !== '' ? config.model : 'claude-3-5-sonnet-latest')
    const baseURL = typeof config.anthropic_base_url === 'string'
      ? config.anthropic_base_url
      : (typeof config.base_url === 'string' && config.base_url.trim() !== '' ? config.base_url : 'https://api.anthropic.com/v1')
    const systemPrompt = typeof config.anthropic_system_prompt === 'string'
      ? config.anthropic_system_prompt
      : (typeof config.system_prompt === 'string' ? config.system_prompt : '')
    const version = typeof config.anthropic_version === 'string'
      ? config.anthropic_version
      : '2023-06-01'

    config = {
      anthropic_api_key: apiKey,
      anthropic_model: model,
      anthropic_base_url: baseURL,
      anthropic_system_prompt: systemPrompt,
      anthropic_version: version,
      api_key: apiKey,
      model,
      base_url: baseURL,
      system_prompt: systemPrompt,
      ...config
    }
  } else if (providerType === 'evaluator') {
    const currentProvider = typeof config.llm_provider === 'string'
      ? config.llm_provider.trim().toLowerCase()
      : ''
    const inferredProvider = ['openai', 'nvidia', 'openrouter', 'anthropic', 'openai_compatible', 'auto'].includes(currentProvider)
      ? currentProvider
      : 'openai'

    const legacyApiKey = typeof config.api_key === 'string' ? config.api_key : ''
    const legacyModel = typeof config.model === 'string' ? config.model : ''
    const legacySystemPrompt = typeof config.system_prompt === 'string' ? config.system_prompt : ''
    const legacyBaseURL = typeof config.base_url === 'string' ? config.base_url : ''
    const legacyPromptID = typeof config.prompt_id === 'string' ? config.prompt_id : ''
    const legacyPromptVersion = typeof config.prompt_version === 'string' ? config.prompt_version : ''
    const legacyProjectID = typeof config.project_id === 'string' ? config.project_id : ''

    const openaiAPIKey = typeof config.openai_api_key === 'string'
      ? config.openai_api_key
      : ((inferredProvider === 'openai' || inferredProvider === 'auto') && legacyApiKey ? legacyApiKey : '')
    const nvidiaAPIKey = typeof config.nvidia_api_key === 'string'
      ? config.nvidia_api_key
      : ((inferredProvider === 'nvidia' && legacyApiKey) ? legacyApiKey : '')
    const openaiPromptID = typeof config.openai_prompt_id === 'string'
      ? config.openai_prompt_id
      : legacyPromptID
    const openaiPromptVersion = typeof config.openai_prompt_version === 'string'
      ? config.openai_prompt_version
      : legacyPromptVersion
    const openaiProjectID = typeof config.openai_project_id === 'string'
      ? config.openai_project_id
      : legacyProjectID

    const openrouterAPIKey = typeof config.openrouter_api_key === 'string'
      ? config.openrouter_api_key
      : ((inferredProvider === 'openrouter' && legacyApiKey) ? legacyApiKey : '')
    const openrouterModel = typeof config.openrouter_model === 'string'
      ? config.openrouter_model
      : ((inferredProvider === 'openrouter' && legacyModel) ? legacyModel : 'openai/gpt-4o-mini')
    const openrouterSystemPrompt = typeof config.openrouter_system_prompt === 'string'
      ? config.openrouter_system_prompt
      : ((inferredProvider === 'openrouter' && legacySystemPrompt) ? legacySystemPrompt : '')
    const openrouterBaseURL = typeof config.openrouter_base_url === 'string'
      ? config.openrouter_base_url
      : ((inferredProvider === 'openrouter' && legacyBaseURL) ? legacyBaseURL : 'https://openrouter.ai/api/v1')
    const openrouterHTTPReferer = typeof config.openrouter_http_referer === 'string'
      ? config.openrouter_http_referer
      : (typeof config.http_referer === 'string' ? config.http_referer : '')
    const openrouterXTitle = typeof config.openrouter_x_title === 'string'
      ? config.openrouter_x_title
      : (typeof config.x_title === 'string' ? config.x_title : '')

    const compatibleAPIKey = typeof config.compatible_api_key === 'string'
      ? config.compatible_api_key
      : ((inferredProvider === 'openai_compatible' && legacyApiKey) ? legacyApiKey : '')
    const compatibleModel = typeof config.compatible_model === 'string'
      ? config.compatible_model
      : ((inferredProvider === 'openai_compatible' && legacyModel) ? legacyModel : 'gpt-4o-mini')
    const compatibleSystemPrompt = typeof config.compatible_system_prompt === 'string'
      ? config.compatible_system_prompt
      : ((inferredProvider === 'openai_compatible' && legacySystemPrompt) ? legacySystemPrompt : '')
    const compatibleBaseURL = typeof config.compatible_base_url === 'string'
      ? config.compatible_base_url
      : ((inferredProvider === 'openai_compatible' && legacyBaseURL) ? legacyBaseURL : '')

    const anthropicAPIKey = typeof config.anthropic_api_key === 'string'
      ? config.anthropic_api_key
      : ((inferredProvider === 'anthropic' && legacyApiKey) ? legacyApiKey : '')
    const anthropicModel = typeof config.anthropic_model === 'string'
      ? config.anthropic_model
      : ((inferredProvider === 'anthropic' && legacyModel) ? legacyModel : 'claude-3-5-sonnet-latest')
    const anthropicSystemPrompt = typeof config.anthropic_system_prompt === 'string'
      ? config.anthropic_system_prompt
      : ((inferredProvider === 'anthropic' && legacySystemPrompt) ? legacySystemPrompt : '')
    const anthropicBaseURL = typeof config.anthropic_base_url === 'string'
      ? config.anthropic_base_url
      : ((inferredProvider === 'anthropic' && legacyBaseURL) ? legacyBaseURL : 'https://api.anthropic.com/v1')
    const anthropicVersion = typeof config.anthropic_version === 'string'
      ? config.anthropic_version
      : '2023-06-01'

    const currentMode = typeof config.openai_mode === 'string'
      ? config.openai_mode.trim().toLowerCase()
      : ''
    const inferredMode = (currentMode === 'managed' || currentMode === 'standard')
      ? currentMode
      : (openaiPromptID.trim() !== '' ? 'managed' : 'standard')
    const openaiModel = typeof config.openai_model === 'string'
      ? config.openai_model
      : ((inferredProvider === 'openai' || inferredProvider === 'auto') && legacyModel ? legacyModel : 'gpt-4o-mini')
    const openaiSystemPrompt = typeof config.openai_system_prompt === 'string'
      ? config.openai_system_prompt
      : ((inferredProvider === 'openai' || inferredProvider === 'auto') && legacySystemPrompt ? legacySystemPrompt : '')
    const nvidiaModel = typeof config.nvidia_model === 'string'
      ? config.nvidia_model
      : ((inferredProvider === 'nvidia' && legacyModel) ? legacyModel : 'meta/llama-3.1-8b-instruct')
    const nvidiaBaseURL = typeof config.nvidia_base_url === 'string'
      ? config.nvidia_base_url
      : ((inferredProvider === 'nvidia' && legacyBaseURL) ? legacyBaseURL : '')
    const nvidiaSystemPrompt = typeof config.nvidia_system_prompt === 'string'
      ? config.nvidia_system_prompt
      : ((inferredProvider === 'nvidia' && legacySystemPrompt) ? legacySystemPrompt : '')

    config = {
      target_agent_id: '',
      llm_provider: inferredProvider,
      openai_mode: inferredMode,
      openai_api_key: openaiAPIKey,
      openai_model: openaiModel,
      openai_system_prompt: openaiSystemPrompt,
      openai_prompt_id: openaiPromptID,
      openai_prompt_version: openaiPromptVersion,
      openai_project_id: openaiProjectID,
      nvidia_api_key: nvidiaAPIKey,
      nvidia_model: nvidiaModel,
      nvidia_base_url: nvidiaBaseURL,
      nvidia_system_prompt: nvidiaSystemPrompt,
      openrouter_api_key: openrouterAPIKey,
      openrouter_model: openrouterModel,
      openrouter_base_url: openrouterBaseURL,
      openrouter_system_prompt: openrouterSystemPrompt,
      openrouter_http_referer: openrouterHTTPReferer,
      openrouter_x_title: openrouterXTitle,
      compatible_api_key: compatibleAPIKey,
      compatible_model: compatibleModel,
      compatible_base_url: compatibleBaseURL,
      compatible_system_prompt: compatibleSystemPrompt,
      anthropic_api_key: anthropicAPIKey,
      anthropic_model: anthropicModel,
      anthropic_base_url: anthropicBaseURL,
      anthropic_system_prompt: anthropicSystemPrompt,
      anthropic_version: anthropicVersion,
      api_key: legacyApiKey,
      model: legacyModel,
      system_prompt: legacySystemPrompt,
      base_url: legacyBaseURL,
      prompt_id: legacyPromptID,
      prompt_version: legacyPromptVersion,
      project_id: legacyProjectID,
      ...config,
      llm_provider: inferredProvider,
      openai_mode: inferredMode,
      openai_api_key: openaiAPIKey,
      openai_model: openaiModel || 'gpt-4o-mini',
      openai_system_prompt: openaiSystemPrompt,
      openai_prompt_id: openaiPromptID,
      openai_prompt_version: openaiPromptVersion,
      openai_project_id: openaiProjectID,
      nvidia_api_key: nvidiaAPIKey,
      nvidia_model: nvidiaModel || 'meta/llama-3.1-8b-instruct',
      nvidia_base_url: nvidiaBaseURL,
      nvidia_system_prompt: nvidiaSystemPrompt,
      openrouter_api_key: openrouterAPIKey,
      openrouter_model: openrouterModel || 'openai/gpt-4o-mini',
      openrouter_base_url: openrouterBaseURL,
      openrouter_system_prompt: openrouterSystemPrompt,
      openrouter_http_referer: openrouterHTTPReferer,
      openrouter_x_title: openrouterXTitle,
      compatible_api_key: compatibleAPIKey,
      compatible_model: compatibleModel || 'gpt-4o-mini',
      compatible_base_url: compatibleBaseURL,
      compatible_system_prompt: compatibleSystemPrompt,
      anthropic_api_key: anthropicAPIKey,
      anthropic_model: anthropicModel || 'claude-3-5-sonnet-latest',
      anthropic_base_url: anthropicBaseURL,
      anthropic_system_prompt: anthropicSystemPrompt,
      anthropic_version: anthropicVersion
    }

    const autoResolvedProvider = config.nvidia_api_key && config.nvidia_api_key.trim() !== ''
      ? 'nvidia'
      : (config.openrouter_api_key && config.openrouter_api_key.trim() !== ''
        ? 'openrouter'
        : (config.anthropic_api_key && config.anthropic_api_key.trim() !== ''
          ? 'anthropic'
        : (config.openai_api_key && config.openai_api_key.trim() !== ''
          ? 'openai'
          : (config.compatible_api_key && config.compatible_api_key.trim() !== '' ? 'openai_compatible' : 'openai'))))

    const legacyProvider = inferredProvider === 'auto'
      ? autoResolvedProvider
      : inferredProvider

    if (legacyProvider === 'nvidia') {
      config.api_key = config.nvidia_api_key || ''
      config.model = config.nvidia_model || ''
      config.system_prompt = config.nvidia_system_prompt || ''
      config.base_url = config.nvidia_base_url || ''
      config.prompt_id = ''
      config.prompt_version = ''
      config.project_id = ''
    } else if (legacyProvider === 'openrouter') {
      config.api_key = config.openrouter_api_key || ''
      config.model = config.openrouter_model || ''
      config.system_prompt = config.openrouter_system_prompt || ''
      config.base_url = config.openrouter_base_url || ''
      config.prompt_id = ''
      config.prompt_version = ''
      config.project_id = ''
      config.http_referer = config.openrouter_http_referer || ''
      config.x_title = config.openrouter_x_title || ''
    } else if (legacyProvider === 'openai_compatible') {
      config.api_key = config.compatible_api_key || ''
      config.model = config.compatible_model || ''
      config.system_prompt = config.compatible_system_prompt || ''
      config.base_url = config.compatible_base_url || ''
      config.prompt_id = ''
      config.prompt_version = ''
      config.project_id = ''
    } else if (legacyProvider === 'anthropic') {
      config.api_key = config.anthropic_api_key || ''
      config.model = config.anthropic_model || ''
      config.system_prompt = config.anthropic_system_prompt || ''
      config.base_url = config.anthropic_base_url || ''
      config.prompt_id = ''
      config.prompt_version = ''
      config.project_id = ''
    } else {
      config.api_key = config.openai_api_key || ''
      config.model = config.openai_model || ''
      config.system_prompt = config.openai_system_prompt || ''
      config.base_url = ''
      config.prompt_id = config.openai_prompt_id || ''
      config.prompt_version = config.openai_prompt_version || ''
      config.project_id = config.openai_project_id || ''
    }
  }

  return config
}

function normalizeManagerAgents(globalAgents) {
  if (!globalAgents) return []
  return (globalAgents || [])
    .map(a => ({
      ...a,
      config: normalizeConfig(a.config, a.provider_type)
    }))
    .sort((a, b) => (a.position || 0) - (b.position || 0))
}

watch(() => props.agents, (newAgents) => {
  // Don't overwrite local state if user is currently editing
  if (isEditing.value) {
    console.log('[AgentManagerModal] Skipping sync while editing')
    return
  }
  
  const merged = normalizeManagerAgents(newAgents)
  const mergedIds = new Set(merged.map(a => a.id))

  // Preserve pending creates that haven't hit the server/broadcast yet
  for (const id of pendingCreateIds.value) {
    if (mergedIds.has(id)) {
      pendingCreateIds.value.delete(id)
    }
  }

  if (pendingCreateIds.value.size > 0) {
    const pendingAgents = localAgents.value.filter(a => pendingCreateIds.value.has(a.id))
    localAgents.value = [...pendingAgents, ...merged]
    return
  }

  localAgents.value = merged
}, { immediate: true })

function startEditing() {
  isEditing.value = true
}

function stopEditing() {
  // Delay stopping to allow save to complete first
  setTimeout(() => {
    isEditing.value = false
  }, 500)
}

function openAgentConfig(agent) {
  editingAgentId.value = agent.id
}

function closeAgentConfig() {
  editingAgentId.value = null
}

onMounted(() => {
  if (!props.initialAgentId) return
  const agent = localAgents.value.find(a => a.id === props.initialAgentId)
  if (agent) {
    editingAgentId.value = agent.id
    activeTab.value = agent.provider_type === 'evaluator' ? 'evaluators' : 'agents'
  }
})

async function addAgent(providerType) {
  if (!props.workspaceId) return
  
  const defaultConfigs = {
    mcp: { mode: 'http', endpoint: '', token: '' },
    nvidia: { api_key: '', model: 'meta/llama-3.1-8b-instruct', base_url: '', system_prompt: '' },
    anthropic: {
      anthropic_api_key: '',
      anthropic_model: 'claude-3-5-sonnet-latest',
      anthropic_base_url: 'https://api.anthropic.com/v1',
      anthropic_system_prompt: '',
      anthropic_version: '2023-06-01',
      api_key: '',
      model: 'claude-3-5-sonnet-latest',
      base_url: 'https://api.anthropic.com/v1',
      system_prompt: ''
    },
    openrouter: {
      openrouter_api_key: '',
      openrouter_model: 'openai/gpt-4o-mini',
      openrouter_base_url: 'https://openrouter.ai/api/v1',
      openrouter_system_prompt: '',
      openrouter_http_referer: '',
      openrouter_x_title: '',
      api_key: '',
      model: 'openai/gpt-4o-mini',
      base_url: 'https://openrouter.ai/api/v1',
      system_prompt: ''
    },
    openai_compatible: {
      compatible_api_key: '',
      compatible_model: 'gpt-4o-mini',
      compatible_base_url: '',
      compatible_system_prompt: '',
      api_key: '',
      model: 'gpt-4o-mini',
      base_url: '',
      system_prompt: ''
    },
    openai: { api_key: '', prompt_id: '', prompt_version: '' },
    evaluator: {
      target_agent_id: '',
      llm_provider: 'openai',
      openai_mode: 'standard',
      openai_api_key: '',
      openai_model: 'gpt-4o-mini',
      openai_system_prompt: '',
      openai_prompt_id: '',
      openai_prompt_version: '',
      openai_project_id: '',
      nvidia_api_key: '',
      nvidia_model: 'meta/llama-3.1-8b-instruct',
      nvidia_base_url: '',
      nvidia_system_prompt: '',
      openrouter_api_key: '',
      openrouter_model: 'openai/gpt-4o-mini',
      openrouter_base_url: 'https://openrouter.ai/api/v1',
      openrouter_system_prompt: '',
      openrouter_http_referer: '',
      openrouter_x_title: '',
      compatible_api_key: '',
      compatible_model: 'gpt-4o-mini',
      compatible_base_url: '',
      compatible_system_prompt: '',
      anthropic_api_key: '',
      anthropic_model: 'claude-3-5-sonnet-latest',
      anthropic_base_url: 'https://api.anthropic.com/v1',
      anthropic_system_prompt: '',
      anthropic_version: '2023-06-01',
      api_key: '',
      model: 'gpt-4o-mini',
      system_prompt: '',
      base_url: '',
      prompt_id: '',
      prompt_version: '',
      project_id: ''
    }
  }

  return addAgentWithConfig(providerType, defaultConfigs[providerType] || {})
}

function addEvaluator(mode = 'standard') {
  const selectedMode = ['managed', 'standard', 'nvidia', 'openrouter', 'anthropic', 'openai_compatible', 'auto'].includes(mode)
    ? mode
    : 'standard'
  const llmProvider = selectedMode === 'managed' || selectedMode === 'standard'
    ? 'openai'
    : selectedMode
  const evaluatorConfig = {
    target_agent_id: '',
    llm_provider: llmProvider,
    openai_mode: selectedMode === 'managed' ? 'managed' : 'standard',
    openai_api_key: '',
    openai_model: 'gpt-4o-mini',
    openai_system_prompt: '',
    openai_prompt_id: '',
    openai_prompt_version: '',
    openai_project_id: '',
    nvidia_api_key: '',
    nvidia_model: 'meta/llama-3.1-8b-instruct',
    nvidia_base_url: '',
    nvidia_system_prompt: '',
    openrouter_api_key: '',
    openrouter_model: 'openai/gpt-4o-mini',
    openrouter_base_url: 'https://openrouter.ai/api/v1',
    openrouter_system_prompt: '',
    openrouter_http_referer: '',
    openrouter_x_title: '',
    compatible_api_key: '',
    compatible_model: 'gpt-4o-mini',
    compatible_base_url: '',
    compatible_system_prompt: '',
    anthropic_api_key: '',
    anthropic_model: 'claude-3-5-sonnet-latest',
    anthropic_base_url: 'https://api.anthropic.com/v1',
    anthropic_system_prompt: '',
    anthropic_version: '2023-06-01',
    api_key: '',
    model: 'gpt-4o-mini',
    system_prompt: '',
    base_url: '',
    prompt_id: '',
    prompt_version: '',
    project_id: ''
  }

  return addAgentWithConfig('evaluator', evaluatorConfig)
}

function openEvaluatorModeModal() {
  showEvaluatorModeModal.value = true
}

function closeEvaluatorModeModal() {
  showEvaluatorModeModal.value = false
}

async function selectEvaluatorMode(mode) {
  closeEvaluatorModeModal()
  await addEvaluator(mode)
}

async function addAgentWithConfig(providerType, customConfig) {
  if (!props.workspaceId) return

  try {
    const newAgent = await wsService.createAgent(props.workspaceId, {
      name: generateAgentName(providerType),
      provider_type: providerType,
      config: customConfig || {}
    })

    const newAgentWithParsedConfig = {
      ...newAgent,
      config: normalizeConfig(newAgent.config, newAgent.provider_type || providerType),
      enabled: true
    }
	    if (newAgent?.id) {
	      pendingCreateIds.value.add(newAgent.id)
	    }
	    localAgents.value.unshift(newAgentWithParsedConfig)
        editingAgentId.value = newAgent.id
        activeTab.value = providerType === 'evaluator' ? 'evaluators' : 'agents'
        capturePostHogEvent('agent_created', {
          agent_id: newAgent?.id || '',
          workspace_id: props.workspaceId || '',
          provider_type: providerType
        })

	    emit('update')
	    showSaveStatus('saved', 'Agent created!')
  } catch (e) {
    console.error('Failed to create agent:', e)
    showSaveStatus('error', 'Failed to create')
  }
}

function getEvaluatorProvider(agent) {
  if (!agent || !agent.config) return 'openai'
  const provider = typeof agent.config.llm_provider === 'string'
    ? agent.config.llm_provider.trim().toLowerCase()
    : ''
  if (provider === 'nvidia' || provider === 'openai' || provider === 'openrouter' || provider === 'anthropic' || provider === 'openai_compatible' || provider === 'auto') {
    return provider
  }
  return 'openai'
}

function showOpenAIEvaluatorFields(agent) {
  const provider = getEvaluatorProvider(agent)
  return provider === 'openai' || provider === 'auto'
}

function showNVIDIAEvaluatorFields(agent) {
  const provider = getEvaluatorProvider(agent)
  return provider === 'nvidia' || provider === 'auto'
}

function showOpenRouterEvaluatorFields(agent) {
  const provider = getEvaluatorProvider(agent)
  return provider === 'openrouter' || provider === 'auto'
}

function showAnthropicEvaluatorFields(agent) {
  const provider = getEvaluatorProvider(agent)
  return provider === 'anthropic' || provider === 'auto'
}

function showCompatibleEvaluatorFields(agent) {
  const provider = getEvaluatorProvider(agent)
  return provider === 'openai_compatible' || provider === 'auto'
}

function onEvaluatorProviderChange(agent) {
  if (!agent || !agent.config) return
  const provider = getEvaluatorProvider(agent)
  if (!agent.config.openai_mode) {
    agent.config.openai_mode = 'standard'
  }
  if (!agent.config.openai_model) {
    agent.config.openai_model = 'gpt-4o-mini'
  }
  if (!agent.config.nvidia_model) {
    agent.config.nvidia_model = 'meta/llama-3.1-8b-instruct'
  }
  if (!agent.config.openrouter_model) {
    agent.config.openrouter_model = 'openai/gpt-4o-mini'
  }
  if (!agent.config.compatible_model) {
    agent.config.compatible_model = 'gpt-4o-mini'
  }
  if (!agent.config.anthropic_model) {
    agent.config.anthropic_model = 'claude-3-5-sonnet-latest'
  }
  if (!agent.config.anthropic_version) {
    agent.config.anthropic_version = '2023-06-01'
  }
  if (provider === 'nvidia' || provider === 'openrouter' || provider === 'anthropic' || provider === 'openai_compatible') {
    agent.config.openai_mode = 'standard'
  }
  markPendingChanges(agent)
}

function getEvaluatorMode(agent) {
  if (!agent || !agent.config) return 'standard'
  const mode = typeof agent.config.openai_mode === 'string'
    ? agent.config.openai_mode.trim().toLowerCase()
    : ''
  if (mode === 'managed' || mode === 'standard') {
    return mode
  }
  const promptID = typeof agent.config.openai_prompt_id === 'string'
    ? agent.config.openai_prompt_id.trim()
    : (typeof agent.config.prompt_id === 'string' ? agent.config.prompt_id.trim() : '')
  return promptID ? 'managed' : 'standard'
}

function getAgentTypeLabel(agent) {
  if (!agent) return 'agent'
  if (agent.provider_type === 'mcp') return 'Corvic'
  if (agent.provider_type === 'nvidia') return 'NVIDIA NIM'
  if (agent.provider_type === 'anthropic') return 'Claude'
  if (agent.provider_type === 'openrouter') return 'OpenRouter'
  if (agent.provider_type === 'openai_compatible') return 'OpenAI-Compatible'
  if (agent.provider_type === 'evaluator') {
    const provider = getEvaluatorProvider(agent)
    if (provider === 'auto') return 'Eval Legacy Auto'
    if (provider === 'nvidia') return 'Eval NVIDIA'
    if (provider === 'anthropic') return 'Eval Claude'
    if (provider === 'openrouter') return 'Eval OpenRouter'
    if (provider === 'openai_compatible') return 'Eval Compatible'
    return getEvaluatorMode(agent) === 'managed' ? 'Eval Managed' : 'Eval Standard'
  }
  return agent.provider_type
}

function showSaveStatus(status, text) {
  saveStatus.value = status
  saveStatusText.value = text
  setTimeout(() => {
    saveStatus.value = null
    saveStatusText.value = ''
  }, 2000)
}

function markPendingChanges(agent) {
  if (agent?.id) {
    dirtyAgentIds.value.add(agent.id)
  }
  pendingChanges.value = true
}

function clearPendingChanges(agentId) {
  if (agentId) {
    dirtyAgentIds.value.delete(agentId)
  }
  pendingChanges.value = dirtyAgentIds.value.size > 0
}

async function handleClose() {
  // Save any pending changes before closing
  if (pendingChanges.value && localAgents.value.length > 0) {
    const saved = await saveAllAgents()
    if (!saved) return
  }
  emit('close')
}

async function saveAgent(agent) {
  saveStatus.value = 'saving'
  saveStatusText.value = 'Saving...'
  
  try {
    agent.config = normalizeConfig(agent.config, agent.provider_type)
    // Always persist agent config/credentials at the workspace level
	    await wsService.updateAgent(agent.id, {
	      name: agent.name,
	      provider_type: agent.provider_type,
	      config: agent.config,
	      enabled: agent.enabled,
	      position: agent.position,
	      max_concurrency: agent.max_concurrency || 5
	    })
	    // NOTE: We intentionally do NOT emit('update') here to avoid the watcher
	    // overwriting localAgents with stale server data. Updates are synced on close.
	    clearPendingChanges(agent.id)
    showSaveStatus('saved', 'Saved ✓')
  } catch (e) {
    console.error('Failed to save agent:', e)
    markPendingChanges(agent)
    showSaveStatus('error', 'Save failed')
  }
}

async function saveAllAgents() {
  saveStatus.value = 'saving'
  saveStatusText.value = 'Saving...'

  try {
    const dirtyIds = Array.from(dirtyAgentIds.value)
    const agentsToSave = dirtyIds.length
      ? localAgents.value.filter(a => dirtyIds.includes(a.id))
      : localAgents.value

	    for (const agent of agentsToSave) {
	      agent.config = normalizeConfig(agent.config, agent.provider_type)
	      await wsService.updateAgent(agent.id, {
	        name: agent.name,
	        provider_type: agent.provider_type,
        config: agent.config,
        enabled: agent.enabled,
        position: agent.position,
        max_concurrency: agent.max_concurrency || 5
	      })
	    }

	    dirtyAgentIds.value.clear()
	    pendingChanges.value = false
    emit('update')
    showSaveStatus('saved', 'All changes saved')
    return true
  } catch (e) {
    console.error('Failed to save agents:', e)
    pendingChanges.value = true
    showSaveStatus('error', 'Save failed')
    return false
  }
}

async function toggleAgent(agent) {
  const newState = !agent.enabled
  const action = newState ? 'enable' : 'disable'
  
  // Confirmation for disabling globally
  if (!newState) {
    const confirmed = confirm(
      `Are you sure you want to ${action} "${agent.name}" globally?\n\n` +
      `This affects all question sets in this workspace. ` +
      `Question sets can still select this agent individually in Run Setup.`
    )
    if (!confirmed) return
  }
  
  agent.enabled = newState
  
  // Only update workspace-level agent, NOT question-set-level
  saveStatus.value = 'saving'
  saveStatusText.value = 'Saving...'
  
  try {
    await wsService.updateAgent(agent.id, {
      name: agent.name,
      provider_type: agent.provider_type,
      config: agent.config,
      enabled: agent.enabled,
      position: agent.position,
      max_concurrency: agent.max_concurrency || 5
    })
    // This is a global (workspace-level) toggle only.
    showSaveStatus('saved', `Agent ${action}d ✓`)
  } catch (e) {
    console.error('Failed to toggle agent:', e)
    agent.enabled = !newState // Revert
    showSaveStatus('error', 'Toggle failed')
  }
}

async function deleteAgent(agent) {
  if (!confirm(`Delete agent "${agent.name}"?`)) return

  try {
    await wsService.deleteAgent(agent.id, false)
    localAgents.value = localAgents.value.filter(a => a.id !== agent.id)
    dirtyAgentIds.value.delete(agent.id)
    pendingCreateIds.value.delete(agent.id)
    pendingChanges.value = dirtyAgentIds.value.size > 0
    emit('update')
    showSaveStatus('saved', 'Agent deleted')
  } catch (e) {
    console.error('Failed to delete agent:', e)
    const errorMsg = e.message || ''
    const isEncryptionIssue = errorMsg.includes('encryption') ||
      errorMsg.includes('failed to find') ||
      errorMsg.includes('failed to load') ||
      agent.config_status === 'needs_recredentials'
    if (isEncryptionIssue) {
      forceDeleteAgent.value = agent
      return
    }
    showSaveStatus('error', errorMsg || 'Delete failed')
  }
}

async function confirmForceDelete() {
  const agent = forceDeleteAgent.value
  forceDeleteAgent.value = null
  if (!agent) return
  try {
    await wsService.deleteAgent(agent.id, true)
    localAgents.value = localAgents.value.filter(a => a.id !== agent.id)
    dirtyAgentIds.value.delete(agent.id)
    pendingCreateIds.value.delete(agent.id)
    pendingChanges.value = dirtyAgentIds.value.size > 0
    emit('update')
    showSaveStatus('saved', 'Agent force-deleted')
  } catch (e) {
    console.error('Failed to force-delete agent:', e)
    showSaveStatus('error', e.message || 'Force delete failed')
  }
}

async function showSpyModal(agent) {
  try {
    spyPayload.value = await wsService.getSpyPayload(agent.id, '[Sample question]')
    spyAgent.value = agent
  } catch (e) {
    console.error('Failed to get spy payload:', e)
  }
}

function startDrag(index) {
  // TODO: Implement drag-and-drop reordering
  console.log('Drag started for index:', index)
}
</script>

<style scoped>
.agent-manager-modal {
  width: 90%;
  max-width: 800px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
}

.modal-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #f8fafc;
}

.agents-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.agent-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 1rem;
  transition: all 0.2s ease;
}

.agent-card.clickable {
  cursor: pointer;
}

.agent-card.clickable:hover {
  border-color: #93c5fd;
  box-shadow: 0 4px 12px -1px rgba(59, 130, 246, 0.15);
}

.agent-tabs {
  display: flex;
  gap: 0;
  border-bottom: 2px solid #e2e8f0;
  margin-bottom: 1rem;
}

.tab-btn {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  padding: 0.5rem 1.25rem;
  font-size: 0.9rem;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  transition: color 0.15s, border-color 0.15s;
}

.tab-btn:hover {
  color: #1e293b;
}

.tab-btn.active {
  color: #3b82f6;
  border-bottom-color: #3b82f6;
}

.tab-count {
  background: #e2e8f0;
  color: #64748b;
  font-size: 0.7rem;
  font-weight: 700;
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  min-width: 18px;
  text-align: center;
}

.tab-btn.active .tab-count {
  background: #dbeafe;
  color: #1e40af;
}

.agent-breadcrumb {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding-bottom: 1rem;
  margin-bottom: 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.btn-back {
  background: none;
  border: 1px solid #e2e8f0;
  padding: 0.3rem 0.75rem;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  color: #64748b;
  transition: all 0.15s;
}

.btn-back:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #1e293b;
}

.breadcrumb-name {
  font-weight: 600;
  color: #1e293b;
  font-size: 0.95rem;
  flex: 1;
}

.agent-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.agent-header.no-mb {
  margin-bottom: 0;
}

.agent-drag {
  cursor: grab;
  color: #94a3b8;
  font-size: 1.2rem;
}

.agent-name-text {
  flex: 1;
  font-size: 1rem;
  font-weight: 600;
  color: #1e293b;
  padding: 0.25rem 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.agent-type-badge {
  font-size: 0.7rem;
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  font-weight: 600;
  text-transform: uppercase;
}

.agent-type-badge.mcp {
  background: #dbeafe;
  color: #1e40af;
}

.agent-type-badge.openai {
  background: #dcfce7;
  color: #166534;
}

.agent-type-badge.nvidia {
  background: #dcfce7;
  color: #14532d;
}

.agent-type-badge.anthropic {
  background: #f3e8ff;
  color: #6b21a8;
}

.agent-type-badge.openrouter {
  background: #e0f2fe;
  color: #075985;
}

.agent-type-badge.openai_compatible {
  background: #ecfccb;
  color: #3f6212;
}

.agent-type-badge.evaluator {
  background: #fef3c7;
  color: #92400e;
}

.shared-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 6px;
  background: #ede9fe;
  color: #6d28d9;
  border: 1px solid #ddd6fe;
  letter-spacing: 0.02em;
}

.creds-lost-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 6px;
  background: #fff3cd;
  color: #856404;
  border: 1px solid #ffc107;
  letter-spacing: 0.02em;
}

.agent-card.shared-card {
  border-left: 3px solid #8b5cf6;
  cursor: default;
  background: #faf5ff;
}

.agent-actions {
  display: flex;
  gap: 0.25rem;
}

.btn-icon {
  background: white;
  border: 1px solid #e2e8f0;
  font-size: 1rem;
  cursor: pointer;
  padding: 0.4rem;
  border-radius: 6px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
}

.btn-icon:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.btn-danger-icon:hover {
  background: #fee2e2;
  border-color: #fca5a5;
  color: #ef4444;
}

.agent-config-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

/* Config Fields */
.config-fields, .config-fields-mcp {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0.75rem;
  background: #f8fafc;
  padding: 1rem;
  border-radius: 8px;
}

.config-fields-mcp {
  grid-template-columns: 1fr 1fr;
}

.field.full-width,
.alert.full-width {
  grid-column: 1 / -1;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.field label {
  font-size: 0.75rem;
  color: #64748b;
  font-weight: 500;
}

.field input,
.field select,
.field textarea {
  padding: 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.875rem;
  color: #1e293b;
  background: white;
}

.field input:focus,
.field select:focus,
.field textarea:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.field textarea {
  resize: vertical;
  min-height: 92px;
}

.field-hint {
  font-size: 0.75rem;
  color: #64748b;
}

.field-hint a {
  color: #2563eb;
  text-decoration: underline;
}

.config-help {
  margin: 0.5rem 0 0;
  font-size: 0.75rem;
  color: #64748b;
  line-height: 1.4;
}

.mode-badge {
  display: inline-block;
  background: #e2e8f0;
  color: #475569;
  font-size: 0.8rem;
  font-weight: 600;
  padding: 0.3rem 0.6rem;
  border-radius: 6px;
  width: fit-content;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.save-status {
  font-size: 0.8rem;
  font-weight: 500;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  animation: fadeIn 0.2s ease;
}

.save-status.saving {
  color: #6b7280;
  background: #f3f4f6;
}

.save-status.saved {
  color: #059669;
  background: #d1fae5;
}

.save-status.error {
  color: #dc2626;
  background: #fee2e2;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}

.alert-info {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
  border: 1px solid rgba(59, 130, 246, 0.2);
  padding: 0.75rem;
  border-radius: 6px;
  font-size: 0.85rem;
}

.mt-2 { margin-top: 0.5rem; }

.alert-warning {
  background: #fff7ed;
  color: #c2410c;
  border: 1px solid #fed7aa;
  padding: 0.75rem;
  border-radius: 6px;
  margin-bottom: 1rem;
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.alert-danger {
  background: #fff1f2;
  color: #9f1239;
  border: 1px solid #fecdd3;
  padding: 0.75rem;
  border-radius: 6px;
  margin-bottom: 1rem;
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
  line-height: 1.5;
}

.disabled-card {
  opacity: 0.7;
  background: #f8fafc;
}

.disabled-card .agent-name-input {
  color: #64748b;
  text-decoration: line-through; 
  text-decoration-color: #cbd5e1;
}

.disabled-card:hover {
  opacity: 1;
}

.evaluator-mode-modal {
  width: 92%;
  max-width: 680px;
}

.evaluator-mode-intro {
  margin: 0 0 1rem;
  color: #475569;
}

.evaluator-mode-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0.75rem;
}

.evaluator-mode-card {
  text-align: left;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 1rem;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  transition: all 0.2s ease;
}

.evaluator-mode-card:hover {
  border-color: #94a3b8;
  box-shadow: 0 4px 10px rgba(2, 6, 23, 0.08);
  transform: translateY(-1px);
}

.mode-title {
  font-size: 1rem;
  font-weight: 700;
  color: #0f172a;
}

.mode-subtitle {
  font-size: 0.8rem;
  color: #0f766e;
  font-weight: 600;
}

.mode-description {
  font-size: 0.82rem;
  line-height: 1.45;
  color: #475569;
}
</style>
