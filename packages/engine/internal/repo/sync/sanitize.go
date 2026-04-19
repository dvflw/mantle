package sync

import "regexp"

// urlCredPattern matches https://user:password@host — the only URL
// form that can embed plaintext credentials that we need to strip
// before putting go-git errors into audit metadata. SSH URLs
// (git@host:path) are left untouched.
var urlCredPattern = regexp.MustCompile(`(https?://[^:\s/]+):([^@\s]+)@`)

// sanitizeURL redacts inline credentials from any HTTP(S) URLs in msg.
// Returns the original string when no credentialed URL is present.
func sanitizeURL(msg string) string {
	return urlCredPattern.ReplaceAllString(msg, "$1:***@")
}
