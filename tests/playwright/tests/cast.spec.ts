import { test, expect, Page } from "@playwright/test";

const navigateToJellyfinVideo = async (
  page: Page,
  name: string
): Promise<void> => {
  await page.getByRole("button", { name: "Search" }).click();
  await page.getByPlaceholder("Search").click();
  await page.getByPlaceholder("Search").fill(name);
  await page.getByPlaceholder("Search").press("Enter");
  await expect(async () => {
    await Promise.any([
      page.getByText(name).click(),
      page.getByRole("link", { name: name }).nth(1).click(),
    ]);
  }).toPass();
};

test('Cast is available', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await page.getByRole('button', { name: 'Cast to Device' }).click();

  await expect(page.getByRole('button', { name: 'My Device admin' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'tests - jellyfin-vlc-shim admin' })).toBeVisible();
});

test('Cast is working', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await page.getByRole('button', { name: 'Cast to Device' }).click();
  await page.getByRole('button', { name: 'tests - jellyfin-vlc-shim admin' }).click();

  await navigateToJellyfinVideo(page, 'mkv_1080_H264_aac');
  await page.getByRole('button', { name: 'Play', exact: true }).click();

  await expect(page.getByRole('button', { name: 'Pause' })).toBeVisible();
});

test("Cast can be paused", async ({ page }) => {
  await page.goto("/");

  await page.getByRole("textbox", { name: "User" }).click();
  await page.getByRole("textbox", { name: "User" }).fill("admin");
  await page.getByRole("textbox", { name: "Password" }).click();
  await page.getByRole("textbox", { name: "Password" }).fill("admin");
  await page.getByRole("button", { name: "Sign In" }).click();

  await page.getByRole("button", { name: "Cast to Device" }).click();
  await page.getByRole("button", { name: "tests - jellyfin-vlc-shim admin" }).click();

  await navigateToJellyfinVideo(page, "mkv_1080_H264_aac");
  await page.getByRole("button", { name: "Play", exact: true }).click();
  await page.getByRole("button", { name: "Pause" }).click();

  await expect(page.getByRole("button", { name: "Play" }).nth(2)).toBeVisible();
});
