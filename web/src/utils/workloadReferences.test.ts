import { describe, expect, test } from "bun:test";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	removeWorkloadReferences,
	renamedAmplifyRule,
	renameWorkloadReferences,
} from "./workloadReferences";

describe("renamedAmplifyRule", () => {
	test("pins the path a service rule derived from its /<name> prefix", () => {
		expect(
			renamedAmplifyRule(
				{ source: "/api/<*>", workload: "api" },
				"api",
				"orders",
			),
		).toEqual({ source: "/api/<*>", workload: "orders", path: "/<*>" });
	});

	test("pins an empty derived path as /", () => {
		expect(
			renamedAmplifyRule({ source: "/api", workload: "api" }, "api", "orders"),
		).toEqual({ source: "/api", workload: "orders", path: "/" });
	});

	test("leaves the path unset when the source never had the prefix", () => {
		expect(
			renamedAmplifyRule(
				{ source: "/v2/<*>", workload: "api" },
				"api",
				"orders",
			),
		).toEqual({ source: "/v2/<*>", workload: "orders" });
	});

	test("does not take a prefix that only starts the same way", () => {
		expect(
			renamedAmplifyRule(
				{ source: "/apis/<*>", workload: "api" },
				"api",
				"orders",
			),
		).toEqual({ source: "/apis/<*>", workload: "orders" });
	});

	test("keeps an explicit path as it is", () => {
		expect(
			renamedAmplifyRule(
				{ source: "/api/<*>", workload: "api", path: "/v1/<*>" },
				"api",
				"orders",
			),
		).toEqual({ source: "/api/<*>", workload: "orders", path: "/v1/<*>" });
	});

	test("never pins a path for the backend, which keeps the full source", () => {
		expect(
			renamedAmplifyRule(
				{ source: "/backend/<*>", workload: "backend" },
				"backend",
				"core",
			),
		).toEqual({ source: "/backend/<*>", workload: "core" });
	});
});

function configWithRules(): YamlInfrastructureConfig {
	return {
		project: "myapp",
		env: "dev",
		amplify_apps: [
			{
				name: "web",
				github_repository: "example/web",
				branches: [{ name: "main" }],
				environment_variables: { AMPLIFY_MONOREPO_APP_ROOT: "apps/web" },
				custom_rules: [
					{ source: "/old", target: "https://example.com/new", status: "301" },
					{ source: "/api/<*>", workload: "api" },
					{ source: "/reports/<*>", workload: "reports" },
				],
			},
			{
				name: "docs",
				github_repository: "example/docs",
				branches: [{ name: "main" }],
				custom_rules: [{ source: "/api", workload: "api" }],
			},
		],
	} as unknown as YamlInfrastructureConfig;
}

describe("Amplify rule references", () => {
	test("a deleted workload's rules are dropped and an emptied list removed", () => {
		const updates = removeWorkloadReferences(configWithRules(), "api");
		expect(updates.amplify_apps).toEqual([
			{
				name: "web",
				github_repository: "example/web",
				branches: [{ name: "main" }],
				environment_variables: { AMPLIFY_MONOREPO_APP_ROOT: "apps/web" },
				custom_rules: [
					{ source: "/old", target: "https://example.com/new", status: "301" },
					{ source: "/reports/<*>", workload: "reports" },
				],
			},
			{
				name: "docs",
				github_repository: "example/docs",
				branches: [{ name: "main" }],
			},
		]);
	});

	test("a rename points the rules at the new name and pins derived paths", () => {
		const updates = renameWorkloadReferences(
			configWithRules(),
			"api",
			"orders",
		);
		const apps = updates.amplify_apps ?? [];
		expect(apps[0].custom_rules).toEqual([
			{ source: "/old", target: "https://example.com/new", status: "301" },
			{ source: "/api/<*>", workload: "orders", path: "/<*>" },
			{ source: "/reports/<*>", workload: "reports" },
		]);
		expect(apps[1].custom_rules).toEqual([
			{ source: "/api", workload: "orders", path: "/" },
		]);
	});

	test("an unrelated workload changes nothing", () => {
		expect(
			removeWorkloadReferences(configWithRules(), "nightly").amplify_apps,
		).toBeUndefined();
	});
});
