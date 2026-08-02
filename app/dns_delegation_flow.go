package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

// sentinel values for the profile picker
const (
	delegateChoiceManual = "meroku:manual"
	delegateChoiceCancel = "meroku:cancel"
)

// runDelegationFlow drives steps 2 and 3 of the delegation handling: ask which
// profile owns the parent zone, verify that answer, write the NS record, and
// wait for it to become visible. Falls back to printed instructions whenever we
// cannot safely or successfully do it ourselves.
//
// res must come from checkDNSPreflight so that ZoneNameservers and ParentDomain
// are populated.
func runDelegationFlow(ctx context.Context, e Env, res dnsPreflightResult) error {
	if res.Plan == dnsPlanNormal || res.Plan == dnsPlanSkip {
		fmt.Print(describeDNSPreflight(res))
		return nil
	}

	if len(res.ZoneNameservers) == 0 {
		return fmt.Errorf("cannot delegate %s: its hosted zone does not exist yet", res.ZoneName)
	}
	if res.ParentDomain == "" {
		return fmt.Errorf("%s is a root domain — delegate it at your registrar, not in a parent zone", res.ZoneName)
	}

	fmt.Printf("\n🌐 %s needs an NS delegation record in %s\n\n", res.ZoneName, res.ParentDomain)

	// What does the internet say the parent's nameservers are? This is the
	// yardstick every candidate zone is measured against.
	publicParentNS, err := queryNameservers(res.ParentDomain)
	if err != nil || len(publicParentNS) == 0 {
		fmt.Print(manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			fmt.Sprintf("could not resolve nameservers for %s, so meroku cannot verify which zone is authoritative", res.ParentDomain),
			res.ZoneNameservers))
		return nil
	}

	if !looksLikeRoute53(publicParentNS) {
		fmt.Print(manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			fmt.Sprintf("%s is not hosted on Route53 (nameservers: %s)",
				res.ParentDomain, strings.Join(normalizeNameservers(publicParentNS), ", ")),
			res.ZoneNameservers))
		return nil
	}

	profiles, err := getLocalAWSProfiles()
	if err != nil || len(profiles) == 0 {
		fmt.Print(manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			"no local AWS profiles found", res.ZoneNameservers))
		return nil
	}

	fmt.Printf("   Looking for the %s zone across %d AWS profiles...\n", res.ParentDomain, len(profiles))
	candidates := scanProfilesForParentZone(ctx, profiles, res.ParentDomain, publicParentNS)

	choice, selected := pickParentZoneProfile(candidates, res.ParentDomain)
	switch choice {
	case delegateChoiceCancel:
		fmt.Println("\nCancelled. Nothing was changed.")
		return nil
	case delegateChoiceManual:
		fmt.Print("\n" + manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			"you chose to add the record yourself", res.ZoneNameservers))
		return nil
	}

	// Refuse to write into a zone we could not prove is the live one. This is the
	// whole safety argument for asking-then-verifying rather than inferring.
	if !selected.Authoritative {
		fmt.Printf("\n⚠️  The %s zone in profile %s does not match public DNS.\n", res.ParentDomain, selected.Profile)
		fmt.Printf("   That zone delegates to: %s\n", strings.Join(normalizeNameservers(selected.Nameservers), ", "))
		fmt.Printf("   The internet uses:      %s\n", strings.Join(normalizeNameservers(publicParentNS), ", "))
		fmt.Print("\n" + manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			"the selected zone is not the one serving this domain, so writing to it would have no effect",
			res.ZoneNameservers))
		return nil
	}

	fmt.Printf("\n   Will add to zone %s (profile %s, account %s):\n", selected.ZoneID, selected.Profile, selected.AccountID)
	fmt.Printf("     %s  NS  %s\n", res.ZoneName, strings.Join(res.ZoneNameservers, ", "))

	confirmed := false
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("Add the NS record for %s?", res.ZoneName)).
		Description("This writes to live DNS. It adds a new record and changes nothing existing.").
		Affirmative("Yes, add it").
		Negative("No").
		Value(&confirmed).
		Run(); err != nil {
		return fmt.Errorf("confirmation cancelled: %w", err)
	}
	if !confirmed {
		fmt.Print("\n" + manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			"you declined the automatic write", res.ZoneNameservers))
		return nil
	}

	req := delegationRequest{
		ParentProfile: selected.Profile,
		ParentZoneID:  selected.ZoneID,
		Subdomain:     res.ZoneName,
		Nameservers:   res.ZoneNameservers,
	}
	if err := applyDelegation(req); err != nil {
		fmt.Printf("\n❌ %v\n\n", err)
		fmt.Print(manualDelegationInstructions(res.ZoneName, res.ParentDomain,
			"the automatic write failed (see the error above)", res.ZoneNameservers))
		return err
	}

	fmt.Println("\n✅ NS record written. Waiting for it to become visible on the public internet...")
	fmt.Println("   (this usually takes under a minute; Ctrl-C to stop waiting — the record is already saved)")

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	observed, ok := waitForDelegation(waitCtx, res.ZoneName, res.ZoneNameservers, 10*time.Second)
	if !ok {
		fmt.Printf("\n⚠️  Not visible yet. The record is written, but resolvers are still catching up.\n")
		if len(observed) > 0 {
			fmt.Printf("   Currently resolving to: %s\n", strings.Join(normalizeNameservers(observed), ", "))
		}
		fmt.Println("   Re-check with: meroku dns validate")
		return nil
	}

	fmt.Printf("\n✅ %s is now delegated to this account.\n", res.ZoneName)

	if err := recordDelegation(delegationRecord{
		Subdomain:       res.ZoneName,
		AccountID:       e.AccountID,
		ZoneID:          res.ZoneID,
		Nameservers:     res.ZoneNameservers,
		ParentDomain:    res.ParentDomain,
		ParentProfile:   selected.Profile,
		ParentZoneID:    selected.ZoneID,
		ParentAccountID: selected.AccountID,
	}); err != nil {
		// Persistence is a convenience, not correctness — the DNS change succeeded.
		fmt.Printf("   (note: could not record this in %s: %v)\n", DNSConfigFile, err)
	}

	fmt.Println("   Certificate validation will now succeed. Run the deploy again.")
	return nil
}

// looksLikeRoute53 reports whether a nameserver set belongs to AWS.
// Route53 always delegates across the four awsdns TLD variants.
func looksLikeRoute53(nameservers []string) bool {
	for _, ns := range normalizeNameservers(nameservers) {
		if strings.Contains(ns, "awsdns") {
			return true
		}
	}
	return false
}

// pickParentZoneProfile asks which profile owns the parent zone.
func pickParentZoneProfile(candidates []parentZoneCandidate, parentDomain string) (string, parentZoneCandidate) {
	byProfile := map[string]parentZoneCandidate{}

	options := lo.FilterMap(candidates, func(c parentZoneCandidate, _ int) (huh.Option[string], bool) {
		// Only offer profiles that actually hold a zone with this name; the rest
		// would just be noise in the list.
		if c.ZoneID == "" {
			return huh.Option[string]{}, false
		}
		byProfile[c.Profile] = c
		return huh.NewOption(c.Label(parentDomain), c.Profile), true
	})

	if len(options) == 0 {
		fmt.Printf("   No local AWS profile has a hosted zone for %s.\n", parentDomain)
		return delegateChoiceManual, parentZoneCandidate{}
	}

	options = append(options,
		huh.NewOption(fmt.Sprintf("None of these — I'll add the record to %s myself", parentDomain), delegateChoiceManual),
		huh.NewOption("Cancel", delegateChoiceCancel),
	)

	choice := ""
	if err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Which AWS profile manages the %s zone?", parentDomain)).
		Description("meroku verifies your choice against public DNS before writing anything.").
		Options(options...).
		Value(&choice).
		Run(); err != nil {
		return delegateChoiceCancel, parentZoneCandidate{}
	}

	return choice, byProfile[choice]
}

// runDNSDelegateCommand implements `meroku dns delegate <env>`.
func runDNSDelegateCommand(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: meroku dns delegate <environment>")
		fmt.Println("Example: meroku dns delegate dev")
		fmt.Println("")
		fmt.Println("Checks whether this environment's DNS zone is delegated from its parent,")
		fmt.Println("and offers to add the missing NS record for you.")
		return nil
	}

	envName := args[0]
	e, err := loadEnv(envName)
	if err != nil {
		return fmt.Errorf("failed to load environment %q: %w", envName, err)
	}

	if e.AWSProfile != "" {
		os.Setenv("AWS_PROFILE", e.AWSProfile)
	}
	if e.Region != "" {
		os.Setenv("AWS_REGION", e.Region)
		os.Setenv("AWS_DEFAULT_REGION", e.Region)
	}

	ctx := context.Background()
	res, err := checkDNSPreflight(ctx, e)
	if err != nil {
		return fmt.Errorf("DNS preflight failed: %w", err)
	}

	return runDelegationFlow(ctx, e, res)
}
