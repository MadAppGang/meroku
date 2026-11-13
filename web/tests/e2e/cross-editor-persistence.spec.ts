import { expect, test } from "./support/harness";
import { seed, openPanel, reopen, addPolicy, addSubscription, resource } from "./support/config-editors";

test("Backend · vault, policy and Events edits all survive a reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  const initial = seed(envYaml, { vaults: [{ name: "shared" }] });
  await app.open();
  await openPanel(page, app, "Backend service", "Vaults");
  await page.getByRole("button", { name: "Connect a vault", exact: true }).click();
  await page.getByRole("textbox", { name: "Prefix (optional)", exact: true }).fill("PAY");
  await page.getByRole("textbox", { name: "Prefix (optional)", exact: true }).press("Tab");
  const backend = () => envYaml.read().workload as any;
  await expect.poll(() => backend().vaults).toMatchObject([{ name: "shared", prefix: "PAY" }]);
  await evidence("cross-editor-vault-edited");

  await openPanel(page, app, "Backend service", "IAM");
  await addPolicy(page);
  await expect.poll(() => backend().policy).toEqual([{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }]);
  await evidence("cross-editor-policy-edited");
  await openPanel(page, app, "Backend service", "Events");
  await addSubscription(page, "payment.completed");
  await expect.poll(() => backend().events?.subscribes).toMatchObject([{ detail_type: "payment.completed" }]);
  await evidence("cross-editor-events-edited");

  await reopen(page, app, "Backend service", "Vaults");
  await expect(page.getByRole("textbox", { name: "Prefix (optional)", exact: true })).toHaveValue("PAY");
  await evidence("cross-editor-vault-reloaded");
  await openPanel(page, app, "Backend service", "IAM");
  await expect(page.getByText("dynamodb:GetItem", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit statement 1", exact: true })).toBeVisible();
  await evidence("cross-editor-policy-reloaded");
  await openPanel(page, app, "Backend service", "Events");
  await expect(page.getByRole("button", { name: "Remove subscription payment-completed", exact: true })).toBeVisible();
  expect(backend()).toMatchObject({
    ...initial.workload,
    vaults: [{ name: "shared", prefix: "PAY" }],
    policy: [{ effect: "Deny", actions: ["dynamodb:GetItem"], resources: [resource] }],
    events: { subscribes: [{ detail_type: "payment.completed", source: "example.payments" }] },
  });
  expect(envYaml.read().vaults).toEqual([{ name: "shared" }]);
  await evidence("cross-editor-all-reloaded");
});
