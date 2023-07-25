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
	for i := 0; i < len(lines); i++ {
		owner := ""
		if m := regexp.MustCompile(`^# owner: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			owner = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		from := ""
		if m := regexp.MustCompile(`^# from: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			from = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		to := ""
		if m := regexp.MustCompile(`^# to: (.+)$`).FindStringSubmatch(lines[i]); m != nil {
			to = m[1]
		} else {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
		}
		i++
		if i >= len(lines) {
			return nil, fmt.Errorf("error parsing rewrite rules (premature end of file)")
		}
		if !regexp.MustCompile(`^rewrite name (exact|regex) (\S+) (\S+)$`).MatchString(lines[i]) {
			return nil, fmt.Errorf("error parsing rewrite rules (at line %d)", i+1)
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
	lines := make([]string, 4*len(rs.rulesByFrom))
	i := 0
	for _, o := range slices.Sort(maps.Keys(rs.rulesByOwner)) {
		r := rs.rulesByOwner[o]
		lines[i] = fmt.Sprintf("# owner: %s", r.Owner)
		i++
		lines[i] = fmt.Sprintf("# from: %s", r.From)
		i++
		lines[i] = fmt.Sprintf("# to: %s", r.To)
		i++
		if r.From[0] == '*' {
			lines[i] = fmt.Sprintf("rewrite name regex %s %s", strings.ReplaceAll(strings.ReplaceAll(r.From, `.`, `\.`), `*`, `.*`), r.To)
		} else {
			lines[i] = fmt.Sprintf("rewrite name exact %s %s", r.From, r.To)
		}
		i++
	}
	return strings.Join(lines, "\n")
}
