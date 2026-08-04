/**
 * Minimal ambient types for Bun's test runner (`task test:web` runs `bun test`).
 *
 * The web package does not depend on `@types/bun`, and pulling it in would drag
 * Bun's full global runtime typings into a browser-targeted tsconfig. This
 * declares only the surface our tests use so `tsc --noEmit` stays clean.
 */
declare module "bun:test" {
	export interface Matchers {
		toBe(expected: unknown): void;
		toEqual(expected: unknown): void;
		toBeDefined(): void;
		toBeUndefined(): void;
		toBeTruthy(): void;
		toBeFalsy(): void;
		toContain(expected: unknown): void;
		toMatch(expected: string | RegExp): void;
		toThrow(expected?: string | RegExp): void;
		readonly not: Matchers;
	}

	export function expect(value: unknown): Matchers;
	export function describe(label: string, fn: () => void): void;
	export function test(label: string, fn: () => void | Promise<void>): void;
	export function it(label: string, fn: () => void | Promise<void>): void;
	export function beforeEach(fn: () => void | Promise<void>): void;
	export function afterEach(fn: () => void | Promise<void>): void;
	export function beforeAll(fn: () => void | Promise<void>): void;
	export function afterAll(fn: () => void | Promise<void>): void;
}
