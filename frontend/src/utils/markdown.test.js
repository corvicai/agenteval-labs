import { describe, it, expect } from 'vitest'
import { processContent } from './markdown.js'

// processContent().html is bound via v-html in ChatPanel/BenchmarkArena, so it
// must always be sanitized — including the branch where the content is not
// detected as markdown (previously returned raw, enabling stored XSS).
describe('processContent — XSS sanitization', () => {
  it('strips event-handler XSS from non-markdown content', () => {
    const out = processContent('<img src=x onerror="alert(document.cookie)">')
    expect(out.html.toLowerCase()).not.toContain('onerror')
  })

  it('strips <script> from non-markdown content', () => {
    const out = processContent('<script>alert(1)</script>hello')
    expect(out.html.toLowerCase()).not.toContain('<script')
    expect(out.html).toContain('hello')
  })

  it('preserves benign non-markdown text', () => {
    const out = processContent('a plain answer')
    expect(out.html).toContain('a plain answer')
  })

  it('still renders markdown (regression guard for marked 18)', () => {
    const out = processContent('**bold**')
    expect(out.html).toContain('<strong>')
  })
})
