import pluginVue from 'eslint-plugin-vue'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import simpleImportSort from 'eslint-plugin-simple-import-sort'
import skipFormattingConfig from '@vue/eslint-config-prettier/skip-formatting'

export default defineConfigWithVueTs(
  {
    ignores: ['node_modules/**', 'dist/**', '.vite/**'],
  },
  pluginVue.configs['flat/recommended'],
  vueTsConfigs.recommendedTypeChecked,
  vueTsConfigs.stylisticTypeChecked,
  {
    files: ['**/*.vue', '**/*.ts'],
    plugins: {
      'simple-import-sort': simpleImportSort,
    },
    rules: {
      'vue/require-default-prop': 'off',
      '@typescript-eslint/use-unknown-in-catch-callback-variable': 'error',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      'vue/no-unused-vars': [
        'error',
        {
          ignorePattern: '^_',
        },
      ],
      'vue/block-order': [
        'error',
        {
          order: ['script', 'template', 'style'],
        },
      ],
      'no-warning-comments': 'warn',
      'simple-import-sort/imports': [
        'error',
        {
          groups: [['^\\u0000', '^node:', '^@?\\w', '^@/', '^\\.', '^\\.\\.', '^.*\\u0000$']],
        },
      ],
    },
  },
  skipFormattingConfig,
)
