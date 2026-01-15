/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TODO: add tests for NewRewriteRule and RewriteRule methods

const (
	owner1 = "owner1"
	owner2 = "owner2"
	owner3 = "owner3"
	owner4 = "owner4"
	owner9 = "owner9"
	from1  = "from1.example.io"
	from2  = "from2.example.io"
	from3  = "*.other.io"
	from4  = "from4.example.io"
	from7  = "*.example.io"
	from8  = "from10.example.io"
	from9  = "from9.other.io"
	to1    = "to1.example.io"
	to2    = "to2.example.io"
	to3    = "to3.example.io"
	to4    = "1.2.3.4"
	to9    = "to9.example.io"
)

func mustNewRewriteRule(owner string, from string, to string) *RewriteRule {
	r, err := NewRewriteRule(owner, from, to)
	if err != nil {
		panic(err)
	}
	return r
}

func checkRuleSetConsistency(rs *RewriteRuleSet) error {
	for o, r := range rs.rulesByOwner {
		if r.owner != o {
			return fmt.Errorf("ruleset inconsistent (1)")
		}
		for _, s := range rs.rulesByOwner {
			if (s.Matches(r.from) || r.Matches(s.from)) && s.owner != o {
				return fmt.Errorf("ruleset inconsistent (2)")
			}
		}
	}
	return nil
}

func createSampleRuleSet() *RewriteRuleSet {
	r1 := mustNewRewriteRule(owner1, from1, to1)
	r2 := mustNewRewriteRule(owner2, from2, to2)
	r3 := mustNewRewriteRule(owner3, from3, to3)
	r4 := mustNewRewriteRule(owner4, from4, to4)

	rs := &RewriteRuleSet{
		rulesByOwner: map[string]*RewriteRule{owner1: r1, owner2: r2, owner3: r3, owner4: r4},
	}

	if err := checkRuleSetConsistency(rs); err != nil {
		panic(err)
	}

	return rs
}

func createSampleRuleSetString() string {
	return fmt.Sprintf("hosts /dev/null {\n  # owner: %[11]s\n  # from: %[12]s\n  # to: %[13]s\n  %[13]s %[12]s\n  ttl 10\n  fallthrough\n}\n# owner: %[1]s\n# from: %[2]s\n# to: %[3]s\nrewrite name exact %[2]s %[3]s\n# owner: %[4]s\n# from: %[5]s\n# to: %[6]s\nrewrite name exact %[5]s %[6]s\n# owner: %[7]s\n# from: %[8]s\n# to: %[9]s\nrewrite name regex %[10]s %[9]s",
		owner1,
		from1,
		to1,
		owner2,
		from2,
		to2,
		owner3,
		from3,
		to3,
		strings.ReplaceAll(strings.ReplaceAll(from3, `.`, `\.`), `*`, `.*`),
		owner4,
		from4,
		to4,
	)
}

func TestGetRule1(t *testing.T) {
	testName := "get existing rule"
	rs := createSampleRuleSet()
	r := rs.GetRule(owner2)
	if r == nil {
		t.Fatalf("%s: unable to get existing rule", testName)
	}
	if !reflect.DeepEqual(r, &RewriteRule{owner: owner2, from: from2, to: to2}) {
		t.Fatalf("%s: got unexpected rule", testName)
	}
}

func TestGetRule2(t *testing.T) {
	testName := "get non-existing rule"
	rs := createSampleRuleSet()
	r := rs.GetRule(owner9)
	if r != nil {
		t.Fatalf("%s: unexpectedly found non-existing rule", testName)
	}
}

func TestAddRule1(t *testing.T) {
	testName := "add identical rule"
	rs := createSampleRuleSet()
	changed, err := rs.AddRule(mustNewRewriteRule(owner1, from1, to1))
	if err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	if changed {
		t.Errorf("%s: ruleset change indicated although there was none", testName)
	}
	rsexp := createSampleRuleSet()
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule2(t *testing.T) {
	testName := "add rule with existing owner and same from"
	rs := createSampleRuleSet()
	changed, err := rs.AddRule(mustNewRewriteRule(owner1, from1, to9))
	if err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	if !changed {
		t.Errorf("%s: no ruleset change indicated although there was one", testName)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner1].to = to9
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule3(t *testing.T) {
	testName := "add rule with existing owner and new from"
	rs := createSampleRuleSet()
	changed, err := rs.AddRule(mustNewRewriteRule(owner1, from8, to9))
	if err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	if !changed {
		t.Errorf("%s: no ruleset change indicated although there was one", testName)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner1].from = from8
	rsexp.rulesByOwner[owner1].to = to9
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule4(t *testing.T) {
	testName := "add rule with existing owner and conflicting from (1)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner1, from2, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule5(t *testing.T) {
	testName := "add rule with existing owner and conflicting from (2)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner1, from9, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule6(t *testing.T) {
	testName := "add rule with existing owner and conflicting from (3)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner1, from7, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule7(t *testing.T) {
	testName := "add rule with new owner and new from"
	rs := createSampleRuleSet()
	changed, err := rs.AddRule(mustNewRewriteRule(owner9, from8, to9))
	if err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	if !changed {
		t.Errorf("%s: no ruleset change indicated although there was one", testName)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner9] = mustNewRewriteRule(owner9, from8, to9)
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule8(t *testing.T) {
	testName := "add rule with new owner and conflicting from (1)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner9, from1, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule9(t *testing.T) {
	testName := "add rule with new owner and conflicting from (2)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner9, from7, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule10(t *testing.T) {
	testName := "add rule with new owner and conflicting from (3)"
	rs := createSampleRuleSet()
	if _, err := rs.AddRule(mustNewRewriteRule(owner9, from9, to9)); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestRemoveRule1(t *testing.T) {
	testName := "remove existing rule"
	rs := createSampleRuleSet()
	changed := rs.RemoveRule(owner1)
	if !changed {
		t.Errorf("%s: no ruleset change indicated although there was one", testName)
	}
	rsexp := createSampleRuleSet()
	delete(rsexp.rulesByOwner, owner1)
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestRemoveRule2(t *testing.T) {
	testName := "remove non-existing rule"
	rs := createSampleRuleSet()
	changed := rs.RemoveRule(owner9)
	if changed {
		t.Errorf("%s: ruleset change indicated although there was none", testName)
	}
	rsexp := createSampleRuleSet()
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestUnparseRuleSet(t *testing.T) {
	testName := "unparse ruleset"
	rs := createSampleRuleSet()
	want := createSampleRuleSetString()
	got := rs.String()
	if got != want {
		t.Fatalf("%s: got unexpected string;\ngot:\n%s\n\nwant:\n%s", testName, got, want)
	}
}

func TestParseRuleSet(t *testing.T) {
	testName := "parse ruleset"
	rs, err := ParseRewriteRuleSet(createSampleRuleSetString())
	if err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if !reflect.DeepEqual(rs, createSampleRuleSet()) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}
