import { createRenderer, createSSRApp, h, nextTick, ref } from 'vue';
import * as vue from 'vue';
import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import { renderToString } from 'vue/server-renderer';
import { afterEach, expect, it, vi } from 'vitest';
import LearningConfigSection from './LearningConfigSection.vue';

afterEach(() => vi.unstubAllGlobals());

it('未操作过的配置默认收起', async () => {
  vi.stubGlobal('localStorage', { getItem: () => null });
  for (const storageKey of ['group:1:verse', 'group:2:verse']) {
    const html = await renderToString(createSSRApp(LearningConfigSection, { title: '背经配置', storageKey }));
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain('display:none');
  }
});

it('切换后恢复当前账号小组的展开状态，其他小组默认收起', async () => {
  vi.stubGlobal('localStorage', { getItem: key => key === 'group:1:verse' ? 'open' : null });
  const expanded = await renderToString(createSSRApp(LearningConfigSection, { title: '背经配置', storageKey: 'group:1:verse' }));
  const collapsed = await renderToString(createSSRApp(LearningConfigSection, { title: '背经配置', storageKey: 'group:2:verse' }));
  expect(expanded).toContain('aria-expanded="true"');
  expect(collapsed).toContain('aria-expanded="false"');
});

it('浏览器存储不可用时仍默认收起', async () => {
  vi.stubGlobal('localStorage', { getItem: () => { throw new Error('storage unavailable'); } });
  const html = await renderToString(createSSRApp(LearningConfigSection, { title: '周任务', storageKey: 'weekly' }));
  expect(html).toContain('aria-expanded="false"');
});

it('展开另一配置自动收起前项、保留内容，并只在主动展开时居中定位', async () => {
  const scrollIntoView = vi.fn();
  vi.stubGlobal('window', { matchMedia: () => ({ matches: true }) });
  const activeKey = ref('verse');
  const { descriptor } = parse(readFileSync(new URL('./LearningConfigSection.vue', import.meta.url), 'utf8'));
  const script = compileScript(descriptor, { id: 'learning-section-test' }).content
    .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
  const bindings = {
    computed: vue.computed, inject: vue.inject, nextTick, ref, useId: vue.useId, watch: vue.watch, ChevronDown: {},
  };
  const Section = new Function('bindings', `const { ${Object.keys(bindings).join(', ')} } = bindings;\n${script}`)(bindings);
  Section.render = () => null;
  const renderer = createRenderer({
    insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
    createElement: () => ({ style: {}, scrollIntoView }), createText: () => ({}), createComment: () => ({}),
    parentNode: () => null, nextSibling: () => null,
  });
  const app = renderer.createApp({ render: () => h('div', [
    h(Section, { title: '背经配置', storageKey: 'verse' }),
    h(Section, { title: '周任务', storageKey: 'weekly' }),
  ]) });
  app.provide('learningConfigAccordion', { activeKey, select: key => { activeKey.value = key; } });
  app.mount({});
  try {
    const [verse, weekly] = app._instance.subTree.children.map(node => node.component);
    weekly.setupState.card = { scrollIntoView };
    expect(verse.setupState.open).toBe(true);
    expect(scrollIntoView).not.toHaveBeenCalled();
    await weekly.setupState.toggle();
    await nextTick();
    expect(verse.setupState.open).toBe(false);
    expect(weekly.setupState.open).toBe(true);
    expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'instant' });
    await weekly.setupState.toggle();
    expect(activeKey.value).toBe('');
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  } finally {
    app.unmount();
  }
});
