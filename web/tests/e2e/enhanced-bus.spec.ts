import { openPanel, reopen, seed } from "./support/config-editors";
import { expect, test } from "./support/harness";

// REQ-11, REQ-12; TEST-5; Standard, Enhanced and RegionUnavailable stories.
for (const region of ["ap-southeast-2", "sa-east-1"]) {
	test(`Enhanced is selectable only where offered (${region})`, async ({
		page,
		app,
		envYaml,
		evidence,
	}) => {
		seed(envYaml, {
			region,
			event_bus: { enabled: true },
			event_processor_tasks: [],
		});
		await app.open();
		await openPanel(page, app, "Amazon EventBridge", "Settings");
		const enhanced = page.getByLabel("Enhanced", { exact: true });
		await expect(page.getByLabel("Standard", { exact: true })).toBeChecked();
		if (region === "sa-east-1") {
			await expect(enhanced).toBeDisabled();
			await expect(
				page.getByText("Not available in sa-east-1 yet."),
			).toBeVisible();
			return;
		}
		await enhanced.click();
		await expect
			.poll(() => envYaml.read().event_bus)
			.toMatchObject({ enabled: true, type: "enhanced" });
		await reopen(page, app, "Amazon EventBridge", "Settings");
		await expect(enhanced).toBeChecked();
		await expect(
			page.getByText("Not shared with any other account."),
		).toBeVisible();
		await evidence("enhanced-project-bus-persisted");
	});
}

// REQ-7, REQ-11, REQ-12; TEST-6; EnhancedOrderedSubscription and EnhancedMixedBusFlow.
test("Enhanced shows FIFO settings for project subscriptions but not default subscriptions", async ({
	page,
	app,
	envYaml,
}) => {
	seed(envYaml, {
		region: "ap-southeast-2",
		event_bus: { enabled: true, type: "enhanced" },
		event_processor_tasks: [],
		workload: {
			...(envYaml.read().workload as Record<string, unknown>),
			events: {
				subscribes: [
					{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					{ name: "payment", detail_type: "payment.done" },
					{
						name: "ecs",
						detail_type: "ECS Task State Change",
						source: "aws.ecs",
						bus: "default",
					},
				],
			},
		},
	});
	await page.route("**/api/events/deployed**", (route) =>
		route.fulfill({
			json: {
				known: true,
				fifoConsumers: [],
				sharedPrincipals: [],
				subscribePrincipals: [],
			},
		}),
	);
	await app.open();
	await openPanel(page, app, "Backend service", "Events");
	await expect(
		page.getByRole("switch", { name: "Keep order for ledger", exact: true }),
	).toBeChecked();
	await expect(
		page.getByRole("switch", { name: "Keep order for payment", exact: true }),
	).not.toBeChecked();
	await expect(
		page.getByRole("switch", { name: "Keep order for ecs", exact: true }),
	).toHaveCount(0);
	await expect(
		page.getByText("EVENT_FIFO_QUEUE_URL", { exact: true }),
	).toBeVisible();
});
