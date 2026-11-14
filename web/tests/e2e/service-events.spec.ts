import { expect, test } from "./support/harness";
import { seed, workload, openPanel, reopen, choose, addSubscription, addProduces } from "./support/config-editors";

test("Backend service Events · subscriptions, produces and queue edits persist without losing workload fields", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(150_000);
  const initial = seed(envYaml);
  const before = { ...workload(initial, "workload", "backend") };
  const events = () => workload(envYaml.read(), "workload", "backend").events ?? {};
  await app.open();
  await openPanel(page, app, "Backend service", "Events");
  await addSubscription(page, "payment.completed");
  await expect.poll(() => events().subscribes).toMatchObject([
    { detail_type: "payment.completed", source: "example.payments" },
  ]);
  await addSubscription(page, "user.", true, "users");
  await expect.poll(() => events().subscribes).toMatchObject([
    { detail_type: "payment.completed", source: "example.payments" },
    { name: "users", detail_type_prefix: "user.", source: "example.payments", bus: "default" },
  ]);
  expect(["project", undefined]).toContain(events().subscribes[0].bus);
  expect(events().subscribes[1].detail_type).toBeUndefined();
  await addProduces(page, "order.created, order.cancelled");
  await expect.poll(() => events().produces).toMatchObject([
    { detail_types: ["order.created", "order.cancelled"] },
  ]);
  await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).toBeChecked();
  await page.getByRole("button", { name: "Advanced queue settings", exact: true }).click();
  const visibility = page.getByRole("textbox", { name: "Visibility timeout (seconds)", exact: true });
  await visibility.fill("90");
  await visibility.press("Tab");
  await expect.poll(() => events().queue?.visibility_timeout).toBe(90);
  const receives = page.getByRole("textbox", { name: "Receives before the dead-letter queue", exact: true });
  await receives.fill("7");
  await receives.press("Tab");
  await expect.poll(() => events().queue).toEqual({ visibility_timeout: 90, max_receive_count: 7 });

  await reopen(page, app, "Backend service", "Events");
  await expect(page.getByRole("button", { name: "Remove subscription payment-completed", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove subscription users", exact: true })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "payment.completed" })).toContainText("myapp-dev");
  await expect(page.getByRole("row").filter({ hasText: "user." })).toContainText("default");
  await expect(page.getByRole("row").filter({ hasText: "order.created" })).toContainText("order.cancelled");
  await page.getByRole("button", { name: "Advanced queue settings", exact: true }).click();
  await expect(visibility).toHaveValue("90");
  await expect(receives).toHaveValue("7");
  const after = { ...workload(envYaml.read(), "workload", "backend") };
  delete after.events;
  expect(after).toMatchObject(before);
  await evidence("backend-events-added-and-queue-edited");

  await page.getByRole("switch", { name: "Own source only for entry 1", exact: true }).uncheck();
  await expect.poll(() => events().produces[0].own_source_only).toBe(false);
  await choose(page, page.getByRole("combobox", { name: "Bus for entry 1", exact: true }), "default", /default/i);
  await expect.poll(() => events().produces[0].bus).toBe("default");
  await reopen(page, app, "Backend service", "Events");
  await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).not.toBeChecked();
  await evidence("backend-events-producer-edited");

  await page.getByRole("button", { name: "Remove subscription payment-completed", exact: true }).click();
  await expect.poll(() => events().subscribes).toHaveLength(1);
  await page.getByRole("button", { name: "Remove subscription users", exact: true }).click();
  await expect.poll(() => events().subscribes ?? []).toEqual([]);
  await page.getByRole("button", { name: "Remove produces entry 1", exact: true }).click();
  await expect.poll(() => events().produces ?? []).toEqual([]);
  await reopen(page, app, "Backend service", "Events");
  await expect(page.getByRole("button", { name: /^Remove subscription / })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Remove produces entry / })).toHaveCount(0);
  await expect(page.getByText("Publishes nothing. Add an entry to allow PutEvents.", { exact: true })).toBeVisible();
  expect(workload(envYaml.read(), "workload", "backend")).toMatchObject(before);
  await evidence("backend-events-removed");
});

test("api Events · subscriptions, produces and queue edits persist without losing workload fields", async ({ page, app, envYaml, evidence }) => {
  test.setTimeout(150_000);
  const initial = seed(envYaml);
  const before = { ...workload(initial, "services", "api") };
  const events = () => workload(envYaml.read(), "services", "api").events ?? {};
  await app.open();
  await openPanel(page, app, "api", "Events");
  await addSubscription(page, "payment.completed");
  await expect.poll(() => events().subscribes).toMatchObject([
    { detail_type: "payment.completed", source: "example.payments" },
  ]);
  await addSubscription(page, "user.", true, "users");
  await expect.poll(() => events().subscribes).toMatchObject([
    { detail_type: "payment.completed", source: "example.payments" },
    { name: "users", detail_type_prefix: "user.", source: "example.payments", bus: "default" },
  ]);
  expect(["project", undefined]).toContain(events().subscribes[0].bus);
  expect(events().subscribes[1].detail_type).toBeUndefined();
  await addProduces(page, "order.created, order.cancelled");
  await expect.poll(() => events().produces).toMatchObject([
    { detail_types: ["order.created", "order.cancelled"] },
  ]);
  await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).toBeChecked();
  await page.getByRole("button", { name: "Advanced queue settings", exact: true }).click();
  const visibility = page.getByRole("textbox", { name: "Visibility timeout (seconds)", exact: true });
  await visibility.fill("90");
  await visibility.press("Tab");
  await expect.poll(() => events().queue?.visibility_timeout).toBe(90);
  const receives = page.getByRole("textbox", { name: "Receives before the dead-letter queue", exact: true });
  await receives.fill("7");
  await receives.press("Tab");
  await expect.poll(() => events().queue).toEqual({ visibility_timeout: 90, max_receive_count: 7 });

  await reopen(page, app, "api", "Events");
  await expect(page.getByRole("button", { name: "Remove subscription payment-completed", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove subscription users", exact: true })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "payment.completed" })).toContainText("myapp-dev");
  await expect(page.getByRole("row").filter({ hasText: "user." })).toContainText("default");
  await expect(page.getByRole("row").filter({ hasText: "order.created" })).toContainText("order.cancelled");
  await page.getByRole("button", { name: "Advanced queue settings", exact: true }).click();
  await expect(visibility).toHaveValue("90");
  await expect(receives).toHaveValue("7");
  const after = { ...workload(envYaml.read(), "services", "api") };
  delete after.events;
  expect(after).toMatchObject(before);
  await evidence("api-events-added-and-queue-edited");

  await page.getByRole("switch", { name: "Own source only for entry 1", exact: true }).uncheck();
  await expect.poll(() => events().produces[0].own_source_only).toBe(false);
  await choose(page, page.getByRole("combobox", { name: "Bus for entry 1", exact: true }), "default", /default/i);
  await expect.poll(() => events().produces[0].bus).toBe("default");
  await reopen(page, app, "api", "Events");
  await expect(page.getByRole("switch", { name: "Own source only for entry 1", exact: true })).not.toBeChecked();
  await evidence("api-events-producer-edited");

  await page.getByRole("button", { name: "Remove subscription payment-completed", exact: true }).click();
  await expect.poll(() => events().subscribes).toHaveLength(1);
  await page.getByRole("button", { name: "Remove subscription users", exact: true }).click();
  await expect.poll(() => events().subscribes ?? []).toEqual([]);
  await page.getByRole("button", { name: "Remove produces entry 1", exact: true }).click();
  await expect.poll(() => events().produces ?? []).toEqual([]);
  await reopen(page, app, "api", "Events");
  await expect(page.getByRole("button", { name: /^Remove subscription / })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Remove produces entry / })).toHaveCount(0);
  await expect(page.getByText("Publishes nothing. Add an entry to allow PutEvents.", { exact: true })).toBeVisible();
  expect(workload(envYaml.read(), "services", "api")).toMatchObject(before);
  await evidence("api-events-removed");
});

test("Events · changes on two services stay separate after reload", async ({ page, app, envYaml, evidence }) => {
  seed(envYaml);
  const api = () => workload(envYaml.read(), "services", "api").events;
  const reports = () => workload(envYaml.read(), "services", "reports").events;
  await app.open();
  await openPanel(page, app, "api", "Events");
  await addSubscription(page, "payment.completed");
  await expect.poll(() => api()?.subscribes).toMatchObject([{ detail_type: "payment.completed" }]);
  await addProduces(page, "order.created");
  await expect.poll(() => api()?.produces).toMatchObject([{ detail_types: ["order.created"] }]);
  await openPanel(page, app, "reports", "Events");
  await expect(page.getByRole("button", { name: /^Remove subscription / })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Remove produces entry / })).toHaveCount(0);
  await addSubscription(page, "report.requested");
  await expect.poll(() => reports()?.subscribes).toMatchObject([{ detail_type: "report.requested" }]);
  await addProduces(page, "report.ready");
  await expect.poll(() => reports()?.produces).toMatchObject([{ detail_types: ["report.ready"] }]);

  await reopen(page, app, "api", "Events");
  await expect(page.getByRole("button", { name: "Remove subscription payment-completed", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove subscription report-requested", exact: true })).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "order.created", exact: true })).toBeVisible();
  await evidence("events-api-isolated");
  await openPanel(page, app, "reports", "Events");
  await expect(page.getByRole("button", { name: "Remove subscription report-requested", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove subscription payment-completed", exact: true })).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "report.ready", exact: true })).toBeVisible();
  expect(api().subscribes).toHaveLength(1);
  expect(reports().subscribes).toHaveLength(1);
  expect(api().produces).toMatchObject([{ detail_types: ["order.created"] }]);
  expect(reports().produces).toMatchObject([{ detail_types: ["report.ready"] }]);
  await evidence("events-reports-isolated");
});
