import { describe, expect, it } from "vitest";
import { applyChanges, computeDiff } from "../diffEngine";
import type { PersistenceChannel } from "../types";
import {
	context,
	field,
	freeze,
	panel,
	parameterField,
} from "./contractFixtures";

// Engine-level coverage only: cancellation/failure UI behavior cannot be proved by
// choosing not to call applyChanges. Those require the public controller contract.
describe("operation-level successful-save baselines — SAV-02/03", () => {
	const channels: PersistenceChannel[] = ["configuration", "live", "layout"];
	const orders: PersistenceChannel[][] = [
		["configuration", "live", "layout"],
		["configuration", "layout", "live"],
		["live", "configuration", "layout"],
		["live", "layout", "configuration"],
		["layout", "configuration", "live"],
		["layout", "live", "configuration"],
	];

	it.each(orders.map((order) => [order.join(" → "), order] as const))(
		"merges only submitted channels in order %s",
		(_name, order) => {
			// A pure diff definition can exercise independent channels without claiming
			// that a real canvas panel exposes infrastructure editing UI.
			const definition = freeze(
				panel([
					field("configuration", { mutability: "TF" }),
					field("live", { mutability: "LIVE-RESTART" }),
					field("layout", {
						mutability: "UI",
						valueKind: "canvas-only",
						section: "layout",
					}),
				]),
			);
			const original = freeze({
				configuration: "old-config",
				live: "old-live",
				layout: { width: 100 },
			});
			const draft = freeze({
				configuration: "new-config",
				live: "new-live",
				layout: { width: 200 },
			});
			let baseline: Record<string, unknown> = original;
			const saved = new Set<PersistenceChannel>();
			for (const channel of order) {
				const result = computeDiff(definition, baseline, draft, context);
				expect([...result.dirtyChannels].sort()).toEqual(
					channels.filter((item) => !saved.has(item)).sort(),
				);
				const next = applyChanges(
					freeze(baseline),
					freeze(result.changes),
					channel,
				);
				expect(next).not.toBe(baseline);
				saved.add(channel);
				for (const key of channels) {
					expect(next[key]).toEqual(
						saved.has(key) ? draft[key] : original[key],
					);
				}
				baseline = next;
			}
			expect(baseline).toEqual(draft);
			expect(original).toEqual({
				configuration: "old-config",
				live: "old-live",
				layout: { width: 100 },
			});
			expect(computeDiff(definition, baseline, draft, context).changes).toEqual(
				[],
			);
		},
	);

	it.each(["live", "configuration"] as const)(
		"same parameter collection retains other-channel operations after %s save",
		(firstChannel) => {
			// Explicit default channel for membership; valueEditChannel remains live.
			// Description live-routing is tested separately: the frozen API cannot
			// express a second per-cell channel/impact rule on this same collection.
			const definition = freeze(
				panel([parameterField({ persistenceChannel: "configuration" })]),
			);
			const baseline = freeze({
				parameters: {
					KEEP: { value: "fake-old", description: "old description" },
					REMOVE: { value: "fake-removed", description: "remove description" },
				},
			});
			const draft = freeze({
				parameters: {
					KEEP: { value: "fake-new", description: "old description" },
					ADD: { value: "fake-added", description: "added description" },
				},
			});
			const first = computeDiff(definition, baseline, draft, context);
			expect([...first.dirtyChannels].sort()).toEqual([
				"configuration",
				"live",
			]);
			expect(first.channelSummaries.configuration).toMatchObject({
				changeCount: 2,
				worstImpact: "rolling-restart",
			});
			expect(first.channelSummaries.live).toMatchObject({
				changeCount: 1,
				worstImpact: "restart-to-observe",
			});
			const intermediate = applyChanges(baseline, first.changes, firstChannel);
			if (firstChannel === "live") {
				expect(intermediate).toEqual({
					parameters: {
						KEEP: draft.parameters.KEEP,
						REMOVE: baseline.parameters.REMOVE,
					},
				});
			} else {
				expect(intermediate).toEqual({
					parameters: {
						KEEP: baseline.parameters.KEEP,
						ADD: draft.parameters.ADD,
					},
				});
			}
			const remainingChannel =
				firstChannel === "live" ? "configuration" : "live";
			const remaining = computeDiff(definition, intermediate, draft, context);
			expect(remaining.dirtyChannels).toEqual([remainingChannel]);
			expect(remaining.changes).toHaveLength(
				remainingChannel === "configuration" ? 2 : 1,
			);
			expect(
				remaining.changes.every(
					(change) => change.persistenceChannel === remainingChannel,
				),
			).toBe(true);
			expect(
				applyChanges(intermediate, remaining.changes, remainingChannel),
			).toEqual(draft);
		},
	);

	it("saving no operations in a channel never acknowledges another channel", () => {
		const definition = panel([field("value", { mutability: "TF-REPLACE" })]);
		const baseline = freeze({ value: "old" });
		const draft = freeze({ value: "new" });
		const result = computeDiff(definition, baseline, draft, context);
		const next = applyChanges(baseline, result.changes, "live");
		expect(next).toEqual(baseline);
		const stillDirty = computeDiff(definition, next, draft, context);
		expect(stillDirty.changes).toEqual(result.changes);
		expect(stillDirty.destructive).toBe(true);
	});

	it("a previously reviewed payload cannot acknowledge a newer draft edit", () => {
		const definition = panel([
			field("value", { mutability: "TF-REPLACE" }),
			field("live", { mutability: "LIVE" }),
		]);
		const baseline = freeze({ value: "original", live: "old" });
		const reviewed = computeDiff(
			definition,
			baseline,
			{ value: "reviewed", live: "old" },
			context,
		);
		freeze(reviewed.changes);
		const laterDraft = freeze({ value: "newer-unreviewed", live: "new-live" });
		const next = applyChanges(baseline, reviewed.changes, "configuration");
		expect(next).toEqual({ value: "reviewed", live: "old" });
		const remaining = computeDiff(definition, next, laterDraft, context);
		expect(
			remaining.changes.find((change) => change.fieldId === "value"),
		).toMatchObject({
			before: "reviewed",
			after: "newer-unreviewed",
			impact: "recreates-data-loss",
		});
		expect([...remaining.dirtyChannels].sort()).toEqual([
			"configuration",
			"live",
		]);
		expect(remaining.destructive).toBe(true);
	});
});

describe("per-channel validation recoverability — frozen API section 2", () => {
	it("invalid configuration does not remove valid live changes", () => {
		const definition = panel([
			field("count", {
				mutability: "TF",
				controlType: "number",
				validation: { min: 1 },
			}),
			field("live", { mutability: "LIVE" }),
		]);
		const baseline = freeze({ count: 1, live: "old" });
		const draft = freeze({ count: 0, live: "new" });
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.validationErrors).toEqual(
			expect.arrayContaining([
				expect.objectContaining({
					fieldId: "count",
					persistenceChannel: "configuration",
					message: expect.any(String),
				}),
			]),
		);
		expect(
			result.validationErrors.some(
				(error) => error.persistenceChannel === "live",
			),
		).toBe(false);
		expect(
			result.changes.find((change) => change.fieldId === "live"),
		).toMatchObject({
			before: "old",
			after: "new",
			persistenceChannel: "live",
		});
		const next = applyChanges(baseline, result.changes, "live");
		expect(next).toEqual({ count: 1, live: "new" });
		const remaining = computeDiff(definition, next, draft, context);
		expect(
			remaining.validationErrors.some((error) => error.fieldId === "count"),
		).toBe(true);
		expect(
			remaining.changes.some((change) => change.persistenceChannel === "live"),
		).toBe(false);
	});
});
