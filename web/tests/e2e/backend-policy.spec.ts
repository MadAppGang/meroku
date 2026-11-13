import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, fillPolicy, condition, resource, editedResource, policyStatements } from "./support/config-editors";

test("Backend service policy · preserves YAML conditions while editing", async ({ page, app, envYaml, evidence }) => {
  const config = seed(envYaml);
  const entry = workload(config, "workload", "backend");
  entry.policy = [{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource], conditions: condition }];
  seed(envYaml, config);
  const policy = () => policyStatements(envYaml.read(), "workload", "backend");
  await app.open();
  await openPanel(page, app, "Backend service", "IAM");
  await page.getByRole("button", { name: "Edit statement 1", exact: true }).click();
  await fillPolicy(page, "Allow", ["dynamodb:Query"], [editedResource]);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(policy).toEqual([{
    effect: "Allow", actions: ["dynamodb:Query"], resources: [editedResource], conditions: condition,
  }]);
  await reopen(page, app, "Backend service", "IAM");
  await expect(page.getByText("dynamodb:Query", { exact: true })).toBeVisible();
  await expect(page.getByText(editedResource, { exact: true })).toBeVisible();
  expect(policy()[0].conditions).toEqual(condition);
  await evidence("backend-policy-conditions-preserved");
});
