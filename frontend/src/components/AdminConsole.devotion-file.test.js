import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { expect, it } from 'vitest';
import AdminConsole from './AdminConsole.vue';
import { useAppStateStore } from '../stores/appState';

it.each([
  ['2026-10-08', '/api/assets/2/download'],
  ['2026-10-09', '/api/assets/1/download'],
])('修改一天文件后，%s 仍显示自身文件或旧计划固定文件', async (date, expected) => {
  const pinia = createPinia();
  const store = useAppStateStore(pinia);
  store.$patch({
    currentGroupID: 6, user: { id: 1, roles: ['group_admin'] }, canEditLearning: true,
    learningConfig: { task_sections: { daily: { devotion: {
      plan_mode: 'custom', custom_path: '/api/assets/1/download',
      plans: [
        { date: '2026-10-08', title: '一天已更换', path: '/api/assets/2/download', type: 'pdf' },
        { date: '2026-10-09', title: '旧日期', page_start: '20', page_end: '22' },
      ],
    } } } },
  });
  let selected;
  const component = {
    ...AdminConsole,
    setup(props, context) {
      const state = AdminConsole.setup(props, context);
      state.dailyPlanDate.value = date;
      selected = state.selectedDailyPlanPath;
      return state;
    },
  };
  await renderToString(createSSRApp(component).use(pinia));
  expect(selected.value).toBe(expected);
  expect(store.learningConfig.task_sections.daily.devotion.custom_path).toBe('/api/assets/1/download');
});
