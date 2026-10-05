import { test, expect, Page, APIRequestContext } from "@playwright/test";

const AUTH_HEADER = 'MediaBrowser Client="playwright", Device="playwright", DeviceId="playwright-cleanup", Version="1.0.0"';

const getToken = async (request: APIRequestContext): Promise<string> => {
  const res = await request.post("/Users/AuthenticateByName", {
    headers: { Authorization: AUTH_HEADER },
    data: { Username: "admin", Pw: "admin" },
  });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).AccessToken;
};

const stopShimPlayback = async (request: APIRequestContext): Promise<void> => {
  const token = await getToken(request);
  const headers = { Authorization: `${AUTH_HEADER}, Token="${token}"` };

  const findShimSession = async () => {
    const res = await request.get("/Sessions", { headers });
    const sessions = await res.json();
    return sessions.find((s: any) => s.Client === "jellyfin-vlc-shim");
  };

  const session = await findShimSession();
  if (!session?.NowPlayingItem) return;

  await request.post(`/Sessions/${session.Id}/Playing/Stop`, { headers });

  // Wait until the shim has reported playback stopped
  await expect
    .poll(async () => (await findShimSession())?.NowPlayingItem ?? null, {
      timeout: 10_000,
    })
    .toBeNull();
};

test.afterEach(async ({ request }) => {
  await stopShimPlayback(request);
});

const navigateToJellyfinVideo = async (
  page: Page,
  name: string
): Promise<void> => {
  await page.getByLabel('Search').click();
  await page.getByPlaceholder("Search").click();
  await page.getByPlaceholder("Search").fill(name);
  await page.getByPlaceholder("Search").press("Enter");
  await expect(async () => {
    await Promise.any([
      page.getByText(name).click(), // jellyfin 10
      page.getByRole("link", { name: name }).nth(1).click(), // jellyfin 10
      page.getByRole("link", { name: name }).click(), // // jellyfin 12
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

  await Promise.any([
    expect(page.getByRole('button', { name: 'tests - jellyfin-vlc-shim admin' })).toBeVisible(), // jellyfin 10
    expect(page.getByText('tests - jellyfin-vlc-shim')).toBeVisible(), // jellyfin 12
  ]);
});

test('Cast is working', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('textbox', { name: 'User' }).click();
  await page.getByRole('textbox', { name: 'User' }).fill('admin');
  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.getByRole('textbox', { name: 'Password' }).fill('admin');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await page.getByRole('button', { name: 'Cast to Device' }).click();
  await Promise.any([
    page.getByRole('button', { name: 'tests - jellyfin-vlc-shim admin' }).click(), // jellyfin 10
    page.getByText('tests - jellyfin-vlc-shim').click(), // jellyfin 12
  ]);

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
  await Promise.any([
    page.getByRole('button', { name: 'tests - jellyfin-vlc-shim admin' }).click(), // jellyfin 10
    page.getByText('tests - jellyfin-vlc-shim').click(), // jellyfin 12
  ]);

  await navigateToJellyfinVideo(page, "mkv_1080_H264_aac");
  await page.getByRole("button", { name: "Play", exact: true }).click();
  await page.getByRole("button", { name: "Pause" }).click();

  await expect(page.getByRole("button", { name: "Play" }).nth(2)).toBeVisible();
});
