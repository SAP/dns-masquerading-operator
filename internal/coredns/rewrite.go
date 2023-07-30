/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sap/go-generics/maps"
	"github.com/sap/go-generics/slices"

	"github.com/sap/dns-masquerading-operator/internal/netutil"
)

type RewriteRule struct {
	Owner string
	From  string
	To    string
}

type RewriteRuleSet struct {
	rulesByFrom  map[string]*RewriteRule
	rulesByOwner map[string]*RewriteRule
}

func NewRewriteRuleSet() *RewriteRuleSet {
	return &RewriteRuleSet{
		rulesByFrom:  make(map[string]*RewriteRule),
		rulesByOwner: make(map[string]*RewriteRule),
	}
}

func ParseRewriteRuleSet(s string) (*RewriteRuleSet, error) {
	rs := NewRewriteRuleSet()
	if s == "" {
		return rs, nil
	}
	lines := strings.Split(s, "\n")
	have_hosts := false
	for i := 0; i < len(lines); i++ {
		if lines[i] == "hosts {" && !have_hosts {
			have_hosts = true
			continue
		}
		if lines[i] == "  fallthrough" && i+1 < len(lines) && lines[i+1] == "}" {
			have_hosts = false
			i++
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
		if err := rs.AddRule(RewriteRule{Owner: owner, From: from, To: to}); err != nil {
			return nil, err
		}
	}
	return rs, nil
}

func (rs *RewriteRuleSet) GetRule(owner string) *RewriteRule {
	if s, ok := rs.rulesByOwner[owner]; ok {
		return &RewriteRule{
			Owner: s.Owner,
			From:  s.From,
			To:    s.To,
		}
	}
	return nil
}

func (rs *RewriteRuleSet) AddRule(r RewriteRule) error {
	if err := netutil.CheckDnsName(r.From, true); err != nil {
		return fmt.Errorf("error adding rewrite rule %s:%s (%s); invalid source: %s", r.From, r.To, r.Owner, err)
	}
	if netutil.IsIpAddress(r.To) {
		if netutil.IsWildcardDnsName(r.From) {
			return fmt.Errorf("error adding rewrite rule %s:%s (%s); wildcards are not allowed with IP address targets", r.From, r.To, r.Owner)
		}
	} else {
		if err := netutil.CheckDnsName(r.To, false); err != nil {
			return fmt.Errorf("error adding rewrite rule %s:%s (%s); invalid target: %s", r.From, r.To, r.Owner, err)
		}
	}
	if s, ok := rs.rulesByFrom[r.From]; ok {
		// a rule with r.From exists
		if s.Owner == r.Owner {
			// and that rule's owner matches r.Owner
			s.To = r.To
		} else {
			// and that rule's owner does not match r.Owner (i.e. duplicate rule)
			return fmt.Errorf("error adding rewrite rule %s:%s (%s); conflicts with rule %s:%s (%s)", r.From, r.To, r.Owner, s.From, s.To, s.Owner)
		}
	} else {
		// no rule with r.From exists
		if s, ok := rs.rulesByOwner[r.Owner]; ok {
			// and a rule with r.Owner exists
			delete(rs.rulesByFrom, s.From)
			s.From = r.From
			s.To = r.To
			rs.rulesByFrom[s.From] = s
		} else {
			// and no rule with r.Owner exists
			rs.rulesByFrom[r.From] = &r
			rs.rulesByOwner[r.Owner] = &r
		}
	}
	return nil
}

func (rs *RewriteRuleSet) RemoveRule(owner string) error {
	if s, ok := rs.rulesByOwner[owner]; ok {
		delete(rs.rulesByFrom, s.From)
		delete(rs.rulesByOwner, owner)
	} else {
		return fmt.Errorf("error deleting rewrite rule with owner %s; no rule found with that owner", owner)
	}
	return nil
}

func (rs *RewriteRuleSet) String() string {
	lines := make([]string, 0, 4*len(rs.rulesByFrom)+3)
	haveHosts := false
	for _, o := range slices.Sort(maps.Keys(rs.rulesByOwner)) {
		r := rs.rulesByOwner[o]
		if !netutil.IsIpAddress(r.To) {
			continue
		}
		if !haveHosts {
			haveHosts = true
			lines = append(lines, "hosts {")
		}
		lines = append(lines, fmt.Sprintf("  # owner: %s", r.Owner))
		lines = append(lines, fmt.Sprintf("  # from: %s", r.From))
		lines = append(lines, fmt.Sprintf("  # to: %s", r.To))
		lines = append(lines, fmt.Sprintf("  %s %s", r.To, r.From))
	}
	if haveHosts {
		haveHosts = false
		lines = append(lines, "  fallthrough")
		lines = append(lines, "}")
	}
	for _, o := range slices.Sort(maps.Keys(rs.rulesByOwner)) {
		r := rs.rulesByOwner[o]
		if netutil.IsIpAddress(r.To) {
			continue
		}
		lines = append(lines, fmt.Sprintf("# owner: %s", r.Owner))
		lines = append(lines, fmt.Sprintf("# from: %s", r.From))
		lines = append(lines, fmt.Sprintf("# to: %s", r.To))
		if netutil.IsWildcardDnsName(r.From) {
			lines = append(lines, fmt.Sprintf("rewrite name regex %s %s", strings.ReplaceAll(strings.ReplaceAll(r.From, `.`, `\.`), `*`, `.*`), r.To))
		} else {
			lines = append(lines, fmt.Sprintf("rewrite name exact %s %s", r.From, r.To))
		}
	}
	return strings.Join(lines, "\n")
}
