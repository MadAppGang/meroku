package awsecs

import "strings"

// SplitImageRef splits a container image reference into the repository part and
// the tag or digest that follows it.
//
//	registry/repo:tag                  -> "registry/repo",      "tag"
//	registry/repo@sha256:deadbeef      -> "registry/repo",      "sha256:deadbeef"
//	registry.example.com:5000/repo:tag -> "registry.example.com:5000/repo", "tag"
//	registry/repo                      -> "registry/repo",      ""
//
// A bare strings.LastIndex(ref, ":") gets both the digest form and the
// host:port form wrong, which made image matching silently skip containers.
func SplitImageRef(ref string) (repo, version string) {
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	i := strings.LastIndex(ref, ":")
	if i < 0 {
		return ref, ""
	}
	// A colon before the last '/' belongs to a registry port, not to a tag.
	if strings.LastIndex(ref, "/") > i {
		return ref, ""
	}
	return ref[:i], ref[i+1:]
}
