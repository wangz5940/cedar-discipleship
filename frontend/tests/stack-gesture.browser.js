// With Vite on port 5174: playwright-cli run-code --filename=frontend/tests/stack-gesture.browser.js
async (page) => {
  const context = await page.context().browser().newContext({ viewport: {width:390,height:844}, isMobile:true, hasTouch:true });
  const test = await context.newPage();
  await test.goto('http://127.0.0.1:5174/tests/fixtures/stack-gesture.html');
  await test.locator('.stacked-wheel__card.active').waitFor();
  const cdp = await context.newCDPSession(test);
  await test.evaluate(()=>{ window.events=[]; for(const type of ['pointerdown','pointermove','pointerup','pointercancel']) document.addEventListener(type,e=>window.events.push({type,x:e.clientX,y:e.clientY,target:e.target.className,primary:e.isPrimary}),true); });
  async function swipe(x,y,dy,steps=12) {
    await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x,y}]});
    for(let i=1;i<=steps;i++) {
      await cdp.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x,y:y+dy*i/steps}]});
      await test.waitForTimeout(16);
    }
    await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
    await test.waitForTimeout(300);
  }
  const report=[];
  for(const width of [360,390,430]) {
    await test.setViewportSize({width,height:844});
    await test.evaluate(()=>window.scrollTo(0,0));
    const box=await test.locator('.stacked-wheel__card.active').boundingBox();
    const before=await test.locator('.stacked-wheel__controls b').innerText();
    await swipe(box.x+box.width/2,box.y+box.height-15,-150);
    const after=await test.locator('.stacked-wheel__controls b').innerText();
    const top=await test.evaluate(()=>window.scrollY);
    if(before===after || top>2) throw Error('Card gesture failed '+JSON.stringify({width,before,after,top,events:await test.evaluate(()=>window.events),box}));
    await test.getByRole('button',{name:'Action',exact:true}).click();
    const clicks=await test.evaluate(()=>window.clicks);
    if(clicks!==report.length+1) throw Error('Action swallowed after drag');
    // A gesture beginning outside the cards continues as page scroll across them.
    const selected=await test.locator('.stacked-wheel__controls b').innerText();
    await swipe(width/2,650,-500);
    if(await test.locator('.stacked-wheel__controls b').innerText()!==selected) throw Error('Page swipe changed card');
    if(await test.evaluate(()=>window.scrollY)<50) throw Error('Page swipe from header blocked');
    await swipe(8,720,-550);
    const scroll=await test.evaluate(()=>window.scrollY);
    if(scroll<100) throw Error('Page gutter swipe blocked');
    // Continue native swipes to the footer, including across the stacked stage.
    for(let n=0;n<4;n++) await swipe(8,720,-550);
    await test.waitForTimeout(300);
    const bottom=await test.evaluate(()=>({y:scrollY,max:document.documentElement.scrollHeight-innerHeight,
      overflow:document.documentElement.scrollWidth>innerWidth,
      roots:['body','#app'].map(s=>getComputedStyle(document.querySelector(s)).overflowY)}));
    if(Math.abs(bottom.y-bottom.max)>3 || bottom.overflow) throw Error('Page end failed '+JSON.stringify(bottom));
    report.push({width,before,after,scroll,bottom});
  }
  await page.evaluate(report=>document.body.innerText=JSON.stringify(report),report);
  await context.close();
}
