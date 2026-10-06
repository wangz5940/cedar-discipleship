import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';

const source = readFileSync(new URL('./CountingAttendance.vue', import.meta.url), 'utf8');
function handler(name, bindings) {
  const body = source.match(new RegExp(`async function ${name}\\([^]*?\\n\\}`))[0];
  return new Function(...Object.keys(bindings), `${body}; return ${name};`)(...Object.values(bindings));
}

it('固定星期即勾即保存，不提交尚未保存的额外日期', async () => {
  const weekdays = { value: [1] };
  const extraDates = { value: ['2026-10-10'] };
  const api = vi.fn().mockResolvedValue({});
  const bindings = {
    props: { groupId: 1 }, weekdays, extraDates, saving: { value: false },
    sheet: { value: { can_manage: true, settings: { extra_dates: ['2026-10-06'] } } },
    api, showToast: vi.fn(),
    loadAttendance: async () => { extraDates.value = ['2026-10-06']; },
  };
  const saveSettings = handler('saveSettings', bindings);
  await handler('toggleWeekday', { ...bindings, saveSettings })(3);
  expect(JSON.parse(api.mock.calls[0][1].body)).toEqual({ weekdays: [1, 3], extra_dates: ['2026-10-06'] });
  expect(extraDates.value).toEqual(['2026-10-10']);
});

it('固定星期保存失败恢复勾选，保留额外日期草稿', async () => {
  const weekdays = { value: [1] };
  const extraDates = { value: ['2026-10-10'] };
  const bindings = {
    props: { groupId: 1 }, weekdays, extraDates, saving: { value: false },
    sheet: { value: { can_manage: true, settings: { extra_dates: [] } } },
    api: vi.fn().mockRejectedValue(new Error('保存失败')), showToast: vi.fn(), loadAttendance: vi.fn(),
  };
  const saveSettings = handler('saveSettings', bindings);
  await handler('toggleWeekday', { ...bindings, saveSettings })(3);
  expect(weekdays.value).toEqual([1]);
  expect(extraDates.value).toEqual(['2026-10-10']);
  expect(bindings.saving.value).toBe(false);
});
