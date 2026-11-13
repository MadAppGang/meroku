import { expect, test } from "./support/harness";
import { seed, openPanel, reopen, choose, expectChoice } from "./support/config-editors";

test("Data bucket · shared folder access and removal survive reload", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(120_000);
  seed(envYaml, { data_bucket: { shared: [] } });
  const folders = () => (envYaml.read().data_bucket as any)?.shared ?? [];
  await app.open();
  await openPanel(page, app, "Amazon S3", "Data bucket");
  await page.getByRole("textbox", { name: "New shared folder", exact: true }).fill("uploads");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect.poll(folders).toMatchObject([{ name: "uploads" }]);
  await choose(page, page.getByRole("combobox", { name: "Access of api to uploads", exact: true }),
    "read_write", "Read and write");
  await expect.poll(folders).toMatchObject([{ name: "uploads", read_write: ["api"] }]);
  await choose(page, page.getByRole("combobox", { name: "Access of reports to uploads", exact: true }),
    "read", "Read");
  await expect.poll(folders).toMatchObject([{ name: "uploads", read_write: ["api"], read: ["reports"] }]);

  await reopen(page, app, "Amazon S3", "Data bucket");
  await expect(page.getByRole("heading", { name: "shared/uploads/", exact: true })).toBeVisible();
  await expectChoice(page.getByRole("combobox", { name: "Access of api to uploads", exact: true }), "Read and write");
  await expectChoice(page.getByRole("combobox", { name: "Access of reports to uploads", exact: true }), "Read");
  expect(folders()).toMatchObject([{ name: "uploads", read_write: ["api"], read: ["reports"] }]);
  await evidence("data-bucket-shared-access");

  await choose(page, page.getByRole("combobox", { name: "Access of api to uploads", exact: true }),
    "read", "Read");
  await expect.poll(() => ({ read: folders()[0]?.read?.slice().sort(), write: folders()[0]?.read_write ?? [] }))
    .toEqual({ read: ["api", "reports"], write: [] });
  await reopen(page, app, "Amazon S3", "Data bucket");
  await expect(page.getByRole("heading", { name: "shared/uploads/", exact: true })).toBeVisible();
  await expectChoice(page.getByRole("combobox", { name: "Access of api to uploads", exact: true }), "Read");
  await evidence("data-bucket-access-edited");

  await page.getByRole("button", { name: "Remove shared folder uploads", exact: true }).click();
  await expect.poll(folders).toEqual([]);
  await reopen(page, app, "Amazon S3", "Data bucket");
  await expect(page.getByRole("heading", { name: "shared/uploads/", exact: true })).toHaveCount(0);
  await evidence("data-bucket-folder-removed");
});
