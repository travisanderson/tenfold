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
