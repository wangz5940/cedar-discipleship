// With Vite on port 5176:
// playwright-cli run-code --filename=tests/pdf-desktop-scroll.browser.js
async (page) => {
  const context = await page.context().browser().newContext({
    viewport: { width: 1200, height: 900 },
  });
  const test = await context.newPage();
  const results = [];

  for (const viewport of [
    { name: 'desktop', width: 1200, height: 900 },
    { name: 'mobile', width: 390, height: 844 },
  ]) {
    await test.setViewportSize({ width: viewport.width, height: viewport.height });
    for (const mode of ['plain', 'sidebar']) {
      const suffix = mode === 'sidebar' ? '?sidebar=1' : '';
      await test.goto(`http://127.0.0.1:5176/tests/fixtures/pdf-desktop-scroll.html${suffix}`);
      await test.locator('.pdf-viewer canvas').waitFor();

      const before = await test.evaluate(() => {
        const body = document.querySelector('.viewer-body');
        const main = document.querySelector('.viewer-main-pdf');
        const stage = document.querySelector('.pdf-viewer-stage');
        const toolbar = document.querySelector('.viewer-main-toolbar');
        const viewer = document.querySelector('.pdf-viewer');
        const canvas = document.querySelector('.pdf-viewer canvas');
        return {
          bodyHeight: body.clientHeight,
          mainHeight: main.clientHeight,
          mainWidth: main.clientWidth,
          stageHeight: stage.clientHeight,
          stageScrollHeight: stage.scrollHeight,
          stageBottom: Math.round(stage.getBoundingClientRect().bottom),
          viewerTop: Math.round(viewer.getBoundingClientRect().top),
          viewerWidth: viewer.clientWidth,
          toolbarBottom: toolbar ? Math.round(toolbar.getBoundingClientRect().bottom) : 0,
          canvasBottom: Math.round(canvas.getBoundingClientRect().bottom),
          bodyBottom: Math.round(body.getBoundingClientRect().bottom),
          pageScrollWidth: document.documentElement.scrollWidth,
        };
      });

      await test.locator('.pdf-viewer-stage').evaluate((stage) => {
        stage.scrollTop = 120;
      });
      const scrollTop = await test.locator('.pdf-viewer-stage').evaluate((stage) => stage.scrollTop);
      const contentFits = before.canvasBottom <= before.stageBottom;

      if (before.mainHeight > before.bodyHeight + 2
        || (viewport.name === 'desktop' && contentFits)
        || (!contentFits && scrollTop === 0)
        || before.viewerWidth < before.mainWidth - 2
        || (before.toolbarBottom && before.viewerTop < before.toolbarBottom - 2)
        || before.pageScrollWidth > viewport.width) {
        throw new Error(`PDF layout failed (${viewport.name}/${mode}): ${JSON.stringify({ before, scrollTop })}`);
      }
      results.push({ viewport: viewport.name, mode, before, scrollTop });
    }
  }

  await page.evaluate((value) => { document.body.innerText = JSON.stringify(value); }, results);
  await context.close();
}
