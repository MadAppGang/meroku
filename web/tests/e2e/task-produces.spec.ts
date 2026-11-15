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
    await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).not.toBeChecked();
    await expect(page.getByTestId(/^produces-row-/).filter({ hasText: "order.created" })).toContainText("order.cancelled");
    await evidence(name + "-produces-added");

    await choose(page, page.getByRole("combobox", { name: "Bus for entry 1", exact: true }), "default", /default/i);
    await expect.poll(() => entries()[0]?.bus).toBe("default");
    await page.getByRole("switch", { name: "Own source only for entry 1", exact: true }).check();
    await expect.poll(() => entries()[0]?.own_source_only).toBeUndefined();
    await reopen(page, app, name, "Events");
    await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).toBeChecked();
    expect(entries()[0]).toMatchObject({
      detail_types: ["order.created", "order.cancelled"], bus: "default",
    });
    expect(entries()[0].own_source_only).toBeUndefined();
    await evidence(name + "-produces-edited");

    // Edit the list in place: add a wildcard type, remove one.
    const addType = page.getByRole("textbox", { name: "Add event type to entry 1", exact: true });
    await addType.fill("invoice.*");
    await addType.press("Enter");
    await expect.poll(() => entries()[0]?.detail_types).toEqual(["order.created", "order.cancelled", "invoice.*"]);
    await page.getByRole("button", { name: "Remove order.cancelled from entry 1", exact: true }).click();
    await expect.poll(() => entries()[0]?.detail_types).toEqual(["order.created", "invoice.*"]);

    await page.getByRole("button", { name: "Remove produces entry 1", exact: true }).click();
    await expect.poll(entries).toEqual([]);
    await reopen(page, app, name, "Events");
    await expect(page.getByRole("button", { name: /^Remove produces entry / })).toHaveCount(0);
    await expect(page.getByText("Sends nothing yet.", { exact: true })).toBeVisible();
    await evidence(name + "-produces-removed");
  });
}
