import { test, expect } from '@playwright/test';

test('Cast is available', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();
  await page.getByRole('button', { name: 'Cast to Device' }).click();

  await expect(page.getByRole('button', { name: 'My Device admin' })).toBeVisible();
});
