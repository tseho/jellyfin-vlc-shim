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
  await expect(page.getByRole('button', { name: 'TV - jellyfin-vlc-shim admin' })).toBeVisible();
});

test('Cast is working', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await page.getByRole('button', { name: 'Cast to Device' }).click();
  await page.getByRole('button', { name: 'TV - jellyfin-vlc-shim admin' }).click();
  await page.getByTitle('Videos').click();
  await page.waitForURL('**/#/list.html**');
  await page.getByText('mp4_1080_H265_10s_5MB').click();
  await page.getByRole('link', { name: 'Big_Buck_Bunny_1080_H265_10s_5MB' }).nth(1).waitFor({ state: 'visible' });
  await page.getByRole('link', { name: 'Big_Buck_Bunny_1080_H265_10s_5MB' }).nth(1).click();
  await page.getByRole('button', { name: 'Play', exact: true }).click();

  await expect(page.getByRole('button', { name: 'Pause' })).toBeVisible();
});

test('Cast can be paused', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await page.getByRole('button', { name: 'Cast to Device' }).click();
  await page.getByRole('button', { name: 'TV - jellyfin-vlc-shim admin' }).click();
  await page.getByTitle('Videos').click();
  await page.waitForURL('**/#/list.html**');
  await page.getByText('mp4_1080_H265_10s_5MB').click();
  await page.getByRole('link', { name: 'Big_Buck_Bunny_1080_H265_10s_5MB' }).nth(1).waitFor({ state: 'visible' });
  await page.getByRole('link', { name: 'Big_Buck_Bunny_1080_H265_10s_5MB' }).nth(1).click();
  await page.getByRole('button', { name: 'Play', exact: true }).click();
  await page.getByRole('button', { name: 'Pause' }).click();

  await expect(page.getByRole('button', { name: 'Play' }).nth(2)).toBeVisible();
});
