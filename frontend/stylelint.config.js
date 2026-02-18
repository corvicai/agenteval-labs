export default {
  overrides: [
    {
      files: ['**/*.vue'],
      customSyntax: 'postcss-html'
    }
  ],
  rules: {
    // Keep this focused on accidental duplication/risky mistakes.
    'no-duplicate-selectors': true,
    'declaration-block-no-duplicate-properties': [
      true,
      {
        ignore: ['consecutive-duplicates-with-different-values']
      }
    ],
    'block-no-empty': true
  }
}
