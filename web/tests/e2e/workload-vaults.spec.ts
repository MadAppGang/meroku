import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, choose, expectChoice } from "./support/config-editors";

for (const [kind, name, title] of [
  ["workload", "backend", "Backend service"], ["services", "api", "api"],
  ["scheduled_tasks", "nightly", "nightly"], ["event_processor_tasks", "mailer", "mailer"],
]) {
  test(title + " Vaults · connection, prefix, access change and disconnect survive reload", async ({ page, app, envYaml, evidence }) => {
    test.setTimeout(120_000);
    seed(envYaml, { vaults: [{ name: "shared" }] });
    const connections = () => workload(envYaml.read(), kind, name).vaults ?? [];
    await app.open();
    await openPanel(page, app, title, "Vaults");
    await page.getByRole("button", { name: "Connect a vault", exact: true }).click();
    await expect.poll(connections).toMatchObject([{ name: "shared" }]);
    await expect(page.getByRole("textbox", { name: "Prefix (optional)", exact: true })).toHaveValue("");
    expect(connections()[0].prefix ?? "").toBe("");
    await choose(page, page.getByRole("combobox", { name: "Access to vault shared", exact: true }),
      "read_write", "Read and write");
    await expect.poll(connections).toMatchObject([{ name: "shared", access: "read_write" }]);
    await page.getByRole("textbox", { name: "Prefix (optional)", exact: true }).fill("PAY");
    await page.getByRole("textbox", { name: "Prefix (optional)", exact: true }).press("Tab");
    await expect.poll(connections).toEqual([{ name: "shared", access: "read_write", prefix: "PAY" }]);

    await reopen(page, app, title, "Vaults");
    await expect(page.getByRole("textbox", { name: "Prefix (optional)", exact: true })).toHaveValue("PAY");
    await expectChoice(page.getByRole("combobox", { name: "Access to vault shared", exact: true }), "Read and write");
    await evidence(name + "-vaults-connected");
    await choose(page, page.getByRole("combobox", { name: "Access to vault shared", exact: true }), "read", "Read");
    await expect.poll(connections).toEqual([{ name: "shared", access: "read", prefix: "PAY" }]);
    await reopen(page, app, title, "Vaults");
    await expect(page.getByRole("textbox", { name: "Prefix (optional)", exact: true })).toHaveValue("PAY");
    await expectChoice(page.getByRole("combobox", { name: "Access to vault shared", exact: true }), "Read");
    await evidence(name + "-vaults-access-edited");

    await page.getByRole("button", { name: "Disconnect vault shared", exact: true }).click();
    await expect.poll(connections).toEqual([]);
    await reopen(page, app, title, "Vaults");
    await expect(page.getByRole("button", { name: "Disconnect vault shared", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Connect a vault", exact: true })).toBeVisible();
    expect(envYaml.read().vaults).toEqual([{ name: "shared" }]);
    await evidence(name + "-vaults-disconnected");
  });
}
