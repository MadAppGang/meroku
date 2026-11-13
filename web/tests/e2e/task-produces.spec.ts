import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, choose, addProduces } from "./support/config-editors";

for (const [kind, name] of [["scheduled_tasks", "nightly"], ["event_processor_tasks", "mailer"]]) {
  test(name + " Events · adding, editing and removing produces entries survives reload", async ({ page, app, envYaml, evidence }) => {
    test.setTimeout(120_000);
    seed(envYaml);
    const entries = () => workload(envYaml.read(), kind, name).events?.produces ?? [];
    await app.open();
    await openPanel(page, app, name, "Events");
    await addProduces(page, "order.created, order.cancelled");
    await expect.poll(entries).toMatchObject([{ detail_types: ["order.created", "order.cancelled"] }]);
    await reopen(page, app, name, "Events");
    await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).toBeChecked();
    await expect(page.getByRole("row").filter({ hasText: "order.created" })).toContainText("order.cancelled");
    await evidence(name + "-produces-added");

    await choose(page, page.getByRole("combobox", { name: "Bus for entry 1", exact: true }), "default", /default/i);
    await expect.poll(() => entries()[0]?.bus).toBe("default");
    await page.getByRole("switch", { name: "Own source only for entry 1", exact: true }).uncheck();
    await expect.poll(() => entries()[0]?.own_source_only).toBe(false);
    await reopen(page, app, name, "Events");
    await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).not.toBeChecked();
    expect(entries()[0]).toMatchObject({
      detail_types: ["order.created", "order.cancelled"], bus: "default", own_source_only: false,
    });
    await evidence(name + "-produces-edited");

    await page.getByRole("button", { name: "Remove produces entry 1", exact: true }).click();
    await expect.poll(entries).toEqual([]);
    await reopen(page, app, name, "Events");
    await expect(page.getByRole("button", { name: /^Remove produces entry / })).toHaveCount(0);
    await expect(page.getByText("Publishes nothing. Add an entry to allow PutEvents.", { exact: true })).toBeVisible();
    await evidence(name + "-produces-removed");
  });
}
