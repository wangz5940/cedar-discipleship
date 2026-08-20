# Unified Resource Downloader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide one authenticated download queue for learning resources and ministry attachments, with batch selection, progress, history, pause/resume, retry, and safe browser fallbacks.

**Architecture:** A Pinia download store owns the queue and limits concurrent transfers. A small runtime module normalizes resource metadata and parses HTTP range headers, while an IndexedDB adapter persists chunks so interrupted same-origin downloads can resume after reload. A global Vue panel exposes queue and history; resource pages only select resources and enqueue normalized descriptors.

**Tech Stack:** Vue 3, Pinia, TypeScript, Fetch Streams, IndexedDB, standard HTTP Range requests, Vitest.

---

## File Map

- Create `frontend/src/runtime/downloads.ts`: pure resource normalization, response parsing, size/progress formatting.
- Create `frontend/src/runtime/downloads.test.ts`: deterministic tests for download helpers.
- Create `frontend/src/runtime/downloadStorage.ts`: IndexedDB chunk persistence with an in-memory compatibility fallback.
- Create `frontend/src/stores/downloadManager.ts`: queue state, bounded concurrency, authenticated fetch, pause/resume/retry, history.
- Create `frontend/src/components/DownloadCenter.vue`: global queue/history UI.
- Modify `frontend/src/App.vue`: mount the global download center.
- Modify `frontend/src/components/AppRoot.vue`: learning-resource single and batch download entry points.
- Modify `frontend/src/components/ContentViewer.vue`: download the currently viewed learning resource.
- Modify `frontend/src/components/MinistryGroups.vue`: ministry-attachment single and batch download entry points.
- Modify `frontend/src/styles.css`: responsive download-center and selection styles.

### Task 1: Resource Contract and HTTP Helpers

- [ ] **Step 1: Write failing helper tests**

Cover:

```ts
normalizeDownloadResource({
  id: 12,
  title: '课程讲义',
  original_name: 'lesson.pdf',
  url: '/api/assets/12/download',
  mime_type: 'application/pdf',
  file_size: 4096,
  source: 'learning',
});

parseContentRange('bytes 1024-2047/4096');
formatDownloadSize(1536);
```

Expected contracts:

```ts
{
  key: 'learning:12',
  name: 'lesson.pdf',
  url: '/api/assets/12/download',
  kind: 'pdf',
  size: 4096,
  source: 'learning',
}
```

- [ ] **Step 2: Run the focused test and confirm it fails**

Run:

```bash
cd frontend && npx vitest run src/runtime/downloads.test.ts
```

Expected: FAIL because `downloads.ts` does not exist.

- [ ] **Step 3: Implement pure helpers**

Implement strict same-origin URL normalization, filename sanitization, MIME/extension classification, `Content-Range` parsing, and human-readable byte/progress formatting.

- [ ] **Step 4: Run focused tests**

Run:

```bash
cd frontend && npx vitest run src/runtime/downloads.test.ts
```

Expected: PASS.

### Task 2: Persistent Download Engine

- [ ] **Step 1: Implement chunk storage**

Use IndexedDB database `agp-downloads-v1` with a `chunks` store keyed by `[taskID, index]`. Expose:

```ts
putChunk(taskID: string, index: number, blob: Blob): Promise<void>
listChunks(taskID: string): Promise<Blob[]>
clearChunks(taskID: string): Promise<void>
chunkSize(taskID: string): Promise<number>
```

If IndexedDB is unavailable, use a process-local `Map`; resume remains available for the current page session.

- [ ] **Step 2: Implement the Pinia queue**

The store must:

```ts
enqueue(resources: DownloadResourceInput[]): void
pause(taskID: string): void
resume(taskID: string): void
cancel(taskID: string): void
retry(taskID: string): void
clearHistory(): void
initialize(): Promise<void>
```

Constraints:

- Maximum two active downloads.
- Maximum 50 resources per batch.
- Maximum managed resource size of 1 GiB.
- Read the latest `agp_token` before each request.
- Send `Range: bytes=<persisted>-` when resuming.
- Append only on HTTP 206; clear stale chunks if a server answers 200.
- Convert in-progress tasks restored after reload to paused.
- Persist task metadata in `localStorage`.
- Check `navigator.storage.estimate()` before starting a known-size transfer.
- Never log tokens or resource response bodies.

- [ ] **Step 3: Verify TypeScript**

Run:

```bash
cd frontend && npm run typecheck
```

Expected: PASS.

### Task 3: Global Download Center

- [ ] **Step 1: Build `DownloadCenter.vue`**

The global component must show:

- A fixed icon button with active-count badge.
- A responsive panel with queue and history tabs.
- File name, source label, size, progress bar, transfer state, and error.
- Pause, continue, retry, cancel, remove, and clear-history controls.
- An explicit message when a server does not support Range or browser storage is insufficient.

- [ ] **Step 2: Mount it in `App.vue`**

Place one instance beside `ContentViewer`, initialize the store on mount, and keep it hidden before authentication.

- [ ] **Step 3: Add responsive styles**

Desktop: compact right-side panel no wider than 420px.

Mobile: bottom sheet constrained to the visual viewport, with controls that do not overflow a 320px screen.

### Task 4: Connect All Resource Entrypoints

- [ ] **Step 1: Connect the learning-resource center**

Add selection checkboxes, “download selected”, “select all”, and a single download icon per resource. Normalize uploaded resources and static resources into the same store input.

- [ ] **Step 2: Connect the content viewer**

Add a download command for the current viewer `sourceURL`, using its original filename/title and media type.

- [ ] **Step 3: Connect ministry attachments**

Replace immediate Blob downloads with queue submission. Add attachment checkboxes and a “download selected attachments” command in the progress view.

- [ ] **Step 4: Preserve preview behavior**

PDF, image, video, audio, and text previews continue to use `openContentTarget`; download commands always use the download store.

### Task 5: Verification

- [ ] **Step 1: Run frontend tests**

```bash
cd frontend && npm test
```

Expected: all tests pass.

- [ ] **Step 2: Run typecheck and production build**

```bash
cd frontend && npm run typecheck
cd frontend && npm run build
```

Expected: both pass.

- [ ] **Step 3: Inspect the final diff**

```bash
git diff --check
git status --short
```

Expected: no whitespace errors and only downloader-related files changed.

- [ ] **Step 4: Commit and push**

```bash
git add docs/superpowers/plans/2026-08-20-unified-resource-downloader.md frontend/src
git commit -m "Add unified resource download manager" -m "Queue authenticated learning resources and ministry attachments with progress, batch selection, persistent history, and resumable range downloads."
git push
```
