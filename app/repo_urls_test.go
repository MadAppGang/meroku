package main

import "testing"

// The repository was renamed from MadAppGang/infrastructure to
// MadAppGang/meroku. GitHub redirects the old name today; nothing guarantees
// it will keep doing so.
func TestRepoURLs_PointAtMeroku(t *testing.T) {
	if infrastructureCloneURL != "https://github.com/MadAppGang/meroku.git" {
		t.Errorf("clone URL = %q", infrastructureCloneURL)
	}
	if latestVersionURL != "https://raw.githubusercontent.com/MadAppGang/meroku/main/version.txt" {
		t.Errorf("version URL = %q", latestVersionURL)
	}
}
