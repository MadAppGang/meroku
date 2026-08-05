package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ECR repository URI pattern: <account-id>.dkr.ecr.<region>.amazonaws.com/<repo-name>
var ecrURIPattern = regexp.MustCompile(`^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com\/[a-zA-Z0-9_-]+$`)

// validateECRConfig validates ECR configuration for a service/task
// Returns an error if the configuration is invalid
func validateECRConfig(config *ECRConfig, serviceName string, env *Env) error {
	if config == nil {
		// No ECR config means use default (create_ecr)
		return nil
	}

	// Normalize mode to default if empty
	mode := config.Mode
	if mode == "" {
		mode = "create_ecr"
	}

	// Validate mode is one of the allowed values
	if mode != "create_ecr" && mode != "manual_repo" && mode != "use_existing" {
		return fmt.Errorf("service '%s': ecr_config.mode must be 'create_ecr', 'manual_repo', or 'use_existing', got '%s'", serviceName, mode)
	}

	// Mode-specific validation
	switch mode {
	case "create_ecr":
		// No additional validation needed - ECR will be created automatically
		return nil

	case "manual_repo":
		// Require repository_uri and validate format
		if config.RepositoryURI == "" {
			return fmt.Errorf("service '%s': ecr_config.repository_uri is required when mode is 'manual_repo'", serviceName)
		}
		if !ecrURIPattern.MatchString(config.RepositoryURI) {
			return fmt.Errorf("service '%s': ecr_config.repository_uri must be in format '<account-id>.dkr.ecr.<region>.amazonaws.com/<repo-name>', got '%s'", serviceName, config.RepositoryURI)
		}
		return nil

	case "use_existing":
		// Require source_service_name and source_service_type
		if config.SourceServiceName == "" {
			return fmt.Errorf("service '%s': ecr_config.source_service_name is required when mode is 'use_existing'", serviceName)
		}
		if config.SourceServiceType == "" {
			return fmt.Errorf("service '%s': ecr_config.source_service_type is required when mode is 'use_existing'", serviceName)
		}

		// Validate source_service_type
		if config.SourceServiceType != "services" && config.SourceServiceType != "event_processor_tasks" && config.SourceServiceType != "scheduled_tasks" {
			return fmt.Errorf("service '%s': ecr_config.source_service_type must be 'services', 'event_processor_tasks', or 'scheduled_tasks', got '%s'", serviceName, config.SourceServiceType)
		}

		// Validate that source service exists and has create_ecr mode
		if err := validateSourceServiceExists(config.SourceServiceName, config.SourceServiceType, serviceName, env); err != nil {
			return err
		}

		return nil

	default:
		return fmt.Errorf("service '%s': unknown ecr_config.mode '%s'", serviceName, mode)
	}
}

// validateSourceServiceExists checks that the source service exists and uses create_ecr mode
func validateSourceServiceExists(sourceServiceName, sourceServiceType, currentServiceName string, env *Env) error {
	var sourceConfig *ECRConfig
	var found bool

	switch sourceServiceType {
	case "services":
		for _, svc := range env.Services {
			if svc.Name == sourceServiceName {
				found = true
				sourceConfig = svc.ECRConfig
				break
			}
		}

	case "event_processor_tasks":
		for _, task := range env.EventProcessorTasks {
			if task.Name == sourceServiceName {
				found = true
				sourceConfig = task.ECRConfig
				break
			}
		}

	case "scheduled_tasks":
		for _, task := range env.ScheduledTasks {
			if task.Name == sourceServiceName {
				found = true
				sourceConfig = task.ECRConfig
				break
			}
		}

	default:
		return fmt.Errorf("service '%s': invalid source_service_type '%s'", currentServiceName, sourceServiceType)
	}

	if !found {
		return fmt.Errorf("service '%s': source service '%s' not found in %s", currentServiceName, sourceServiceName, sourceServiceType)
	}

	// Validate that source service uses create_ecr mode (default or explicit)
	sourceMode := "create_ecr"
	if sourceConfig != nil && sourceConfig.Mode != "" {
		sourceMode = sourceConfig.Mode
	}

	if sourceMode != "create_ecr" {
		return fmt.Errorf("service '%s': source service '%s' must have ecr_config.mode='create_ecr', but has mode='%s'", currentServiceName, sourceServiceName, sourceMode)
	}

	return nil
}

// validateAppSyncAuth is the single source of truth for "is this AppSync
// authorization configuration deployable".
//
// modules/appsync supports three modes, and each one needs a different field.
// The module states the same requirements as plan-time preconditions, but a
// terraform failure arrives after an init, a provider download and a refresh;
// this arrives in under a second and can name the YAML key to edit.
//
// cognitoEnabled is the *environment's* cognito.enabled, not something inside
// the appsync block: cognito mode wires AppSync to the user pool this
// environment creates, so if that module is not instantiated there is no pool
// id to pass and the generated main.tf would not even parse.
func validateAppSyncAuth(enabled bool, mode string, cognitoEnabled bool, oidcIssuer, jwksURI string, authLambda bool) error {
	return validateAppSyncAuthFull(appSyncAuthInput{
		Enabled:        enabled,
		Mode:           mode,
		CognitoEnabled: cognitoEnabled,
		OIDCIssuer:     oidcIssuer,
		JWKSURI:        jwksURI,
		AuthLambda:     authLambda,
	})
}

// appSyncAuthInput is everything the AppSync checks need. It exists so callers
// do not grow an eighth positional bool.
type appSyncAuthInput struct {
	Enabled        bool
	Mode           string
	CognitoEnabled bool
	OIDCIssuer     string
	JWKSURI        string
	AuthLambda     bool
	RequiredClaims map[string][]string
}

func validateAppSyncAuthFull(in appSyncAuthInput) error {
	if err := validateAppSyncModeRequirements(in); err != nil {
		return err
	}
	return validateAppSyncRequiredClaims(in)
}

// validateAppSyncRequiredClaims refuses a claim policy that nothing will read.
//
// Only the Lambda authorizer can check a claim beyond issuer and audience;
// AMAZON_COGNITO_USER_POOLS and OPENID_CONNECT cannot. Passing required_claims
// in those modes is not a harmless no-op — the operator believes a rule is being
// enforced and it is not, which is precisely the failure mode this module has
// already shipped twice.
func validateAppSyncRequiredClaims(in appSyncAuthInput) error {
	if !in.Enabled || len(in.RequiredClaims) == 0 {
		return nil
	}

	if mode := normalizeAppSyncAuthMode(in.Mode); mode != AppSyncAuthLambda {
		return fmt.Errorf(`pubsub_appsync.required_claims is set but pubsub_appsync.auth_mode is %q, which cannot enforce it

Only the Lambda authorizer checks claims beyond the issuer and the audience.
%s verifies the signature, the issuer and the audience and nothing else, so
these requirements would be silently ignored — you would believe a rule is in
force that is not.

Either enforce them:

  pubsub_appsync:
    auth_mode: "lambda"
    jwks_uri: "https://<your-idp-host>/.well-known/jwks.json"
    required_claims:
      role: ["admin"]

or drop required_claims and keep %q.

If all you need is to accept several audiences, you do not need lambda mode:
  cognito -> cognito_app_id_client_regex: "1F4G9H|1J6L4B"
  oidc    -> oidc_client_id: "1F4G9H|1J6L4B"`,
			mode, appSyncAuthTypeName(mode), mode)
	}

	for claim, accepted := range in.RequiredClaims {
		if strings.TrimSpace(claim) == "" {
			return fmt.Errorf(`pubsub_appsync.required_claims contains an empty claim name

Every entry must name the JWT claim to check, for example:

  required_claims:
    tenant_id: []             # must be present
    role: ["admin", "ops"]    # must be one of these`)
		}
		for _, value := range accepted {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf(`pubsub_appsync.required_claims["%s"] contains an empty accepted value

An empty string is not a value a token can carry. To require only that the claim
is present, give it an empty list instead:

  required_claims:
    %s: []`, claim, claim)
			}
		}
		// `sub` identifies one caller, so a fixed list of accepted values is
		// either a one-user API or a deployment-wide allowlist maintained in
		// Terraform. Neither is authorization. Say so here rather than letting
		// someone build it and find out.
		if claim == "sub" && len(accepted) > 0 {
			return fmt.Errorf(`pubsub_appsync.required_claims must not pin "sub" to a fixed set of values

"sub" identifies the individual caller, so listing accepted values here is a
deployment-wide allowlist of user ids, redeployed on every change — not
authorization.

Check policy claims instead:

  required_claims:
    role: ["admin"]
    tenant_id: []

and make per-user decisions in your resolvers, which read the verified "sub"
from $context.identity.resolverContext.sub. Requiring "sub" to merely be
present is fine: sub: []`)
		}
	}

	return nil
}

// appSyncAuthTypeName maps a mode to the AppSync authentication type it
// deploys, so error messages speak the language of the AWS console as well as
// the language of the YAML.
func appSyncAuthTypeName(mode string) string {
	switch normalizeAppSyncAuthMode(mode) {
	case AppSyncAuthCognito:
		return "AMAZON_COGNITO_USER_POOLS"
	case AppSyncAuthOIDC:
		return "OPENID_CONNECT"
	case AppSyncAuthLambda:
		return "AWS_LAMBDA"
	default:
		return mode
	}
}

func validateAppSyncModeRequirements(in appSyncAuthInput) error {
	enabled, mode, cognitoEnabled := in.Enabled, in.Mode, in.CognitoEnabled
	oidcIssuer, jwksURI, authLambda := in.OIDCIssuer, in.JWKSURI, in.AuthLambda

	if !enabled {
		return nil
	}

	switch normalizeAppSyncAuthMode(mode) {
	case AppSyncAuthCognito:
		if !cognitoEnabled {
			return fmt.Errorf(`pubsub_appsync.auth_mode is "cognito" but cognito.enabled is false

Cognito mode points AppSync at the user pool this environment creates, so the
pool has to exist. Either enable it in your <env>.yaml:

  cognito:
    enabled: true

or pick a mode that does not need one:

  pubsub_appsync:
    auth_mode: "oidc"     # AWS verifies tokens against your issuer's JWKS
    oidc_issuer: "https://<your-idp-host>"

  pubsub_appsync:
    auth_mode: "lambda"   # runs the bundled authorizer against jwks_uri
    jwks_uri: "https://<your-idp-host>/.well-known/jwks.json"`)
		}
		return nil

	case AppSyncAuthOIDC:
		oidcIssuer = strings.TrimSpace(oidcIssuer)
		if oidcIssuer == "" {
			return fmt.Errorf(`pubsub_appsync.oidc_issuer is required when pubsub_appsync.auth_mode is "oidc"

AppSync fetches this issuer's OIDC discovery document and its signing keys, and
accepts every token they validate. There is no default: whoever controls that
URL can mint tokens your API accepts, so meroku will not pick one for you.

Set it in your <env>.yaml:

  pubsub_appsync:
    auth_mode: "oidc"
    oidc_issuer: "https://<your-idp-host>"

Optionally pin the audience, which AppSync spells client_id:

    oidc_client_id: "<your-app-client-id>"`)
		}
		if !strings.HasPrefix(oidcIssuer, "https://") {
			return fmt.Errorf(`pubsub_appsync.oidc_issuer must be an https:// URL, got %q

Over plaintext http an attacker on the network can swap the discovery document
and the signing keys, and mint tokens your API accepts`, oidcIssuer)
		}
		return nil

	case AppSyncAuthLambda:
		return validateAppSyncAuthorizer(enabled, authLambda, jwksURI)

	default:
		return fmt.Errorf(`pubsub_appsync.auth_mode must be one of %s, got %q

  "cognito" - AWS validates the token against this environment's Cognito user
              pool. No Lambda runs.
  "oidc"    - AWS validates the token against pubsub_appsync.oidc_issuer. No
              Lambda runs.
  "lambda"  - the bundled authorizer verifies RS256 JWTs against
              pubsub_appsync.jwks_uri. Use this only when neither native mode
              can express your provider; it adds a Lambda invocation to
              uncached requests.

Leave it unset to keep the previous behaviour, which is %q.`,
			strings.Join(quoteAll(appSyncAuthModes), ", "), mode, AppSyncAuthLambda)
	}
}

// quoteAll renders a list of allowed values the way the error messages show
// them, so the set and its presentation cannot drift apart.
func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return quoted
}

// validateAppSyncAuthorizer is the single source of truth for "is this AppSync
// Lambda authorizer safe to deploy".
//
// The AppSync Lambda authorizer trusts exactly one thing: the JWKS endpoint it
// fetches signing keys from. It used to fall back to a hardcoded third-party
// endpoint, so an unconfigured API accepted tokens minted by a stranger.
// modules/appsync/variables.tf now declares jwks_uri with no default; this check
// turns the resulting terraform failure into an actionable message at generate
// time, before anyone waits on an apply.
//
// The requirement is keyed on `enabled`, not on `auth_lambda`: `auth_lambda`
// only chooses whose authorizer *source* is packaged (the project's own tree
// versus the module's), so lambda mode needs a key source either way. authLambda
// only sharpens the wording.
//
// Reached only for auth_mode = "lambda"; validateAppSyncAuth dispatches. In
// cognito and oidc mode no authorizer is built at all, so jwks_uri is not just
// unnecessary, it is unread.
func validateAppSyncAuthorizer(enabled, authLambda bool, jwksURI string) error {
	if !enabled {
		return nil
	}

	jwksURI = strings.TrimSpace(jwksURI)

	if jwksURI == "" {
		trigger := "pubsub_appsync.enabled is true"
		if authLambda {
			trigger = "pubsub_appsync.auth_lambda is true"
		}
		return fmt.Errorf(`pubsub_appsync.jwks_uri is required when %s and pubsub_appsync.auth_mode is "lambda"

The AppSync Lambda authorizer verifies every JWT against this JWKS endpoint, and
it has no default: whoever controls that endpoint can mint tokens your API will
accept, so meroku will not pick one for you.

Set it in your <env>.yaml:

  pubsub_appsync:
    auth_mode: "lambda"
    jwks_uri: "https://<your-idp-host>/.well-known/jwks.json"

Optionally pin the claims your provider issues:

    jwt_issuer: "https://<your-idp-host>/"
    jwt_audience: "<your-api-audience>"

Before you do: lambda mode exists for providers the native modes cannot express,
and it puts a Lambda invocation on the request path. If your provider publishes
an OIDC discovery document, prefer

  pubsub_appsync:
    auth_mode: "oidc"
    oidc_issuer: "https://<your-idp-host>"

and if this environment already has a Cognito user pool, prefer

  pubsub_appsync:
    auth_mode: "cognito"

In both cases AWS verifies the token itself. Or set pubsub_appsync.enabled: false
if you do not need AppSync in this environment.`, trigger)
	}

	// Mirrors the module's own validation, but fails here with an explanation
	// instead of a terraform plan error.
	if !strings.HasPrefix(jwksURI, "https://") {
		return fmt.Errorf(`pubsub_appsync.jwks_uri must be an https:// URL, got %q

Over plaintext http an attacker on the network can swap the signing keys and
mint tokens your API accepts`, jwksURI)
	}

	return nil
}

// validateAppSyncConfig validates the AppSync block of a loaded Env struct.
func validateAppSyncConfig(env *Env) error {
	if env == nil {
		return nil
	}
	a := env.AppSyncPubSub
	return validateAppSyncAuthFull(appSyncAuthInput{
		Enabled:        a.Enabled,
		Mode:           a.AuthMode,
		CognitoEnabled: env.Cognito.Enabled,
		OIDCIssuer:     a.OIDCIssuer,
		JWKSURI:        a.JWKSURI,
		AuthLambda:     a.AuthLambda,
		RequiredClaims: a.RequiredClaims,
	})
}

// yamlSubMap pulls a nested section out of the raw template map.
//
// YAML gives map[interface{}]interface{}; convertToJSONCompatible gives
// map[string]interface{}. Both are accepted so a check cannot be bypassed by
// whichever loader the caller used.
func yamlSubMap(envMap map[string]interface{}, key string) (map[string]interface{}, bool) {
	raw, exists := envMap[key]
	if !exists || raw == nil {
		return nil, false
	}

	switch m := raw.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			if ks, ok := k.(string); ok {
				out[ks] = v
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// validateAppSyncConfigMap validates the AppSync block of the raw map that is
// handed to the template engine. Generation renders from the map, not the
// struct, so this is the check that actually stands between a misconfigured
// project and a generated main.tf.
func validateAppSyncConfigMap(envMap map[string]interface{}) error {
	appsync, ok := yamlSubMap(envMap, "pubsub_appsync")
	if !ok {
		return nil
	}

	enabled, _ := appsync["enabled"].(bool)
	authMode, _ := appsync["auth_mode"].(string)
	authLambda, _ := appsync["auth_lambda"].(bool)
	oidcIssuer, _ := appsync["oidc_issuer"].(string)
	jwksURI, _ := appsync["jwks_uri"].(string)

	// Cognito mode reaches outside the appsync block: it wires AppSync to the
	// pool `module "cognito"` creates, and that module is only emitted when
	// cognito.enabled is true.
	cognitoEnabled := false
	if cognito, ok := yamlSubMap(envMap, "cognito"); ok {
		cognitoEnabled, _ = cognito["enabled"].(bool)
	}

	return validateAppSyncAuthFull(appSyncAuthInput{
		Enabled:        enabled,
		Mode:           authMode,
		CognitoEnabled: cognitoEnabled,
		OIDCIssuer:     oidcIssuer,
		JWKSURI:        jwksURI,
		AuthLambda:     authLambda,
		RequiredClaims: yamlClaimMap(appsync["required_claims"]),
	})
}

// yamlClaimMap coerces required_claims out of raw YAML.
//
// A malformed shape yields an entry the checks above reject by name, rather than
// being dropped: a claim policy that vanishes because it was written as a string
// instead of a list is exactly the silent-no-op this validation exists to
// prevent. Values are stringified because JWT claims are compared as strings.
func yamlClaimMap(raw interface{}) map[string][]string {
	var entries map[string]interface{}
	switch m := raw.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		entries = m
	case map[interface{}]interface{}:
		entries = make(map[string]interface{}, len(m))
		for k, v := range m {
			entries[fmt.Sprintf("%v", k)] = v
		}
	default:
		// Not a mapping at all (a string, a list). Surface it as one unnamed
		// entry so validation complains instead of ignoring it.
		return map[string][]string{"": nil}
	}

	claims := make(map[string][]string, len(entries))
	for name, raw := range entries {
		var accepted []string
		switch values := raw.(type) {
		case nil:
			accepted = nil
		case []interface{}:
			for _, v := range values {
				accepted = append(accepted, fmt.Sprintf("%v", v))
			}
		default:
			// A scalar where a list belongs. Keep it, so the "empty accepted
			// value" / mode checks still see the claim rather than losing it.
			accepted = []string{fmt.Sprintf("%v", values)}
		}
		claims[name] = accepted
	}
	return claims
}

// ValidateAllECRConfigs validates ECR configurations for all services, event processors, and scheduled tasks
func ValidateAllECRConfigs(env *Env) error {
	var errors []string

	// Validate services
	for _, svc := range env.Services {
		if err := validateECRConfig(svc.ECRConfig, svc.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Validate event processor tasks
	for _, task := range env.EventProcessorTasks {
		if err := validateECRConfig(task.ECRConfig, task.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Validate scheduled tasks
	for _, task := range env.ScheduledTasks {
		if err := validateECRConfig(task.ECRConfig, task.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("ECR configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}
