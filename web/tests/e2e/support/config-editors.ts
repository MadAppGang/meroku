import { writeFileSync } from "node:fs";
import yaml from "js-yaml";
import type { Locator, Page } from "@playwright/test";
import { expect } from "./harness";
import type { EnvYaml } from "./harness";
import type { MerokuApp } from "../pages/meroku-app";

// YAML is the public persistence boundary; it is not an application store.
export type Config = Record<string, any>;
export const condition = [
  { test: "StringEquals", variable: "aws:RequestedRegion", values: ["us-east-1"] },
  { test: "StringEquals", variable: "aws:PrincipalTag/team", values: ["example"] },
];
export const resource = "arn:aws:dynamodb:us-east-1:000000000000:table/example";
export const editedResource = "arn:aws:dynamodb:us-east-1:000000000000:table/reports";

export function seed(env: EnvYaml, patch: Config = {}): Config {
  const config: Config = {
    ...env.read(),
    services: ["api", "reports"].map(name => ({
      name, container_port: 8080, cpu: 256, memory: 512, desired_count: 1,
    })),
    scheduled_tasks: [{ name: "nightly", schedule: "rate(1 day)" }],
    event_processor_tasks: [{
      name: "mailer",
      rules: [{ name: "r1", sources: ["example.payments"], detail_types: ["payment.completed"] }],
    }],
    event_bus: { enabled: true },
    ...patch,
  };
  writeFileSync(env.path(), yaml.dump(config), "utf8");
  return config;
}

export function workload(config: Config, kind: string, name?: string): Config {
  return kind === "workload" ? config.workload :
    config[kind].find((entry: Config) => entry.name === name);
}

export async function openPanel(page: Page, app: MerokuApp, title: string, tab: string) {
  // The snapshot contract records dispatchEvent for nodes obscured by canvas edges.
  const node = app.canvas().getByRole("button").filter({
    has: page.getByRole("heading", { level: 3, name: title, exact: true }),
  });
  await expect(node).toHaveCount(1);
  await node.dispatchEvent("click");
  await expect(app.panelTitle(title)).toBeVisible();
  await page.getByRole("button", { name: tab, exact: true }).last().click();
}

export async function reopen(page: Page, app: MerokuApp, title: string, tab: string) {
  await page.reload();
  await app.open("dev");
  await openPanel(page, app, title, tab);
}

async function isNativeSelect(combo: Locator) {
  return (await combo.locator("xpath=self::select").count()) > 0;
}

export async function choose(page: Page, combo: Locator, value: string, label: string | RegExp) {
  if (await isNativeSelect(combo)) {
    const labels = await combo.getByRole("option").allTextContents();
    const match = labels.find(text => typeof label === "string" ? text.trim() === label : label.test(text));
    expect(match, `An option for ${value} must be offered`).toBeDefined();
    await combo.selectOption({ label: match! });
  } else {
    await combo.click();
    await page.getByRole("option", { name: label, exact: typeof label === "string" }).click();
  }
}

export async function expectChoice(combo: Locator, label: string | RegExp) {
  const selected = async () => {
    const text = await (await isNativeSelect(combo) ? combo.locator("option:checked").first() : combo).textContent();
    return text?.trim();
  };
  if (typeof label === "string") await expect.poll(selected).toBe(label);
  else await expect.poll(selected).toMatch(label);
}

export function policyStatements(config: Config, kind: string, name?: string) {
  return (workload(config, kind, name).policy ?? []).map((statement: Config) => ({
    ...statement, effect: statement.effect ?? "Allow",
  }));
}

export async function fillPolicy(page: Page, effect: string, actions: string[], resources: string[]) {
  await choose(page, page.getByRole("combobox", { name: "Effect", exact: true }), effect, effect);
  await page.getByRole("textbox", { name: "Actions (one per line)", exact: true }).fill(actions.join("\n"));
  await page.getByRole("textbox", { name: "Resources (one per line)", exact: true }).fill(resources.join("\n"));
}

export async function addPolicy(page: Page, actions = ["dynamodb:GetItem"], resources = [resource]) {
  await page.getByRole("button", { name: "Add statement", exact: true }).click();
  await fillPolicy(page, "Deny", actions, resources);
  // Two Add statement buttons exist in the captured form: launcher then submit.
  await page.getByRole("button", { name: "Add statement", exact: true }).last().click();
}

export async function addSubscription(page: Page, detail: string, prefix = false, name?: string) {
  if (prefix) {
    await choose(page, page.getByRole("combobox", { name: "Match", exact: true }),
      "prefix", /prefix|starts with/i);
  }
  await page.getByRole("textbox", { name: "New subscription", exact: true }).fill(detail);
  if (name) await page.getByRole("textbox", { name: "Name", exact: true }).fill(name);
  await page.getByRole("textbox", { name: "Source", exact: true }).fill("example.payments");
  if (prefix) {
    await choose(page, page.getByRole("combobox", { name: "Bus for the new subscription", exact: true }),
      "default", /default/i);
  }
  // Snapshot order is Subscribes Add followed by Produces Add.
  await page.getByRole("button", { name: "Add", exact: true }).first().click();
}

export async function addProduces(page: Page, detail = "order.created") {
  await page.getByRole("textbox", { name: "New entry", exact: true }).fill(detail);
  await page.getByRole("button", { name: "Add", exact: true }).last().click();
}
