import { expect, test } from "./support/harness";
import { seed, workload, openPanel } from "./support/config-editors";

test("Backend env vars · names generate refuses are flagged inline and cannot be added", async ({ page, app, envYaml, evidence }) => {
  const config = seed(envYaml);
  workload(config, "workload").backend_env_variables = { LOG_LEVEL: "info", AWS_S3_BUCKET: "legacy-bucket" };
  seed(envYaml, config);
  await app.open();
  await openPanel(page, app, "Backend service", "Env Vars");

  // A YAML value that generate now refuses is flagged on its row.
  await expect(page.getByRole("alert").filter({ hasText: "AWS_S3_BUCKET is reserved" })).toBeVisible();
  await evidence("backend-env-reserved-existing");

  const name = page.getByRole("textbox", { name: "Variable Name", exact: true });
  const add = page.getByRole("button", { name: "Add Variable", exact: true });
  await page.getByRole("textbox", { name: "Value", exact: true }).fill("x");

  for (const reserved of ["ENV", "AWS_S3_BUCKET", "DATA_BUCKET_SHARED_UPLOADS"]) {
    await name.fill(reserved);
    await expect(name).toHaveAttribute("aria-invalid", "true");
    await expect(add).toBeDisabled();
  }
  await expect(page.locator(`[id="${await name.getAttribute("aria-describedby")}"]`))
    .toContainText("DATA_BUCKET_SHARED_* is reserved");
  await evidence("backend-env-reserved-new-name");

  await name.fill("FEATURE_FLAG");
  await expect(name).toHaveAttribute("aria-invalid", "false");
  await add.click();
  await expect.poll(() => workload(envYaml.read(), "workload").backend_env_variables)
    .toEqual({ LOG_LEVEL: "info", AWS_S3_BUCKET: "legacy-bucket", FEATURE_FLAG: "x" });
});
