import { describe, expect, test } from "bun:test";
import { describeProduceEntry } from "./workloadEvents";

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
