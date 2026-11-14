import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * The environment's domain as module "domain" in env/main.hbs builds it, and as
 * amplifyEnvDomain in app/amplify_rewrites.go mirrors it: a delegated subdomain
 * (`domain.root_zone_id` set) is always `<env>.<domain>`; otherwise
 * `add_env_domain_prefix`, which defaults to true, decides. Empty when no
 * domain name is set.
 */
export function envDomainName(config: YamlInfrastructureConfig): string {
	const domain = config.domain;
	const name = domain?.domain_name ?? "";
	if (!name) return "";
	const addEnvPrefix = domain?.add_env_domain_prefix ?? true;
	if (domain?.root_zone_id || addEnvPrefix) {
		return `${config.env || "dev"}.${name}`;
	}
	return name;
}

/**
 * The host an Amplify app is served on: its custom domain, else
 * `<subdomain_prefix>.<env domain>`. Empty when the app has only its
 * *.amplifyapp.com host.
 */
export function amplifyAppHost(
	config: YamlInfrastructureConfig,
	app: { subdomain_prefix?: string; custom_domain?: string },
): string {
	if (app.custom_domain) return app.custom_domain.replace(/\.$/, "");
	const envDomain = envDomainName(config);
	if (!app.subdomain_prefix || !envDomain) return "";
	return `${app.subdomain_prefix}.${envDomain}`;
}

/**
 * The API host a workload rewrite targets: the API domain in ALB mode, and in
 * API Gateway mode when an API prefix is set. Null means the API Gateway
 * execute-api endpoint, which is only known after deploy and is never an
 * Amplify app's host.
 */
export function workloadApiHost(
	config: YamlInfrastructureConfig,
): string | null {
	const domain = config.domain;
	if (!domain?.enabled) return null;
	const prefix = domain.api_domain_prefix ?? "";
	if (config.alb?.enabled !== true && !prefix) return null;
	const envDomain = envDomainName(config);
	if (!envDomain) return null;
	return prefix ? `${prefix}.${envDomain}` : envDomain;
}
