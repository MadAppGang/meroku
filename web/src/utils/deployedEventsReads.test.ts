import { describe, expect, test } from "bun:test";
import { deployedEventsReads } from "./deployedEventsReads";
import type { DeployedEvents } from "./workloadEvents";

function deployed(fifoConsumers: string[] = []): DeployedEvents {
	return {
		known: true,
		fifoConsumers,
		sharedPrincipals: [],
		subscribePrincipals: [],
	};
}

/** A load whose answers the test hands out, in the order reads started. */
function controlledLoad() {
	const pending: {
		env: string;
		resolve: (v: DeployedEvents) => void;
		reject: (e: Error) => void;
	}[] = [];
	const load = (env: string) =>
		new Promise<DeployedEvents>((resolve, reject) => {
			pending.push({ env, resolve, reject });
		});
	return { load, pending };
}

describe("deployedEventsReads", () => {
	test("a decision read keeps its answer when a refresh started after it", async () => {
		const { load, pending } = controlledLoad();
		const published: (DeployedEvents | null)[] = [];
		const read = deployedEventsReads(
			load,
			() => "dev",
			(_, v) => published.push(v),
		);
		const decision = read("dev", new AbortController().signal);
		const refresh = read("dev", new AbortController().signal);
		pending[0].resolve(deployed());
		pending[1].resolve(deployed(["orders"]));
		expect(await decision).toEqual(deployed());
		expect(await refresh).toEqual(deployed(["orders"]));
		// Only the latest read started publishes.
		expect(published).toEqual([deployed(["orders"])]);
	});

	test("an older read finishing last does not replace the newer answer", async () => {
		const { load, pending } = controlledLoad();
		const published: (DeployedEvents | null)[] = [];
		const read = deployedEventsReads(
			load,
			() => "dev",
			(_, v) => published.push(v),
		);
		const older = read("dev", new AbortController().signal);
		const newer = read("dev", new AbortController().signal);
		pending[1].resolve(deployed(["orders"]));
		await newer;
		pending[0].resolve(deployed());
		expect(await older).toEqual(deployed());
		expect(published).toEqual([deployed(["orders"])]);
	});

	test("an aborted read answers unknown and publishes nothing", async () => {
		const { load, pending } = controlledLoad();
		const published: (DeployedEvents | null)[] = [];
		const read = deployedEventsReads(
			load,
			() => "dev",
			(_, v) => published.push(v),
		);
		const controller = new AbortController();
		const result = read("dev", controller.signal);
		controller.abort();
		pending[0].resolve(deployed());
		expect(await result).toBeUndefined();
		expect(published).toEqual([]);
	});

	test("a read of an env no longer shown answers unknown", async () => {
		const { load, pending } = controlledLoad();
		let shown = "dev";
		const read = deployedEventsReads(
			load,
			() => shown,
			() => {},
		);
		const result = read("dev", new AbortController().signal);
		shown = "prod";
		pending[0].resolve(deployed());
		expect(await result).toBeUndefined();
	});

	test("a failed read answers unknown and, as the latest, publishes null", async () => {
		const { load, pending } = controlledLoad();
		const published: (DeployedEvents | null)[] = [];
		const read = deployedEventsReads(
			load,
			() => "dev",
			(_, v) => published.push(v),
		);
		const result = read("dev", new AbortController().signal);
		const error = console.error;
		console.error = () => {};
		pending[0].reject(new Error("state unreadable"));
		expect(await result).toBeUndefined();
		console.error = error;
		expect(published).toEqual([null]);
	});
});
