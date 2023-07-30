/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func checkRuleSetConsistency(rs *RewriteRuleSet) error {
	/*
	   assumptions:
	     for all keys rulesByOwner (owner)
	       rulesByOwner[owner].Owner == owner
	       rulesByFrom[rulesByOwner[owner].From] == rulesByOwner[owner]
	     for all keys rulesByFrom (from)
	       rulesByFrom[from].From == from
	       rulesByOwner[rulesByFrom[from].Owner] == rulesByFrom[from]
	   consequences:
	     values rulesByOwner == values rulesByFrom := values
	     keys rulesByOwner == values.collect(.Owner)
	     keys rulesByFrom == values.collect(.From)
	*/
	for o, r := range rs.rulesByOwner {
		if r.Owner != o {
			return fmt.Errorf("ruleset inconsistent (1)")
		}
		if rs.rulesByFrom[r.From] != r {
			return fmt.Errorf("ruleset inconsistent (2)")
		}
	}
	for f, r := range rs.rulesByFrom {
		if r.From != f {
			return fmt.Errorf("ruleset inconsistent (3)")
		}
		if rs.rulesByOwner[r.Owner] != r {
			return fmt.Errorf("ruleset inconsistent (4)")
		}
	}
	return nil
}

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
	from9  = "from9.other.io"
	to1    = "to1.example.io"
	to2    = "to2.example.io"
	to3    = "to3.example.io"
	to4    = "1.2.3.4"
	to9    = "to9.example.io"
)

func createSampleRuleSet() *RewriteRuleSet {
	r1 := &RewriteRule{Owner: owner1, From: from1, To: to1}
	r2 := &RewriteRule{Owner: owner2, From: from2, To: to2}
	r3 := &RewriteRule{Owner: owner3, From: from3, To: to3}
	r4 := &RewriteRule{Owner: owner4, From: from4, To: to4}

	rs := &RewriteRuleSet{
		rulesByFrom:  map[string]*RewriteRule{from1: r1, from2: r2, from3: r3, from4: r4},
		rulesByOwner: map[string]*RewriteRule{owner1: r1, owner2: r2, owner3: r3, owner4: r4},
	}

	if err := checkRuleSetConsistency(rs); err != nil {
		panic(err)
	}

	return rs
}

func createSampleRuleSetString() string {
	return fmt.Sprintf("hosts {\n  # owner: %[11]s\n  # from: %[12]s\n  # to: %[13]s\n  %[13]s %[12]s\n  fallthrough\n}\n# owner: %[1]s\n# from: %[2]s\n# to: %[3]s\nrewrite name exact %[2]s %[3]s\n# owner: %[4]s\n# from: %[5]s\n# to: %[6]s\nrewrite name exact %[5]s %[6]s\n# owner: %[7]s\n# from: %[8]s\n# to: %[9]s\nrewrite name regex %[10]s %[9]s",
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
	if !reflect.DeepEqual(r, &RewriteRule{Owner: owner2, From: from2, To: to2}) {
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
	if err := rs.AddRule(RewriteRule{Owner: owner1, From: from1, To: to1}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: add identical rule: ruleset changed although it shouldn't be", testName)
	}
}

func TestAddRule2(t *testing.T) {
	testName := "add rule with existing owner and same from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: owner1, From: from1, To: to9}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner1].To = to9
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule3(t *testing.T) {
	testName := "add rule with existing owner and new from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: owner1, From: from9, To: to9}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner1].From = from9
	rsexp.rulesByOwner[owner1].To = to9
	rsexp.rulesByFrom[from9] = rsexp.rulesByOwner[owner1]
	delete(rsexp.rulesByFrom, from1)
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule4(t *testing.T) {
	testName := "add rule with existing owner and conflicting from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: owner1, From: from2, To: to9}); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule5(t *testing.T) {
	testName := "add rule with new owner and new from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: owner9, From: from9, To: to9}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner[owner9] = &RewriteRule{Owner: owner9, From: from9, To: to9}
	rsexp.rulesByFrom[from9] = rsexp.rulesByOwner[owner9]
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule6(t *testing.T) {
	testName := "add rule with new owner and existing from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: owner9, From: from1, To: to9}); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestRemoveRule1(t *testing.T) {
	testName := "remove existing rule"
	rs := createSampleRuleSet()
	if err := rs.RemoveRule(owner1); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	delete(rsexp.rulesByOwner, owner1)
	delete(rsexp.rulesByFrom, from1)
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestRemoveRule2(t *testing.T) {
	testName := "remove non-existing rule"
	rs := createSampleRuleSet()
	if err := rs.RemoveRule(owner9); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
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
