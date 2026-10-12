import { test, expect, type Page } from "@playwright/test";

test.skip(
  !process.env.TENFOLD_TEST_EMAIL || !process.env.TENFOLD_TEST_PASSWORD,
  "Set the test account environment variables to run live checks.",
);
test.setTimeout(120_000);
async function signIn(page: Page) {
  await page.goto("/sign-in");
  await page
    .getByLabel("Email", { exact: true })
    .fill(process.env.TENFOLD_TEST_EMAIL!);
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.TENFOLD_TEST_PASSWORD!);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).not.toHaveURL(/sign-in/, { timeout: 15_000 });
}

test("signed-out requests cannot read uploads or progress", async ({
  page,
  request,
}) => {
  await page.goto("/uploads");
  await expect(page).toHaveURL(/sign-in/);
  const response = await request.get(
    "/api/uploads/00000000-0000-4000-8000-000000000000/progress",
  );
  expect(response.status()).toBe(401);
});

test("live demo, progress, durable investigation, search, and scorecard", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const directBackend: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith("https://tenfold-api.up.railway.app"))
      directBackend.push(request.url());
  });
  await signIn(page);
  await page.goto("/uploads");
  await page
    .getByRole("button", { name: "Load demo data", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Analyze the good lines" }),
  ).toBeVisible({ timeout: 30_000 });
  await expect(
    page.getByRole("heading", { name: "48 of 48,002 lines could not be read" }),
  ).toBeVisible();
  await expect(
    page.locator(".pipeline-canvas .react-flow__controls"),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Analyze the good lines" }).click();
  await expect(
    page.getByRole("heading", { name: "Analysis complete · 5 incidents" }),
  ).toBeVisible({ timeout: 30_000 });
  await page.getByRole("link", { name: "Open triage", exact: true }).click();
  await expect(page.locator(".incident-card")).toHaveCount(5);
  await page.reload();
  await expect(page.locator(".incident-card")).toHaveCount(5);
  const triageUrl = page.url();
  await page
    .getByRole("link", { name: "Open incident", exact: true })
    .first()
    .click();
  await expect(page.locator(".incident-heading h1")).toContainText(
    "files-sync-cdn.net",
  );
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" }),
  ).toContainText("files-sync-cdn.net");
  await page.getByRole("button", { name: "Assign to me", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Unassign", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Add a note")
    .fill("Live integration check: reviewed the cited transfer events.");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.getByText("Note saved.", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Confirm malicious", exact: true })
    .click();
  // A decision returns to triage. Reopen the incident from Decided to
  // check that everything on it was stored.
  await expect(page).toHaveURL(/\/triage\?/);
  await expect(
    page.getByRole("status").filter({ hasText: "Decision saved" }),
  ).toBeVisible();
  await page.getByRole("tab", { name: /Decided/ }).click();
  await page
    .getByRole("link", { name: "Review incident", exact: true })
    .click();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Unassign", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Live integration check: reviewed the cited transfer events.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(page.locator(".incident-toolbar")).toContainText("confirmed");
  await page.locator("#nav-search").fill("domain:files-sync-cdn.net");
  await page.locator("#nav-search").press("Enter");
  await expect(page.locator(".table-footer")).toContainText(
    "21 of 21 matching events loaded",
  );
  const search = page
    .getByRole("main")
    .getByRole("textbox", { name: "Search events", exact: true });
  await search.fill("no-matching-host.invalid");
  await expect(
    page.getByRole("heading", { name: "No events match these filters." }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await expect(page.locator(".table-footer")).toContainText(
    "200 of 47,954 matching events loaded",
  );
  await page.getByRole("button", { name: "Load more events" }).click();
  await expect(page.locator(".table-footer")).toContainText(
    "400 of 47,954 matching events loaded",
  );
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Scorecard", exact: true })
    .click();
  await expect(
    page.getByText("No second judge ran. Agreement is not available."),
  ).toBeVisible();
  await expect(page.locator(".judge")).toHaveCount(1);
  await page.goto(triageUrl);
  await page
    .getByRole("link", { name: "See anomalies set aside, and why" })
    .click();
  await expect(page.locator("tbody tr")).toHaveCount(41);
  await page.getByRole("combobox").last().selectOption("judged_benign");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.goto(triageUrl);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
  await page.screenshot({
    path: "test-results/live-triage-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page).toHaveURL(/sign-in/);
  await page.goto(triageUrl);
  await expect(page).toHaveURL(/sign-in/);
  expect(errors).toEqual([]);
  expect(directBackend).toEqual([]);
});

test("a .log with an empty browser MIME type uploads, pauses, and cancels", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/uploads");
  const { readFile } = await import("node:fs/promises");
  const file = await readFile("tests/fixtures/upload-check.log");
  const filename = `upload-check-${Date.now()}.log`;
  const storagePut = page.waitForRequest(
    (request) =>
      request.method() === "PUT" &&
      request.url().includes(".supabase.co/storage/"),
  );
  await page
    .getByLabel("Upload log files")
    .setInputFiles({ name: filename, mimeType: "", buffer: file });
  const request = await storagePut;
  expect(request.headers()["content-type"]).toBe("text/plain");
  expect(request.headers()["authorization"]).toBeUndefined();
  await expect(
    page.getByRole("heading", { name: "1 of 2 lines could not be read" }),
  ).toBeVisible({ timeout: 30_000 });
  await page
    .getByRole("button", { name: "Cancel this upload", exact: true })
    .click();
  await expect(page.locator(".recent-uploads")).not.toContainText(filename);
  await page.reload();
  await expect(page.locator(".recent-uploads")).not.toContainText(filename);
});

test("navigation keeps the current page and reuses server-rendered results", async ({
  page,
}) => {
  await signIn(page);
  await page.locator(".triage-hero, .uploads-hero").waitFor();
  test.skip(
    (await page.locator(".incident-card").count()) === 0,
    "Requires an existing completed demo; this check creates no data.",
  );
  const triageUrl = page.url();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let started!: () => void;
  const requested = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route("**/events?**", async (route) => {
    if (
      route.request().method() === "GET" &&
      route.request().headers().rsc === "1"
    ) {
      started();
      await gate;
    }
    await route.continue();
  });
  let clientRequests = 0;
  page.on("request", (request) => {
    if (request.method() === "POST") clientRequests++;
  });
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Events", exact: true })
    .click();
  await requested;
  try {
    await expect(page.locator(".triage-hero")).toBeVisible();
    await expect(
      page.getByRole("navigation", { name: "Main navigation" }),
    ).toBeVisible();
    await expect(
      page.getByText("Loading your investigation…", { exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("status", { name: "Opening page" }),
    ).toBeVisible();
  } finally {
    release();
  }
  await expect(page.locator(".events-heading")).toBeVisible();
  await page.waitForTimeout(800); // Detect an unwanted post-hydration refetch.
  expect(clientRequests).toBe(0);
  await expect(page.locator(".table-footer")).toContainText("200 of");
  let releaseFilter!: () => void;
  const filterGate = new Promise<void>((resolve) => {
    releaseFilter = resolve;
  });
  await page.route("**/events?**", async (route) => {
    if (route.request().method() === "POST") await filterGate;
    await route.continue();
  });
  await page
    .getByRole("main")
    .getByRole("textbox", { name: "Search events", exact: true })
    .fill("no-matching-host.invalid");
  try {
    await expect(page.locator(".skeleton-row").first()).toBeVisible();
    await expect(page.locator(".events-heading")).toBeVisible();
    await expect(page.locator(".explorer-panel")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    await page.screenshot({
      path: "test-results/events-filter-skeleton.png",
      fullPage: true,
    });
  } finally {
    releaseFilter();
  }
  await expect(
    page.getByRole("heading", { name: "No events match these filters." }),
  ).toBeVisible();
  expect(clientRequests).toBe(1);
  await page.goto(triageUrl);
  clientRequests = 0;
  await page
    .getByRole("link", { name: "See anomalies set aside, and why" })
    .click();
  await expect(
    page.getByRole("heading", { name: "What was set aside, and why." }),
  ).toBeVisible();
  await page.waitForTimeout(800);
  expect(clientRequests).toBe(0);
});
