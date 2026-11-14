import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen } from "./support/config-editors";

test("Vaults · deleting a vault disconnects every workload and keeps other vaults", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  const config = seed(envYaml, { vaults: [{ name: "payments" }] });
  for (const [kind, name] of [["workload", "backend"], ["services", "api"], ["scheduled_tasks", "nightly"], ["event_processor_tasks", "mailer"]]) {
    workload(config, kind, name).vaults = [{ name: "payments", access: "read", prefix: "PAY" }];
  }
  seed(envYaml, config);
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  await page.getByRole("textbox", { name: "New vault", exact: true }).fill("shared");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect.poll(() => envYaml.read().vaults).toEqual([{ name: "payments" }, { name: "shared", secrets: [] }]);
  await reopen(page, app, "Vaults", "Vaults");
  await expect(page.getByRole("heading", { name: "shared", exact: true })).toBeVisible();
  await evidence("vaults-added");

  await page.getByRole("button", { name: "Remove vault payments", exact: true }).click();
  await expect.poll(() => envYaml.read().vaults).toEqual([{ name: "shared", secrets: [] }]);
  const targets = [["workload", "backend", "Backend service"], ["services", "api", "api"],
    ["scheduled_tasks", "nightly", "nightly"], ["event_processor_tasks", "mailer", "mailer"]];
  await expect.poll(() => targets.map(([kind, name]) =>
    workload(envYaml.read(), kind, name).vaults ?? [])).toEqual([[], [], [], []]);
  await reopen(page, app, "Vaults", "Vaults");
  await expect(page.getByRole("button", { name: "Remove vault payments", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "shared", exact: true })).toBeVisible();
  await evidence("vaults-deleted-no-references");
  for (const [, , title] of targets) {
    await openPanel(page, app, title, "Vaults");
    await expect(page.getByRole("button", { name: "Disconnect vault payments", exact: true })).toHaveCount(0);
    await evidence("vaults-disconnected-" + title.replaceAll(" ", "-"));
  }
  await openPanel(page, app, "Vaults", "Vaults");
  await page.getByRole("button", { name: "Remove vault shared", exact: true }).click();
  await expect.poll(() => envYaml.read().vaults ?? []).toEqual([]);
  await reopen(page, app, "Vaults", "Vaults");
  await expect(page.getByRole("button", { name: /^Remove vault / })).toHaveCount(0);
  await evidence("vaults-empty");
});
