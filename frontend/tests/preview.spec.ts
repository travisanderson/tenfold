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
