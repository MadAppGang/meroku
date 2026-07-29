import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * The Route53 domain resolved by modules/domain.
 *
 * Production never receives an environment prefix. Non-production environments
 * with delegated DNS always do; the standard path follows add_env_domain_prefix.
 */
export function getEnvironmentDomain(config: YamlInfrastructureConfig): string {
	if (!config.domain?.enabled || !config.domain.domain_name) {
		return "";
	}

	const baseDomain = config.domain.domain_name;
	if (config.is_prod) {
		return baseDomain;
	}

	const usesDelegatedEnvironmentZone = Boolean(config.domain.root_zone_id);
	const addEnvironmentPrefix =
		usesDelegatedEnvironmentZone ||
		(config.domain.add_env_domain_prefix ?? true);

	return addEnvironmentPrefix ? `${config.env}.${baseDomain}` : baseDomain;
}

/**
 * The public API hostname for an environment.
 *
 * This is the same on both ingress paths: API Gateway and the ALB are mutually exclusive,
 * and whichever is active serves this hostname. Toggling `alb.enabled` does not change it.
 *
 * Keep this in step with env/main.hbs and modules/domain/main.tf.
 */
export function getApiDomainUrl(config: YamlInfrastructureConfig): string {
	const environmentDomain = getEnvironmentDomain(config);
	if (!environmentDomain) {
		return "";
	}

	const apiPrefix = config.domain?.api_domain_prefix?.trim();
	return apiPrefix ? `${apiPrefix}.${environmentDomain}` : environmentDomain;
}

/**
 * Optional extra ALB hostname. backend_alb_domain_name is stored as a prefix in YAML;
 * workloads/alb.tf qualifies it with the environment-resolved domain.
 */
export function getAdditionalAlbDomain(
	config: YamlInfrastructureConfig,
): string {
	const prefix = config.workload?.backend_alb_domain_name?.trim();
	const environmentDomain = getEnvironmentDomain(config);

	return prefix && environmentDomain ? `${prefix}.${environmentDomain}` : "";
}
