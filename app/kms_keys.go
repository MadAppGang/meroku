package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// KMS signing and HMAC keys (modules/kms_keys). The key_spec decides the kind:
// an HMAC_* spec is a GENERATE_VERIFY_MAC key, every other spec an asymmetric
// SIGN_VERIFY key. A workload connected to a key may sign, verify or both (on an
// HMAC key: generate or verify a MAC), and gets the key's ARN as a variable,
// plus its public key when the key is asymmetric:
//
//	kms_keys:
//	  - name: auth                 # KMS_AUTH_KEY_ARN, KMS_AUTH_PUBLIC_KEY
//	    key_spec: ECC_NIST_P256    # the default
//	  - name: requests             # KMS_REQUESTS_KEY_ARN only
//	    key_spec: HMAC_256
//	  - name: legacy
//	    retired: true              # still deployed; nothing may connect to it
//	services:
//	  - name: api
//	    kms_keys:
//	      - { name: auth, access: sign }
//
// The backend connects under workload.kms_keys, tasks under their own kms_keys
// key. See ai_docs/KMS_KEYS.md.

// kmsKeysKey is the envMap key holding every declared key, retired ones
// included, so a retired key stays deployed: name -> {key_spec}.
const kmsKeysKey = "_kms_keys"

// kmsConnectionsKey is the envMap key holding every connection the template
// renders, one entry per workload and key: {workload, key, access}.
const kmsConnectionsKey = "_kms_connections"

// kmsKeyNamePattern matches modules/kms_keys' validation of a key name. The
// name is upper-cased into the variable names and is part of the alias.
var kmsKeyNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// kmsDefaultKeySpec is the key type of a key without key_spec.
const kmsDefaultKeySpec = "ECC_NIST_P256"

// kmsKeySpecs are the key types meroku accepts, in the order the messages list
// them: the SIGN_VERIFY types, then the GENERATE_VERIFY_MAC (HMAC) types.
var kmsKeySpecs = []string{
	"ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_P521", "ECC_SECG_P256K1",
	"RSA_2048", "RSA_3072", "RSA_4096",
	"HMAC_224", "HMAC_256", "HMAC_384", "HMAC_512",
}

// kmsIsHMACKeySpec reports whether a key type is an HMAC one. An HMAC key has
// no public key, so a workload connected to it gets only the key's ARN.
func kmsIsHMACKeySpec(spec string) bool { return strings.HasPrefix(spec, "HMAC_") }

const (
	kmsAccessSign   = "sign"
	kmsAccessVerify = "verify"
	kmsAccessBoth   = "both"
)

// kmsDeclaredKey is one validated entry of the top-level kms_keys list.
type kmsDeclaredKey struct {
	keySpec string
	retired bool
}

// kmsKeyArnEnvName and kmsPublicKeyEnvName are the variables a key gives a
// connected workload; an HMAC key gives only the first.
func kmsKeyArnEnvName(key string) string { return "KMS_" + strings.ToUpper(key) + "_KEY_ARN" }

func kmsPublicKeyEnvName(key string) string { return "KMS_" + strings.ToUpper(key) + "_PUBLIC_KEY" }

// declaredKMSKeys reads the top-level kms_keys list. A key whose own fields
// have a problem is still returned, so a connection to it is not also reported
// as a connection to an unknown key.
func declaredKMSKeys(envMap map[string]interface{}) (map[string]kmsDeclaredKey, []string) {
	keys := map[string]kmsDeclaredKey{}
	raw, present := envMap["kms_keys"]
	if !present || raw == nil {
		return keys, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return keys, []string{"kms_keys must be a list of { name: <key>, key_spec: <type>, retired: <bool> }"}
	}
	var problems []string
	first := map[string]int{}
	for i, entryRaw := range list {
		where := fmt.Sprintf("kms_keys[%d]", i)
		entry, ok := computeNodeMap(entryRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with a name", where))
			continue
		}
		name := stringFromMap(entry, "name")
		switch {
		case name == "":
			problems = append(problems, fmt.Sprintf("%s has no name", where))
			continue
		case !kmsKeyNamePattern.MatchString(name):
			problems = append(problems, fmt.Sprintf(
				"%s name %q: use 1 to 31 lower-case letters, digits and underscores, starting with a letter", where, name))
			continue
		}
		if at, dup := first[name]; dup {
			problems = append(problems, fmt.Sprintf("%s name %q is already used by kms_keys[%d]", where, name, at))
			continue
		}
		first[name] = i

		key := kmsDeclaredKey{keySpec: kmsDefaultKeySpec}
		if rawSpec, has := entry["key_spec"]; has && rawSpec != nil {
			s, isString := rawSpec.(string)
			switch {
			case isString && s == "":
			case isString && containsString(kmsKeySpecs, s):
				key.keySpec = s
			default:
				problems = append(problems, fmt.Sprintf(
					"%s key_spec %s: use one of %s", where, formatYAMLValue(rawSpec), strings.Join(kmsKeySpecs, ", ")))
			}
		}
		if rawRetired, has := entry["retired"]; has && rawRetired != nil {
			b, isBool := rawRetired.(bool)
			if !isBool {
				problems = append(problems, fmt.Sprintf("%s retired %s: use true or false", where, formatYAMLValue(rawRetired)))
			}
			key.retired = b
		}
		keys[name] = key
	}
	return keys, problems
}

// kmsConnection is one validated connection.
type kmsConnection struct {
	workload string
	key      string
	access   string
}

// validateKMSKeysConfigMap checks the kms_keys list and every workload's
// connections, and writes every declared key under kmsKeysKey and the
// connections under kmsConnectionsKey.
//
// Errors: a malformed or duplicate key name; an unknown key_spec; a retired
// value that is not a boolean; a connection to a key that is not declared or is
// retired; an access other than sign, verify or both; the same key twice on one
// workload. A YAML variable with the name of one a key gives the workload is
// refused afterwards by validateWorkloadReservedEnvConfigMap, which reads
// kmsReservedWorkloadEnv.
func validateKMSKeysConfigMap(envMap map[string]interface{}) error {
	envMap[kmsKeysKey] = map[string]interface{}{}
	envMap[kmsConnectionsKey] = []interface{}{}

	keys, problems := declaredKMSKeys(envMap)
	var connections []kmsConnection

	for _, src := range workloadConnectionSources(envMap, "kms_keys") {
		if src.raw == nil {
			continue
		}
		list, ok := src.raw.([]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a list of { name, access }", src.field))
			continue
		}
		seen := map[string]int{}
		for i, entryRaw := range list {
			where := fmt.Sprintf("%s[%d]", src.field, i)
			entry, ok := computeNodeMap(entryRaw)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s must be a mapping with name and access", where))
				continue
			}
			name := stringFromMap(entry, "name")
			if name == "" {
				problems = append(problems, fmt.Sprintf("%s has no kms key name", where))
				continue
			}
			key, declared := keys[name]
			if !declared {
				problems = append(problems, fmt.Sprintf(
					"%s connects kms key %q, which is not in the top-level kms_keys list; add it under kms_keys first", where, name))
				continue
			}
			if key.retired {
				problems = append(problems, fmt.Sprintf(
					"%s: kms key %q is retired: remove the connection, or set retired: false", where, name))
				continue
			}
			if at, dup := seen[name]; dup {
				problems = append(problems, fmt.Sprintf(
					"%s connects kms key %q, already connected at index %d; connect a key once per workload (both gives sign and verify)", where, name, at))
				continue
			}
			seen[name] = i

			access, _ := entry["access"].(string)
			switch access {
			case kmsAccessSign, kmsAccessVerify, kmsAccessBoth:
			default:
				problems = append(problems, fmt.Sprintf(
					"%s access %s: use sign, verify or both", where, formatYAMLValue(entry["access"])))
				continue
			}

			connections = append(connections, kmsConnection{workload: src.workload, key: name, access: access})
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("kms_keys:\n  %s", strings.Join(problems, "\n  "))
	}

	rendered := make(map[string]interface{}, len(keys))
	for name, key := range keys {
		rendered[name] = map[string]interface{}{"key_spec": key.keySpec}
	}
	envMap[kmsKeysKey] = rendered

	list := make([]interface{}, 0, len(connections))
	for _, c := range connections {
		list = append(list, map[string]interface{}{
			"workload": c.workload,
			"key":      c.key,
			"access":   c.access,
		})
	}
	envMap[kmsConnectionsKey] = list
	return nil
}

// kmsReservedWorkloadEnv lists the variables the normalised connections give
// each workload, as rows of the reserved-env table: a workload's YAML variable
// or vault secret of the same name is refused by the checks that read the
// table. KMS_<NAME>_PUBLIC_KEY is a row only for an asymmetric key: an HMAC key
// has no public key. Empty until validateKMSKeysConfigMap has run, which
// preprocessEnvMap does before those checks.
func kmsReservedWorkloadEnv(envMap map[string]interface{}) []reservedWorkloadEnv {
	keys, _ := envMap[kmsKeysKey].(map[string]interface{})
	connections, _ := envMap[kmsConnectionsKey].([]interface{})
	var rows []reservedWorkloadEnv
	for _, raw := range connections {
		c, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		workload := stringFromMap(c, "workload")
		key := stringFromMap(c, "key")
		applies := func(ctx workloadEnvContext) bool { return ctx.name == workload }
		setBy := fmt.Sprintf("kms key %q", key)
		rows = append(rows, reservedWorkloadEnv{name: kmsKeyArnEnvName(key), setBy: setBy, appliesTo: applies})
		spec := kmsDefaultKeySpec
		if declared, ok := computeNodeMap(keys[key]); ok {
			spec = stringFromMap(declared, "key_spec")
		}
		if !kmsIsHMACKeySpec(spec) {
			rows = append(rows, reservedWorkloadEnv{name: kmsPublicKeyEnvName(key), setBy: setBy, appliesTo: applies})
		}
	}
	return rows
}

// moveServiceKMSKeysOut deletes each service's kms_keys key once it is
// normalised under kmsConnectionsKey (see deleteServiceKey).
func moveServiceKMSKeysOut(envMap map[string]interface{}) {
	deleteServiceKey(envMap, "kms_keys")
}

// kmsKeysBridgeBlock is the kms part of local.bridge (generateBridgeFile): each
// key's ARN by name, {} when no key is declared.
func kmsKeysBridgeBlock(envMap map[string]interface{}) string {
	block := "\n    # KMS signing keys (kms_keys), ARN by key name\n"
	if keys, _ := envMap[kmsKeysKey].(map[string]interface{}); len(keys) > 0 {
		return block + "    kms_key_arns = module.kms_keys.key_arns\n"
	}
	return block + "    kms_key_arns = {}\n"
}
