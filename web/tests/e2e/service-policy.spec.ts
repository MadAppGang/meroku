import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, fillPolicy, addPolicy, condition, resource, editedResource, policyStatements, expectChoice } from "./support/config-editors";

test("api policy · adds, edits and removes a statement after reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml);
  const policy = () => policyStatements(envYaml.read(), "services", "api");
  await app.open();
  await openPanel(page, app, "api", "IAM");
  await addPolicy(page);
  await expect.poll(policy).toEqual([{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }]);
  await reopen(page, app, "api", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await expectChoice(page.getByRole("combobox", { name: "Effect", exact: true }), "Deny");
  await evidence("api-policy-added");

  await fillPolicy(page, "Allow", ["dynamodb:Query", "dynamodb:Scan"], [editedResource, resource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query", "dynamodb:Scan"], resources: [editedResource, resource],
  }]);
  await reopen(page, app, "api", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:Scan", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  await evidence("api-policy-edited");

  await page.getByRole("button", { name: "Delete statement 1", exact: true }).click();
  await expect.poll(policy).toEqual([]);
  await reopen(page, app, "api", "IAM");
  await expect(page.getByRole("button", { name: "Edit statement 1", exact: true })).toHaveCount(0);
  await expect(page.getByText("dynamodb:Query", { exact: true })).toHaveCount(0);
  await evidence("api-policy-removed");
});

test("api policy · preserves YAML conditions while editing", async ({ page, app, envYaml, evidence }) => {
  const config = seed(envYaml);
  const entry = workload(config, "services", "api");
  entry.policy = [{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource], conditions: condition }];
  seed(envYaml, config);
  const policy = () => policyStatements(envYaml.read(), "services", "api");
  await app.open();
  await openPanel(page, app, "api", "IAM");
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await fillPolicy(page, "Allow", ["dynamodb:Query"], [editedResource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query"], resources: [editedResource], conditions: condition,
  }]);
  await reopen(page, app, "api", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  expect(policy()[0].conditions).toEqual(condition);
  await evidence("api-policy-conditions-preserved");
});

test("Service policy · switching nodes keeps saved statements and editor drafts separate", async ({ page, app, envYaml, evidence }) => {
  seed(envYaml);
  const api = () => workload(envYaml.read(), "services", "api").policy ?? [];
  const reports = () => workload(envYaml.read(), "services", "reports").policy ?? [];
  await app.open();
  await openPanel(page, app, "api", "IAM");
  await addPolicy(page);
  await expect.poll(api).toHaveLength(1);
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await page.getByRole("textbox", { name: "Actions (one per line)", exact: true }).fill("dynamodb:DeleteItem");

  await openPanel(page, app, "reports", "IAM");
  await expect(page.getByRole("button", { name: "Edit statement 1", exact: true })).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Actions (one per line)", exact: true })).toHaveCount(0);
  await addPolicy(page, ["dynamodb:Query"], [editedResource]);
  await expect.poll(reports).toEqual([{ effect: "Deny", actions: ["dynamodb:Query"], resources: [editedResource] }]);
  await openPanel(page, app, "api", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:Query", { exact: true })).toHaveCount(0);
  await evidence("service-policy-api-isolated");

  await reopen(page, app, "reports", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toHaveCount(0);
  expect(api()).toEqual([{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }]);
  expect(reports()).toEqual([{ effect: "Deny", actions: ["dynamodb:Query"], resources: [editedResource] }]);
  await evidence("service-policy-reports-isolated");
  await openPanel(page, app, "api", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:Query", { exact: true })).toHaveCount(0);
  await evidence("service-policy-api-reloaded");
});
