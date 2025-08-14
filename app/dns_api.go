package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

// queryNameservers queries nameservers for a domain
func queryNameservers(domain string) ([]string, error) {
	ns, err := net.LookupNS(domain)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup nameservers: %w", err)
	}

	var nameservers []string
	for _, n := range ns {
		nameservers = append(nameservers, strings.TrimSuffix(n.Host, "."))
	}

	return nameservers, nil
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