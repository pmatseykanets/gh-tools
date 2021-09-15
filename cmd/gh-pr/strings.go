package main

import "strings"

// Returns true if the string needle is in the slice hay.
// It uses case insensitive comparison.
func contains(hay []string, needle string) bool {
	for _, val := range hay {
		if strings.EqualFold(val, needle) {
			return true
		}
	}
	return false
}
