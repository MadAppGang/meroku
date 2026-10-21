import { describe, expect, it } from "vitest";
import { deriveTabs } from "../tabDerivation";
import type {
	ClosedSectionKey,
	MutabilityClass,
	PropertyFieldDefinition,
	PropertyTabId,
	SectionArchetype,
} from "../types";
import { context, field, freeze, panel } from "./contractFixtures";

const expectedOrder: Record<PropertyTabId, ClosedSectionKey[]> = {
	overview: ["identity", "endpoints", "relationships", "current-state"],
	configuration: [
		"naming-lifecycle",
		"source",
		"compute-scaling",
		"network-routing",
		"storage-data",
		"security-access",
		"environment-secrets",
		"observability",
		"advanced",
	],
	operations: ["deployments", "health-metrics", "actions"],
	logs: ["log-viewer", "destination", "retention"],
	layout: ["layout"],
};

function placed(
	id: string,
	section: ClosedSectionKey,
	tab: PropertyTabId,
): PropertyFieldDefinition {
	if (tab === "overview")
		return field(id, { section, mutability: "RO", valueKind: "aws-assigned" });
	if (tab === "operations")
		return field(id, {
			section,
			mutability: section === "actions" ? "LIVE" : "RUNTIME",
			valueKind: section === "health-metrics" ? "telemetry" : "editable",
		});
	if (tab === "logs")
		return field(id, {
			section: "observability",
			logRole:
				section === "log-viewer"
					? "viewer"
					: section === "destination"
						? "destination"
						: "retention",
		});
	return field(id, { section });
}

describe("mutability-derived tabs — NAV-01/03/06", () => {
	it("keeps Overview on an empty standard panel", () => {
		expect(deriveTabs(panel([]), {}, context).tabs).toEqual([
			{ id: "overview", sections: [] },
		]);
	});

	it.each<MutabilityClass>([
		"CREATE",
		"TF",
		"TF-DEPLOY",
		"TF-REPLACE",
		"CREATE / TF-REPLACE",
		"TF-REPLACE / TF-DEPLOY",
	])("%s produces desired-state Configuration", (mutability) => {
		const result = deriveTabs(
			panel([field("setting", { mutability })]),
			{},
			context,
		);
		expect(result.tabs.map((tab) => tab.id)).toEqual([
			"overview",
			"configuration",
		]);
		expect(result.placement.setting).toEqual({
			tab: "configuration",
			section: "source",
		});
	});

	it.each<MutabilityClass>(["LIVE", "LIVE-RESTART", "LIVE (action)"])(
		"%s stored values belong to Operations Actions",
		(mutability) => {
			const result = deriveTabs(
				panel([field("value", { mutability })]),
				{},
				context,
			);
			expect(result.tabs.map((tab) => tab.id)).toEqual([
				"overview",
				"operations",
			]);
			expect(result.placement.value).toEqual({
				tab: "operations",
				section: "actions",
			});
		},
	);

	it.each<MutabilityClass>([
		"CREATE",
		"TF",
		"TF-DEPLOY",
		"TF-REPLACE",
		"LIVE",
		"LIVE-RESTART",
		"RUNTIME",
		"LIVE (action)",
		"UI",
	])(
		"derived overrides %s and cannot create Configuration or Operations",
		(mutability) => {
			const result = deriveTabs(
				panel([
					field("routing", {
						mutability,
						valueKind: "derived",
						section: "network-routing",
					}),
				]),
				{},
				context,
			);
			expect(result.tabs.map((tab) => tab.id)).toEqual(["overview"]);
			expect(result.placement.routing).toEqual({
				tab: "overview",
				section: "relationships",
			});
		},
	);

	it("hides an optional tab when its last field becomes invisible", () => {
		const definition = panel([
			field("enabled", {
				section: "identity",
				valueKind: "aws-assigned",
				mutability: "RO",
			}),
			field("desired", {
				visibleWhen: { all: [{ kind: "equals", key: "enabled", value: true }] },
			}),
		]);
		expect(
			deriveTabs(definition, { enabled: true }, context).tabs.map(
				(tab) => tab.id,
			),
		).toEqual(["overview", "configuration"]);
		const hidden = deriveTabs(definition, { enabled: false }, context);
		expect(hidden.tabs.map((tab) => tab.id)).toEqual(["overview"]);
		expect(hidden.placement).not.toHaveProperty("desired");
	});

	it("places assigned identity, telemetry, rollout, and commands by explicit metadata", () => {
		const result = deriveTabs(
			panel([
				field("identity", {
					valueKind: "aws-assigned",
					mutability: "RO",
					section: "source",
				}),
				field("endpoint", {
					valueKind: "aws-assigned",
					mutability: "RO",
					section: "endpoints",
				}),
				field("health", { valueKind: "telemetry", mutability: "RUNTIME" }),
				field("rollout", { mutability: "RUNTIME", section: "deployments" }),
				field("command", { valueKind: "action", mutability: "LIVE (action)" }),
			]),
			{},
			context,
		);
		expect(result.placement).toEqual({
			identity: { tab: "overview", section: "identity" },
			endpoint: { tab: "overview", section: "endpoints" },
			health: { tab: "operations", section: "health-metrics" },
			rollout: { tab: "operations", section: "deployments" },
			command: { tab: "operations", section: "actions" },
		});
	});
});

describe("canonical navigation and explicit archetypes — NAV-02", () => {
	it("orders all infrastructure tabs and sections canonically, independent of declaration order", () => {
		const tabs: PropertyTabId[] = [
			"logs",
			"operations",
			"configuration",
			"overview",
		];
		const fields = tabs.flatMap((tab) =>
			[...expectedOrder[tab]]
				.reverse()
				.map((section) => placed(`${tab}-${section}`, section, tab)),
		);
		const definition = freeze(panel(fields));
		const result = deriveTabs(
			definition,
			freeze({}),
			freeze({ ...context, cloudWatchDestination: "/synthetic/logs" }),
		);
		expect(result.tabs.map((tab) => tab.id)).toEqual([
			"overview",
			"configuration",
			"operations",
			"logs",
		]);
		for (const tab of result.tabs) {
			expect(tab.sections.map((section) => section.key)).toEqual(
				expectedOrder[tab.id],
			);
			expect(tab.sections.every((section) => section.fieldIds.length > 0)).toBe(
				true,
			);
		}
	});

	it("omits empty sections while retaining semantic placement of dangerous fields", () => {
		const result = deriveTabs(
			panel([
				field("danger", { section: "storage-data", mutability: "TF-REPLACE" }),
				field("first", { section: "naming-lifecycle" }),
			]),
			{},
			context,
		);
		expect(
			result.tabs
				.find((tab) => tab.id === "configuration")
				?.sections.map((section) => section.key),
		).toEqual(["naming-lifecycle", "storage-data"]);
		expect(result.placement.danger.section).toBe("storage-data");
	});

	it.each<SectionArchetype>([
		"container-process",
		"autoscaling-group",
		"registry-strategy",
		"environment-variables",
		"readonly-identity",
		"telemetry-metrics",
		"action-list",
		"standard",
	])(
		"retains explicitly declared %s archetype without label heuristics",
		(archetype) => {
			const definition = panel(
				[field("unrelated-id", { label: "Unrelated label" })],
				{
					sections: [{ key: "source", archetype, fieldIds: ["unrelated-id"] }],
				},
			);
			expect(
				deriveTabs(definition, {}, context).tabs.find(
					(tab) => tab.id === "configuration",
				)?.sections,
			).toEqual([{ key: "source", archetype, fieldIds: ["unrelated-id"] }]);
		},
	);
});

describe("real log destinations and canvas exceptions — NAV-04/05", () => {
	const log = field("retention", {
		section: "observability",
		logRole: "retention",
	});

	it.each([null, ""])(
		"does not fabricate Logs for destination %j",
		(cloudWatchDestination) => {
			const result = deriveTabs(
				panel([log]),
				{},
				{ ...context, cloudWatchDestination },
			);
			expect(result.tabs.map((tab) => tab.id)).toEqual([
				"overview",
				"configuration",
			]);
			expect(result.placement.retention).toEqual({
				tab: "configuration",
				section: "observability",
			});
		},
	);

	it("a declared CloudWatch destination enables explicit log-role placement", () => {
		const result = deriveTabs(
			panel([log]),
			{},
			{ ...context, cloudWatchDestination: "/synthetic/ecs/logs" },
		);
		expect(result.tabs.map((tab) => tab.id)).toEqual(["overview", "logs"]);
		expect(result.placement.retention).toEqual({
			tab: "logs",
			section: "retention",
		});
	});

	it("S3 access logging alone remains configuration, not a CloudWatch viewer", () => {
		const result = deriveTabs(
			panel([
				field("access-logging", {
					section: "observability",
					controlType: "boolean",
				}),
			]),
			{ "access-logging": true },
			context,
		);
		expect(result.tabs.map((tab) => tab.id)).toEqual([
			"overview",
			"configuration",
		]);
	});

	it("derived precedence also outranks an explicit log role", () => {
		const result = deriveTabs(
			panel([{ ...log, valueKind: "derived" }]),
			{},
			{ ...context, cloudWatchDestination: "/synthetic/logs" },
		);
		expect(result.tabs.map((tab) => tab.id)).toEqual(["overview"]);
		expect(result.placement.retention).toEqual({
			tab: "overview",
			section: "relationships",
		});
	});

	it.each(["group", "dynamicGroup"])(
		"%s has only Layout despite distracting destination metadata",
		(id) => {
			const definition = panel(
				[
					field("dimensions", {
						valueKind: "canvas-only",
						mutability: "UI",
						section: "layout",
						controlType: "layout-dimensions",
					}),
				],
				{ id, canvasOnly: true },
			);
			const result = deriveTabs(
				definition,
				{},
				{ ...context, cloudWatchDestination: "/synthetic/not-for-canvas" },
			);
			expect(result.tabs.map((tab) => tab.id)).toEqual(["layout"]);
			expect(result.tabs[0].sections.map((section) => section.key)).toEqual([
				"layout",
			]);
			expect(result.placement.dimensions).toEqual({
				tab: "layout",
				section: "layout",
			});
		},
	);
});
