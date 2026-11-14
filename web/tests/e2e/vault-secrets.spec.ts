import type { Page, Request } from "@playwright/test";
import { expect, test } from "./support/harness";
import type { EnvYaml } from "./support/harness";
import { seed, openPanel } from "./support/config-editors";

// The vault secrets API is stubbed: AWS is an in-memory map of parameter
// names and versions. Values are never stored, and no stubbed answer carries
// one, as in the real API. The stub answers as app/api_vault_secrets.go does:
// GET, PUT and DELETE only; 404 for a vault the saved YAML does not declare;
// the same name rules and status codes. Every value here is synthetic.

const SECRET_NAME = /^[a-z][a-z0-9_]{0,62}$/;
const DELETABLE_NAME = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/;
const STILL_LISTED = "still listed in the YAML; connected tasks will fail to start until it is set again or removed and applied";

interface Sent {
  method: string;
  url: URL;
  body: Record<string, unknown> | null;
  /** For a PUT: whether the YAML on disk listed the name when it arrived. */
  declaredOnDisk?: boolean;
}

function vaultsOnDisk(envYaml: EnvYaml): Array<{ name: string; secrets?: string[] }> {
  return (envYaml.read().vaults as Array<{ name: string; secrets?: string[] }>) ?? [];
}

function secretsOnDisk(envYaml: EnvYaml, vault: string): string[] | undefined {
  return vaultsOnDisk(envYaml).find(v => v.name === vault)?.secrets;
}

/**
 * Stubs /api/vaults/secrets over `aws` (vault → parameter name → version).
 * While `deniedPuts` holds a name, AWS refuses its PUT with 403.
 */
async function fakeVaultApi(page: Page, envYaml: EnvYaml, aws: Record<string, Record<string, number>>, deniedPuts = new Set<string>()) {
  const sent: Sent[] = [];
  const valueReads: string[] = [];
  page.on("request", (request: Request) => {
    // The only endpoint that returns a parameter's value; the Vaults tab must never call it.
    if (new URL(request.url()).pathname === "/api/ssm/parameter" && request.method() === "GET") {
      valueReads.push(request.url());
    }
  });
  await page.route("**/api/vaults/secrets**", async route => {
    const request = route.request();
    const url = new URL(request.url());
    const vault = url.searchParams.get("vault") ?? "";
    const body = request.postData() ? JSON.parse(request.postData()!) : null;
    const entry: Sent = { method: request.method(), url, body };
    sent.push(entry);
    const method = request.method();
    if (method !== "GET" && method !== "PUT" && method !== "DELETE") {
      return route.fulfill({ status: 405, json: { error: "method not allowed: use GET, PUT or DELETE" } });
    }
    if (!vaultsOnDisk(envYaml).some(v => v.name === vault)) {
      return route.fulfill({ status: 404, json: { error: `vault "${vault}" is not declared in dev.yaml` } });
    }
    const params = (aws[vault] ??= {});
    const declared = secretsOnDisk(envYaml, vault) ?? [];
    if (method === "PUT") {
      const name = String(body?.name ?? "");
      if (!SECRET_NAME.test(name)) {
        return route.fulfill({ status: 400, json: { error: "invalid secret name: use 1 to 63 lower-case letters, digits and underscores, starting with a letter" } });
      }
      if (!body?.value) return route.fulfill({ status: 400, json: { error: "value is required" } });
      entry.declaredOnDisk = declared.includes(name);
      if (!entry.declaredOnDisk) {
        return route.fulfill({ status: 409, json: { error: `${name} is not in the vault's secrets list: save it to the YAML first` } });
      }
      if (deniedPuts.has(name)) {
        return route.fulfill({ status: 403, json: { error: "User: arn:aws:iam::000000000000:user/example is not authorized to perform: ssm:PutParameter" } });
      }
      params[name] = (params[name] ?? 0) + 1;
      return route.fulfill({ json: { name, version: params[name] } });
    }
    if (method === "DELETE") {
      const name = url.searchParams.get("name") ?? "";
      if (!DELETABLE_NAME.test(name)) {
        return route.fulfill({ status: 400, json: { error: "invalid secret name: use up to 128 letters, digits, underscores, dots and hyphens" } });
      }
      if (!(name in params)) return route.fulfill({ status: 404, json: { error: `${name} has no value in vault "${vault}"` } });
      delete params[name];
      return route.fulfill({ json: { name, deleted: true, ...(declared.includes(name) ? { warning: STILL_LISTED } : {}) } });
    }
    const names = [...new Set([...declared, ...Object.keys(params)])].sort();
    return route.fulfill({
      json: {
        vault,
        path: `/dev/example/vault/${vault}/`,
        secrets: names.map(name => ({
          name,
          envName: name.toUpperCase(),
          declared: declared.includes(name),
          set: name in params,
          ...(name in params ? { type: "SecureString", version: params[name], lastModified: "2026-11-01T10:00:00Z" } : {}),
        })),
      },
    });
  });
  return { sent, valueReads };
}

/** Every password field on the page is empty. */
async function expectNoValueShown(page: Page) {
  for (const field of await page.locator('input[type="password"]').all()) {
    await expect(field).toHaveValue("");
  }
}

function expectWriteOnly(api: { sent: Sent[]; valueReads: string[] }) {
  expect(api.valueReads).toEqual([]);
  for (const s of api.sent) {
    if (s.method === "GET") expect(s.url.searchParams.has("name")).toBe(false);
  }
}

test("Vault secrets · add, replace and remove a secret, warn about one with no value", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, {
    vaults: [{ name: "payments", secrets: ["api_key", "signing_key"] }],
    workload: { ...(envYaml.read().workload as object), vaults: [{ name: "payments", prefix: "PAY" }] },
  });
  const aws = { payments: { api_key: 2, old_token: 1 } };
  const api = await fakeVaultApi(page, envYaml, aws);
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  const card = page.getByTestId("vault-payments");

  await expect(card.getByTestId("vault-secret-api_key")).toContainText("version 2");
  await expect(card.getByTestId("vault-secret-signing_key")).toContainText("No value yet");
  await expect(card.getByRole("button", { name: "Set value of signing_key", exact: true })).toBeVisible();
  await expectNoValueShown(page);
  await evidence("vault-secrets-unset-warning");

  // Add: the name reaches the YAML on disk first, then the value is sent.
  await card.getByLabel("New secret name", { exact: true }).fill("webhook_secret");
  await card.getByLabel("Value", { exact: true }).fill("fake-value-do-not-use");
  await card.getByRole("button", { name: "Add secret to payments", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["api_key", "signing_key", "webhook_secret"]);
  await expect.poll(() => api.sent.filter(s => s.method === "PUT" && s.declaredOnDisk).map(s => s.body))
    .toEqual([{ name: "webhook_secret", value: "fake-value-do-not-use" }]);
  await expect(card.getByTestId("vault-secret-webhook_secret")).toContainText("version 1");
  await expect(card.getByText("Apply to give the connected workloads this change.")).toBeVisible();
  await expect(card.getByLabel("New secret name", { exact: true })).toHaveValue("");
  await expectNoValueShown(page);
  await evidence("vault-secrets-added");

  // Replace: the field starts empty, the new value is sent, the hint says redeploy.
  await card.getByRole("button", { name: "Replace value of api_key", exact: true }).click();
  const field = card.getByLabel("New value for api_key", { exact: true });
  await expect(field).toHaveValue("");
  await expect(field).toHaveAttribute("type", "password");
  await field.fill("fake-rotated-do-not-use");
  await card.getByRole("button", { name: "Save value", exact: true }).click();
  await expect.poll(() => api.sent.filter(s => s.method === "PUT" && s.body?.name === "api_key").map(s => s.body))
    .toEqual([{ name: "api_key", value: "fake-rotated-do-not-use" }]);
  await expect(card.getByTestId("vault-secret-api_key")).toContainText("version 3");
  await expect(card.getByText("Redeploy the connected workloads to pick up the new value.")).toBeVisible();
  await expectNoValueShown(page);
  await evidence("vault-secrets-replaced");

  // Remove: the YAML loses the name, the value stays in AWS.
  await card.getByRole("button", { name: "Remove api_key from the YAML", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["signing_key", "webhook_secret"]);
  await expect(card.getByTestId("vault-secret-api_key")).toContainText("Not in the YAML");
  await expect(card.getByText("Apply, then delete the value, so running tasks never point at a missing secret.")).toBeVisible();
  // Removed but not applied: the row and the dialog say the deployed
  // workloads still use it, and the delete stays offered.
  const removedRow = card.getByTestId("vault-secret-api_key");
  await expect(removedRow).toContainText("Not in the YAML any more, and not applied yet.");
  await expect(removedRow).toContainText("The deployed workloads still use this secret until you apply.");
  await removedRow.getByRole("button", { name: "Delete api_key from AWS", exact: true }).click();
  const removeDialog = page.getByRole("alertdialog", { name: "Delete api_key from AWS?" });
  await expect(removeDialog).toContainText("The deployed workloads still use this secret until you apply.");
  await expect(removeDialog).toContainText("Deleting it now makes their new tasks fail to start. Apply first, then delete.");
  await expect(removeDialog).not.toContainText("no workload gets it");
  await expect(removeDialog.getByRole("button", { name: "Delete from AWS", exact: true })).toBeEnabled();
  await evidence("vault-secrets-removed-delete-warning");
  await removeDialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(removeDialog).toHaveCount(0);
  expect(api.sent.filter(s => s.method === "DELETE")).toEqual([]);
  expect(aws.payments).toHaveProperty("api_key");
  await evidence("vault-secrets-removed");

  expectWriteOnly(api);
});

test("Vault secrets · a value in AWS the YAML does not list: add it to the YAML or delete it", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { vaults: [{ name: "payments", secrets: [] }] });
  const aws = { payments: { old_token: 4, legacy_key: 1 } };
  const api = await fakeVaultApi(page, envYaml, aws);
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  const card = page.getByTestId("vault-payments");

  await expect(card.getByTestId("vault-secret-old_token")).toContainText("Not in the YAML");
  await expect(card.getByTestId("vault-secret-legacy_key")).toContainText("Not in the YAML");
  await evidence("vault-secrets-undeclared");

  await card.getByRole("button", { name: "Add old_token to the YAML", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["old_token"]);
  await expect(card.getByTestId("vault-secret-old_token")).toContainText("version 4");
  await expect(card.getByTestId("vault-secret-old_token")).not.toContainText("Not in the YAML");

  await card.getByRole("button", { name: "Delete legacy_key from AWS", exact: true }).click();
  const dialog = page.getByRole("alertdialog", { name: "Delete legacy_key from AWS?" });
  await expect(dialog).toBeVisible();
  await evidence("vault-secrets-delete-confirm");
  expect(api.sent.filter(s => s.method === "DELETE")).toEqual([]);
  await dialog.getByRole("button", { name: "Delete from AWS", exact: true }).click();
  await expect.poll(() => api.sent.filter(s => s.method === "DELETE").map(s => s.url.searchParams.get("name")))
    .toEqual(["legacy_key"]);
  await expect(card.getByTestId("vault-secret-legacy_key")).toHaveCount(0);
  expect(secretsOnDisk(envYaml, "payments")).toEqual(["old_token"]);
  await evidence("vault-secrets-deleted-from-aws");

  expectWriteOnly(api);
});

test("Vault secrets · a refused add keeps the value; a name already in AWS is refused in the form", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { vaults: [{ name: "payments", secrets: [] }] });
  const aws = { payments: { old_token: 4 } };
  const deniedPuts = new Set(["webhook_secret"]);
  const api = await fakeVaultApi(page, envYaml, aws, deniedPuts);
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  const card = page.getByTestId("vault-payments");
  await expect(card.getByTestId("vault-secret-old_token")).toContainText("Not in the YAML");

  const nameField = card.getByLabel("New secret name", { exact: true });
  const valueField = card.getByLabel("Value", { exact: true });
  const addButton = card.getByRole("button", { name: "Add secret to payments", exact: true });

  // A name AWS already has: the form says to use the row, and sends nothing.
  await nameField.fill("old_token");
  await valueField.fill("fake-value-do-not-use");
  await expect(card.getByText("old_token is already in AWS — use Add to YAML on its row.")).toBeVisible();
  await expect(addButton).toBeDisabled();
  await evidence("vault-secrets-add-name-in-aws");

  // AWS refuses the write: the error shows and both fields keep what was typed.
  await nameField.fill("webhook_secret");
  await addButton.click();
  await expect(card.getByText(/AWS refused the request/)).toBeVisible();
  await expect(nameField).toHaveValue("webhook_secret");
  await expect(valueField).toHaveValue("fake-value-do-not-use");
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["webhook_secret"]);
  await evidence("vault-secrets-add-refused");

  // Sent again from the same fields once AWS allows it; then they clear.
  deniedPuts.clear();
  await expect(addButton).toBeEnabled();
  await addButton.click();
  await expect(card.getByTestId("vault-secret-webhook_secret")).toContainText("version 1");
  await expect(nameField).toHaveValue("");
  await expectNoValueShown(page);
  expect(secretsOnDisk(envYaml, "payments")).toEqual(["webhook_secret"]);
  // A PUT may arrive before the background save and be retried, so only the
  // name is pinned here; the value was written once (version 1).
  expect(new Set(api.sent.filter(s => s.method === "PUT").map(s => s.body?.name))).toEqual(new Set(["webhook_secret"]));
  expect(aws.payments).toEqual({ old_token: 4, webhook_secret: 1 });

  expectWriteOnly(api);
});

test("Vault secrets · a vault without a secrets list imports the names in AWS or starts empty", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { vaults: [{ name: "payments" }, { name: "shared" }] });
  const api = await fakeVaultApi(page, envYaml, { payments: { database_url: 2, api_key: 1 }, shared: {} });
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  const payments = page.getByTestId("vault-payments");
  const shared = page.getByTestId("vault-shared");

  await expect(payments.getByText("meroku needs this vault's secret names")).toBeVisible();
  await expect(payments.getByText("Found in AWS: api_key, database_url.")).toBeVisible();
  await expect(shared.getByRole("button", { name: "Import names from AWS", exact: true })).toBeDisabled();
  await evidence("vault-secrets-import-banner");

  await payments.getByRole("button", { name: "Import names from AWS", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["api_key", "database_url"]);
  await expect(payments.getByTestId("vault-secret-database_url")).toContainText("version 2");

  await shared.getByRole("button", { name: "Start empty", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "shared")).toEqual([]);
  await expect(shared.getByText("This vault has no secrets yet.")).toBeVisible();
  await expect(page.getByText("meroku needs this vault's secret names")).toHaveCount(0);
  await expectNoValueShown(page);
  await evidence("vault-secrets-imported");

  expect(api.sent.filter(s => s.method !== "GET")).toEqual([]);
  expectWriteOnly(api);
});

test("Vault secrets · an upper-case name in AWS is never added to the YAML, only deleted", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { vaults: [{ name: "payments" }] });
  const aws = { payments: { DATABASE_URL: 3, api_key: 1 } };
  const api = await fakeVaultApi(page, envYaml, aws);
  await app.open();
  await openPanel(page, app, "Vaults", "Vaults");
  const card = page.getByTestId("vault-payments");

  // Import takes the lower-case name only, and says why the other stays out.
  await expect(card.getByText("Found in AWS: api_key.")).toBeVisible();
  await expect(card.getByText(/Not imported, because a secret name must be lower case: DATABASE_URL/)).toBeVisible();
  await evidence("vault-secrets-upper-case-import");
  await card.getByRole("button", { name: "Import names from AWS", exact: true }).click();
  await expect.poll(() => secretsOnDisk(envYaml, "payments")).toEqual(["api_key"]);

  // The row explains the rule and offers deletion only.
  const row = card.getByTestId("vault-secret-DATABASE_URL");
  await expect(row).toContainText("a secret name must be lower case");
  await expect(row).toContainText("set it again under a lower-case name");
  await expect(row.getByRole("button", { name: "Add DATABASE_URL to the YAML", exact: true })).toHaveCount(0);
  await expect(row.getByRole("button", { name: /Set value|Replace value|Remove/ })).toHaveCount(0);
  await evidence("vault-secrets-upper-case-row");

  await row.getByRole("button", { name: "Delete DATABASE_URL from AWS", exact: true }).click();
  await page.getByRole("alertdialog", { name: "Delete DATABASE_URL from AWS?" })
    .getByRole("button", { name: "Delete from AWS", exact: true }).click();
  await expect.poll(() => api.sent.filter(s => s.method === "DELETE").map(s => s.url.searchParams.get("name")))
    .toEqual(["DATABASE_URL"]);
  await expect(card.getByTestId("vault-secret-DATABASE_URL")).toHaveCount(0);
  expect(aws.payments).not.toHaveProperty("DATABASE_URL");
  expect(secretsOnDisk(envYaml, "payments")).toEqual(["api_key"]);
  expect(api.sent.filter(s => s.method === "PUT")).toEqual([]);

  expectWriteOnly(api);
});
