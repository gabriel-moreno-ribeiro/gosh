package main

import "strings"

// cut splits s around the first instance of sep; the same as strings.Cut, which only
// exists from Go 1.18 on.
func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}
