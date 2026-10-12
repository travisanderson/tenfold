import { test, expect } from '@playwright/test';
for (const width of [1440, 390]) {
  test(`uploads at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto('/uploads');
    await expect(page.getByRole('heading', { name: 'anomalies judged so far.' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Load demo data' })).toBeVisible();
    await expect(page.getByText('3 Judging', { exact: true })).toBeVisible();
    await expect(page.locator('body')).toHaveJSProperty('scrollWidth', width);
    await page.screenshot({ path: `test-results/uploads-${width}.png`, fullPage: true });
  });
}

for (const width of [1440, 390]) {
 test(`triage at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 1000 });
  await page.goto('/triage');
  await expect(page.getByRole('heading', { name: 'moments crossed the anomaly threshold.' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open incident', exact: true })).toHaveCount(3);
  await expect(page.locator('body')).toHaveJSProperty('scrollWidth', width);
  await page.screenshot({ path: `test-results/triage-${width}.png`, fullPage: true });
 });
}
for (const width of [1440, 390]) {
 test(`incident at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 1000 });
  await page.goto('/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b');
  await page.getByRole('button', { name: 'e1048–e1066', exact: true }).click();
  await expect(page.getByText('19 events selected', {exact: true})).toBeVisible();
  await expect(page.locator('.evidence-table tr.selected-row')).toHaveCount(19);
  await page.getByRole('button', {name:'Assign to me',exact:true}).click();
  await expect(page.getByRole('button', {name:'Unassign',exact:true})).toBeVisible();
  await page.getByLabel('Add a note').fill('Called Dana to verify the transfer.');
  await page.getByRole('button',{name:'Save note',exact:true}).click();
  await expect(page.getByText('Called Dana to verify the transfer.',{exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Confirm malicious',exact:true}).click();
  await expect(page.getByRole('status').filter({hasText:'Analyst decision saved: confirmed.'})).toBeVisible();
  await expect(page.locator('body')).toHaveJSProperty('scrollWidth',width);
  await page.screenshot({path:`test-results/incident-${width}.png`,fullPage:true});
 });
}
for (const width of [1440, 390]) {
 test(`events at ${width}px`, async ({page})=>{
  await page.setViewportSize({width,height:1000});await page.goto('/events');
  await expect(page.getByRole('heading',{name:'Every event, as logged.'})).toBeVisible();
  await page.getByRole('main').getByRole('textbox',{name:'Search events',exact:true}).fill('id:e1048');
  await expect(page.getByRole('button',{name:'Open event e1048'})).toBeVisible();
  await page.getByRole('button',{name:'Open event e1048'}).click();
  await expect(page.getByRole('heading',{name:'Event e1048 · all 21 fields'})).toBeVisible();
  await page.getByRole('main').getByRole('textbox',{name:'Search events',exact:true}).fill('user:missing');
  await expect(page.getByRole('heading',{name:'No events match these filters.'})).toBeVisible();
  await page.getByRole('button',{name:'Clear filters',exact:true}).click();
  await page.getByRole('region',{name:/Event results/}).focus();await page.keyboard.press('j');
  await expect(page.getByRole('heading',{name:'Event e1049 · all 21 fields'})).toBeVisible();
  await expect(page.locator('body')).toHaveJSProperty('scrollWidth',width);
  await page.screenshot({path:`test-results/events-${width}.png`,fullPage:true});
 });
}
test('wheel scrolls the page without zooming either graph',async({page})=>{
 for(const route of ['/uploads','/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b']){
  await page.goto(route);
  const flow=page.locator('.react-flow');await flow.scrollIntoViewIfNeeded();
  const viewport=flow.locator('.react-flow__viewport');
  await expect(viewport).not.toHaveAttribute('style',/scale\(0\)/);
  const before=await viewport.getAttribute('style');await flow.hover();await page.mouse.wheel(0,200);
  await expect(viewport).toHaveAttribute('style',before!);
  await flow.scrollIntoViewIfNeeded();await flow.getByRole('button',{name:'zoom in'}).click();
  await expect(viewport).not.toHaveAttribute('style',before!);
 }
});
for(const width of [1440,390]){
 test(`operational states at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.goto('/states');
 await expect(page.getByRole('heading',{name:'What an upload can turn into.'})).toBeVisible();
 await expect(page.getByRole('button',{name:'Analyse the good lines'})).toBeVisible();
 await expect(page.locator('body')).toHaveJSProperty('scrollWidth',width);
 await page.screenshot({path:`test-results/states-${width}.png`,fullPage:true});
 await page.getByRole('button',{name:'Try the AI stages again'}).click();
 await expect(page.getByRole('status').filter({hasText:'Analysis resumed in this preview.'})).toBeVisible();
 });
}

for(const width of [1440,390]){
 test(`teaching dialog at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.goto('/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b');
 await page.getByRole('button',{name:'Dismiss as false positive',exact:true}).click();
 await expect(page.getByRole('dialog')).toBeVisible();await expect(page.getByLabel('The lesson')).toBeFocused();
 await page.getByLabel('The lesson').fill('Approved transfer confirmed with Dana.');
 await page.getByLabel('This domain files-sync-cdn.net').check();
 await page.screenshot({path:`test-results/teaching-${width}.png`,fullPage:true});
 await page.getByRole('button',{name:'Dismiss and teach',exact:true}).click();
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await expect(page.getByRole('status').filter({hasText:'Incident dismissed and lesson saved in this preview.'})).toBeVisible();
 });
}
