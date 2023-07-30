/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package netutil

import (
	"fmt"
	"net"
	"regexp"
)

func CheckDnsName(s string, allowWildcard bool) error {
	if allowWildcard {
		s = regexp.MustCompile(`^\*\.(.+)$`).ReplaceAllString(s, `wildcard.$1`)
	}
	if len(s) > 255 {
		return fmt.Errorf("not a valid DNS name")
	}
	if !regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`).MatchString(s) {
		return fmt.Errorf("not a valid DNS name")
	}
	return nil
}

func IsWildcardDnsName(s string) bool {
	return len(s) > 0 && s[0] == '*'
}

func IsIpAddress(s string) bool {
	return net.ParseIP(s) != nil
}
