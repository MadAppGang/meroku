import { expect, test } from "./support/harness";
import { seed, openPanel, reopen } from "./support/config-editors";

test("Project bus · switching on and off persists after reload", async ({ page, app, envYaml, evidence }) => {
  seed(envYaml, { event_bus: { enabled: false } });
  await app.open();
  await openPanel(page, app, "Amazon EventBridge", "Settings");
  const toggle = page.getByRole("switch", { name: "Use the project bus", exact: true });
  await expect(toggle).not.toBeChecked();
  await toggle.check();
  await expect.poll(() => (envYaml.read().event_bus as any)?.enabled).toBe(true);
  await reopen(page, app, "Amazon EventBridge", "Settings");
  await expect(toggle).toBeChecked();
  await expect(page.getByRole("heading", { name: "Project bus", exact: true })).toBeVisible();
  await evidence("project-bus-enabled");

  await toggle.uncheck();
  await expect.poll(() => (envYaml.read().event_bus as any)?.enabled).toBe(false);
  await reopen(page, app, "Amazon EventBridge", "Settings");
  await expect(toggle).not.toBeChecked();
  expect(envYaml.read().event_bus).toMatchObject({ enabled: false });
  await evidence("project-bus-disabled");
});
