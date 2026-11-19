import type { Route } from "@playwright/test";
import { openPanel, seed, workload } from "./support/config-editors";
import { expect, test } from "./support/harness";

// The dev server renders the app under React StrictMode, which runs every
// state updater twice. A save sent from inside an updater would reach the
// backend twice; this counts the requests the backend receives.
test("Backend service Events · turning the last ordered subscription off sends exactly one save, after the fresh read", async ({
	page,
	app,
	envYaml,
}) => {
	seed(envYaml, {
		event_bus: { enabled: true, type: "enhanced" },
		workload: {
			...(envYaml.read().workload as Record<string, unknown>),
			events: {
				subscribes: [
					{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
				],
			},
		},
	});

	const saves: string[] = [];
	page.on("request", (request) => {
		if (
			request.method() === "POST" &&
			request.url().includes("/api/environment/update")
		) {
			saves.push(request.postData() ?? "");
		}
	});

	// Reads of the deployed events state are answered at once, except while
	// `held` is set: then they wait until the test releases them.
	const answer = {
		known: true,
		fifoConsumers: ["backend"],
		sharedPrincipals: [],
		subscribePrincipals: [],
	};
	let held = false;
	let reads = 0;
	const waiting: Route[] = [];
	await page.route("**/api/events/deployed**", async (route) => {
		reads += 1;
		if (held) {
			waiting.push(route);
			return;
		}
		await route.fulfill({ json: answer });
	});

	await app.open();
	await openPanel(page, app, "Backend service", "Events");
	const order = page.getByRole("switch", {
		name: "Keep order for ledger",
		exact: true,
	});
	await expect(order).toBeChecked();

	held = true;
	await order.click();
	await expect.poll(() => waiting.length).toBe(1);
	await expect(
		page.getByText(/The events settings are locked until the check is done/),
	).toBeVisible();
	await expect(order).toBeDisabled();
	expect(saves).toHaveLength(0);

	held = false;
	const readsBefore = reads;
	await waiting.shift()?.fulfill({ json: answer });

	const events = () => workload(envYaml.read(), "workload").events ?? {};
	await expect.poll(() => events().queue?.fifo_retired).toBe(true);
	expect(events().subscribes).toEqual([
		{ name: "ledger", detail_type_prefix: "ledger." },
	]);
	// A successful save starts a routine read; by then a second save from the
	// same decision would already have been sent.
	await expect.poll(() => reads).toBeGreaterThan(readsBefore);
	expect(saves).toHaveLength(1);
	await expect(order).toBeEnabled();
	await expect(order).not.toBeChecked();
});
