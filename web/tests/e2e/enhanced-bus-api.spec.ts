import { rmSync, writeFileSync } from "node:fs";
import yaml from "js-yaml";
import { seed } from "./support/config-editors";
import { expect, test } from "./support/harness";

// REQ-8, REQ-11, REQ-12; TEST-7.
test("Share targets distinguish eligible siblings from duplicate declarations", async ({
	page,
	envYaml,
}) => {
	const config = seed(envYaml, { region: "ap-southeast-2" });
	// The served project is shared by tests, so restore just the sibling files this test adds.
	const { account_id: _account, ...withoutAccount } = config;
	const fixtures = [
		[
			"bus-share-stage",
			{ ...config, env: "blind-stage", account_id: "111111111111" },
		],
		[
			"bus-share-qa",
			{ ...config, env: "blind-qa", account_id: "222222222222" },
		],
		["bus-share-qa-copy", { ...withoutAccount, env: "blind-qa" }],
	] as const;
	const created: string[] = [];
	try {
		for (const [file, body] of fixtures) {
			const path = envYaml.path(file);
			writeFileSync(path, yaml.dump(body), { flag: "wx" });
			created.push(path);
		}
		const missing = await page.request.get("/api/event-bus/share-targets");
		expect(missing.status()).toBe(400);
		expect(await missing.json()).toMatchObject({ error: expect.any(String) });
		const unknown = await page.request.get(
			"/api/event-bus/share-targets?env=blind-nonexistent",
		);
		expect(unknown.status()).toBe(404);
		expect(await unknown.json()).toMatchObject({
			error: expect.stringContaining("blind-nonexistent.yaml"),
		});
		const response = await page.request.get(
			"/api/event-bus/share-targets?env=dev",
		);
		expect(response.status()).toBe(200);
		const targets = await response.json();
		expect(
			targets.filter((target: { env: string }) => target.env === "blind-qa"),
		).toEqual([
			expect.objectContaining({
				env: "blind-qa",
				status: "ambiguous",
				files: expect.arrayContaining([
					"bus-share-qa.yaml",
					"bus-share-qa-copy.yaml",
				]),
			}),
		]);
		expect(targets).toContainEqual(
			expect.objectContaining({
				env: "blind-stage",
				status: "ok",
				accountId: "111111111111",
				region: "ap-southeast-2",
				files: ["bus-share-stage.yaml"],
			}),
		);
	} finally {
		for (const path of created) rmSync(path, { force: true });
	}
});

// REQ-11, REQ-12; TEST-8. Both requests must be refused before AWS publishing.
for (const { type, eventBusName } of [
	{ type: "classic", eventBusName: "myapp-dev" },
	{ type: "enhanced", eventBusName: "default" },
]) {
	test(`Test Event rejects a group targeting ${eventBusName} in a ${type} environment`, async ({
		page,
		envYaml,
	}) => {
		seed(envYaml, {
			region: "ap-southeast-2",
			event_bus: { enabled: true, type },
			event_processor_tasks: [],
		});
		const response = await page.request.post(
			"/api/eventbridge/send-test-event",
			{
				data: {
					env: "dev",
					eventBusName,
					source: "myapp.backend",
					detailType: "payment.done",
					detail: { synthetic: true },
					eventGroupId: "payment-1",
				},
			},
		);
		expect(response.status()).toBe(400);
		expect(await response.json()).toEqual({
			success: false,
			message: "eventGroupId needs the enhanced project bus",
		});
	});
}

// REQ-10, REQ-11, REQ-12; TEST-9.
test("Warnings report an unsupported enhanced region as configError without failing HTTP", async ({
	page,
	envYaml,
}) => {
	seed(envYaml, {
		region: "sa-east-1",
		event_bus: { enabled: true, type: "enhanced" },
		event_processor_tasks: [],
	});
	const response = await page.request.get("/api/config-warnings?env=dev");
	expect(response.status()).toBe(200);
	expect(await response.json()).toMatchObject({
		env: "dev",
		warnings: expect.any(Array),
		configError: expect.stringContaining(
			"event_bus.type enhanced is not available in sa-east-1",
		),
	});
});
