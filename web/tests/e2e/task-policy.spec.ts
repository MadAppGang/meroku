import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, fillPolicy, addPolicy, condition, resource, editedResource, policyStatements, expectChoice } from "./support/config-editors";

test("nightly policy · adds, edits and removes a statement after reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml);
  const policy = () => policyStatements(envYaml.read(), "scheduled_tasks", "nightly");
  await app.open();
  await openPanel(page, app, "nightly", "IAM");
  await addPolicy(page);
  await expect.poll(policy).toEqual([{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }]);
  await reopen(page, app, "nightly", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await expectChoice(page.getByRole("combobox", { name: "Effect", exact: true }), "Deny");
  await evidence("nightly-policy-added");

  await fillPolicy(page, "Allow", ["dynamodb:Query", "dynamodb:Scan"], [editedResource, resource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query", "dynamodb:Scan"], resources: [editedResource, resource],
  }]);
  await reopen(page, app, "nightly", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:Scan", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  await evidence("nightly-policy-edited");

  await page.getByRole("button", { name: "Delete statement 1", exact: true }).click();
  await expect.poll(policy).toEqual([]);
  await reopen(page, app, "nightly", "IAM");
  await expect(page.getByRole("button", { name: "Edit statement 1", exact: true })).toHaveCount(0);
  await expect(page.getByText("dynamodb:Query", { exact: true })).toHaveCount(0);
  await evidence("nightly-policy-removed");
});

test("mailer policy · adds, edits and removes a statement after reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml);
  const policy = () => policyStatements(envYaml.read(), "event_processor_tasks", "mailer");
  await app.open();
  await openPanel(page, app, "mailer", "IAM");
  await addPolicy(page);
  await expect.poll(policy).toEqual([{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }]);
  await reopen(page, app, "mailer", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await expectChoice(page.getByRole("combobox", { name: "Effect", exact: true }), "Deny");
  await evidence("mailer-policy-added");

  await fillPolicy(page, "Allow", ["dynamodb:Query", "dynamodb:Scan"], [editedResource, resource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query", "dynamodb:Scan"], resources: [editedResource, resource],
  }]);
  await reopen(page, app, "mailer", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText("dynamodb:Scan", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  await evidence("mailer-policy-edited");

  await page.getByRole("button", { name: "Delete statement 1", exact: true }).click();
  await expect.poll(policy).toEqual([]);
  await reopen(page, app, "mailer", "IAM");
  await expect(page.getByRole("button", { name: "Edit statement 1", exact: true })).toHaveCount(0);
  await expect(page.getByText("dynamodb:Query", { exact: true })).toHaveCount(0);
  await evidence("mailer-policy-removed");
});

test("nightly policy · preserves YAML conditions while editing", async ({ page, app, envYaml, evidence }) => {
  const config = seed(envYaml);
  const entry = workload(config, "scheduled_tasks", "nightly");
  entry.policy = [{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource], conditions: condition }];
  seed(envYaml, config);
  const policy = () => policyStatements(envYaml.read(), "scheduled_tasks", "nightly");
  await app.open();
  await openPanel(page, app, "nightly", "IAM");
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await fillPolicy(page, "Allow", ["dynamodb:Query"], [editedResource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query"], resources: [editedResource], conditions: condition,
  }]);
  await reopen(page, app, "nightly", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  expect(policy()[0].conditions).toEqual(condition);
  await evidence("nightly-policy-conditions-preserved");
});

test("mailer policy · preserves YAML conditions while editing", async ({ page, app, envYaml, evidence }) => {
  const config = seed(envYaml);
  const entry = workload(config, "event_processor_tasks", "mailer");
  entry.policy = [{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource], conditions: condition }];
  seed(envYaml, config);
  const policy = () => policyStatements(envYaml.read(), "event_processor_tasks", "mailer");
  await app.open();
  await openPanel(page, app, "mailer", "IAM");
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await fillPolicy(page, "Allow", ["dynamodb:Query"], [editedResource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query"], resources: [editedResource], conditions: condition,
  }]);
  await reopen(page, app, "mailer", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  expect(policy()[0].conditions).toEqual(condition);
  await evidence("mailer-policy-conditions-preserved");
});
