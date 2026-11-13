import { expect, test } from "./support/harness";
import { seed, openPanel, reopen, choose, expectChoice } from "./support/config-editors";

test("Amplify Rewrites · URL and backend rules keep their order, fields and removal after reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(150_000);
  seed(envYaml, { amplify_apps: [{
    name: "web", github_repository: "example/web", branches: [{ name: "main" }],
    environment_variables: { AMPLIFY_MONOREPO_APP_ROOT: "apps/web" }, custom_rules: [],
  }] });
  const rules = () => (envYaml.read().amplify_apps as any[])[0].custom_rules ?? [];
  const list = page.getByRole("list", { name: "Rewrite rules", exact: true });
  const row = (index: number) => list.getByRole("listitem").filter({
    has: page.getByRole("button", { name: "Remove rule " + index, exact: true }),
  });
  await app.open();
  await openPanel(page, app, "web", "Rewrites");
  await page.getByRole("button", { name: "Add rule", exact: true }).click();
  await choose(page, row(1).getByRole("combobox", { name: "Send to", exact: true }), "target", /URL/i);
  await row(1).getByRole("textbox", { name: "Source", exact: true }).fill("/old-pricing");
  await row(1).getByRole("textbox", { name: /target/i }).fill("https://example.com/pricing");
  await choose(page, row(1).getByRole("combobox", { name: "Status", exact: true }), "301", /301/);
  await expect.poll(rules).toMatchObject([{
    source: "/old-pricing", target: "https://example.com/pricing", status: "301",
  }]);
  expect(rules()[0].workload).toBeUndefined();

  await page.getByRole("button", { name: "Add rule", exact: true }).click();
  await choose(page, row(2).getByRole("combobox", { name: "Send to", exact: true }), "workload", "A workload");
  await choose(page, row(2).getByRole("combobox", { name: "Workload", exact: true }), "backend", "backend");
  await row(2).getByRole("textbox", { name: "Source", exact: true }).fill("/api/<*>");
  await row(2).getByRole("textbox", { name: "Path on the workload", exact: true }).fill("/v1/<*>");
  await row(2).getByRole("textbox", { name: "Path on the workload", exact: true }).press("Tab");
  await expect.poll(rules).toMatchObject([
    { source: "/old-pricing", target: "https://example.com/pricing", status: "301" },
    { source: "/api/<*>", workload: "backend", path: "/v1/<*>" },
  ]);
  expect(rules()[1].status ?? "200").toBe("200");
  expect(rules()[1].target).toBeUndefined();

  await reopen(page, app, "web", "Rewrites");
  await expect(list.getByRole("listitem")).toHaveCount(2);
  await expect(row(1).getByRole("textbox", { name: "Source", exact: true })).toHaveValue("/old-pricing");
  await expect(row(1).getByRole("textbox", { name: /target/i })).toHaveValue("https://example.com/pricing");
  await expectChoice(row(1).getByRole("combobox", { name: "Status", exact: true }), /301/);
  await expect(row(2).getByRole("textbox", { name: "Source", exact: true })).toHaveValue("/api/<*>");
  await expect(row(2).getByRole("textbox", { name: "Path on the workload", exact: true })).toHaveValue("/v1/<*>");
  await evidence("amplify-rewrites-added-in-order");

  await choose(page, row(1).getByRole("combobox", { name: "Status", exact: true }), "302", /302/);
  await row(1).getByRole("textbox", { name: /target/i }).fill("https://example.com/new-pricing");
  await row(1).getByRole("textbox", { name: /target/i }).press("Tab");
  await expect.poll(rules).toMatchObject([
    { source: "/old-pricing", target: "https://example.com/new-pricing", status: "302" },
    { source: "/api/<*>", workload: "backend", path: "/v1/<*>" },
  ]);
  await reopen(page, app, "web", "Rewrites");
  await expect(row(1).getByRole("textbox", { name: /target/i })).toHaveValue("https://example.com/new-pricing");
  await expectChoice(row(1).getByRole("combobox", { name: "Status", exact: true }), /302/);
  await evidence("amplify-rewrites-edited");

  await row(1).getByRole("button", { name: "Remove rule 1", exact: true }).click();
  await expect.poll(rules).toHaveLength(1);
  await expect.poll(rules).toMatchObject([{ source: "/api/<*>", workload: "backend", path: "/v1/<*>" }]);
  await reopen(page, app, "web", "Rewrites");
  await expect(list.getByRole("listitem")).toHaveCount(1);
  await expect(row(1).getByRole("textbox", { name: "Source", exact: true })).toHaveValue("/api/<*>");
  expect((envYaml.read().amplify_apps as any[])[0].environment_variables)
    .toEqual({ AMPLIFY_MONOREPO_APP_ROOT: "apps/web" });
  await evidence("amplify-rewrites-removed-first-rule");
});
