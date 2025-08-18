package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/miekg/dns"
)


// createHostedZone creates a Route53 hosted zone and returns zone ID and nameservers
func createHostedZone(profile, domain string) (string, []string, error) {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := route53.NewFromConfig(cfg)

	// Create hosted zone
	callerRef := fmt.Sprintf("%s-%d", domain, time.Now().Unix())
	createInput := &route53.CreateHostedZoneInput{
		Name:            aws.String(domain),
		CallerReference: aws.String(callerRef),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: aws.String(fmt.Sprintf("DNS zone for %s", domain)),
		},
	}

	resp, err := client.CreateHostedZone(ctx, createInput)
	if err != nil {
		// Check if zone already exists
		if strings.Contains(err.Error(), "HostedZoneAlreadyExists") {
			// Try to get existing zone
			listResp, listErr := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
				DNSName: aws.String(domain),
			})
			if listErr != nil {
				return "", nil, fmt.Errorf("zone exists but failed to retrieve: %w", listErr)
			}
			
			for _, zone := range listResp.HostedZones {
				if strings.TrimSuffix(*zone.Name, ".") == strings.TrimSuffix(domain, ".") {
					// Use full zone ID for API call
					fullZoneID := *zone.Id
					zoneID := strings.TrimPrefix(fullZoneID, "/hostedzone/")
					nameservers, nsErr := getZoneNameservers(ctx, client, fullZoneID)
					if nsErr != nil {
						return "", nil, fmt.Errorf("failed to get nameservers: %w", nsErr)
					}
					return zoneID, nameservers, nil
				}
			}
		}
		return "", nil, fmt.Errorf("failed to create hosted zone: %w", err)
	}

	// Store the full zone ID for API calls
	fullZoneID := *resp.HostedZone.Id
	// Store the clean zone ID for returning
	zoneID := strings.TrimPrefix(fullZoneID, "/hostedzone/")
	
	// Get nameservers - use the full zone ID with prefix
	nameservers, err := getZoneNameservers(ctx, client, fullZoneID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get nameservers: %w", err)
	}

	return zoneID, nameservers, nil
}

// getZoneNameservers retrieves nameservers for a hosted zone
func getZoneNameservers(ctx context.Context, client *route53.Client, zoneID string) ([]string, error) {
	// Ensure zoneID has the proper format
	if !strings.HasPrefix(zoneID, "/hostedzone/") {
		zoneID = "/hostedzone/" + zoneID
	}
	
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	}

	resp, err := client.ListResourceRecordSets(ctx, listInput)
	if err != nil {
		return nil, err
	}

	var nameservers []string
	for _, recordSet := range resp.ResourceRecordSets {
		if recordSet.Type == types.RRTypeNs {
			for _, record := range recordSet.ResourceRecords {
				nameservers = append(nameservers, strings.TrimSuffix(*record.Value, "."))
			}
			break
		}
	}

	return nameservers, nil
}

// checkDNSPropagation checks if DNS has propagated to various servers
func checkDNSPropagation(domain string, expectedNS []string) (map[string]bool, error) {
	servers := map[string]string{
		"8.8.8.8":        "Google",
		"1.1.1.1":        "Cloudflare",
		"9.9.9.9":        "Quad9",
		"208.67.222.222": "OpenDNS",
	}

	results := make(map[string]bool)
	
	for server := range servers {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Second * 5,
				}
				return d.DialContext(ctx, network, server+":53")
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ns, err := r.LookupNS(ctx, domain)
		if err != nil {
			results[server] = false
			continue
		}

		// Check if returned nameservers match expected
		matched := false
		for _, n := range ns {
			nsHost := strings.TrimSuffix(n.Host, ".")
			for _, expected := range expectedNS {
				if strings.EqualFold(nsHost, expected) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		results[server] = matched
	}

	return results, nil
}

// createDNSDelegationRole creates an IAM role for cross-account DNS delegation
func createDNSDelegationRole(profile string, trustedAccounts []string) (string, error) {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get current account ID
	stsClient := sts.NewFromConfig(cfg)
	_, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	iamClient := iam.NewFromConfig(cfg)
	
	roleName := "dns-delegation-role"
	
	trustPolicy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string][]string{
					"AWS": func() []string {
						arns := make([]string, len(trustedAccounts))
						for i, account := range trustedAccounts {
							arns[i] = fmt.Sprintf("arn:aws:iam::%s:root", account)
						}
						return arns
					}(),
				},
				"Action": "sts:AssumeRole",
			},
		},
	}
	
	trustPolicyJSON, err := json.Marshal(trustPolicy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal trust policy: %w", err)
	}

	// Create the role
	createRoleInput := &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(string(trustPolicyJSON)),
		Description:              aws.String("Role for cross-account DNS delegation"),
	}

	roleResp, err := iamClient.CreateRole(ctx, createRoleInput)
	if err != nil {
		// Check if role already exists
		if strings.Contains(err.Error(), "EntityAlreadyExists") {
			getRoleResp, getRoleErr := iamClient.GetRole(ctx, &iam.GetRoleInput{
				RoleName: aws.String(roleName),
			})
			if getRoleErr != nil {
				return "", fmt.Errorf("role exists but failed to retrieve: %w", getRoleErr)
			}
			// Update trust policy for existing role
			_, updateErr := iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
				RoleName:       aws.String(roleName),
				PolicyDocument: aws.String(string(trustPolicyJSON)),
			})
			if updateErr != nil {
				return "", fmt.Errorf("failed to update role trust policy: %w", updateErr)
			}
			return *getRoleResp.Role.Arn, nil
		}
		return "", fmt.Errorf("failed to create role: %w", err)
	}

	// Attach Route53 policy
	policyArn := "arn:aws:iam::aws:policy/AmazonRoute53FullAccess"
	_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(policyArn),
	})
	if err != nil && !strings.Contains(err.Error(), "Duplicate") {
		return "", fmt.Errorf("failed to attach policy: %w", err)
	}

	return *roleResp.Role.Arn, nil
}

// createNSRecordDelegation creates NS records in root zone for subdomain delegation
func createNSRecordDelegation(rootProfile, childProfile, rootZoneID, subdomain string, nsRecords []string) error {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(rootProfile),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := route53.NewFromConfig(cfg)

	// Create NS record set
	var resourceRecords []types.ResourceRecord
	for _, ns := range nsRecords {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(ns),
		})
	}

	changeInput := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(rootZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(subdomain),
						Type:            types.RRTypeNs,
						TTL:             aws.Int64(300),
						ResourceRecords: resourceRecords,
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Delegation for %s", subdomain)),
		},
	}

	_, err = client.ChangeResourceRecordSets(ctx, changeInput)
	if err != nil {
		return fmt.Errorf("failed to create NS records: %w", err)
	}

	return nil
}

// assumeRoleAndCreateNSRecords assumes role and creates NS records in root account
func assumeRoleAndCreateNSRecords(roleArn, rootZoneID, subdomain string, nsRecords []string) error {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client to assume role
	stsClient := sts.NewFromConfig(cfg)
	
	// Assume the role
	assumeRoleResp, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String("dns-delegation-session"),
	})
	if err != nil {
		return fmt.Errorf("failed to assume role: %w", err)
	}

	// Create new config with assumed role credentials
	assumedCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				*assumeRoleResp.Credentials.AccessKeyId,
				*assumeRoleResp.Credentials.SecretAccessKey,
				*assumeRoleResp.Credentials.SessionToken,
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create config with assumed role: %w", err)
	}

	client := route53.NewFromConfig(assumedCfg)

	// Create NS record set
	var resourceRecords []types.ResourceRecord
	for _, ns := range nsRecords {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: aws.String(ns),
		})
	}

	changeInput := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(rootZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(subdomain),
						Type:            types.RRTypeNs,
						TTL:             aws.Int64(300),
						ResourceRecords: resourceRecords,
					},
				},
			},
			Comment: aws.String(fmt.Sprintf("Delegation for %s", subdomain)),
		},
	}

	_, err = client.ChangeResourceRecordSets(ctx, changeInput)
	if err != nil {
		return fmt.Errorf("failed to create NS records: %w", err)
	}

	return nil
}

// queryDNSOverHTTPS queries using DNS-over-HTTPS to bypass ISP interception
func queryDNSOverHTTPS(domain string) ([]string, error) {
	// Clean domain - remove trailing dot for URL
	domain = strings.TrimSuffix(domain, ".")
	
	// Use Google's DNS-over-HTTPS endpoint
	url := fmt.Sprintf("https://dns.google/resolve?name=%s&type=NS", domain)
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-json")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query DoH: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH returned status %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var dohResponse struct {
		Status int `json:"Status"`
		Answer []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
			Data string `json:"data"`
			TTL  int    `json:"TTL"`
		} `json:"Answer"`
	}
	
	if err := json.Unmarshal(body, &dohResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	// Check if we got a valid response
	if dohResponse.Status != 0 {
		return nil, fmt.Errorf("DNS query failed with status %d", dohResponse.Status)
	}
	
	var nameservers []string
	for _, answer := range dohResponse.Answer {
		if answer.Type == 2 { // NS record
			nameservers = append(nameservers, strings.TrimSuffix(answer.Data, "."))
		}
	}
	
	if len(nameservers) == 0 {
		return nil, fmt.Errorf("no NS records found for %s", domain)
	}
	
	return nameservers, nil
}

// queryStandardDNS queries using standard DNS protocol (may be intercepted)
func queryStandardDNS(domain string) ([]string, error) {
	// Ensure domain ends with a dot for FQDN
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
	
	c := new(dns.Client)
	c.Net = "udp"
	c.Timeout = 3 * time.Second
	
	m := new(dns.Msg)
	m.SetQuestion(domain, dns.TypeNS)
	m.RecursionDesired = true
	
	r, _, err := c.Exchange(m, "8.8.8.8:53")
	if err != nil {
		return nil, err
	}
	
	if r.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS query failed with %s", dns.RcodeToString[r.Rcode])
	}
	
	var nameservers []string
	for _, ans := range r.Answer {
		if ns, ok := ans.(*dns.NS); ok {
			nameservers = append(nameservers, strings.TrimSuffix(ns.Ns, "."))
		}
	}
	
	return nameservers, nil
}

// queryNameserversWithDebug queries nameservers and returns debug output with TTL
func queryNameserversWithDebug(domain string) ([]string, []string, uint32, error) {
	debugLogs := []string{}
	var detectedTTL uint32
	
	// Capture all the debug output
	debugLogs = append(debugLogs, "============================================")
	debugLogs = append(debugLogs, fmt.Sprintf("Starting DNS query for domain: %s", domain))
	debugLogs = append(debugLogs, "")
	
	// Clean and prepare the domain
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	
	// PRIMARY METHOD: DNS-over-HTTPS (DoH) - Bypasses ISP interception
	debugLogs = append(debugLogs, "[PRIMARY] Using DNS-over-HTTPS (DoH)...")
	debugLogs = append(debugLogs, "DoH bypasses ISP interception and returns real-time results")
	debugLogs = append(debugLogs, "")
	
	dohNS, dohErr := queryDNSOverHTTPS(domain)
	if dohErr == nil && len(dohNS) > 0 {
		// Sort for consistent display
		sort.Strings(dohNS)
		
		debugLogs = append(debugLogs, fmt.Sprintf("✅ DoH SUCCESS! Got %d nameservers", len(dohNS)))
		debugLogs = append(debugLogs, "Current nameservers from Google DNS:")
		for i, ns := range dohNS {
			debugLogs = append(debugLogs, fmt.Sprintf("  %d. %s", i+1, ns))
		}
		debugLogs = append(debugLogs, "")
		
		// Also try standard DNS to show the difference if there's ISP interception
		debugLogs = append(debugLogs, "[COMPARISON] Checking standard DNS (may be intercepted)...")
		standardNS, standardErr := queryStandardDNS(domain)
		if standardErr == nil && len(standardNS) > 0 {
			sort.Strings(standardNS)
			if !equalStringSlices(dohNS, standardNS) {
				debugLogs = append(debugLogs, "⚠️  ISP INTERCEPTION DETECTED!")
				debugLogs = append(debugLogs, "Standard DNS returns different (cached) values:")
				for i, ns := range standardNS {
					debugLogs = append(debugLogs, fmt.Sprintf("  %d. %s (cached)", i+1, ns))
				}
				debugLogs = append(debugLogs, "Your ISP is intercepting DNS queries to 8.8.8.8")
				debugLogs = append(debugLogs, "Using DoH results which are accurate")
			} else {
				debugLogs = append(debugLogs, "✅ Standard DNS matches DoH (no interception detected)")
			}
		} else {
			debugLogs = append(debugLogs, "Standard DNS check failed (not critical)")
		}
		
		// DoH responses are authoritative and not cached locally
		return dohNS, debugLogs, 0, nil
	}
	
	// If DoH fails, log the error and try fallback methods
	if dohErr != nil {
		debugLogs = append(debugLogs, fmt.Sprintf("❌ DoH failed: %v", dohErr))
		debugLogs = append(debugLogs, "")
	}
	
	debugLogs = append(debugLogs, "[FALLBACK] Trying standard DNS...")
	debugLogs = append(debugLogs, "⚠️  Results may be cached or intercepted")
	
	// Ensure domain ends with a dot for FQDN
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
	
	debugLogs = append(debugLogs, fmt.Sprintf("Querying for FQDN: '%s'", domain))
	
	// First, try to get authoritative servers for the parent domain
	parentDomain := getParentDomain(domain)
	if parentDomain != "" {
		debugLogs = append(debugLogs, fmt.Sprintf("Parent domain: %s", parentDomain))
		authNS := queryAuthoritativeServers(parentDomain, domain)
		if len(authNS) > 0 {
			debugLogs = append(debugLogs, "✅ Got authoritative answer!")
			// Authoritative answers have no TTL caching
			return authNS, debugLogs, 0, nil
		}
	}
	
	// List of public DNS servers to query
	// Include secondary IPs to potentially hit different cache nodes
	publicDNSServers := []string{
		"8.8.8.8:53",        // Google Primary
		"8.8.4.4:53",        // Google Secondary (often different cache)
		"1.1.1.1:53",        // Cloudflare Primary
		"1.0.0.1:53",        // Cloudflare Secondary
	}
	
	// Map to store results from each server
	serverResults := make(map[string][]string)
	var finalNameservers []string
	var lastErr error
	
	// Query DNS servers
	for _, server := range publicDNSServers {
		debugLogs = append(debugLogs, fmt.Sprintf("--- Querying %s ---", server))
		
		// Create a new DNS client - try TCP first
		c := new(dns.Client)
		c.Net = "tcp"
		c.Timeout = 3 * time.Second
		
		// Create NS query message with cache-busting techniques
		m := new(dns.Msg)
		m.SetQuestion(domain, dns.TypeNS)
		m.RecursionDesired = true
		
		// Add EDNS0 with DO bit (DNSSEC OK) - helps bypass some caches
		m.SetEdns0(4096, true)
		
		// Set Checking Disabled (CD) flag - tells resolver to skip DNSSEC validation
		// This can help bypass DNSSEC validation cache
		m.CheckingDisabled = true
		
		// Use a unique query ID to avoid hitting query cache
		m.Id = uint16(time.Now().UnixNano() & 0xFFFF)
		
		debugLogs = append(debugLogs, fmt.Sprintf("Query: %s Type=NS TCP CD=true", domain))
		
		// Query the DNS server using TCP
		r, rtt, err := c.Exchange(m, server)
		
		// Check if we might be hitting an ISP interceptor
		if err == nil && r != nil && r.Id != m.Id {
			debugLogs = append(debugLogs, "⚠️  Query ID mismatch - possible ISP DNS interception!")
		}
		
		// If TCP fails, fallback to UDP
		if err != nil {
			debugLogs = append(debugLogs, fmt.Sprintf("TCP failed (%v), trying UDP...", err))
			c.Net = "udp"
			r, rtt, err = c.Exchange(m, server)
		}
		
		if err != nil {
			debugLogs = append(debugLogs, fmt.Sprintf("❌ Query failed: %v", err))
			serverResults[server] = []string{"ERROR: " + err.Error()}
			lastErr = err
			continue
		}
		
		debugLogs = append(debugLogs, fmt.Sprintf("✓ Response in %v", rtt))
		debugLogs = append(debugLogs, fmt.Sprintf("Response code: %s", dns.RcodeToString[r.Rcode]))
		debugLogs = append(debugLogs, fmt.Sprintf("Answer: %d records, Authority: %d records", len(r.Answer), len(r.Ns)))
		
		// Check for TTL to warn about caching
		if r != nil && len(r.Answer) > 0 {
			for _, ans := range r.Answer {
				if ns, ok := ans.(*dns.NS); ok {
					ttl := ns.Header().Ttl
					// Store the TTL for display
					if detectedTTL == 0 || ttl > detectedTTL {
						detectedTTL = ttl
					}
					if ttl > 3600 { // More than 1 hour
						hours := ttl / 3600
						debugLogs = append(debugLogs, fmt.Sprintf("⚠️ HIGH TTL: %d seconds (~%d hours) - cached response!", ttl, hours))
						debugLogs = append(debugLogs, "This server has cached the old values and won't check for updates until TTL expires")
					}
					break // Just check the first one
				}
			}
		}
		
		var currentNS []string
		
		// If we got a valid response with NS records
		if r != nil && r.Rcode == dns.RcodeSuccess {
			// Extract NS records from answer section
			for _, ans := range r.Answer {
				if ns, ok := ans.(*dns.NS); ok {
					nsValue := strings.TrimSuffix(ns.Ns, ".")
					currentNS = append(currentNS, nsValue)
				}
			}
			
			// Sort for consistent comparison
			if len(currentNS) > 0 {
				sort.Strings(currentNS)
				serverResults[server] = currentNS
				debugLogs = append(debugLogs, fmt.Sprintf("Found %d NS records:", len(currentNS)))
				for i, ns := range currentNS {
					debugLogs = append(debugLogs, fmt.Sprintf("  %d. %s", i+1, ns))
				}
				// Use the first successful result
				if len(finalNameservers) == 0 {
					finalNameservers = currentNS
				}
			} else {
				debugLogs = append(debugLogs, "⚠️ No NS records found")
				serverResults[server] = []string{"NO_NS_RECORDS"}
			}
		}
	}
	
	// Check if servers agree
	var inconsistent bool
	var firstResult []string
	for _, results := range serverResults {
		if len(firstResult) == 0 && len(results) > 0 && !strings.HasPrefix(results[0], "ERROR") {
			firstResult = results
		} else if len(results) > 0 && !strings.HasPrefix(results[0], "ERROR") {
			if !equalStringSlices(firstResult, results) {
				inconsistent = true
			}
		}
	}
	
	if inconsistent {
		debugLogs = append(debugLogs, "⚠️ DNS servers returning DIFFERENT results!")
		debugLogs = append(debugLogs, "DNS propagation is still in progress.")
	} else if len(finalNameservers) > 0 {
		debugLogs = append(debugLogs, "✅ All DNS servers agree on nameservers.")
	}
	
	if len(finalNameservers) > 0 {
		return finalNameservers, debugLogs, detectedTTL, nil
	}
	
	if lastErr != nil {
		return nil, debugLogs, detectedTTL, fmt.Errorf("failed to query nameservers: %w", lastErr)
	}
	
	return nil, debugLogs, detectedTTL, fmt.Errorf("no nameservers found for domain %s", strings.TrimSuffix(domain, "."))
}

// queryNameservers queries nameservers using DoH first, then standard DNS as fallback
func queryNameservers(domain string) ([]string, error) {
	// Clean and prepare the domain
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	
	// Try DNS-over-HTTPS first (bypasses ISP interception)
	dohNS, dohErr := queryDNSOverHTTPS(domain)
	if dohErr == nil && len(dohNS) > 0 {
		sort.Strings(dohNS)
		return dohNS, nil
	}
	
	// If DoH fails, try standard DNS as fallback
	fmt.Printf("[DNS DEBUG] DoH failed (%v), trying standard DNS...\n", dohErr)
	
	// Clean and prepare the domain
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	
	// Ensure domain ends with a dot for FQDN
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
	
	fmt.Printf("[DNS DEBUG] Querying for FQDN: '%s'\n", domain)
	
	// First, try to get authoritative servers for the parent domain
	parentDomain := getParentDomain(domain)
	if parentDomain != "" {
		fmt.Printf("[DNS DEBUG] Parent domain: %s\n", parentDomain)
		authNS := queryAuthoritativeServers(parentDomain, domain)
		if len(authNS) > 0 {
			fmt.Printf("[DNS DEBUG] ✅ Got authoritative answer!\n")
			return authNS, nil
		}
	}
	
	// List of public DNS servers to query
	// We query ALL of them to see differences
	publicDNSServers := []string{
		"8.8.8.8:53",        // Google
		"8.8.4.4:53",        // Google Secondary
		"1.1.1.1:53",        // Cloudflare
		"1.0.0.1:53",        // Cloudflare Secondary
		"9.9.9.9:53",        // Quad9
		"208.67.222.222:53", // OpenDNS
		"208.67.220.220:53", // OpenDNS Secondary
	}
	
	// Map to store results from each server
	serverResults := make(map[string][]string)
	var finalNameservers []string
	var lastErr error
	
	// Query ALL DNS servers to see what each one returns
	for _, server := range publicDNSServers {
		fmt.Printf("\n[DNS DEBUG] --- Querying %s ---\n", server)
		
		// Create a new DNS client - try TCP first as it often bypasses caches
		c := new(dns.Client)
		c.Net = "tcp" // Use TCP instead of UDP to bypass some caches
		c.Timeout = 3 * time.Second
		
		// Create NS query message
		m := new(dns.Msg)
		m.SetQuestion(domain, dns.TypeNS)
		m.RecursionDesired = true
		
		// Add EDNS0 with DO bit to bypass DNSSEC validation cache
		m.SetEdns0(4096, true)
		
		// Debug: Show the actual DNS query being sent
		fmt.Printf("[DNS DEBUG] Query details: Question=%s Type=NS RD=true DO=true Protocol=TCP\n", domain)
		
		// Query the DNS server using TCP
		r, rtt, err := c.Exchange(m, server)
		
		// If TCP fails, fallback to UDP
		if err != nil {
			fmt.Printf("[DNS DEBUG] TCP failed (%v), trying UDP...\n", err)
			c.Net = "udp"
			r, rtt, err = c.Exchange(m, server)
		}
		if err != nil {
			fmt.Printf("[DNS DEBUG] ❌ Query failed: %v\n", err)
			serverResults[server] = []string{"ERROR: " + err.Error()}
			lastErr = err
			continue
		}
		
		fmt.Printf("[DNS DEBUG] ✓ Got response in %v\n", rtt)
		fmt.Printf("[DNS DEBUG] Response code: %s\n", dns.RcodeToString[r.Rcode])
		fmt.Printf("[DNS DEBUG] Answer section has %d records\n", len(r.Answer))
		fmt.Printf("[DNS DEBUG] Authority section has %d records\n", len(r.Ns))
		
		var currentNS []string
		
		// If we got a valid response with NS records
		if r != nil && r.Rcode == dns.RcodeSuccess {
			// Extract NS records from answer section
			for _, ans := range r.Answer {
				if ns, ok := ans.(*dns.NS); ok {
					nsValue := strings.TrimSuffix(ns.Ns, ".")
					currentNS = append(currentNS, nsValue)
				}
			}
			
			// If we didn't find NS records in answer, check authority section
			if len(currentNS) == 0 {
				for _, auth := range r.Ns {
					if ns, ok := auth.(*dns.NS); ok {
						nsValue := strings.TrimSuffix(ns.Ns, ".")
						currentNS = append(currentNS, nsValue)
					}
				}
			}
			
			// Sort for consistent comparison
			sort.Strings(currentNS)
			serverResults[server] = currentNS
			
			// Print what this server returned
			if len(currentNS) > 0 {
				fmt.Printf("[DNS DEBUG] Found %d NS records:\n", len(currentNS))
				for i, ns := range currentNS {
					fmt.Printf("[DNS DEBUG]   %d. %s\n", i+1, ns)
				}
				// Use the first successful result as our final result
				if len(finalNameservers) == 0 {
					finalNameservers = currentNS
				}
			} else {
				fmt.Printf("[DNS DEBUG] ⚠️  No NS records found\n")
				serverResults[server] = []string{"NO_NS_RECORDS"}
			}
		} else if r != nil {
			fmt.Printf("[DNS DEBUG] ⚠️  Response code: %s\n", dns.RcodeToString[r.Rcode])
			serverResults[server] = []string{"RCODE: " + dns.RcodeToString[r.Rcode]}
		}
	}
	
	// Print comparison of all server results
	fmt.Printf("\n[DNS DEBUG] ============================================\n")
	fmt.Printf("[DNS DEBUG] COMPARISON OF ALL DNS SERVER RESPONSES:\n")
	fmt.Printf("[DNS DEBUG] ============================================\n")
	
	for server, results := range serverResults {
		fmt.Printf("\n[DNS DEBUG] %s:\n", server)
		if len(results) == 0 {
			fmt.Printf("[DNS DEBUG]   (no results)\n")
		} else {
			for _, ns := range results {
				fmt.Printf("[DNS DEBUG]   - %s\n", ns)
			}
		}
	}
	
	// Check if all servers agree
	var inconsistent bool
	var firstResult []string
	for _, results := range serverResults {
		if len(firstResult) == 0 && !strings.HasPrefix(results[0], "ERROR") && !strings.HasPrefix(results[0], "RCODE") {
			firstResult = results
		} else if len(results) > 0 && !strings.HasPrefix(results[0], "ERROR") && !strings.HasPrefix(results[0], "RCODE") {
			if !equalStringSlices(firstResult, results) {
				inconsistent = true
			}
		}
	}
	
	if inconsistent {
		fmt.Printf("\n[DNS DEBUG] ⚠️  WARNING: DNS servers returning DIFFERENT results!\n")
		fmt.Printf("[DNS DEBUG] This indicates DNS propagation is still in progress.\n")
	} else if len(finalNameservers) > 0 {
		fmt.Printf("\n[DNS DEBUG] ✅ All responding DNS servers agree on the nameservers.\n")
	}
	
	fmt.Printf("[DNS DEBUG] ============================================\n\n")
	
	// Return the nameservers we found
	if len(finalNameservers) > 0 {
		return finalNameservers, nil
	}
	
	// If we couldn't get NS records from any server, return error
	if lastErr != nil {
		return nil, fmt.Errorf("failed to query nameservers: %w", lastErr)
	}
	
	return nil, fmt.Errorf("no nameservers found for domain %s", strings.TrimSuffix(domain, "."))
}

// Helper function to compare string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// getParentDomain returns the parent domain for a given domain
func getParentDomain(domain string) string {
	domain = strings.TrimSuffix(domain, ".")
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return "" // It's already a TLD or root domain
	}
	return strings.Join(parts[1:], ".") + "."
}

// queryAuthoritativeServers queries the authoritative servers directly
func queryAuthoritativeServers(parentDomain, targetDomain string) []string {
	// For .agency domains, we know the TLD servers
	if strings.HasSuffix(strings.TrimSuffix(targetDomain, "."), ".agency") {
		// Query .agency TLD nameservers directly
		agencyServers := []string{
			"65.22.161.17:53",   // a.nic.agency
			"65.22.162.17:53",   // b.nic.agency
			"65.22.163.17:53",   // c.nic.agency
			"65.22.164.17:53",   // d.nic.agency
		}
		
		c := new(dns.Client)
		c.Net = "tcp"
		c.Timeout = 3 * time.Second
		
		for _, server := range agencyServers {
			m := new(dns.Msg)
			m.SetQuestion(targetDomain, dns.TypeNS)
			m.RecursionDesired = false
			
			r, _, err := c.Exchange(m, server)
			if err == nil && r != nil {
				var results []string
				// Check both answer and authority sections
				for _, rr := range r.Answer {
					if ns, ok := rr.(*dns.NS); ok {
						results = append(results, strings.TrimSuffix(ns.Ns, "."))
					}
				}
				for _, rr := range r.Ns {
					if ns, ok := rr.(*dns.NS); ok {
						results = append(results, strings.TrimSuffix(ns.Ns, "."))
					}
				}
				if len(results) > 0 {
					return results
				}
			}
		}
	}
	
	fmt.Printf("[DNS DEBUG] Trying to find authoritative servers for parent %s\n", parentDomain)
	
	// First, get the NS records for the parent domain to find its authoritative servers
	c := new(dns.Client)
	c.Net = "tcp"
	c.Timeout = 3 * time.Second
	
	m := new(dns.Msg)
	m.SetQuestion(parentDomain, dns.TypeNS)
	m.RecursionDesired = false // Don't use recursion - we want authoritative answer
	
	// Try root servers first to get truly authoritative path
	rootServers := []string{
		"198.41.0.4:53",   // a.root-servers.net
		"199.9.14.201:53", // b.root-servers.net
		"192.33.4.12:53",  // c.root-servers.net
	}
	
	var parentNS []string
	for _, root := range rootServers {
		r, _, err := c.Exchange(m, root)
		if err == nil && r != nil {
			// Look for NS records in the authority section
			for _, rr := range r.Ns {
				if ns, ok := rr.(*dns.NS); ok {
					parentNS = append(parentNS, ns.Ns)
				}
			}
			if len(parentNS) > 0 {
				break
			}
		}
	}
	
	// If we couldn't get from root servers, use public DNS
	if len(parentNS) == 0 {
		m.RecursionDesired = true
		r, _, err := c.Exchange(m, "8.8.8.8:53")
		if err == nil && r != nil {
			for _, rr := range r.Answer {
				if ns, ok := rr.(*dns.NS); ok {
					parentNS = append(parentNS, ns.Ns)
				}
			}
		}
	}
	
	if len(parentNS) == 0 {
		fmt.Printf("[DNS DEBUG] Could not find parent nameservers\n")
		return nil
	}
	
	fmt.Printf("[DNS DEBUG] Found %d parent nameservers, querying them for %s\n", len(parentNS), targetDomain)
	
	// Now query the parent's authoritative servers for our target domain
	var results []string
	for _, nsServer := range parentNS {
		if !strings.Contains(nsServer, ":") {
			nsServer = strings.TrimSuffix(nsServer, ".") + ":53"
		}
		
		m := new(dns.Msg)
		m.SetQuestion(targetDomain, dns.TypeNS)
		m.RecursionDesired = false // We want authoritative answer only
		
		r, _, err := c.Exchange(m, nsServer)
		if err != nil {
			continue
		}
		
		if r != nil && r.Authoritative {
			fmt.Printf("[DNS DEBUG] Got AUTHORITATIVE answer from %s\n", nsServer)
			for _, rr := range r.Answer {
				if ns, ok := rr.(*dns.NS); ok {
					results = append(results, strings.TrimSuffix(ns.Ns, "."))
				}
			}
			if len(results) > 0 {
				return results
			}
		}
	}
	
	return nil
}

// getAllAWSAccounts retrieves all configured AWS accounts
func getAllAWSAccounts() ([]AccountInfo, error) {
	ctx := context.Background()

	var accounts []AccountInfo
	
	// This is a simplified version - in real implementation,
	// you would iterate through all profiles in ~/.aws/config
	profiles := []string{"default", "prod", "dev", "staging"}
	
	for _, profile := range profiles {
		profileCfg, err := config.LoadDefaultConfig(ctx,
			config.WithSharedConfigProfile(profile),
		)
		if err != nil {
			continue // Skip profiles that can't be loaded
		}

		stsClient := sts.NewFromConfig(profileCfg)
		identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			continue // Skip if we can't get account ID
		}

		accounts = append(accounts, AccountInfo{
			AccountID: *identity.Account,
			Profile:   profile,
			Region:    profileCfg.Region,
		})
	}

	return accounts, nil
}

// deleteDNSZone deletes a Route53 hosted zone
func deleteDNSZone(profile, zoneID string) error {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := route53.NewFromConfig(cfg)

	// Ensure zoneID has the proper format for API calls
	if !strings.HasPrefix(zoneID, "/hostedzone/") {
		zoneID = "/hostedzone/" + zoneID
	}

	// First, delete all non-default record sets
	listResp, err := client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to list record sets: %w", err)
	}

	var changes []types.Change
	for _, recordSet := range listResp.ResourceRecordSets {
		// Skip default NS and SOA records
		if (recordSet.Type == types.RRTypeNs || recordSet.Type == types.RRTypeSoa) && 
			!strings.Contains(*recordSet.Name, ".") {
			continue
		}
		
		changes = append(changes, types.Change{
			Action:            types.ChangeActionDelete,
			ResourceRecordSet: &recordSet,
		})
	}

	if len(changes) > 0 {
		_, err = client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &types.ChangeBatch{
				Changes: changes,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete record sets: %w", err)
		}
	}

	// Delete the hosted zone
	_, err = client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to delete hosted zone: %w", err)
	}

	return nil
}

// validateHostedZone checks if a hosted zone exists and returns its details
func validateHostedZone(profile, zoneID string) (string, []string, error) {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := route53.NewFromConfig(cfg)

	// Ensure zoneID has the proper format for API call
	if !strings.HasPrefix(zoneID, "/hostedzone/") {
		zoneID = "/hostedzone/" + zoneID
	}
	
	// Get the hosted zone details
	resp, err := client.GetHostedZone(ctx, &route53.GetHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get hosted zone: %w", err)
	}

	// Get the domain name
	domain := strings.TrimSuffix(*resp.HostedZone.Name, ".")
	
	// Get nameservers
	nameservers, err := getZoneNameservers(ctx, client, zoneID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get nameservers: %w", err)
	}

	return domain, nameservers, nil
}