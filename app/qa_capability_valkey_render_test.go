package main

import "testing"

func TestQACapabilityValkeyRendersOnlyEnabledAccess(t *testing.T) {
	for _, mode := range []string{"absent", "disabled", "defaults", "configured"} {
		t.Run(mode, func(t *testing.T) {
			m := qaCapabilityEnv()
			switch mode {
			case "absent":
				delete(m, "valkey")
			case "disabled":
				m["valkey"] = qaCapabilityMap{"enabled": false}
			case "defaults":
				m["valkey"] = qaCapabilityMap{"enabled": true, "access": []interface{}{"api"}}
			case "configured":
				m["valkey"] = qaCapabilityMap{"enabled": true, "engine_version": "9", "max_storage_gb": 3, "access": []interface{}{"backend", "nightly", "mailer"}}
			}
			rendered := renderMainHBS(t, m)
			body := qaCapabilityParse(t, rendered)
			cache := qaCapabilityFindModule(body, "valkey")
			if mode == "absent" || mode == "disabled" {
				if cache != nil {
					count := cache.Attributes["count"]
					if count == nil {
						t.Fatal("disabled cache renders an active module")
					}
					qaCapabilityEqual(t, qaCapabilityJSON(t, count.Expr, nil), float64(0))
				}
				return
			}
			if cache == nil {
				t.Fatal("enabled cache module is missing")
			}
			version, cap := "8", float64(1)
			access := []string{"api"}
			if mode == "configured" {
				version, cap, access = "9", float64(3), []string{"backend", "nightly", "mailer"}
			}
			qaCapabilityEqual(t, qaCapabilityLiteral(t, cache, "engine_version"), version)
			qaCapabilityEqual(t, qaCapabilityLiteral(t, cache, "max_storage_gb"), cap)
			qaCapabilityStringSet(t, qaCapabilityLiteral(t, cache, "access"), access...)
			qaCapabilityAttr(t, cache, "workloads")
		})
	}
}
