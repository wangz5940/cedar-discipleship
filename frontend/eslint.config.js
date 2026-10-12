import globals from 'globals';
import typescriptParser from '@typescript-eslint/parser';
import vueParser from 'vue-eslint-parser';
import vue from 'eslint-plugin-vue';

const restrictedAt = {
  selector: 'CallExpression[callee.type="MemberExpression"][callee.computed=false][callee.property.name="at"], CallExpression[callee.type="MemberExpression"][callee.computed=true][callee.property.value="at"]',
  message: '兼容 iOS 15.0–15.3 Safari：浏览器代码不可调用 .at()，请使用下标或等价兼容写法。',
};

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
    files: ['src/**/*.{js,ts,vue}', 'public/**/*.js'],
    ignores: ['src/**/*.test.*'],
    rules: { 'no-restricted-syntax': ['error', restrictedAt] },
  },
  {
    files: ['src/**/*.ts'],
    languageOptions: { parser: typescriptParser },
  },
  {
    files: ['src/**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: { parser: typescriptParser, ecmaVersion: 'latest', sourceType: 'module' },
    },
    plugins: { vue },
    rules: { 'vue/no-restricted-syntax': ['error', restrictedAt] },
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
