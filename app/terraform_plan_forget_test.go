package main

import "testing"

// A `removed { lifecycle { destroy = false } }` block plans the action
// "forget": the object leaves state and stays in AWS. The plan viewer must
// never present or count it as a deletion. The SES merge-mode migration
// relies on this: it forgets the whole-set DNS records of zones meroku did
// not create, which must not be read as "meroku deletes my DNS records".
func TestPlanViewerForgetIsNotADeletion(t *testing.T) {
	changes := []ResourceChange{
		{Address: `module.ses.aws_route53_record.spf_released["example.com"]`, Type: "aws_route53_record"},
		{Address: `module.ses.terraform_data.dns_slot["example.com|@|spf"]`, Type: "terraform_data"},
	}
	changes[0].Change.Actions = []string{"forget"}
	changes[1].Change.Actions = []string{"create"}

	groups := groupResourceChanges(changes)
	if len(groups.deletes) != 0 || len(groups.replaces) != 0 {
		t.Errorf("a forget was grouped as a deletion: %+v", groups)
	}
	stats := calculateStatistics(groups)
	if stats.byAction["delete"] != 0 {
		t.Errorf("a forget was counted as a delete: %+v", stats.byAction)
	}
	if got := getActionIcon("forget"); got == getActionIcon("delete") {
		t.Errorf("forget shows the delete icon %q", got)
	}
}
