import { describe, expect, it } from 'vitest'

import { resolveQuestionSetSelection } from './questionSet.js'

describe('resolveQuestionSetSelection', () => {
  const setA = { id: 'set-a', name: 'Set A' }
  const setB = { id: 'set-b', name: 'Set B' }

  it('prefers the explicitly requested question set over the last selected one', () => {
    const result = resolveQuestionSetSelection({
      questionSets: [setA, setB],
      preferredId: 'set-b',
      lastQuestionSetId: 'set-a',
      currentQuestionSet: setA
    })

    expect(result).toEqual(setB)
  })

  it('preserves the current selection when a preferred id was created locally before store sync catches up', () => {
    const localDraft = { id: 'set-local', name: 'Imported Set' }

    const result = resolveQuestionSetSelection({
      questionSets: [setA, setB],
      preferredId: 'set-local',
      lastQuestionSetId: 'set-a',
      currentQuestionSet: localDraft
    })

    expect(result).toEqual(localDraft)
  })

  it('falls back to the last selected question set when no preferred id matches', () => {
    const result = resolveQuestionSetSelection({
      questionSets: [setA, setB],
      lastQuestionSetId: 'set-b'
    })

    expect(result).toEqual(setB)
  })

  it('returns the current in-store selection when no other preference matches', () => {
    const result = resolveQuestionSetSelection({
      questionSets: [setA, setB],
      currentQuestionSet: { id: 'set-a', name: 'Older Set A' }
    })

    expect(result).toEqual(setA)
  })
})
