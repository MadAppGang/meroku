import { describe, expect, test } from "bun:test";
import type {
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	clearOrdering,
	deployedSharePrincipal,
	deployedSharesRemoved,
	describeProduceEntry,
	eventFlows,
	filterFieldsNest,
	isCatchAllWildcard,
	orderedQueueMayBeDeployed,
	orderedQueueRereader,
	orderingDecisionUpdate,
	parseFilterValues,
	type ShareTargetInfo,
	sameSharePrincipal,
	shareComparisonUnknown,
	shareDuplicateError,
	sharePrincipalLabel,
	shareSubscribeDowngraded,
	subscriptionErrors,
	subscriptionEventTypeDisplay,
	subscriptionFilterSummary,
	subscriptionKey,
	updateConfigOfEnv,
	updateWorkloadEvents,
	withOrderingCleared,
} from "./workloadEvents";

describe("describeProduceEntry", () => {
	test("listed detail-types with the workload's own source by default", () => {
		expect(
			describeProduceEntry(
				{ detail_types: ["order.created", "order.paid"] },
				"myapp-dev",
				"myapp.api",
			),
		).toBe(
			"myapp-dev: detail-type order.created, order.paid, source myapp.api",
		);
	});

	test("any: true lifts the detail-type condition", () => {
		expect(
			describeProduceEntry({ any: true }, "default", "myapp.backend"),
		).toBe("default: any detail-type, source myapp.backend");
	});

	test("own_source_only: false lifts the source condition", () => {
		expect(
			describeProduceEntry(
				{ detail_types: ["x"], own_source_only: false },
				"myapp-dev",
				"myapp.api",
			),
		).toBe("myapp-dev: detail-type x, any source");
	});
});

describe("subscriptionKey", () => {
	test("derives key from exact and wildcard detail_type", () => {
		expect(subscriptionKey({ detail_type: "job.completed" }, true)).toBe(
			"job-completed",
		);
		expect(subscriptionKey({ detail_type: "job.*" }, true)).toBe("job");
		expect(
			subscriptionKey({ detail_type: "job.*", bus: "default" }, true),
		).toBe("job-default");
		expect(subscriptionKey({ detail_type: "*" }, true)).toBe("");
		expect(subscriptionKey({ name: "all-jobs", detail_type: "*" }, true)).toBe(
			"all-jobs",
		);
	});

	test("derives key from legacy detail_type_prefix", () => {
		expect(
			subscriptionKey({ detail_type_prefix: "user.", bus: "default" }, true),
		).toBe("user-prefix-default");
	});
});

describe("subscriptionEventTypeDisplay", () => {
	test("renders detail_type verbatim and detail_type_prefix with a trailing *", () => {
		expect(subscriptionEventTypeDisplay({ detail_type: "job.*" })).toBe(
			"job.*",
		);
		expect(subscriptionEventTypeDisplay({ detail_type_prefix: "user." })).toBe(
			"user.*",
		);
	});
});

describe("subscriptionFilterSummary", () => {
	test("counts source and field conditions", () => {
		expect(subscriptionFilterSummary({ detail_type: "job.created" })).toBe(
			"Filter (optional)",
		);
		expect(
			subscriptionFilterSummary({
				detail_type: "job.created",
				source: "myapp.orders",
			}),
		).toBe("1 filter");
		expect(
			subscriptionFilterSummary({
				detail_type: "job.created",
				source: "myapp.orders",
				filters: [{ field: "detail.status", values: ["done", "failed"] }],
			}),
		).toBe("2 filters");
	});
});

describe("subscriptionErrors", () => {
	test("flags invalid field path, duplicate field, empty values, and catch-all without name", () => {
		expect(
			subscriptionErrors({ detail_type: "*" }, 0, [{ detail_type: "*" }], true),
		).toContain(
			"No key can be derived from this event type; open Advanced and give it a name.",
		);
		expect(isCatchAllWildcard("*")).toBe(true);
		expect(isCatchAllWildcard("job.*")).toBe(false);
		expect(parseFilterValues("done, failed , ")).toEqual(["done", "failed"]);

		const badFilter = {
			detail_type: "job.*",
			filters: [
				{ field: "status", values: ["done"] },
				{ field: "detail.region", values: ["eu"] },
				{ field: "detail.region", values: ["us"] },
			],
		};
		const errs = subscriptionErrors(badFilter, 0, [badFilter], true);
		expect(errs.some((e) => e.includes("must start with detail."))).toBe(true);
		expect(errs.some((e) => e.includes("already filtered"))).toBe(true);
	});
});

describe("filterFieldsNest", () => {
	test("a field inside another overlaps, in either order", () => {
		expect(filterFieldsNest("detail.order", "detail.order.type")).toBe(true);
		expect(filterFieldsNest("detail.order.type", "detail.order")).toBe(true);
	});
	test("siblings, shared prefixes and equal fields do not nest", () => {
		expect(filterFieldsNest("detail.order", "detail.orders")).toBe(false);
		expect(filterFieldsNest("detail.status", "detail.region")).toBe(false);
		expect(filterFieldsNest("detail.status", "detail.status")).toBe(false);
	});
	test("subscriptionErrors refuses nested filter fields", () => {
		const sub = {
			detail_type: "job.created",
			filters: [
				{ field: "detail.order", values: ["a"] },
				{ field: "detail.order.type", values: ["b"] },
			],
		};
		const errors = subscriptionErrors(sub, 0, [sub], true);
		expect(errors.some((e) => e.includes("overlap"))).toBe(true);
	});
});

const enhancedConfig = (
	events: WorkloadEvents | undefined,
): YamlInfrastructureConfig =>
	({
		project: "demo",
		env: "dev",
		region: "us-east-1",
		account_id: "000000000000",
		event_bus: { enabled: true, type: "enhanced" },
		workload: { events },
	}) as YamlInfrastructureConfig;

describe("shareDuplicateError", () => {
	test("finds a duplicate after the entry as well as before it", () => {
		const share = [
			{ account: "333333333333", access: "publish" as const },
			{ account: "333333333333", access: "full" as const },
		];
		expect(shareDuplicateError(share, 0, [])).toContain("entry 2");
		expect(shareDuplicateError(share, 1, [])).toContain("entry 1");
	});
	test("two env entries for one environment match before targets load", () => {
		const share = [
			{ env: "staging", access: "publish" as const },
			{ env: "staging", access: "subscribe" as const },
		];
		expect(shareDuplicateError(share, 0, undefined)).toContain("entry 2");
	});
	test("distinct principals are not duplicates", () => {
		const share = [
			{ account: "222222222222", access: "publish" as const },
			{ account: "333333333333", access: "publish" as const },
		];
		expect(shareDuplicateError(share, 0, [])).toBe(null);
	});
});

const staging: ShareTargetInfo = {
	env: "staging",
	status: "ok",
	accountId: "111111111111",
	region: "us-east-1",
	files: ["staging.yaml"],
};

describe("share comparisons while targets are unresolved", () => {
	const envEntry = { env: "staging", access: "publish" as const };
	const account = (id: string) => ({ account: id, access: "publish" as const });

	test("env against account is unknown while targets are pending or failed", () => {
		expect(
			sameSharePrincipal(envEntry, account("111111111111"), undefined),
		).toBe("unknown");
		const share = [envEntry, account("111111111111")];
		expect(shareComparisonUnknown(share, 1, undefined)).toEqual(envEntry);
		expect(shareComparisonUnknown(share, 0, undefined)).toEqual(
			account("111111111111"),
		);
		expect(shareDuplicateError(share, 1, undefined)).toBe(null);
	});
	test("two different environments are unknown before targets load", () => {
		const share = [envEntry, { env: "qa", access: "publish" as const }];
		expect(shareComparisonUnknown(share, 1, undefined)).toEqual(envEntry);
	});
	test("the same environment is a duplicate, not unknown", () => {
		const share = [envEntry, { ...envEntry, access: "full" as const }];
		expect(sameSharePrincipal(share[0], share[1], undefined)).toBe(true);
		expect(shareComparisonUnknown(share, 1, undefined)).toBe(null);
	});
	test("organization and OU never wait for targets", () => {
		const share = [
			envEntry,
			{ organization: true, access: "publish" as const },
			{ ou: "ou-ab12-cd34ef56", access: "publish" as const },
		];
		expect(shareComparisonUnknown(share, 1, undefined)).toBe(null);
		expect(shareComparisonUnknown(share, 2, undefined)).toBe(null);
	});
	test("loaded targets resolve the comparison", () => {
		const share = [envEntry, account("111111111111")];
		expect(shareComparisonUnknown(share, 1, [staging])).toBe(null);
		expect(shareDuplicateError(share, 1, [staging])).toContain(
			"environment staging",
		);
		expect(
			shareDuplicateError([envEntry, account("222222222222")], 1, [staging]),
		).toBe(null);
	});
	test("accounts compare without targets", () => {
		const share = [account("222222222222"), account("333333333333")];
		expect(shareComparisonUnknown(share, 1, undefined)).toBe(null);
	});
});

describe("deployed share principals", () => {
	test("reads account:<id>, ou:<id> and organization", () => {
		expect(deployedSharePrincipal("account:222222222222")).toBe("222222222222");
		expect(deployedSharePrincipal("ou:ou-ab12-cd34ef56")).toBe(
			"ou-ab12-cd34ef56",
		);
		expect(deployedSharePrincipal("organization")).toBe("organization");
		expect(deployedSharePrincipal("account:12345")).toBe(null);
		expect(deployedSharePrincipal("nonsense")).toBe(null);
	});
	const deployed = (sharedPrincipals: string[], known = true) => ({
		known,
		fifoConsumers: [],
		sharedPrincipals,
		subscribePrincipals: [],
	});
	test("a deployed principal no entry names is removed, whatever the access", () => {
		const share = [
			{ account: "222222222222", access: "publish" as const },
			{ organization: true, access: "full" as const },
		];
		expect(
			deployedSharesRemoved(
				deployed([
					"account:222222222222",
					"organization",
					"ou:ou-ab12-cd34ef56",
				]),
				share,
				[],
			),
		).toEqual(["ou-ab12-cd34ef56"]);
	});
	test("an env entry resolves through the targets", () => {
		const share = [{ env: "staging", access: "subscribe" as const }];
		const shared = deployed(["account:111111111111"]);
		expect(deployedSharesRemoved(shared, share, [staging])).toEqual([]);
		// Before the targets load, the env may still name the account.
		expect(deployedSharesRemoved(shared, share, undefined)).toEqual([]);
		expect(deployedSharesRemoved(shared, [], undefined)).toEqual([
			"111111111111",
		]);
	});
	test("a deployed subscriber whose entry may now only send is downgraded", () => {
		const state = {
			known: true,
			fifoConsumers: [],
			sharedPrincipals: ["account:111111111111", "account:222222222222"],
			subscribePrincipals: ["account:111111111111"],
		};
		const publish = (e: object) => ({ ...e, access: "publish" as const });
		expect(
			shareSubscribeDowngraded(publish({ account: "111111111111" }), state, []),
		).toBe(true);
		// Deployed for sending only: nothing to revoke.
		expect(
			shareSubscribeDowngraded(publish({ account: "222222222222" }), state, []),
		).toBe(false);
		// Still allowed to subscribe.
		expect(
			shareSubscribeDowngraded(
				{ account: "111111111111", access: "full" },
				state,
				[],
			),
		).toBe(false);
		// An env resolves through the targets, and is unknown before them.
		expect(
			shareSubscribeDowngraded(publish({ env: "staging" }), state, [staging]),
		).toBe(true);
		expect(
			shareSubscribeDowngraded(publish({ env: "staging" }), state, undefined),
		).toBe(false);
		expect(
			shareSubscribeDowngraded(
				publish({ account: "111111111111" }),
				{ ...state, known: false },
				[],
			),
		).toBe(false);
	});
	test("unknown state reports nothing", () => {
		expect(
			deployedSharesRemoved(deployed(["account:222222222222"], false), [], []),
		).toEqual([]);
		expect(deployedSharesRemoved(undefined, [], [])).toEqual([]);
	});
	test("labels name the environment of an account when known", () => {
		expect(sharePrincipalLabel("111111111111", [staging])).toBe(
			"environment staging (account 111111111111)",
		);
		expect(sharePrincipalLabel("222222222222", [staging])).toBe(
			"account 222222222222",
		);
		expect(sharePrincipalLabel("organization", undefined)).toBe(
			"the whole organization",
		);
	});
});

describe("ordered queue retirement", () => {
	const ordered = {
		subscribes: [{ detail_type: "ledger.posted", ordered: true }],
	};
	const turnedOff = { subscribes: [{ detail_type: "ledger.posted" }] };

	test("a queue that may be deployed is kept with fifo_retired", () => {
		expect(withOrderingCleared(ordered, turnedOff)?.queue?.fifo_retired).toBe(
			true,
		);
	});
	test("a queue never deployed gets no fifo_retired", () => {
		expect(
			withOrderingCleared(ordered, turnedOff, false)?.queue?.fifo_retired,
		).toBeUndefined();
	});
	test("the deployed state tells; unknown keeps the queue", () => {
		const deployed = (fifoConsumers: string[], known = true) => ({
			known,
			fifoConsumers,
			sharedPrincipals: [],
			subscribePrincipals: [],
		});
		expect(orderedQueueMayBeDeployed(deployed([]), "backend")).toBe(false);
		expect(orderedQueueMayBeDeployed(deployed(["backend"]), "backend")).toBe(
			true,
		);
		expect(orderedQueueMayBeDeployed(deployed(["orders"]), "backend")).toBe(
			false,
		);
		expect(orderedQueueMayBeDeployed(deployed([], false), "backend")).toBe(
			true,
		);
		expect(orderedQueueMayBeDeployed(undefined, "backend")).toBe(true);
	});
	test("a fresh read decides; a failed or unknown read keeps the queue", async () => {
		const answer =
			(fifoConsumers: string[], known = true) =>
			() =>
				Promise.resolve({
					known,
					fifoConsumers,
					sharedPrincipals: [],
					subscribePrincipals: [],
				});
		expect(orderedQueueRereader(undefined, "backend")).toBeUndefined();
		expect(await orderedQueueRereader(answer([]), "backend")?.()).toBe(false);
		expect(await orderedQueueRereader(answer(["backend"]), "backend")?.()).toBe(
			true,
		);
		expect(await orderedQueueRereader(answer([], false), "backend")?.()).toBe(
			true,
		);
		expect(
			await orderedQueueRereader(
				() => Promise.resolve(undefined),
				"backend",
			)?.(),
		).toBe(true);
		expect(
			await orderedQueueRereader(
				() => Promise.reject(new Error("offline")),
				"backend",
			)?.(),
		).toBe(true);
	});
	test("clearOrdering keeps only the queues that may be deployed", () => {
		const config = enhancedConfig(ordered);
		const never = clearOrdering(config, {
			known: true,
			fifoConsumers: [],
			sharedPrincipals: [],
			subscribePrincipals: [],
		});
		expect(never.affected).toEqual(["backend"]);
		expect(never.kept).toEqual([]);
		expect(never.updates.workload?.events?.queue?.fifo_retired).toBeUndefined();
		const maybe = clearOrdering(config);
		expect(maybe.kept).toEqual(["backend"]);
		expect(maybe.updates.workload?.events?.queue?.fifo_retired).toBe(true);
	});
});

describe("eventFlows", () => {
	test("draws default-bus rules apart from enhanced-bus subscribers", () => {
		const flows = eventFlows(enhancedConfig({}), "backend", {
			subscribes: [
				{ detail_type: "a.b" },
				{ detail_type: "c.d" },
				{ detail_type: "e.f", bus: "default" },
			],
		});
		expect(flows.length).toBe(1);
		expect(flows[0].paths).toEqual([
			{ bus: "demo-dev", delivery: "Subscribers", summary: "2 subscriptions" },
			{ bus: "default", delivery: "Rules", summary: "1 subscription" },
		]);
	});
	test("an active ordered set has one ordered subscriber path", () => {
		const flows = eventFlows(enhancedConfig({}), "backend", {
			subscribes: [{ detail_type: "a.b", ordered: true }],
		});
		expect(flows.map((f) => f["data-testid"])).toEqual([
			"event-flow",
			"event-flow-fifo",
		]);
		expect(flows[0].paths).toEqual([]);
		expect(flows[1].paths[0].delivery).toBe("Ordered subscriber");
		expect(flows[1].queue.name).toBe("demo-events-backend-dev.fifo");
	});
	test("a retired ordered set has no active path", () => {
		const flows = eventFlows(enhancedConfig({}), "backend", {
			subscribes: [{ detail_type: "a.b" }],
			queue: { fifo_retired: true },
		});
		expect(flows[1].paths).toEqual([]);
		expect(flows[1].idle?.name).toBe("retired, delivers nothing");
	});
	test("a workload that subscribes to nothing has no flow", () => {
		expect(eventFlows(enhancedConfig({}), "backend", undefined)).toEqual([]);
	});
});

describe("orderingDecisionUpdate through updateWorkloadEvents", () => {
	const ordered: WorkloadEvents = {
		subscribes: [{ name: "ledger", detail_type: "ledger.x", ordered: true }],
	};
	const turnedOff: WorkloadEvents = {
		subscribes: [{ name: "ledger", detail_type: "ledger.x" }],
	};
	const config = (
		services: YamlInfrastructureConfig["services"],
	): YamlInfrastructureConfig =>
		({ project: "myapp", env: "dev", services }) as YamlInfrastructureConfig;

	// The decision was made on the config at the edit; a service was added
	// while the read was pending. The update lands on the config of then.
	test("a services-array edit made while the read was pending is kept", () => {
		const atEdit = config([{ name: "orders", events: ordered }]);
		const decide = orderingDecisionUpdate(ordered, turnedOff, false);
		const meanwhile: YamlInfrastructureConfig = {
			...atEdit,
			services: [...(atEdit.services ?? []), { name: "billing" }],
		} as YamlInfrastructureConfig;
		const after = updateConfigOfEnv(meanwhile, "dev", (c) =>
			updateWorkloadEvents(c, "orders", decide),
		);
		expect(after.services?.map((s) => s.name)).toEqual(["orders", "billing"]);
		expect(after.services?.[0].events).toEqual(turnedOff);
	});

	test("the queue is kept with fifo_retired unless the read says never deployed", () => {
		const atEdit = config([{ name: "orders", events: ordered }]);
		const after = updateWorkloadEvents(
			atEdit,
			"orders",
			orderingDecisionUpdate(ordered, turnedOff, true),
		);
		expect(after.services?.[0].events?.queue?.fifo_retired).toBe(true);
	});

	test("events changed meanwhile: the decision no longer applies", () => {
		const changed: WorkloadEvents = {
			subscribes: [
				...(ordered.subscribes ?? []),
				{ name: "audit", detail_type: "audit.x" },
			],
		};
		const meanwhile = config([{ name: "orders", events: changed }]);
		const after = updateWorkloadEvents(
			meanwhile,
			"orders",
			orderingDecisionUpdate(ordered, turnedOff, false),
		);
		expect(after).toBe(meanwhile);
	});

	test("another environment's config is left alone", () => {
		const prod = {
			...config([{ name: "orders", events: ordered }]),
			env: "prod",
		} as YamlInfrastructureConfig;
		const after = updateConfigOfEnv(prod, "dev", (c) =>
			updateWorkloadEvents(
				c,
				"orders",
				orderingDecisionUpdate(ordered, turnedOff, false),
			),
		);
		expect(after).toBe(prod);
	});

	test("a removed service is left alone, and the backend is updated in place", () => {
		const none = config([
			{ name: "billing" },
		] as YamlInfrastructureConfig["services"]);
		expect(
			updateWorkloadEvents(
				none,
				"orders",
				orderingDecisionUpdate(ordered, turnedOff, false),
			),
		).toBe(none);
		const backend = {
			...config([]),
			workload: { events: ordered },
		} as YamlInfrastructureConfig;
		const after = updateWorkloadEvents(
			backend,
			"backend",
			orderingDecisionUpdate(ordered, turnedOff, true),
		);
		expect(after.workload?.events?.queue?.fifo_retired).toBe(true);
		expect(after.services).toEqual([]);
	});
});
