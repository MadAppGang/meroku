import { writeFileSync } from "node:fs";
import type { Page } from "@playwright/test";
import yaml from "js-yaml";
import { expect, test } from "./support/harness";
import type { EnvYaml } from "./support/harness";
import { fillPolicy, openPanel } from "./support/config-editors";

const serviceName = "magento-bridge";
const serviceNodeID = `rf__node-service-${serviceName}`;
const healthWarningTitle =
	"service 'magento-bridge' is network_access: internal and has no health_check_command";
const healthWarningDocs =
	"https://github.com/MadAppGang/meroku/blob/main/ai_docs/SERVICE_NETWORK_ACCESS.md#check-that-the-tool-is-in-the-image";

function internalService(extra: Record<string, unknown> = {}) {
	return {
		name: serviceName,
		network_access: "internal",
		container_port: 8080,
		cpu: 256,
		memory: 512,
		desired_count: 1,
		...extra,
	};
}

function writeConfig(env: EnvYaml, patch: Record<string, unknown>) {
	writeFileSync(env.path(), yaml.dump({ ...env.read(), ...patch }), "utf8");
}

async function fetchWarnings(page: Page) {
	const response = await page.request.get("/api/config-warnings?env=dev");
	expect(response.status()).toBe(200);
	return response.json();
}

// REQ-FR-4, REQ-FR-5, AC-1: API data reaches the real canvas and sidebar.
test("Config warnings · internal service · API, orange node and sidebar details agree", async ({
	page,
	app,
	envYaml,
}) => {
	writeConfig(envYaml, { services: [internalService()] });
	const report = await fetchWarnings(page);
	expect(report).toMatchObject({
		env: "dev",
		warnings: [{
			code: "service_no_health_check",
			item: { kind: "service", name: serviceName },
			title: healthWarningTitle,
			docs: healthWarningDocs,
		}],
	});
	expect(report.warnings).toHaveLength(1);
	expect(report.warnings[0].detail).toContain("Add a health_check_command");

	await app.open("dev");
	const node = page.getByTestId(serviceNodeID);
	await expect(node).toHaveAttribute("data-warning", "true");
	await expect(node.getByTestId("node-warning-badge")).toContainText("1");

	await app.openNode(serviceName);
	const sidebar = page.getByRole("region", { name: "Configuration warnings" });
	await expect(sidebar).toBeVisible();
	await expect(sidebar.getByText(healthWarningTitle, { exact: true })).toBeVisible();
	await expect(sidebar.getByText(/Add a health_check_command/)).toBeVisible();
	const docs = sidebar.getByRole("link", { name: /how to fix this/i });
	await expect(docs).toHaveAttribute("href", healthWarningDocs);
	await expect(docs).toHaveAttribute("target", "_blank");
	await expect(docs).toHaveAttribute("rel", "noreferrer");
});

// REQ-FR-5, REQ-FR-6, AC-4: clean configuration has no warning chrome or web deploy.
test("Config warnings · clean YAML · no node state, environment panel, status control or deploy", async ({
	page,
	app,
}) => {
	const report = await fetchWarnings(page);
	expect(report).toEqual({ env: "dev", warnings: [] });

	await app.open("dev");
	await expect(page.locator("[data-warning]")).toHaveCount(0);
	await expect(page.getByTestId("env-warnings")).toHaveCount(0);
	await expect(page.getByTestId("config-warnings-status")).toHaveCount(0);
	await expect(page.getByRole("button", { name: /^deploy(?:\s|$)/i })).toHaveCount(0);
});

// REQ-FR-5; contract 8.5: counts do not rely on colour and the status lists every warning.
test("Config warnings · two rules on one service · badge count and status list show both", async ({
	page,
	app,
	envYaml,
}) => {
	writeConfig(envYaml, {
		services: [internalService({
			policy: [{ actions: ["*"], resources: ["*"] }],
		})],
	});
	const report = await fetchWarnings(page);
	expect(report.warnings.map((warning: { code: string }) => warning.code).sort()).toEqual([
		"policy_wildcard_action",
		"service_no_health_check",
	]);

	await app.open("dev");
	await expect(page.getByTestId(serviceNodeID).getByTestId("node-warning-badge")).toContainText("2");
	await page.getByRole("button", { name: "2 warnings", exact: true }).click();
	for (const warning of report.warnings) {
		await expect(page.getByText(warning.title, { exact: true })).toBeVisible();
	}
});

// REQ-FR-5; contract 8.5: env-wide warnings need no canvas node or selection.
test("Config warnings · overlapping sibling VPC · environment region lists the warning", async ({
	page,
	app,
	envYaml,
}) => {
	// The sibling keeps dev's CIDR deliberately; all values remain synthetic.
	writeFileSync(
		envYaml.path("staging"),
		envYaml.text().replace("env: dev", "env: staging"),
		"utf8",
	);
	const report = await fetchWarnings(page);
	const overlap = report.warnings.find(
		(warning: { code: string }) => warning.code === "vpc_cidr_overlap",
	);
	expect(overlap).toBeDefined();
	expect(overlap.item).toEqual({ kind: "env", name: "" });

	await app.open("dev");
	const region = page.getByRole("region", { name: "Environment warnings" });
	await expect(region).toBeVisible();
	await expect(region.getByText(overlap.title, { exact: true })).toBeVisible();
});

// REQ-FR-4, REQ-FR-5; contract 8.5: a successful save triggers a fresh report.
test("Config warnings · successful save · warnings are refetched and remain current", async ({
	page,
	app,
	envYaml,
}) => {
	writeConfig(envYaml, { services: [internalService()] });
	await app.open("dev");
	const node = page.getByTestId(serviceNodeID);
	await expect(node).toHaveAttribute("data-warning", "true");
	await openPanel(page, app, serviceName, "IAM");
	await page.getByRole("button", { name: "Add statement", exact: true }).click();
	await fillPolicy(
		page,
		"Deny",
		["dynamodb:GetItem"],
		["arn:aws:dynamodb:us-east-1:000000000000:table/example"],
	);

	const refreshed = page.waitForResponse((response) => {
		const url = new URL(response.url());
		return response.status() === 200 &&
			response.request().method() === "GET" &&
			url.pathname === "/api/config-warnings" &&
			url.searchParams.get("env") === "dev";
	});
	await page.getByRole("button", { name: "Add statement", exact: true }).last().click();
	await refreshed;
	await expect(node).toHaveAttribute("data-warning", "true");
	await expect(node.getByTestId("node-warning-badge")).toContainText("1");
});

// REQ-FR-4, REQ-FR-5: a refused YAML still has a useful report and visible error.
test("Config warnings · configError only · environment panel explains the refusal", async ({
	page,
	app,
	envYaml,
}) => {
	const config = envYaml.read();
	config.schema_version = 999999;
	writeConfig(envYaml, config);
	const report = await fetchWarnings(page);
	expect(report.warnings).toEqual([]);
	expect(report.configError).toEqual(expect.any(String));

	await app.open("dev");
	const region = page.getByTestId("env-warnings");
	await expect(region).toBeVisible();
	await expect(region).toContainText(report.configError);
});
