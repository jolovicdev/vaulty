import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'wailsjs'] },
  js.configs.recommended,
  tseslint.configs.recommended,
  reactHooks.configs.flat['recommended-latest'],
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
  },
  {
    // These components read from the Go bridge when they mount or when the
    // selection changes. That is the "subscribe to an external system" case
    // the rule is written to allow, but it cannot see through the async
    // helper that awaits the bridge call, so it reports the await as a
    // synchronous update. Every state update behind these calls is guarded
    // by a live flag or keyed by the id it belongs to.
    files: ['src/App.tsx', 'src/components/EntryEditor.tsx', 'src/components/Generator.tsx'],
    rules: { 'react-hooks/set-state-in-effect': 'off' },
  },
)
