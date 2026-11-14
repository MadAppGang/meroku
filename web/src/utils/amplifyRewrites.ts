import type { AmplifyRewriteRule } from "../types/yamlConfig";

export const AMPLIFY_WILDCARD = "<*>";
export const AMPLIFY_BACKEND = "backend";

/** Mirrors amplifyTargetPath in app/amplify_rewrites.go. */
export function amplifyTargetPath(rule: AmplifyRewriteRule): string {
	if (rule.path) return rule.path;
	const workload = rule.workload ?? "";
	if (workload === AMPLIFY_BACKEND) return rule.source;
	const prefix = `/${workload}`;
	if (rule.source === prefix) return "";
	if (rule.source.startsWith(`${prefix}/`)) {
		return rule.source.slice(prefix.length);
	}
	return rule.source;
}

/**
 * The routed service a backend target path reaches instead of the backend, or
 * null. Mirrors amplifyShadowingService in app/amplify_rewrites.go: the API
 * Gateway and the ALB send /<service>/... to a public or full_open service
 * before the backend's catch-all, so /<service>, /<service>/... and
 * /<service><*> never reach the backend.
 */
export function shadowingService(
	targetPath: string,
	routedServices: string[],
): string | null {
	for (const name of [...routedServices].sort()) {
		const prefix = `/${name}`;
		if (
			targetPath === prefix ||
			targetPath === `${prefix}${AMPLIFY_WILDCARD}` ||
			targetPath.startsWith(`${prefix}/`)
		) {
			return name;
		}
	}
	return null;
}
