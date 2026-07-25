import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';

/* Lint is about correctness, not formatting: Prettier owns layout, so no
   stylistic rule is enabled here. The rules below are the ones that would have
   caught real defects in this codebase — unchecked promises, stale hook
   dependencies, and unsafe `any` slipping past the strict compiler.
   Type-aware rules are scoped to the application sources that the tsconfig
   projects actually cover; plain Node scripts get the untyped baseline. */
export default tseslint.config(
  { ignores: ['dist', 'coverage', 'node_modules'] },
  js.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      ...tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat['recommended-latest'],
    ],
    languageOptions: {
      globals: { ...globals.browser },
      parserOptions: {
        project: ['./tsconfig.app.json', './tsconfig.node.json', './tsconfig.e2e.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // The API layer deliberately types network payloads as `unknown` and
      // narrows them with explicit guards; that is the safe pattern, not a
      // violation.
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/restrict-template-expressions': ['error', { allowNumber: true }],
      // A leading underscore is the established signal for a parameter a
      // signature requires but the implementation deliberately ignores.
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
    },
  },
  {
    // Tests build deliberately malformed payloads to assert the guards reject
    // them, so the strict-shape rules would fight the point of the test.
    // unbound-method fires on vi.fn() spies passed to matchers, which is the
    // intended way to assert on a mock.
    files: ['**/*.test.{ts,tsx}', 'src/test/**'],
    rules: {
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/unbound-method': 'off',
    },
  },
  {
    // The media-capture tool is a standalone Node script, outside the browser
    // bundle and outside the TypeScript projects. Its page.evaluate callbacks
    // are serialized and run inside the browser, so both global sets apply.
    files: ['**/*.mjs', 'eslint.config.js'],
    languageOptions: { globals: { ...globals.node, ...globals.browser } },
  },
);
