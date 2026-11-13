import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen } from "./support/config-editors";

const fixedRule = { source: "/old-pricing", target: "https://example.com/pricing", status: "301" };
const amplifyApp = {
  name: "web", github_repository: "example/web", branches: [{ name: "main" }],
  environment_variables: { AMPLIFY_MONOREPO_APP_ROOT: "apps/web" }, spa_mode: true,
  custom_rules: [fixedRule, { source: "/api/<*>", workload: "api" }],
};

test("Service deletion · removes its shared-folder, cache, vault and Amplify references without changing survivors", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  const config = seed(envYaml, {
    vaults: [{ name: "shared" }],
    data_bucket: { shared: [{ name: "uploads", read_write: ["api", "backend"], read: ["reports"] },
      { name: "exports", read: ["api", "reports"] }] },
    valkey: { enabled: true, engine_version: "8", max_storage_gb: 1, access: ["api", "reports"] },
    amplify_apps: [amplifyApp],
  });
  workload(config, "services", "api").vaults = [{ name: "shared", access: "read_write", prefix: "PAY" }];
  workload(config, "services", "reports").vaults = [{ name: "shared", access: "read" }];
  seed(envYaml, config);
  await app.open();
  await openPanel(page, app, "api", "Vaults");
  await expect(page.getByRole("button", { name: "Disconnect vault shared", exact: true })).toBeVisible();
  await evidence("service-delete-connected");
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await expect.poll(() => (envYaml.read().services as any[]).map(s => s.name)).toEqual(["reports"]);
  await expect.poll(() => (envYaml.read().valkey as any).access).toEqual(["reports"]);
  const shared = () => (envYaml.read().data_bucket as any).shared;
  await expect.poll(() => shared().flatMap((f: any) => [...(f.read ?? []), ...(f.read_write ?? [])])).not.toContain("api");
  expect(shared()[0]).toMatchObject({ name: "uploads", read_write: ["backend"], read: ["reports"] });
  expect(shared()[1]).toMatchObject({ name: "exports", read: ["reports"] });
  expect(workload(envYaml.read(), "services", "reports").vaults).toEqual([{ name: "shared", access: "read" }]);
  // The rule proxying to api is dropped; the fixed-target rule and the rest of the app are kept.
  const { custom_rules: _rules, ...appFields } = amplifyApp;
  await expect.poll(() => (envYaml.read().amplify_apps as any[])[0].custom_rules).toEqual([fixedRule]);
  expect((envYaml.read().amplify_apps as any[])[0]).toMatchObject(appFields);

  await reopen(page, app, "Vaults", "Vaults");
  await expect(page.getByRole("heading", { name: "shared", exact: true })).toBeVisible();
  await expect(app.canvas().getByRole("heading", { name: "api", exact: true })).toHaveCount(0);
  await evidence("service-delete-vault-survives");
  await openPanel(page, app, "Amazon S3", "Data bucket");
  await expect(page.getByRole("combobox", { name: "Access of api to uploads", exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "Access of reports to uploads", exact: true })).toBeVisible();
  await evidence("service-delete-data-access-clean");
  await openPanel(page, app, "Valkey", "Settings");
  await expect(page.getByRole("checkbox", { name: "api", exact: true })).toHaveCount(0);
  await expect(page.getByRole("checkbox", { name: "reports", exact: true })).toBeChecked();
  await evidence("service-delete-valkey-access-clean");
  await reopen(page, app, "web", "Rewrites");
  const rules = page.getByRole("list", { name: "Rewrite rules", exact: true }).getByRole("listitem");
  await expect(rules).toHaveCount(1);
  await expect(rules.first().getByRole("textbox", { name: "Source", exact: true })).toHaveValue("/old-pricing");
  await expect(rules.first().getByRole("textbox", { name: /target/i })).toHaveValue("https://example.com/pricing");
  expect((envYaml.read().amplify_apps as any[])[0]).toMatchObject({ ...appFields, custom_rules: [fixedRule] });
  await evidence("service-delete-amplify-rule-dropped");
});
