import { describe, expect, test } from "bun:test";
import {
	describeProduceEntry,
	filterFieldsNest,
	isCatchAllWildcard,
	parseFilterValues,
	subscriptionErrors,
	subscriptionEventTypeDisplay,
	subscriptionFilterSummary,
	subscriptionKey,
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
