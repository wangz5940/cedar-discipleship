import globals from 'globals';

export default [
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**', 'public/ovcm-player/assets/**'] },
  {
    files: ['**/*.js'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: globals.browser,
    },
    rules: { 'no-undef': 'error' },
  },
  {
    files: ['*.config.js', 'src/**/*.test.js'],
    languageOptions: { globals: globals.node },
  },
  {
    files: ['src/runtime/appVersion.js'],
    languageOptions: { globals: { __APP_BUILD_VERSION__: 'readonly' } },
  },
];
