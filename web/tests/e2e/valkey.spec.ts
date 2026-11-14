import { expect, test } from "./support/harness";
import { seed, openPanel, reopen, choose, expectChoice } from "./support/config-editors";

test("Valkey · enablement, version, storage cap and access survive edits and disablement", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { valkey: { enabled: false, engine_version: "8", max_storage_gb: 1, access: [] } });
  const cache = () => envYaml.read().valkey as any;
  await app.open();
  await openPanel(page, app, "Valkey", "Settings");
  const toggle = page.getByRole("switch", { name: "Enable Valkey", exact: true });
  await expect(toggle).not.toBeChecked();
  await toggle.check();
  await expect.poll(() => cache().enabled).toBe(true);
  await choose(page, page.getByRole("combobox", { name: "Valkey version", exact: true }), "9", "Valkey 9");
  await expect.poll(() => cache().engine_version).toBe("9");
  const storage = page.getByRole("spinbutton", { name: "Storage cap (GB)", exact: true });
  await storage.fill("2");
  await storage.press("Tab");
  await expect.poll(() => cache().max_storage_gb).toBe(2);
  for (const name of ["api", "nightly"]) {
    await page.getByRole("checkbox", { name, exact: true }).check();
    await expect.poll(() => cache().access).toContain(name);
  }
  await expect.poll(() => cache().access.slice().sort()).toEqual(["api", "nightly"]);
  await reopen(page, app, "Valkey", "Settings");
  await expect(toggle).toBeChecked();
  await expectChoice(page.getByRole("combobox", { name: "Valkey version", exact: true }), "Valkey 9");
  await expect(storage).toHaveValue("2");
  await expect(page.getByRole("checkbox", { name: "api", exact: true })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "nightly", exact: true })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "reports", exact: true })).not.toBeChecked();
  await evidence("valkey-enabled-settings");

  await page.getByRole("checkbox", { name: "api", exact: true }).uncheck();
  await expect.poll(() => cache().access).toEqual(["nightly"]);
  await reopen(page, app, "Valkey", "Settings");
  await expect(page.getByRole("checkbox", { name: "api", exact: true })).not.toBeChecked();
  await expect(page.getByRole("checkbox", { name: "nightly", exact: true })).toBeChecked();
  await evidence("valkey-access-edited");

  await toggle.uncheck();
  await expect.poll(() => cache().enabled).toBe(false);
  await reopen(page, app, "Valkey", "Settings");
  await expect(toggle).not.toBeChecked();
  expect(cache().enabled).toBe(false);
  await evidence("valkey-disabled");
});
