import { test, expect, type Page } from "@playwright/test";
test.skip(
  process.env.TENFOLD_FIXTURE_MODE !== "true",
  "Run against an explicit fixture-mode server.",
);
async function loadPreview(page: Page, route: string) {
  await page.goto("/uploads");
  await page
    .getByRole("button", { name: "Load demo data", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "moments crossed the anomaly threshold.",
    }),
  ).toBeVisible();
  if (route.startsWith("/incidents/")) {
    await page
      .getByRole("link", { name: "Open incident", exact: true })
      .first()
      .click();
  } else if (route !== "/triage") {
    const label = {
      "/events": "Events",
      "/scorecard": "Scorecard",
      "/uploads": "Uploads",
    }[route];
    if (!label) throw new Error(`Unsupported preview route: ${route}`);
    await page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: label, exact: true })
      .click();
  }
}
for (const width of [1440, 390]) {
  test(`uploads at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/uploads");
    await expect(
      page.getByRole("heading", { name: "Your investigation starts here." }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Load demo data" }),
    ).toBeVisible();
    await expect(page.locator(".pipeline-canvas")).toHaveCount(0);
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/uploads-${width}.png`,
      fullPage: true,
    });
  });
}

for (const width of [1440, 390]) {
  test(`triage at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await loadPreview(page, "/triage");
    await expect(
      page.getByRole("heading", {
        name: "moments crossed the anomaly threshold.",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Open incident", exact: true }),
    ).toHaveCount(3);
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/triage-${width}.png`,
      fullPage: true,
    });
  });
}
for (const width of [1440, 390]) {
  test(`incident at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await loadPreview(page, "/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b");
    await page
      .getByRole("button", { name: "e1048–e1066", exact: true })
      .click();
    if (width === 390) await page.locator(".evidence-section>summary").click();
    await expect(
      page.getByText("19 events selected", { exact: true }),
    ).toBeVisible();
    await expect(page.locator(".evidence-table tr.selected-row")).toHaveCount(
      19,
    );
    await page
      .getByRole("button", { name: "Assign to me", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Unassign", exact: true }),
    ).toBeVisible();
    if (width === 390) await page.locator(".notes-disclosure>summary").click();
    await page
      .getByLabel("Add a note")
      .fill("Called Dana to verify the transfer.");
    await page.getByRole("button", { name: "Save note", exact: true }).click();
    await expect(
      page.getByText("Called Dana to verify the transfer.", { exact: true }),
    ).toBeVisible();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/incident-${width}.png`,
      fullPage: true,
    });
    // A decision is recorded, then the analyst is back on triage, where
    // the card has moved to Decided.
    await page
      .getByRole("button", { name: "Confirm malicious", exact: true })
      .click();
    await expect(page).toHaveURL(/\/triage\?/);
    await expect(
      page.getByRole("status").filter({ hasText: "Decision saved" }),
    ).toBeVisible();
    await expect(page.locator(".incident-card")).toHaveCount(2);
    await page.getByRole("tab", { name: /Decided/ }).click();
    await expect(page.locator(".incident-card")).toHaveCount(1);
    await expect(page.locator(".verdict-badge")).toHaveText("confirmed");
    await page.screenshot({
      path: `test-results/triage-decided-${width}.png`,
      fullPage: true,
    });
  });
}
for (const width of [1440, 390]) {
  test(`events at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await loadPreview(page, "/events");
    await expect(
      page.getByRole("heading", { name: "Every event, as logged." }),
    ).toBeVisible();
    await page
      .getByRole("main")
      .getByRole("textbox", { name: "Search events", exact: true })
      .fill("id:e1048");
    await expect(
      page.getByRole("button", { name: "Open event e1048" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Open event e1048" }).click();
    await expect(
      page.getByRole("heading", { name: "Event e1048 · all 21 fields" }),
    ).toBeVisible();
    await page
      .getByRole("main")
      .getByRole("textbox", { name: "Search events", exact: true })
      .fill("user:missing");
    await expect(
      page.getByRole("heading", { name: "No events match these filters." }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Clear filters", exact: true })
      .click();
    await page.getByRole("region", { name: /Event results/ }).focus();
    await page.keyboard.press("j");
    await expect(
      page.getByRole("heading", { name: "Event e1049 · all 21 fields" }),
    ).toBeVisible();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/events-${width}.png`,
      fullPage: true,
    });
  });
}
test("wheel scrolls the page without zooming either graph", async ({
  page,
}) => {
  for (const route of [
    "/uploads",
    "/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b",
  ]) {
    await loadPreview(page, route);
    const flow = page.locator(".react-flow");
    await flow.scrollIntoViewIfNeeded();
    const viewport = flow.locator(".react-flow__viewport");
    await expect(viewport).not.toHaveAttribute("style", /scale\(0\)/);
    const before = await viewport.getAttribute("style");
    await flow.hover();
    await page.mouse.wheel(0, 200);
    await expect(viewport).toHaveAttribute("style", before!);
    await flow.scrollIntoViewIfNeeded();
    if (route === "/uploads") {
      await expect(flow.locator(".react-flow__controls")).toHaveCount(0);
      await flow
        .locator(".react-flow__pane")
        .dblclick({ position: { x: 30, y: 30 } });
      await expect(viewport).toHaveAttribute("style", before!);
      const box = await flow.boundingBox();
      await page.mouse.move(box!.x + 30, box!.y + 30);
      await page.mouse.down();
      await page.mouse.move(box!.x + 130, box!.y + 80, { steps: 5 });
      await page.mouse.up();
      await expect(viewport).toHaveAttribute("style", before!);
    } else {
      await flow.getByRole("button", { name: "zoom in" }).click();
      await expect(viewport).not.toHaveAttribute("style", before!);
    }
  }
});
for (const width of [1440, 390]) {
  test(`operational states at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/states");
    await expect(
      page.getByRole("heading", { name: "What an upload can turn into." }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Analyze the good lines" }),
    ).toBeVisible();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/states-${width}.png`,
      fullPage: true,
    });
    await page.getByRole("button", { name: "Try the AI stages again" }).click();
    await expect(
      page
        .getByRole("status")
        .filter({ hasText: "Analysis resumed in this preview." }),
    ).toBeVisible();
  });
}

for (const width of [1440, 390]) {
  test(`teaching dialog at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await loadPreview(page, "/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b");
    await page
      .getByRole("button", { name: "Dismiss as false positive", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(page.getByLabel("The lesson")).toBeFocused();
    await page
      .getByLabel("The lesson")
      .fill("Approved transfer confirmed with Dana.");
    await page.getByLabel("This domain files-sync-cdn.net").check();
    await page.screenshot({
      path: `test-results/teaching-${width}.png`,
      fullPage: true,
    });
    await page
      .getByRole("button", { name: "Dismiss and teach", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({
        hasText: "Incident dismissed and lesson saved in this preview.",
      }),
    ).toBeVisible();
  });
}
for (const width of [1440, 390]) {
  test(`scorecard at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await loadPreview(page, "/scorecard");
    await expect(
      page.getByRole("heading", {
        name: "Demo data, marked against its answer key.",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Judge against judge", exact: true }),
    ).toBeVisible();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/scorecard-${width}.png`,
      fullPage: true,
    });
    await page.getByRole("link", { name: "See the 13 they split on" }).click();
    await expect(
      page.getByRole("heading", { name: "What was set aside, and why." }),
    ).toBeVisible();
    await expect(page.locator("tbody tr")).toHaveCount(13);
  });
}
test("mobile incident starts with AI summary and collapsed investigation panels", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await loadPreview(page, "/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b");
  await expect(page.locator(".evidence-section")).not.toHaveAttribute(
    "open",
    "",
  );
  await expect(page.locator(".incident-graph-section")).not.toHaveAttribute(
    "open",
    "",
  );
  await expect(page.locator(".notes-disclosure")).not.toHaveAttribute(
    "open",
    "",
  );
  await page.screenshot({
    path: "test-results/mobile-incident.png",
    fullPage: true,
  });
  await page.locator(".evidence-section>summary").click();
  await expect(
    page.getByRole("button", { name: "› e1041", exact: true }),
  ).toBeVisible();
});
test("a fresh sample keeps its analyst decision separate from the original upload", async ({
  page,
}) => {
  await loadPreview(page, "/uploads");
  await page
    .getByRole("button", { name: "Load demo data", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "moments crossed the anomaly threshold.",
    }),
  ).toBeVisible();
  await page
    .getByRole("link", { name: "Open incident", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Confirm malicious", exact: true })
    .click();
  await expect(page).toHaveURL(/\/triage\?/);
  await expect(page.locator(".stats")).toContainText("1/3");
  await page.getByRole("link", { name: "Uploads", exact: true }).click();
  const newest = page.locator(".recent-uploads tbody tr").first();
  await expect(newest).toContainText("attack-day.log");
  await expect(newest).toContainText("1 of 3 decided");
  await expect(page.locator(".recent-uploads tbody tr").last()).toContainText(
    "0 of 3 decided",
  );
});

test("home gates empty sessions, then logo returns to triage and incidents use breadcrumbs", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/uploads$/);
  await expect(page.locator("#nav-search")).toBeDisabled();
  await page.getByRole("link", { name: "tenfold", exact: true }).click();
  await expect(page).toHaveURL(/\/uploads$/);
  await page
    .getByRole("button", { name: "Load demo data", exact: true })
    .click();
  await expect(page).toHaveURL(/\/triage\?upload=/);
  const nav = page.getByRole("navigation", { name: "Main navigation" });
  await expect(
    nav.getByRole("link", { name: "Incident", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("link", { name: "Open incident", exact: true })
    .nth(1)
    .click();
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" }),
  ).toContainText("Clockwork requests from WS-114");
  await expect(page.getByText(/Fixture preview/)).toHaveCount(0);
  await page.getByRole("link", { name: "tenfold", exact: true }).click();
  await expect(page).toHaveURL(/\/triage$/);
  await expect(
    page.getByRole("heading", {
      name: "moments crossed the anomaly threshold.",
    }),
  ).toBeVisible();
});
test("global search opens and filters loaded sample evidence", async ({
  page,
}) => {
  await loadPreview(page, "/triage");
  await page.locator("#nav-search").fill("id:e1048");
  await page.locator("#nav-search").press("Enter");
  await expect(page).toHaveURL(/\/events\?.*q=id%3Ae1048/);
  await expect(
    page.getByRole("main").getByRole("textbox", { name: "Search events" }),
  ).toHaveValue("id:e1048");
  await expect(
    page.getByRole("button", { name: "Open event e1048", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Open event e1049", exact: true }),
  ).toHaveCount(0);
});

test("clear data requires confirmation and returns to the empty upload screen", async ({
  page,
}) => {
  await loadPreview(page, "/uploads");
  await page.getByRole("button", { name: "Clear data", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Clear your data?" });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.locator(".recent-uploads tbody tr")).toHaveCount(1);
  await page.getByRole("button", { name: "Clear data", exact: true }).click();
  await dialog.getByLabel("Also delete saved lessons").check();
  await dialog.getByRole("button", { name: "Clear data", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your investigation starts here." }),
  ).toBeVisible();
  await expect(page.locator(".recent-uploads tbody tr")).toHaveCount(0);
  await page.getByRole("link", { name: "tenfold", exact: true }).click();
  await expect(page).toHaveURL(/\/uploads$/);
});
