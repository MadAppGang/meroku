import type { Page, Request } from "@playwright/test";
import { expect, test } from "./support/harness";
import type { EnvYaml } from "./support/harness";
import { seed, openPanel } from "./support/config-editors";

// The vault secrets API is stubbed: AWS is an in-memory map of parameter
// names and versions. Values are never stored, and no stubbed answer carries
// one, as in the real API. Every value here is synthetic.

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

/** Stubs /api/vaults/secrets over `aws` (vault → parameter name → version). */
async function fakeVaultApi(page: Page, envYaml: EnvYaml, aws: Record<string, Record<string, number>>) {
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
    const params = (aws[vault] ??= {});
    const declared = secretsOnDisk(envYaml, vault) ?? [];
    if (request.method() === "PUT") {
      const name = String(body.name);
      entry.declaredOnDisk = declared.includes(name);
      if (!entry.declaredOnDisk) {
        return route.fulfill({ status: 409, json: { error: `${name} is not in the vault's secrets list: save it to the YAML first` } });
      }
      params[name] = (params[name] ?? 0) + 1;
      return route.fulfill({ json: { name, version: params[name] } });
    }
    if (request.method() === "DELETE") {
      const name = url.searchParams.get("name") ?? "";
      if (!(name in params)) return route.fulfill({ status: 404, json: { error: `${name} not found` } });
      delete params[name];
      return route.fulfill({ json: { name, deleted: true } });
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
