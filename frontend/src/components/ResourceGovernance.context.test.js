import { createPinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { beforeEach, expect, it, vi } from 'vitest';
import ResourceGovernance from './ResourceGovernance.vue';
import { useAppStateStore } from '../stores/appState';
const { api, toast, confirmDialog } = vi.hoisted(() => ({ api: vi.fn(), toast: vi.fn(), confirmDialog: vi.fn() }));
vi.mock('../legacy-app', () => ({ api, toast, loadAdminData: vi.fn(), reloadApp: vi.fn() }));
vi.mock('../ui/dialog', () => ({ confirmDialog, promptDialog: vi.fn() }));
beforeEach(() => { vi.resetAllMocks(); api.mockResolvedValue({}); });
async function withGovernance(check) {
  const pinia = createPinia();
  const app = useAppStateStore(pinia);
  app.user = { id: 1 }; app.currentGroupID = 1;
  const component = { ...ResourceGovernance, async setup(props, context) {
    const state = ResourceGovernance.setup(props, context);
    await check(state, app);
    return state;
  } };
  await renderToString(createSSRApp(component).use(pinia));
}
it('ignores governance data arriving after a group switch and clears old import selections', async () => {
  await withGovernance(async (state, app) => {
    const pending = [];
    api.mockImplementation(path => {
      if (app.currentGroupID === 1) return new Promise(resolve => pending.push({ path, resolve }));
      if (path.endsWith('shared-resources')) return Promise.resolve({ resources: [{ asset_id: 22 }] });
      return Promise.resolve({});
    });
    state.selectedSharedAssetIDs.value = [11];
    await state.openImport({ asset_id: 11 });
    const previous = state.loadGovernance();
    app.currentGroupID = 2;
    await state.loadGovernance();
    for (const { path, resolve } of pending) resolve(path.endsWith('shared-resources') ? { resources: [{ asset_id: 11 }] } : {});
    await previous;
    expect(state.sharedResources.value).toEqual([{ asset_id: 22 }]);
    expect(state.importDialog.value).toBeNull();
    expect(state.selectedSharedAssetIDs.value).toEqual([]);
    await state.confirmImport();
    expect(api.mock.calls.some(([path]) => path === '/admin/resource-imports')).toBe(false);
  });
});
it('keeps only the latest graph filters and clears a failed graph with a local message', async () => {
  await withGovernance(async state => {
    let finish; api.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const old = state.loadGraph();
    state.depthFilter.value = 1;
    api.mockResolvedValue({ nodes: [{ id: 'latest', kind: 'group' }], edges: [] });
    await state.loadGraph();
    finish({ nodes: [{ id: 'old', kind: 'group' }], edges: [] }); await old;
    expect(state.graph.value.nodes[0].id).toBe('latest');
    api.mockRejectedValueOnce(new Error('graph_failed'));
    await state.loadGraph();
    expect(state.graph.value.nodes).toEqual([]);
    expect(toast).toHaveBeenCalledWith('graph_failed');
  });
});
it('does not send an old group removal after a delayed confirmation', async () => {
  await withGovernance(async (state, app) => {
    let confirm; confirmDialog.mockReturnValue(new Promise(resolve => { confirm = resolve; }));
    const operation = state.removeImport({ id: 11, title: 'Old group asset' });
    app.currentGroupID = 2;
    confirm(true); await operation;
    expect(api.mock.calls.some(([, options]) => options?.method === 'DELETE')).toBe(false);
  });
});
it('does not reopen an old share dialog after switching groups', async () => {
  await withGovernance(async (state, app) => {
    let finish; api.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const operation = state.openShare({ id: 11 });
    app.currentGroupID = 2;
    finish({ sharing: { scope: 'private' } }); await operation;
    expect(state.shareDialog.value).toBeNull();
  });
});
