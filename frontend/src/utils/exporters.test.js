import { describe, expect, it } from 'vitest'

import { exportResultsReport } from './exporters.js'

describe('exportResultsReport', () => {
  it('includes trimmed question set notes in the summary payload', () => {
    const report = exportResultsReport({
      agentsRef: [
        {
          id: 'agent-1',
          provider: 'openai',
          config: { name: 'Primary Agent' },
          results: [
            {
              question: {
                question: 'What is the launch date?'
              },
              answer: 'Tomorrow',
              duration: 1.2
            }
          ]
        }
      ],
      calculateStats: () => ({
        validations: {
          positive: 1,
          negative: 0,
          alternative: 0,
          partial: 0,
          notEvaluated: 0
        },
        answered: 1,
        totalQuestions: 1,
        errors: 0,
        avgDuration: '1.2',
        percentages: {
          positive: 100,
          negative: 0,
          alternative: 0,
          partial: 0
        }
      }),
      questionSetData: {
        notes: '  Include launch dependencies and rollout caveats.  '
      }
    })

    expect(report.summary.notes).toBe('Include launch dependencies and rollout caveats.')
  })
})
