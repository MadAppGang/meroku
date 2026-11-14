package main

import (
	"strings"
	"testing"
)

func TestQACapabilityVaultsValidateDeclarationsAndConnections(t *testing.T) {
	for _, name := range []string{"a", "0", "shared-vault", "shared_vault", strings.Repeat("a", 32)} {
		t.Run("valid name "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": name})
			qaCapabilityOwner(m, "backend")["vaults"] = qaCapabilityList(qaCapabilityMap{"name": name})
			qaCapabilityNoError(t, validateVaultsConfigMap(m))
		})
	}
	for _, name := range []string{"", "-a", "a-", "_a", "a_", "A", "a/b", strings.Repeat("a", 33)} {
		t.Run("invalid name "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": name})
			qaCapabilityRequireError(t, validateVaultsConfigMap(m), "vaults", "name")
		})
	}
	t.Run("duplicate declarations", func(t *testing.T) {
		m := qaCapabilityEnv()
		m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"}, qaCapabilityMap{"name": "shared"})
		qaCapabilityRequireError(t, validateVaultsConfigMap(m), "shared")
	})
	invalid := []struct {
		name, field string
		connections []interface{}
	}{
		{"unknown vault", "missing", qaCapabilityList(qaCapabilityMap{"name": "missing"})},
		{"same vault twice", "shared", qaCapabilityList(qaCapabilityMap{"name": "shared"}, qaCapabilityMap{"name": "shared", "prefix": "OTHER"})},
		{"invalid access", "access", qaCapabilityList(qaCapabilityMap{"name": "shared", "access": "write"})},
		{"lowercase prefix", "prefix", qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "pay"})},
		{"digit first", "prefix", qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "1PAY"})},
		{"underscore first", "prefix", qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "_PAY"})},
		{"hyphen in prefix", "prefix", qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "PAY-A"})},
	}
	for _, kind := range qaCapabilityKinds() {
		for _, tt := range invalid {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				m := qaCapabilityEnv()
				m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
				qaCapabilityOwner(m, kind)["vaults"] = qaCapabilityCloneValue(tt.connections)
				qaCapabilityRequireError(t, validateVaultsConfigMap(m), tt.field)
			})
		}
		for _, access := range []string{"", "read", "read_write"} {
			for _, prefix := range []string{"", "PAY", "PAY_1"} {
				t.Run(kind+"/valid "+access+"/"+prefix, func(t *testing.T) {
					m := qaCapabilityEnv()
					m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
					connection := qaCapabilityMap{"name": "shared"}
					if access != "" {
						connection["access"] = access
					}
					if prefix != "" {
						connection["prefix"] = prefix
					}
					qaCapabilityOwner(m, kind)["vaults"] = qaCapabilityList(connection)
					qaCapabilityNoError(t, validateVaultsConfigMap(m))
				})
			}
		}
	}
}
