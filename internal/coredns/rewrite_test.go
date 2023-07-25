/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"fmt"
	"reflect"
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

func createSampleRuleSet() *RewriteRuleSet {
	r1 := &RewriteRule{Owner: "o1", From: "f1", To: "ta"}
	r2 := &RewriteRule{Owner: "o2", From: "f2", To: "tb"}
	r3 := &RewriteRule{Owner: "o3", From: "*f3", To: "tb"}

	rs := &RewriteRuleSet{
		rulesByFrom:  map[string]*RewriteRule{"f1": r1, "f2": r2, "*f3": r3},
		rulesByOwner: map[string]*RewriteRule{"o1": r1, "o2": r2, "o3": r3},
	}

	if err := checkRuleSetConsistency(rs); err != nil {
		panic(err)
	}

	return rs
}

func createSampleRuleSetString() string {
	return "# owner: o1\n# from: f1\n# to: ta\nrewrite name exact f1 ta\n# owner: o2\n# from: f2\n# to: tb\nrewrite name exact f2 tb\n# owner: o3\n# from: *f3\n# to: tb\nrewrite name regex .*f3 tb"
}

func TestGetRule1(t *testing.T) {
	testName := "get existing rule"
	rs := createSampleRuleSet()
	r := rs.GetRule("o2")
	if r == nil {
		t.Fatalf("%s: unable to get existing rule", testName)
	}
	if !reflect.DeepEqual(r, &RewriteRule{Owner: "o2", From: "f2", To: "tb"}) {
		t.Fatalf("%s: got unexpected rule", testName)
	}
}

func TestGetRule2(t *testing.T) {
	testName := "get non-existing rule"
	rs := createSampleRuleSet()
	r := rs.GetRule("o9")
	if r != nil {
		t.Fatalf("%s: unexpectedly found non-existing rule", testName)
	}
}

func TestAddRule1(t *testing.T) {
	testName := "add identical rule"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: "o1", From: "f1", To: "ta"}); err != nil {
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
	if err := rs.AddRule(RewriteRule{Owner: "o1", From: "f1", To: "tx"}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner["o1"].To = "tx"
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule3(t *testing.T) {
	testName := "add rule with existing owner and new from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: "o1", From: "f9", To: "tx"}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner["o1"].From = "f9"
	rsexp.rulesByOwner["o1"].To = "tx"
	rsexp.rulesByFrom["f9"] = rsexp.rulesByOwner["o1"]
	delete(rsexp.rulesByFrom, "f1")
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule4(t *testing.T) {
	testName := "add rule with existing owner and conflicting from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: "o1", From: "f2", To: "tx"}); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestAddRule5(t *testing.T) {
	testName := "add rule with new owner and new from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: "o9", From: "f9", To: "tx"}); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	if err := checkRuleSetConsistency(rs); err != nil {
		t.Fatalf("%s: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	rsexp.rulesByOwner["o9"] = &RewriteRule{Owner: "o9", From: "f9", To: "tx"}
	rsexp.rulesByFrom["f9"] = rsexp.rulesByOwner["o9"]
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestAddRule6(t *testing.T) {
	testName := "add rule with new owner and existing from"
	rs := createSampleRuleSet()
	if err := rs.AddRule(RewriteRule{Owner: "o9", From: "f1", To: "tx"}); err == nil {
		t.Fatalf("%s: got unexpected success", testName)
	} else {
		t.Logf("%s: got error: %s", testName, err)
	}
}

func TestRemoveRule1(t *testing.T) {
	testName := "remove existing rule"
	rs := createSampleRuleSet()
	if err := rs.RemoveRule("o1"); err != nil {
		t.Fatalf("%s: got unexpected error: %s", testName, err)
	}
	rsexp := createSampleRuleSet()
	delete(rsexp.rulesByOwner, "o1")
	delete(rsexp.rulesByFrom, "f1")
	if !reflect.DeepEqual(rs, rsexp) {
		t.Errorf("%s: unexpected ruleset", testName)
	}
}

func TestRemoveRule2(t *testing.T) {
	testName := "remove non-existing rule"
	rs := createSampleRuleSet()
	if err := rs.RemoveRule("o9"); err == nil {
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
