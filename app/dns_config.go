package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

const DNSConfigFile = "dns.yaml"

func loadDNSConfig() (*DNSConfig, error) {
	var config DNSConfig

	data, err := os.ReadFile(DNSConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading DNS config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling DNS config: %v", err)
	}

	return &config, nil
}

func saveDNSConfig(config *DNSConfig) error {
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling DNS config: %v", err)
	}

	err = os.WriteFile(DNSConfigFile, yamlData, 0o644)
	if err != nil {
		return fmt.Errorf("error writing DNS config file: %v", err)
	}

	return nil
}

func findDelegatedZone(config *DNSConfig, subdomain string) *DelegatedZone {
	if config == nil {
		return nil
	}

	for i := range config.DelegatedZones {
		if config.DelegatedZones[i].Subdomain == subdomain {
			return &config.DelegatedZones[i]
		}
	}
	return nil
}

func removeDelegatedZone(config *DNSConfig, subdomain string) bool {
	if config == nil {
		return false
	}

	for i, zone := range config.DelegatedZones {
		if zone.Subdomain == subdomain {
			config.DelegatedZones = append(config.DelegatedZones[:i], config.DelegatedZones[i+1:]...)
			return true
		}
	}
	return false
}

// findParentZone returns the remembered profile for a parent domain, if any.
//
// Matching is case-insensitive and ignores a trailing dot so that a zone
// recorded as "coretechx.dev." still matches a lookup for "coretechx.dev".
func findParentZone(config *DNSConfig, domain string) *ParentZoneRef {
	if config == nil {
		return nil
	}

	want := normalizeDomain(domain)
	for i := range config.ParentZones {
		if normalizeDomain(config.ParentZones[i].Domain) == want {
			return &config.ParentZones[i]
		}
	}
	return nil
}

func addOrUpdateParentZone(config *DNSConfig, ref ParentZoneRef) {
	if config == nil || ref.Domain == "" || ref.Profile == "" {
		return
	}

	want := normalizeDomain(ref.Domain)
	for i, existing := range config.ParentZones {
		if normalizeDomain(existing.Domain) == want {
			config.ParentZones[i] = ref
			return
		}
	}
	config.ParentZones = append(config.ParentZones, ref)
}

func normalizeDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

func addOrUpdateDelegatedZone(config *DNSConfig, zone DelegatedZone) {
	if config == nil {
		return
	}

	for i, existing := range config.DelegatedZones {
		if existing.Subdomain == zone.Subdomain {
			config.DelegatedZones[i] = zone
			return
		}
	}
	config.DelegatedZones = append(config.DelegatedZones, zone)
}
