import { ESLint } from 'eslint';
import { expect, it } from 'vitest';

const eslint = new ESLint();

it.each([
  ['src/compatibility.js', 'const values = []; values.at(-1);'],
  ['src/compatibility.ts', 'const values: number[] = []; values.at(-1);'],
  ['src/compatibility.js', 'const values = []; values?.at?.(-1);'],
  ['src/compatibility.js', 'const values = []; values["at"](-1);'],
  ['public/compatibility.js', 'new Uint8Array().at(-1);'],
  ['src/Compatibility.vue', '<script setup>const last = "name".at(-1);</script><template>{{ last }}</template>'],
  ['src/Compatibility.vue', '<template>{{ values.at(-1) }}</template>'],
  ['src/Compatibility.vue', '<template>{{ values?.["at"]?.(-1) }}</template>'],
])('rejects unsupported runtime calls in %s: %s', async (filePath, code) => {
  const [result] = await eslint.lintText(code, { filePath });
  expect(result.messages.some(message => ['no-restricted-syntax', 'vue/no-restricted-syntax'].includes(message.ruleId))).toBe(true);
  expect(result.fatalErrorCount).toBe(0);
});

it.each([
  ['src/compatibility.js', 'const values = []; const last = values[values.length - 1];'],
  ['src/compatibility.ts', 'const record: { at: string } = { at: "2026-10-12" }; const date = record.at;'],
  ['src/Compatibility.vue', '<template>{{ record.at }}</template>'],
  ['src/runtime/compatibility.test.js', 'const calls = []; calls.at(-1);'],
])('allows compatible indexing, timestamp fields and Node tests in %s', async (filePath, code) => {
  const [result] = await eslint.lintText(code, { filePath });
  expect(result.errorCount).toBe(0);
});
