<template>
  <div class="print-report" :class="{ 'print-report-compact': isQuestionCardReport }">
    <template v-if="isQuestionCardReport">
      <section class="compact-report-header">
        <div class="compact-report-topbar">
          <div class="logo-wrapper compact-logo-wrapper">
            <CorvicLogo width="160px" height="44px" />
          </div>
          <div class="compact-meta-row">
            <div class="meta-badge">
              <span class="meta-label">Workspace:</span>
              <span class="meta-value">{{ workspaceName }}</span>
            </div>
            <div class="meta-badge">
              <span class="meta-label">Generated:</span>
              <span class="meta-value">{{ reportDate }}</span>
            </div>
          </div>
        </div>

        <div class="compact-title-block">
          <span class="compact-kicker">Question Snapshot</span>
          <h1>{{ compactReportTitle }}</h1>
          <p v-if="compactReportSubtitle" class="compact-subtitle">{{ compactReportSubtitle }}</p>
        </div>
      </section>

      <section class="compact-card-grid">
        <article
          v-for="card in normalizedQuestionCards"
          :key="card.id || card.questionNumber"
          class="analysis-card"
        >
          <div class="analysis-card-header">
            <div class="analysis-card-number">Q{{ card.questionNumber }}</div>
            <div
              v-if="card.evaluationScore"
              class="analysis-score-chip"
              :class="getCompactScoreClass(card)"
            >
              {{ card.evaluationScore }}
            </div>
          </div>

          <div class="analysis-section">
            <div class="analysis-label">Question</div>
            <div class="analysis-question">{{ card.question }}</div>
          </div>

          <div v-if="card.grounding" class="analysis-section">
            <div class="analysis-label">Grounding</div>
            <div class="analysis-markdown" v-html="renderMarkdown(card.grounding)"></div>
          </div>

          <div class="analysis-section">
            <div class="analysis-label">Response</div>
            <div class="analysis-markdown" v-html="renderMarkdown(card.response || '_No response available._')"></div>
          </div>

          <div class="analysis-section">
            <div class="analysis-label">Evaluation</div>
            <div class="analysis-markdown" v-html="renderMarkdown(card.evaluation || '_No evaluation available._')"></div>
          </div>
        </article>
      </section>
    </template>

    <template v-else>
      <!-- Cover Page -->
      <div class="print-page cover-page">
        <div class="header-decoration"></div>
        
        <div class="report-header">
          <div class="logo-wrapper">
            <CorvicLogo width="180px" height="50px" />
          </div>
          <h1>LLM Benchmark Analysis</h1>
          <div class="report-meta">
            <div class="meta-badge">
              <span class="meta-label">Workspace:</span>
              <span class="meta-value">{{ workspaceName }}</span>
            </div>
            <div class="meta-badge">
              <span class="meta-label">Generated:</span>
              <span class="meta-value">{{ reportDate }}</span>
            </div>
          </div>
        </div>

        <div class="summary-section-print" v-if="summaryStats && summaryStats.agents">
          <h2 class="section-title">Performance Executive Summary</h2>
          
          <div class="summary-grid-print">
            <div 
              v-for="(agent, idx) in summaryStats.agents" 
              :key="agent.name"
              class="agent-summary-card"
              :class="'card-accent-' + (idx % 4 + 1)"
            >
              <div class="card-header">
                <h3>{{ agent.name }}</h3>
                <div class="evaluator-badge" v-if="isEvaluatorSummaryAgent(agent)">
                  Evaluator
                </div>
                <div class="quality-circle" v-else>
                  <span class="score">{{ agent.qualityScore }}%</span>
                  <span class="label">Quality</span>
                </div>
              </div>
              
              <div class="stats-mini-list" v-if="agent.stats">
                <div class="mini-stat">
                  <span class="val">{{ shouldShowStatus(agent) ? `${agent.stats.answered} / ${agent.stats.totalQuestions}` : agent.stats.answered }}</span>
                  <span class="lab">Answered</span>
                </div>
                <div class="mini-stat">
                  <span class="val">{{ formatDuration(agent.stats.avgDuration) }}</span>
                  <span class="lab">Avg Speed</span>
                </div>
                <div class="mini-stat">
                  <span class="val">{{ isEvaluatorSummaryAgent(agent) ? agent.stats.answered : (agent.stats.percentages.positive || 0) + '%' }}</span>
                  <span class="lab">{{ isEvaluatorSummaryAgent(agent) ? 'Evaluations' : 'Precision' }}</span>
                </div>
              </div>

              <div class="validations-bar" v-if="!isEvaluatorSummaryAgent(agent) && agent.stats && agent.stats.percentages">
                <div class="v-segment pos" :style="{ width: agent.stats.percentages.positive + '%' }"></div>
                <div class="v-segment alt" :style="{ width: agent.stats.percentages.alternative + '%' }"></div>
                <div class="v-segment par" :style="{ width: agent.stats.percentages.partial + '%' }"></div>
                <div class="v-segment neg" :style="{ width: agent.stats.percentages.negative + '%' }"></div>
              </div>

              <div class="evaluator-notice" v-if="isEvaluatorSummaryAgent(agent)">
                This agent is only an evaluator of the quality of the responses generated by the evaluated agent.
              </div>
            </div>
          </div>

          <div class="findings-container" v-if="summaryStats.comparison">
            <h2 class="section-title">Benchmark Insight</h2>
            <div class="findings-grid">
              <div class="finding-box best-quality">
                <div class="finding-icon">🏆</div>
                <div class="finding-content">
                  <span class="f-label">Quality Leader</span>
                  <span class="f-value">{{ summaryStats.comparison.betterRatedAgent }}</span>
                </div>
              </div>
              <div class="finding-box fastest">
                <div class="finding-icon">⚡</div>
                <div class="finding-content">
                  <span class="f-label">Speed Leader</span>
                  <span class="f-value">{{ summaryStats.comparison.fasterAgent }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <div class="footer-decoration">
          <p>This report was automatically generated by Corvic AI Comparison Suite.</p>
          <div class="dot-pattern"></div>
        </div>
      </div>

      <!-- Results Pages -->
      <div 
        v-for="(question, qIndex) in results" 
        :key="qIndex"
        class="print-page result-page"
      >
        <div class="page-header-mini">
          <span class="workspace-ref">{{ workspaceName }} Benchmark</span>
          <span class="page-num">Item #{{ qIndex + 1 }}</span>
        </div>

        <div class="question-container">
          <div class="q-badge">Question</div>
          <p class="q-text">{{ typeof question.question === 'object' ? question.question.question : question.question }}</p>
        </div>

        <div class="question-container expected-container" v-if="question.expectedAnswer">
          <div class="q-badge expected-badge">Expected Answer</div>
          <div class="q-text expected-text" v-html="renderMarkdown(question.expectedAnswer)"></div>
        </div>

        <div class="responses-comparison-grid">
          <div
            v-for="(ans, aIndex) in orderedQuestionAgents(question)"
            :key="aIndex"
            class="response-card"
          >
            <div class="response-header">
              <div class="agent-info">
                <span class="agent-name">{{ ans.name }}</span>
                <span class="provider-pill">{{ ans.provider }}</span>
              </div>
              <div v-if="ans.humanValidation" class="validation-status" :class="ans.humanValidation">
                {{ getValidationIcon(ans.humanValidation) }}
                <span class="v-text">{{ ans.humanValidation }}</span>
              </div>
            </div>
            
            <div class="response-body">
              <div v-if="ans.error" class="error-box">
                <span class="error-icon">❌</span>
                <div class="error-content">
                  <strong>Execution Error</strong>
                  <p>{{ ans.error }}</p>
                </div>
              </div>
              <div 
                v-else-if="ans.answer" 
                class="markdown-content" 
                v-html="renderMarkdown(ans.answerText || ans.answer)"
              ></div>
              <div v-else class="empty-state">
                <em>Waiting for manual response input...</em>
              </div>
            </div>

            <div class="response-footer" v-if="ans.duration">
              <div class="meta-pill">
                <span class="icon">⏱️</span>
                {{ ans.duration }}s
              </div>
            </div>
          </div>
        </div>
        
        <div class="page-footer-mini">
          Confidential - Generated for {{ workspaceName }}
        </div>
      </div>
    </template>
  </div>
</template>

<script>
import CorvicLogo from './CorvicLogo.vue'
import { processContent } from '../utils/markdown.js'

export default {
  name: 'PrintReport',
  components: {
    CorvicLogo
  },
  props: {
    workspaceName: {
      type: String,
      default: ''
    },
    summaryStats: {
      type: Object,
      default: () => ({})
    },
    results: {
      type: Array,
      default: () => []
    },
    reportVariant: {
      type: String,
      default: 'full'
    },
    reportTitle: {
      type: String,
      default: ''
    },
    reportSubtitle: {
      type: String,
      default: ''
    },
    questionCards: {
      type: Array,
      default: () => []
    }
  },
  computed: {
    reportDate() {
      return new Date().toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      })
    },
    isQuestionCardReport() {
      return this.reportVariant === 'question_cards'
    },
    normalizedQuestionCards() {
      return Array.isArray(this.questionCards) ? this.questionCards : []
    },
    compactReportTitle() {
      if (this.reportTitle) return this.reportTitle
      return this.normalizedQuestionCards.length === 1 ? 'Question Analysis' : 'Questions Analysis'
    },
    compactReportSubtitle() {
      if (this.reportSubtitle) return this.reportSubtitle
      const count = this.normalizedQuestionCards.length
      if (count === 0) return ''
      return `${count} question${count === 1 ? '' : 's'} selected for compact analysis.`
    }
  },
  methods: {
    isEvaluatorSummaryAgent(agent) {
      const provider = String(agent?.provider || '').toLowerCase()
      const name = String(agent?.name || '').toLowerCase()
      return provider === 'evaluator' || (provider === 'openai' && name.includes('evaluator'))
    },
    isEvaluatorAnswer(answer) {
      const provider = String(answer?.provider || '').toLowerCase()
      const name = String(answer?.name || '').toLowerCase()
      return provider === 'evaluator' || (provider === 'openai' && name.includes('evaluator'))
    },
    orderedQuestionAgents(question) {
      const list = Array.isArray(question?.agents) ? [...question.agents] : []
      return list.sort((a, b) => {
        const aEval = this.isEvaluatorAnswer(a)
        const bEval = this.isEvaluatorAnswer(b)
        if (aEval && !bEval) return 1
        if (!aEval && bEval) return -1
        return 0
      })
    },
    renderMarkdown(text) {
      if (!text) return ''
      return processContent(text).html || text
    },
    formatDuration(value) {
      const seconds = parseFloat(value)
      if (Number.isFinite(seconds)) {
        return seconds >= 60 ? `${(seconds / 60).toFixed(1)} min` : `${seconds.toFixed(1)} s`
      }
      return '0 s'
    },
    getValidationIcon(type) {
      const icons = {
        positive: '👍',
        negative: '👎',
        alternative: '🔄',
        partial: '⚠️'
      }
      return icons[type] || ''
    },
    getCompactScoreClass(card) {
      const severity = String(card?.evaluationSeverity || '').toLowerCase()
      if (severity === 'danger') return 'analysis-score-chip-danger'
      if (severity === 'warning') return 'analysis-score-chip-warning'
      return 'analysis-score-chip-ok'
    },
    shouldShowStatus(agent) {
      if (!agent.stats) return false
      const stats = agent.stats
      const isRunComplete = (stats.answered + stats.errors) === stats.totalQuestions
      
      if (!isRunComplete) return true
      if (stats.errors > 0) return true
      
      if (!this.isEvaluatorSummaryAgent(agent)) {
         if (stats.validations.notEvaluated > 0) return true
      }
      
      return false
    }
  }
}
</script>

<style scoped>
.print-report {
  --primary: #49399d;
  --primary-light: #667eea;
  --secondary: #764ba2;
  --success: #10b981;
  --danger: #ef4444;
  --warning: #f59e0b;
  --info: #3b82f6;
  --bg-page: #ffffff;
  --text-main: #1f2937;
  --text-muted: #6b7280;
  --border: #e5e7eb;
  
  color: var(--text-main);
  font-family: 'Outfit', 'Inter', system-ui, sans-serif;
  background: var(--bg-page);
  -webkit-print-color-adjust: exact;
  print-color-adjust: exact;
}

/* Page Setup */
.print-page {
  padding: 50px;
  position: relative;
  box-sizing: border-box;
  background: white;
  margin-bottom: 2rem; /* Spacing for screen view */
}

@media print {
  .print-page {
    page-break-after: always;
    display: block !important;
    min-height: auto !important;
    height: auto !important;
    margin-bottom: 0;
  }
  .print-page:last-child {
    page-break-after: auto;
  }
  
  /* Fix for Cover Page Centering */
  .print-page.cover-page {
    display: flex !important;
    /* A3 Landscape height is 297mm. With 1cm margins, available height is ~277mm. 
       Using 100vh causes spillover. 270mm is safe. */
    height: 270mm !important; 
    min-height: 270mm !important;
    justify-content: center !important;
    align-items: center !important;
    margin: 0 !important;
    padding: 0 !important; /* Remove padding to maximize centering space */
  }
}

/* Cover Page Decorations */
.header-decoration {
  position: absolute;
  top: 0;
  right: 0;
  width: 300px;
  height: 300px;
  background: linear-gradient(135deg, var(--primary-light) 0%, var(--secondary) 100%);
  clip-path: polygon(0 0, 100% 0, 100% 100%);
  opacity: 0.1;
  z-index: 0;
}

.footer-decoration {
  margin-top: 3rem;
  width: 100%;
  max-width: 900px;
  border-top: 1px solid var(--border);
  padding-top: 20px;
  color: var(--text-muted);
  font-size: 0.85rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.dot-pattern {
  width: 100px;
  height: 40px;
  background-image: radial-gradient(var(--border) 1.5px, transparent 1.5px);
  background-size: 8px 8px;
  opacity: 0.5;
}

/* Cover Page Content */
.cover-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  text-align: center;
}

.logo-wrapper {
  margin-bottom: 2rem;
  filter: drop-shadow(0 4px 6px rgba(0,0,0,0.1));
}

.report-header h1 {
  font-size: 3.5rem;
  font-weight: 800;
  margin: 0 0 1.5rem 0;
  background: linear-gradient(90deg, var(--primary), var(--secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
}

.report-meta {
  display: flex;
  gap: 15px;
  margin-bottom: 3rem;
}

.meta-badge {
  background: #f3f4f6;
  padding: 8px 16px;
  border-radius: 999px;
  font-size: 0.95rem;
  border: 1px solid var(--border);
}

.meta-label {
  color: var(--text-muted);
  font-weight: 500;
  margin-right: 6px;
}

.meta-value {
  color: var(--primary);
  font-weight: 700;
}

/* Summary Section */
.summary-section-print {
  width: 100%;
  max-width: 900px;
  text-align: left;
}

.section-title {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--primary);
  margin: 2rem 0 1.5rem 0;
  display: flex;
  align-items: center;
  gap: 10px;
}
.section-title::after {
  content: '';
  flex: 1;
  height: 2px;
  background: linear-gradient(90deg, var(--border), transparent);
}

.summary-grid-print {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 25px;
  margin-bottom: 3rem;
}

.agent-summary-card {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 24px;
  position: relative;
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.05);
}

.agent-summary-card h3 {
  font-size: 1.25rem;
  margin: 0 0 1.5rem 0;
  color: #111827;
}

.quality-circle {
  position: absolute;
  top: 20px;
  right: 24px;
  width: 60px;
  height: 60px;
  border-radius: 50%;
  background: #f0fdf4;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  border: 2px solid var(--success);
}

.evaluator-badge {
  position: absolute;
  top: 20px;
  right: 24px;
  padding: 4px 12px;
  border-radius: 999px;
  background: #fdf2f8;
  color: #db2777;
  border: 1px solid #f9a8d4;
  font-size: 0.75rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.evaluator-notice {
  margin-top: 15px;
  padding-top: 15px;
  border-top: 1px dashed var(--border);
  font-size: 0.75rem;
  color: var(--text-muted);
  font-style: italic;
  line-height: 1.4;
}

.quality-circle .score {
  font-weight: 800;
  font-size: 1rem;
  color: var(--success);
  line-height: 1;
}

.quality-circle .label {
  font-size: 0.65rem;
  text-transform: uppercase;
  color: var(--success);
  font-weight: 700;
}

.stats-mini-list {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-bottom: 20px;
}

.mini-stat {
  display: flex;
  flex-direction: column;
}

.mini-stat .val {
  font-weight: 700;
  font-size: 1.1rem;
  color: #111827;
}

.mini-stat .lab {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.validations-bar {
  display: flex;
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: #f3f4f6;
}

.v-segment { height: 100%; }
.v-segment.pos { background: var(--success); }
.v-segment.alt { background: var(--info); }
.v-segment.par { background: var(--warning); }
.v-segment.neg { background: var(--danger); }

/* Findings */
.findings-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}

.finding-box {
  background: #f9fafb;
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 15px;
}

.finding-icon {
  font-size: 1.5rem;
}

.finding-content {
  display: flex;
  flex-direction: column;
}

.f-label {
  font-size: 0.8rem;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
}

.f-value {
  font-weight: 800;
  color: var(--primary);
  font-size: 1rem;
}

/* Result Pages */
.result-page {
  display: flex;
  flex-direction: column;
}

.page-header-mini {
  display: flex;
  justify-content: space-between;
  font-size: 0.8rem;
  color: var(--text-muted);
  text-transform: uppercase;
  font-weight: 700;
  letter-spacing: 0.05em;
  margin-bottom: 2rem;
  border-bottom: 1px solid var(--border);
  padding-bottom: 10px;
}

.question-container {
  margin-bottom: 2rem;
  background: #f8fafc;
  padding: 24px;
  border-left: 6px solid var(--primary);
  border-radius: 0 12px 12px 0;
}

.q-badge {
  font-size: 0.7rem;
  background: var(--primary);
  color: white;
  padding: 3px 8px;
  border-radius: 4px;
  display: inline-block;
  margin-bottom: 8px;
  text-transform: uppercase;
  font-weight: 800;
}

.q-text {
  font-size: 1.4rem;
  font-weight: 700;
  color: #0f172a;
  margin: 0;
  line-height: 1.4;
}

.expected-container {
  background: #f0fdf4;
  border-left-color: #10b981;
  margin-top: -1rem;
  margin-bottom: 2rem;
  padding: 16px 24px;
}

.expected-badge {
  background: #10b981;
}

.expected-text {
  font-size: 1.1rem;
  font-weight: 500;
  color: #166534;
}

.responses-comparison-grid {
  display: flex;
  gap: 20px;
  align-items: stretch;
}

.response-card {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  background: #ffffff;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05);
  page-break-inside: auto; 
  break-inside: auto;
}

.response-header {
  padding: 16px 20px;
  background: #fdfdfd;
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.agent-name {
  font-weight: 800;
  font-size: 1.1rem;
  color: #111827;
  margin-right: 10px;
}

.provider-pill {
  font-size: 0.7rem;
  color: var(--primary);
  background: #eff6ff;
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 700;
  text-transform: uppercase;
}

.validation-status {
  display: flex;
  align-items: center;
  gap: 5px;
  font-weight: 700;
  font-size: 0.85rem;
  padding: 4px 10px;
  border-radius: 6px;
}

.validation-status.positive { background: #dcfce7; color: #166534; }
.validation-status.negative { background: #fee2e2; color: #991b1b; }
.validation-status.alternative { background: #e0f2fe; color: #075985; }
.validation-status.partial { background: #fef3c7; color: #92400e; }

.response-body {
  padding: 20px;
  font-size: 1rem;
  line-height: 1.6;
  color: #374151;
}

.markdown-content :deep(pre) {
  background: #1e293b;
  color: #f8fafc;
  padding: 16px;
  border-radius: 8px;
  font-size: 0.9rem;
  margin: 1rem 0;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.markdown-content :deep(code) {
  font-family: 'Fira Code', 'Courier New', monospace;
}

.markdown-content :deep(img) {
  max-width: 100%;
  border-radius: 8px;
}

.markdown-content :deep(ul), 
.markdown-content :deep(ol) {
  padding-left: 1.5rem;
  margin: 1rem 0;
}

.markdown-content :deep(li) {
  margin-bottom: 0.5rem;
}

.response-footer {
  padding: 12px 20px;
  background: #f9fafb;
  border-top: 1px solid var(--border);
}

.meta-pill {
  font-size: 0.8rem;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: white;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid var(--border);
}

.error-box {
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 8px;
  padding: 16px;
  display: flex;
  gap: 12px;
}

.error-content strong {
  display: block;
  color: #991b1b;
  margin-bottom: 4px;
}

.error-content p {
  margin: 0;
  color: #dc2626;
  font-size: 0.9rem;
}

.page-footer-mini {
  margin-top: 20px;
  padding-top: 20px;
  font-size: 0.75rem;
  color: var(--text-muted);
  text-align: center;
}

/* Card Accents */
.card-accent-1 { border-top: 5px solid var(--primary); }
.card-accent-2 { border-top: 5px solid var(--secondary); }
.card-accent-3 { border-top: 5px solid var(--info); }
.card-accent-4 { border-top: 5px solid var(--success); }

.print-report-compact {
  padding: 28px;
}

.compact-report-header {
  margin-bottom: 24px;
  padding: 28px 32px;
  border: 1px solid var(--border);
  border-radius: 24px;
  background:
    radial-gradient(circle at top right, rgba(118, 75, 162, 0.14), transparent 32%),
    linear-gradient(135deg, rgba(102, 126, 234, 0.12), rgba(255, 255, 255, 0.96));
}

.compact-report-topbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 28px;
}

.compact-logo-wrapper {
  margin-bottom: 0;
}

.compact-meta-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 12px;
}

.compact-title-block h1 {
  margin: 8px 0 0;
  font-size: 2.4rem;
  line-height: 1.05;
  letter-spacing: -0.03em;
  color: #111827;
}

.compact-kicker {
  display: inline-flex;
  align-items: center;
  padding: 5px 10px;
  border-radius: 999px;
  background: rgba(73, 57, 157, 0.1);
  color: var(--primary);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.compact-subtitle {
  margin: 10px 0 0;
  max-width: 720px;
  color: var(--text-muted);
  font-size: 1rem;
  line-height: 1.6;
}

.compact-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 20px;
}

.analysis-card {
  border: 1px solid var(--border);
  border-radius: 22px;
  padding: 24px;
  background: white;
  box-shadow: 0 12px 24px rgba(15, 23, 42, 0.06);
  break-inside: avoid;
  page-break-inside: avoid;
}

.analysis-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 20px;
}

.analysis-card-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52px;
  height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 0.8rem;
  font-weight: 800;
  letter-spacing: 0.04em;
}

.analysis-score-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  font-size: 0.8rem;
  font-weight: 800;
}

.analysis-score-chip-ok {
  background: #dcfce7;
  color: #166534;
}

.analysis-score-chip-warning {
  background: #fef3c7;
  color: #92400e;
}

.analysis-score-chip-danger {
  background: #fee2e2;
  color: #991b1b;
}

.analysis-section + .analysis-section {
  margin-top: 18px;
}

.analysis-label {
  margin-bottom: 8px;
  color: #64748b;
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.analysis-question {
  color: #0f172a;
  font-size: 1.18rem;
  font-weight: 700;
  line-height: 1.5;
}

.analysis-markdown {
  padding: 16px 18px;
  border-radius: 16px;
  background: #f8fafc;
  color: #334155;
  line-height: 1.65;
  border: 1px solid rgba(226, 232, 240, 0.9);
}

.analysis-markdown :deep(p) {
  margin: 0 0 0.75rem;
}

.analysis-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.analysis-markdown :deep(ul),
.analysis-markdown :deep(ol) {
  margin: 0.75rem 0;
  padding-left: 1.35rem;
}

.analysis-markdown :deep(pre) {
  margin: 0.75rem 0;
  padding: 14px;
  border-radius: 12px;
  background: #0f172a;
  color: #e2e8f0;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.analysis-markdown :deep(code) {
  font-family: 'Fira Code', 'Courier New', monospace;
}

.analysis-markdown :deep(blockquote) {
  margin: 0.75rem 0;
  padding-left: 12px;
  border-left: 3px solid #cbd5e1;
  color: #475569;
}

.analysis-markdown :deep(img) {
  max-width: 100%;
  border-radius: 12px;
}

@media print {
  .print-report-compact {
    padding: 0;
  }

  .compact-report-header {
    break-inside: avoid;
    page-break-inside: avoid;
    margin-bottom: 16px;
  }

  .compact-card-grid {
    gap: 14px;
  }

  .analysis-card {
    box-shadow: none;
  }
}
</style>
