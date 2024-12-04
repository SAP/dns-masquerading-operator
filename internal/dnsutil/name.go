/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package dnsutil

import (
	"fmt"
	"regexp"
)

var (
	anycaseRegex   = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
	lowercaseRegex = regexp.MustCompile(`^([a-z0-9]|[a-z0-9][a-z0-9\-]{0,61}[a-z0-9])(\.([a-z0-9]|[a-z0-9][a-z0-9\-]{0,61}[a-z0-9]))*$`)
)

// Check that given string represents a valid DNS name according to RFC1123;
// allowUppercase is self-explanatory;
// allowWildcard means that the first DNS label may be an asterisk (*).
func CheckDnsName(s string, allowUppercase bool, allowWildcard bool) error {
	if allowWildcard {
		s = regexp.MustCompile(`^\*(.*)$`).ReplaceAllString(s, `wildcard$1`)
	}
	if len(s) > 255 {
		return fmt.Errorf("not a valid DNS name")
	}
	var regex *regexp.Regexp
	if allowUppercase {
		regex = anycaseRegex
	} else {
		regex = lowercaseRegex
	}
	if !regex.MatchString(s) {
		return fmt.Errorf("not a valid DNS name")
	}
	return nil
}
