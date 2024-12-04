/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/sap/dns-masquerading-operator/internal/dnsutil"
	"github.com/sap/go-generics/maps"
	"github.com/sap/go-generics/slices"
)

// Rewrite rule (usually derived from a MasqueradingRule object)
type RewriteRule struct {
	owner string
	from  string
	to    string
}

// Create new RewriteRule object (and validate input)
func NewRewriteRule(owner string, from string, to string) (*RewriteRule, error) {
	if err := dnsutil.CheckDnsName(from, false, true); err != nil {
		return nil, err
	}
	if net.ParseIP(to) == nil {
		if err := dnsutil.CheckDnsName(to, false, false); err != nil {
			return nil, err
		}
	} else {
		if strings.Split(from, ".")[0] == "*" {
			return nil, fmt.Errorf("error validating rewrite rule: source must not be a wildcard DNS name if target is an IP address")
		}
	}
	return &RewriteRule{owner: owner, from: from, to: to}, nil
}

// Return owner of a RewriteRule
func (r *RewriteRule) Owner() string {
	return r.owner
}

// Return rewrite source (from) of a RewriteRule
func (r *RewriteRule) From() string {
	return r.from
}

// Return rewrite target (to) of a RewriteRule
func (r *RewriteRule) To() string {
	return r.to
}

// Check if RewriteRule matches given DNS name; that is, if the rewrite rule's source
// is a wildcard DNS name, it is checked whether that wildcard name matches host
// (note that in that case, host may be a - less specific - wildcard pattern itself);
// otherwise, just check for equality of the rewrite rule's source and host.
func (r *RewriteRule) Matches(host string) bool {
	if strings.Split(r.from, ".")[0] == "*" {
		return strings.HasSuffix(host, r.from[1:])
	} else {
		return host == r.from
	}
}

// check if rewrite rule source is a wildcard DNS name
func (r *RewriteRule) fromIsWildcard() bool {
	return strings.Split(r.from, ".")[0] == "*"
}

// check if rewrite rule target is an IP address
func (r *RewriteRule) toIsIpaddress() bool {
	return net.ParseIP(r.to) != nil
}

// Set of RewriteRule
type RewriteRuleSet struct {
	rulesByOwner map[string]*RewriteRule
}

// Create empty RewriteRuleSet; RewriteRuleSet gives the following guarantees:
//   - uniquness of owners, that is, for a given owner, the set contains
//     at most one RewriteRule with that owner
//   - rewrite sources in the set are free of clashes; that is, for a given DNS name,
//     there will be at most one RewriteRule matching that DNS name (via Matches()).
func NewRewriteRuleSet() *RewriteRuleSet {
	return &RewriteRuleSet{
		rulesByOwner: make(map[string]*RewriteRule),
	}
}

// Parse RewriteRuleSet from a coredns config file format
func ParseRewriteRuleSet(s string) (*RewriteRuleSet, error) {
	rs := NewRewriteRuleSet()
	if s == "" {
		return rs, nil
	}
	lines := strings.Split(s, "\n")
	have_hosts := false
	for i := 0; i < len(lines); i++ {
		if lines[i] == "hosts /dev/null {" && !have_hosts {
			have_hosts = true
			continue
		}
		if i+2 < len(lines) && lines[i] == "  ttl 10" && lines[i+1] == "  fallthrough" && lines[i+2] == "}" {
			have_hosts = false
			i += 2
			continue
		}
		owner := ""
		if m := regexp.MustCompile(`^\s*# owner: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			owner = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		from := ""
		if m := regexp.MustCompile(`^\s*# from: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			from = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		to := ""
		if m := regexp.MustCompile(`^\s*# to: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			to = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		if have_hosts {
			if !regexp.MustCompile(`^\s*\S+\s+\S+$`).MatchString(lines[i]) {
				return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
			}
		} else {
			if !regexp.MustCompile(`^\s*rewrite name (exact|regex) (\S+) (\S+)$`).MatchString(lines[i]) {
				return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
			}
		}
		r, err := NewRewriteRule(owner, from, to)
		if err != nil {
			return nil, err
		}
		if _, err := rs.AddRule(r); err != nil {
			return nil, err
		}
	}
	return rs, nil
}

// Get RewriteRule for specified owner; return nil if none was found;
// otherwise, the result is unique because of the guarantees given by RewriteRuleSet.
func (rs *RewriteRuleSet) GetRule(owner string) *RewriteRule {
	if r, ok := rs.rulesByOwner[owner]; ok {
		return r
	}
	return nil
}

// Find RewriteRule matching given DNS name; return nil if none was found;
// otherwise, the result is unique because of the guarantees given by RewriteRuleSet.
func (rs *RewriteRuleSet) FindMatchingRule(host string) *RewriteRule {
	for _, s := range rs.rulesByOwner {
		if s.Matches(host) {
			return s
		}
	}
	return nil
}

// Add RewriteRule to set; may fail if the given rule would violate the consistency guarantees of the RewriteRuleSet;
// the boolean return value indicates whether something changed in the set (true) or if the rule was already there (false).
func (rs *RewriteRuleSet) AddRule(r *RewriteRule) (bool, error) {
	var s *RewriteRule
	for _, t := range rs.rulesByOwner {
		if t.owner != r.owner && (t.Matches(r.from) || r.Matches(t.from)) {
			s = t
			break
		}
	}
	if s != nil {
		return false, fmt.Errorf("error adding rewrite rule %s:%s (%s); conflicts with rule %s:%s (%s)", r.from, r.to, r.owner, s.from, s.to, s.owner)
	}
	s = rs.rulesByOwner[r.owner]
	changed := s == nil || r.from != s.from || r.to != s.to
	rs.rulesByOwner[r.owner] = r
	return changed, nil
}

// Remove rule with given owner from set;
// the boolean return value indicates whether something changed in the set (true) or if no rule with that owner was existing (false).
func (rs *RewriteRuleSet) RemoveRule(owner string) bool {
	if _, ok := rs.rulesByOwner[owner]; ok {
		delete(rs.rulesByOwner, owner)
		return true
	}
	return false
}

// Serialize RewriteRuleSet into coredns config file format
func (rs *RewriteRuleSet) String() string {
	lines := make([]string, 0, 4*len(rs.rulesByOwner)+3)
	haveHosts := false
	for _, o := range slices.Sort(maps.Keys(rs.rulesByOwner)) {
		r := rs.rulesByOwner[o]
		if !r.toIsIpaddress() {
			continue
		}
		if !haveHosts {
			haveHosts = true
			lines = append(lines, "hosts /dev/null {")
		}
		lines = append(lines, fmt.Sprintf("  # owner: %s", r.owner))
		lines = append(lines, fmt.Sprintf("  # from: %s", r.from))
		lines = append(lines, fmt.Sprintf("  # to: %s", r.to))
		lines = append(lines, fmt.Sprintf("  %s %s", r.to, r.from))
	}
	if haveHosts {
		haveHosts = false
		lines = append(lines, "  ttl 10")
		lines = append(lines, "  fallthrough")
		lines = append(lines, "}")
	}
	for _, o := range slices.Sort(maps.Keys(rs.rulesByOwner)) {
		r := rs.rulesByOwner[o]
		if r.toIsIpaddress() {
			continue
		}
		lines = append(lines, fmt.Sprintf("# owner: %s", r.owner))
		lines = append(lines, fmt.Sprintf("# from: %s", r.from))
		lines = append(lines, fmt.Sprintf("# to: %s", r.to))
		if r.fromIsWildcard() {
			lines = append(lines, fmt.Sprintf("rewrite name regex %s %s", strings.ReplaceAll(strings.ReplaceAll(r.from, `.`, `\.`), `*`, `.*`), r.to))
		} else {
			lines = append(lines, fmt.Sprintf("rewrite name exact %s %s", r.from, r.to))
		}
	}
	return strings.Join(lines, "\n")
}
