import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { expect, it } from 'vitest';
import AdminConsole from './AdminConsole.vue';
import { useAppStateStore } from '../stores/appState';

const localAudio = { id: 11, title: '本组课程乙', type: 'audio', category: 'audio', url: '/api/assets/11/download' };
const localVideo = { id: 12, title: '本组课程甲', type: 'video', category: 'video', url: '/api/assets/12/download' };
const externalURL = 'https://ovcm.net/tx2026/#/course/course/lesson';

async function renderPicker({ resources = [localAudio, localVideo], binding = {}, query = '', courses = true, groupID = 1 } = {}) {
  const pinia = createPinia();
  const store = useAppStateStore(pinia);
  store.$patch({
    currentGroupID: groupID, canEditLearning: true, canEditStudyWeeks: true,
    user: { id: 1, roles: ['group_admin'] },
    resourceLibrary: [{ key: 'uploaded_video', items: resources }],
    weekDraft: { id: 1, start: '2026-10-05', end: '2026-10-11', readings: [], videos: [binding] },
  });
  const component = {
    ...AdminConsole,
    setup(props, context) {
      const state = AdminConsole.setup(props, context);
      // SSR does not run onMounted; seed the catalogue and search input before rendering.
      state.ovcmCourses.value = courses ? [
        { id: 'course', title: '外部课程', lessons: [{ id: 'lesson', title: '第一课', type: 'audio' }] },
      ] : [];
      state.videoQuery.value = query;
      return state;
    },
  };
  const html = await renderToString(createSSRApp(component).use(pinia));
  const select = html.match(/<select aria-label="周任务音视频资源"[^>]*>[\s\S]*?<\/select>/)[0];
  const values = text => [...text.matchAll(/<option[^>]* value="([^"]*)"/g)].map(match => match[1]);
  return {
    store, select, values: values(select),
    groups: [...select.matchAll(/<optgroup label="([^"]*)"[^>]*>([\s\S]*?)<\/optgroup>/g)]
      .map(match => ({ label: match[1], values: values(match[2]) })),
  };
}

it('周任务先展示本组音频和视频，再展示外部课程，保留各来源原顺序与资源值', async () => {
  const picker = await renderPicker();
  expect(picker.groups).toEqual([
    { label: '本组资料', values: ['asset:11', 'asset:12'] },
    { label: '外部资料（OVCM）', values: [`url:${externalURL}`] },
  ]);
  expect(picker.values[0]).toBe('');
});

it.each([
  [{ asset_id: 11 }, '外部', 'asset:11', ['asset:11', `url:${externalURL}`]],
  [{ url: externalURL }, '本组', `url:${externalURL}`, ['asset:11', 'asset:12', `url:${externalURL}`]],
  [{ title: '本组课程乙' }, '无匹配', 'asset:11', ['asset:11']],
])('搜索时保留已选或旧标题匹配资源及绑定值：%j', async (binding, query, selected, values) => {
  const picker = await renderPicker({ binding, query });
  expect(picker.select.slice(0, picker.select.indexOf('>'))).toContain(`value="${selected}"`);
  expect(picker.groups.flatMap(group => group.values)).toEqual(values);
  expect(picker.store.weekDraft.videos[0]).toEqual(binding);
});

it('隐藏空来源，其他小组只展示自己的资源，外部目录仍在后面', async () => {
  const empty = await renderPicker({ resources: [] });
  expect(empty.groups).toEqual([{ label: '外部资料（OVCM）', values: [`url:${externalURL}`] }]);
  const other = await renderPicker({ groupID: 2, resources: [localVideo] });
  expect(other.groups).toEqual([
    { label: '本组资料', values: ['asset:12'] },
    { label: '外部资料（OVCM）', values: [`url:${externalURL}`] },
  ]);
  const unmatched = await renderPicker({ query: '无匹配' });
  expect(unmatched.groups).toEqual([]);
  expect(unmatched.values).toEqual(['']);
});

it('没有外部课程时仍可选择本组资料', async () => {
  const picker = await renderPicker({ courses: false });
  expect(picker.groups).toEqual([{ label: '本组资料', values: ['asset:11', 'asset:12'] }]);
});
