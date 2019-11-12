// Copyright (c) 2018-2019, AT&T Intellectual Property
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only
//
// These tests verify that the split function defined in main.go correctly
// splits a string in to fields. Special attention is given to stripping
// extra white space and dealing with quoted fields, which may embed escaped
// quotation marks.

package main

import (
	"testing"
)

func checkSplit(t *testing.T, source string, expect []string) {
	results := split(source)
	if len(results) != len(expect) {
		t.Fatalf("Length of split results is incorrect:\n  Got: %#v\n  Exp: %#v\n", results, expect)
	}

	for k, _ := range results {
		if results[k] != expect[k] {
			t.Fatalf("Split results is incorrect:\n  Got: %#v\n  Exp: %#v\n", results, expect)
		}
	}
}

// Simple split of string in to fields
func TestSplit(t *testing.T) {
	checkSplit(t,
		"interfaces dataplane dp0s3 description test",
		[]string{"interfaces", "dataplane", "dp0s3", "description", "test"})
}

// Split string, removing extraneous space
func TestSplitStripSpaces(t *testing.T) {
	checkSplit(t,
		"    interfaces  dataplane dp0s3  description test	",
		[]string{"interfaces", "dataplane", "dp0s3", "description", "test"})
}

// Split string, stripping outer quotes
func TestBasicQuoted(t *testing.T) {
	checkSplit(t,
		"    interfaces  dataplane dp0s3  'description' \"test\"	",
		[]string{"interfaces", "dataplane", "dp0s3", "description", "test"})
}

// Split string, preserving spaces within quotes
func TestBasicQuotedSpaces(t *testing.T) {
	checkSplit(t,
		"    interfaces  dataplane dp0s3  'description' \" test   description \"	",
		[]string{"interfaces", "dataplane", "dp0s3", "description", " test   description "})
}

// Split string, preserving same, escaped quote
func TestBasicQuotesInQuotes(t *testing.T) {
	checkSplit(t,
		"    interfaces  dataplane dp0s3  description 'test   \"description \"'",
		[]string{"interfaces", "dataplane", "dp0s3", "description", "test   \"description \""})
}

func checkArgs(t *testing.T, args []string, expected *WaitInput) {
	wi := getArgs(args)

	if wi.timeout != expected.timeout {
		t.Fatalf("Timeouts mismatch:  Got: %d\n Exp: %d\n", wi.timeout, expected.timeout)
	}
	if wi.verbose != expected.verbose {
		t.Fatalf("Verbose mismatch:  Got: %t\n Exp: %t\n", wi.verbose, expected.verbose)
	}

	if len(wi.set) != len(expected.set) {
		t.Fatalf("set length mismatch:  Got: %d\n Exp: %d\n", len(wi.set), len(expected.set))
	}

	for i, _ := range wi.set {
		if wi.set[i] != expected.set[i] {
			t.Fatalf("set mismatch:  Got: %#v\n Exp: %#v\n", wi.set, expected.set)
		}
	}

	if len(wi.delete) != len(expected.delete) {
		t.Fatalf("set length mismatch:  Got: %d\n Exp: %d\n", len(wi.delete), len(expected.delete))
	}

	for i, _ := range wi.delete {
		if wi.delete[i] != expected.delete[i] {
			t.Fatalf("set mismatch:  Got: %#v\n Exp: %#v\n", wi.delete, expected.delete)
		}
	}
}

// Simple arguments
func TestBasicArgs(t *testing.T) {
	checkArgs(t,
		[]string{"verbose", "timeout", "64", "dp0s3", "set", "abc def",
			"delete", "interfaces dataplane dp0s9", "dp0s4"},
		&WaitInput{timeout: 64, verbose: true,
			set:    []string{"abc def"},
			delete: []string{"interfaces dataplane dp0s9"},
			intf:   []string{"dp0s3"}})
}

// Arguments with timeout and verbose as set/delete fields
func TestBasicArgsTwo(t *testing.T) {
	checkArgs(t,
		[]string{"timeout", "64", "dp0s3", "set", "timeout",
			"delete", "verbose"},
		&WaitInput{timeout: 64, verbose: false,
			set:    []string{"timeout"},
			delete: []string{"verbose"},
			intf:   []string{"dp0s3"}})
}

// Check that absent verbose is false
func TestVerboseFalse(t *testing.T) {
	checkArgs(t,
		[]string{"timeout", "64", "dp0s3", "set", "abc def",
			"delete", "interfaces dataplane dp0s9"},
		&WaitInput{timeout: 64, verbose: false,
			set:    []string{"abc def"},
			delete: []string{"interfaces dataplane dp0s9"},
			intf:   []string{"dp0s3"}})
}

// Test empty set args
func TestNoSets(t *testing.T) {
	checkArgs(t,
		[]string{"verbose", "timeout", "64", "dp0s3",
			"delete", "interfaces dataplane dp0s9"},
		&WaitInput{timeout: 64, verbose: true,
			set:    []string{},
			delete: []string{"interfaces dataplane dp0s9"},
			intf:   []string{"dp0s3"}})
}

// Test empty delete args
func TestNoDeletes(t *testing.T) {
	checkArgs(t,
		[]string{"verbose", "timeout", "64", "dp0s3",
			"set", "interfaces dataplane dp0s9"},
		&WaitInput{timeout: 64, verbose: true,
			set:    []string{"interfaces dataplane dp0s9"},
			delete: []string{},
			intf:   []string{"dp0s3"}})
}

// Test no interfaces args
func TestNoInterfaces(t *testing.T) {
	checkArgs(t,
		[]string{"verbose", "timeout", "64",
			"set", "interfaces dataplane dp0s8",
			"delete", "interfaces dataplane dp0s9"},
		&WaitInput{timeout: 64, verbose: true,
			set:    []string{"interfaces dataplane dp0s8"},
			delete: []string{"interfaces dataplane dp0s9"},
			intf:   []string{}})
}

// Test multiple, mixed arguments
func TestMiltipleMixed(t *testing.T) {
	checkArgs(t,
		[]string{"timeout", "64", "dp0s3", "set", "abc def",
			"delete", "interfaces dataplane dp0s9",
			"dp0s4", "verbose", "set", "interfaces tunnel",
			"delete", "def gef"},
		&WaitInput{timeout: 64, verbose: true,
			set: []string{"abc def", "interfaces tunnel"},
			delete: []string{"interfaces dataplane dp0s9",
				"def gef"},
			intf: []string{"dp0s3", "dp0s4"}})
}
