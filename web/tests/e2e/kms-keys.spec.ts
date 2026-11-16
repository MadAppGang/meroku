import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, choose, expectChoice } from "./support/config-editors";

// Every workload kind, with the access its connection is given.
const targets = [
  ["workload", "backend", "Backend service", "sign", "Sign"],
  ["services", "api", "api", "verify", "Verify"],
  ["scheduled_tasks", "nightly", "nightly", "both", "Sign and verify"],
  ["event_processor_tasks", "mailer", "mailer", "sign", "Sign"],
] as const;

test("KMS keys · add, connect with each access, retire, restore and delete survive reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(180_000);
  seed(envYaml, { kms_keys: [{ name: "key4" }] });
  const keys = () => envYaml.read().kms_keys;
  const connections = () => targets.map(([kind, name]) => workload(envYaml.read(), kind, name).kms_keys ?? []);
  await app.open();

  // Add: the name defaults to key<n> after the highest number, the type to ECC P-256.
  await openPanel(page, app, "KMS keys", "KMS keys");
  const newName = page.getByRole("textbox", { name: "New key name", exact: true });
  await expect(newName).toHaveValue("key5");
  await expectChoice(page.getByRole("combobox", { name: "Type of the new key", exact: true }),
    "ECC P-256 (ES256) — recommended for JWTs");
  await page.getByRole("button", { name: "Add key", exact: true }).click();
  await expect.poll(keys).toEqual([{ name: "key4" }, { name: "key5", key_spec: "ECC_NIST_P256" }]);
  await expect(newName).toHaveValue("key6");

  // A named RSA key; its type stays editable until the panel opens again.
  await newName.fill("Auth");
  await expect(page.getByText("Use 1 to 31 lower-case letters, digits and underscores, starting with a letter.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Add key", exact: true })).toBeDisabled();
  await newName.fill("auth");
  await choose(page, page.getByRole("combobox", { name: "Type of the new key", exact: true }),
    "RSA_2048", "RSA 2048 (RS256) — RS256 compatibility");
  await page.getByRole("button", { name: "Add key", exact: true }).click();
  await expect.poll(keys).toEqual([
    { name: "key4" }, { name: "key5", key_spec: "ECC_NIST_P256" }, { name: "auth", key_spec: "RSA_2048" },
  ]);
  await choose(page, page.getByRole("combobox", { name: "Key type of auth", exact: true }),
    "ECC_NIST_P384", "ECC P-384 (ES384) — when ES384 is required");
  await expect.poll(() => (keys() as any[])[2]).toEqual({ name: "auth", key_spec: "ECC_NIST_P384" });
  await reopen(page, app, "KMS keys", "KMS keys");
  await expect(page.getByRole("combobox", { name: "Key type of auth", exact: true })).toHaveCount(0);
  await expect(page.getByTestId("kms-key-auth").getByText("ES384", { exact: true })).toBeVisible();
  await evidence("kms-keys-added");

  // Connect every workload kind, one access each.
  for (const [kind, name, title, access, label] of targets) {
    await openPanel(page, app, title, "KMS keys");
    await page.getByRole("button", { name: "Connect a key", exact: true }).click();
    await expect.poll(() => workload(envYaml.read(), kind, name).kms_keys).toEqual([{ name: "key4", access: "verify" }]);
    await choose(page, page.getByRole("combobox", { name: "Key of connection key4", exact: true }), "auth", "auth");
    await expect.poll(() => workload(envYaml.read(), kind, name).kms_keys).toEqual([{ name: "auth", access: "verify" }]);
    if (access !== "verify") {
      await choose(page, page.getByRole("combobox", { name: "Access to key auth", exact: true }), access, label);
    }
    await expect.poll(() => workload(envYaml.read(), kind, name).kms_keys).toEqual([{ name: "auth", access }]);
    await expect(page.getByText("KMS_AUTH_KEY_ARN", { exact: true })).toBeVisible();
    await expect(page.getByText("KMS_AUTH_PUBLIC_KEY", { exact: true })).toBeVisible();
  }
  await reopen(page, app, "nightly", "KMS keys");
  await expectChoice(page.getByRole("combobox", { name: "Access to key auth", exact: true }), "Sign and verify");
  await evidence("kms-keys-connected-task");
  await openPanel(page, app, "KMS keys", "KMS keys");
  const auth = page.getByTestId("kms-key-auth");
  await expect(auth.getByText("1 full", { exact: true })).toHaveAttribute("title", "nightly");
  await expect(auth.getByText("2 sign", { exact: true })).toHaveAttribute("title", "backend, mailer");
  await expect(auth.getByText("1 verify", { exact: true })).toHaveAttribute("title", "api");
  await evidence("kms-keys-connected");

  // Retire: every connection goes, the key stays with retired: true.
  await page.getByRole("button", { name: "Retire key auth", exact: true }).click();
  await expect.poll(() => (keys() as any[])[2]).toEqual({ name: "auth", key_spec: "ECC_NIST_P384", retired: true });
  await expect.poll(connections).toEqual([[], [], [], []]);
  await reopen(page, app, "KMS keys", "KMS keys");
  await expect(auth.getByText("Retired", { exact: true })).toBeVisible();
  await expect(auth.getByText("Apply, then delete the key.")).toBeVisible();
  await evidence("kms-keys-retired");
  await openPanel(page, app, "api", "KMS keys");
  await expect(page.getByRole("button", { name: "Disconnect key auth", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Connect a key", exact: true }).click();
  await page.getByRole("combobox", { name: "Key of connection key4", exact: true }).click();
  await expect(page.getByRole("option", { name: "auth", exact: true })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Disconnect key key4", exact: true }).click();
  await expect.poll(() => workload(envYaml.read(), "services", "api").kms_keys ?? []).toEqual([]);

  // Restore, then retire again.
  await openPanel(page, app, "KMS keys", "KMS keys");
  await page.getByRole("button", { name: "Restore key auth", exact: true }).click();
  await expect.poll(() => (keys() as any[])[2]).toEqual({ name: "auth", key_spec: "ECC_NIST_P384" });
  await page.getByRole("button", { name: "Retire key auth", exact: true }).click();
  await expect.poll(() => (keys() as any[])[2]).toEqual({ name: "auth", key_spec: "ECC_NIST_P384", retired: true });

  // Delete: only a retired key, after the 7-day note.
  await expect(page.getByRole("button", { name: "Delete key key4", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Delete key auth", exact: true }).click();
  const dialog = page.getByRole("alertdialog", { name: "Delete key auth?" });
  await expect(dialog.getByText(/stops working at once, and AWS deletes it for good after 7 days/)).toBeVisible();
  await evidence("kms-keys-delete-confirm");
  await dialog.getByRole("button", { name: "Delete key", exact: true }).click();
  await expect.poll(keys).toEqual([{ name: "key4" }, { name: "key5", key_spec: "ECC_NIST_P256" }]);
  await reopen(page, app, "KMS keys", "KMS keys");
  await expect(page.getByTestId("kms-key-auth")).toHaveCount(0);
  await expect(page.getByTestId("kms-key-key5")).toBeVisible();
  await expect(page.getByRole("textbox", { name: "New key name", exact: true })).toHaveValue("key6");
  await evidence("kms-keys-deleted");
});

test("KMS keys · an HMAC key gives only its ARN and grants MAC access", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { kms_keys: [{ name: "auth" }] });
  const keys = () => envYaml.read().kms_keys;
  const api = () => workload(envYaml.read(), "services", "api").kms_keys;
  await app.open();

  // Add an HMAC key: its types follow the signing ones in the same list.
  await openPanel(page, app, "KMS keys", "KMS keys");
  await page.getByRole("textbox", { name: "New key name", exact: true }).fill("requests");
  await choose(page, page.getByRole("combobox", { name: "Type of the new key", exact: true }),
    "HMAC_256", "HMAC-256 (HS256) — shared secret for service-to-service requests");
  await page.getByRole("button", { name: "Add key", exact: true }).click();
  await expect.poll(keys).toEqual([{ name: "auth" }, { name: "requests", key_spec: "HMAC_256" }]);
  await expect(page.getByText(/An HMAC key has no public key/)).toBeVisible();
  await reopen(page, app, "KMS keys", "KMS keys");
  await expect(page.getByTestId("kms-key-requests").getByText("HS256", { exact: true })).toBeVisible();
  await evidence("kms-keys-hmac-added");

  // Connect api to both keys: the HMAC key reads as MAC access and gives only its ARN.
  await openPanel(page, app, "api", "KMS keys");
  await page.getByRole("button", { name: "Connect a key", exact: true }).click();
  await expect.poll(api).toEqual([{ name: "auth", access: "verify" }]);
  await page.getByRole("button", { name: "Connect a key", exact: true }).click();
  await expect.poll(api).toEqual([{ name: "auth", access: "verify" }, { name: "requests", access: "verify" }]);
  await choose(page, page.getByRole("combobox", { name: "Access to key requests", exact: true }), "sign", "Create MAC");
  await expect.poll(api).toEqual([{ name: "auth", access: "verify" }, { name: "requests", access: "sign" }]);
  const requests = page.getByTestId("kms-key-connection-requests");
  await expect(requests.getByText("KMS_REQUESTS_KEY_ARN", { exact: true })).toBeVisible();
  await expect(requests.getByText("KMS_REQUESTS_PUBLIC_KEY", { exact: true })).toHaveCount(0);
  await expect(page.getByTestId("kms-key-connection-auth").getByText("KMS_AUTH_PUBLIC_KEY", { exact: true })).toBeVisible();
  await reopen(page, app, "api", "KMS keys");
  await expectChoice(page.getByRole("combobox", { name: "Access to key requests", exact: true }), "Create MAC");
  await evidence("kms-keys-hmac-connected");
});
