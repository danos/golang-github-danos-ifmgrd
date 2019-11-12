// Copyright (c) 2018-2019, AT&T Intellectual Property
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

/*
 qa_notify handles the vRouter operational CLI command
 "qa-notify wait ifmgr", which waits for ifmgrd running configuration to
 reach a specified state.

Usage:

        qa-notify [verbose] [timeout <seconds>] [<interface>]
	          [set <path>] [delete <path>]

	timeout <seconds>
		Defines a timeout period in seconds.
		If the specified state is not seen within the timeout
		specified, a timeout expired error will be shown, and
		qa-notify will exit with an error code.
		When not specified, it defaults to 15 seconds.

	verbose
		Switch on verbose output that can be useful in debugging
		issues.

	<interface-name>
		An interface name, as specified in the configuration
		"set interfaces <interface-type> <interface-name>". When
		specified, qa-notify will wait wait until the interfaces
		configuration agree between configd and ifmgrd.
		Note: Multitple interfaces can be specified

	set <path>
		Specifies a configuration path of the form
		"interfaces <interface-type> <interface-name>...."
		When specified, qa-notify will wait until the specified
		configuration is present in ifmgrd's running configuration.
		Note: Multiple set paths can be specified

	delete <path>
		Specifies a configuration path of the form
		"interfaces <interface-type> <interface-name>...."
		When specified, qa-notify will wait until the specified
		configuration is NOT present in ifmgrd's running configuration.
		Note: Multiple delete paths can be specified

Examples:
    "qa-notify verbose timeout 60 dp0s3 tun8 dp0p1s1"

	Will wait up to 60 seconds for the configuration of interfaces
	"dp0s3", "tun8" and "dp0p1s1" to agree when comparing configd
	and ifmgrd views of the configuration


    "qa-notify set "interfaces dataplane dp0s3 description test"

	Will wait until ifmgrd has "interfaces dataplane dp0s3 description test"	is present in the running configuration.

*/

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/danos/config/data"
	"github.com/danos/config/diff"
	"github.com/danos/config/schema"
	"github.com/danos/config/union"
	"github.com/danos/config/yangconfig"
	configd_client "github.com/danos/configd/client"
	"github.com/danos/configd/rpc"
	"github.com/danos/ifmgrd"
	"github.com/danos/vci"
	"github.com/danos/yang/compile"
)

type WaitInput struct {
	path    string
	set     []string
	delete  []string
	intf    []string
	timeout uint32
	verbose bool
}

var waitInput *WaitInput

// Split a string into fields, accounting for quotes
// as an example:
// interfaces dataplane dp0s3 desc "test desc"
// returns
// []string{"interfaces", "dataplane", "dp0s3", "desc", "test desc"}
//
func split(path string) []string {
	quote := rune(0)
	space := 0
	escaped := 0
	splitter := func(c rune) bool {
		if space > 0 {
			space--
		}
		if escaped > 0 {
			escaped--
		}

		switch {
		case c == quote:
			if escaped == 0 {
				quote = rune(0)
				return true
			}
			return false
		case quote != rune(0):
			// ignore all runes until next matching quote
			return false
		case unicode.In(c, unicode.Quotation_Mark):
			if space > 0 {
				// a quote with preceeding space
				// opening quote
				quote = c
				return true
			}
			return false
		case unicode.IsSpace(c):
			space = 2
			return true
		case c == rune('\\'):
			if escaped > 0 {
				escaped = 0
			} else {
				escaped = 2
			}
			return false
		}
		return false
	}
	return strings.FieldsFunc(path, splitter)
}

// Determine if the specified config path is present in an interfaces
// running config
func configured(client *ifmgrd.Client, st schema.Node, path string) (bool, error) {
	ps := split(path)
	if len(ps) < 3 {
		return false, nil
	}
	cfg, err := client.Running(ps[2])
	if cfg == "" {
		cfg = "{}"
	}
	run, err := union.UnmarshalJSONWithoutValidation(st, []byte(cfg))
	if err != nil {
		return false, err
	}
	run.Merge()
	exists := run.Exists(nil, ps)
	return exists == nil, nil
}

// Check if individual set/delete configuration items are present
// in the current ifmgr RUNNING config
func isSet(client *ifmgrd.Client, st schema.Node, w *WaitInput) (bool, error) {
	result := true
	for _, s := range w.set {
		b, err := configured(client, st, s)
		if err != nil {
			return false, err
		}
		if b == false {
			if w.verbose {
				fmt.Printf("\nSet path not present: [%s]\n", s)
			}
			result = false
		}
	}

	for _, s := range w.delete {
		b, err := configured(client, st, s)
		if b == true || err != nil {
			if err != nil {
				return false, err
			}
			if w.verbose {
				fmt.Printf("\nDelete path present: [%s]\n", s)
			}
			result = false
		}
	}
	return result, nil
}

// Get an interfaces configuration
func findCommitRoot(name string, tree *data.Node) *data.Node {
	path := []string{"interfaces"}
	intfTree := tree.Child("interfaces")
	for _, intfType := range intfTree.Children() {
		pathToType := append(path, intfType.Name())
		for _, intf := range intfType.Children() {
			if intf.Name() == name {
				out := data.New("root")
				n := out
				for _, ch := range pathToType {
					chn := data.New(ch)
					n.AddChild(chn)
					n = chn
				}
				n.AddChild(intf)
				return out
			}
		}
	}
	return nil
}

func schemaGet() (schema.Node, error) {
	ycfg := yangconfig.NewConfig().IncludeYangDirs("/usr/share/configd/yang").
		IncludeFeatures(compile.DefaultCapsLocation).SystemConfig()

	return schema.CompileDir(
		&compile.Config{
			YangLocations: ycfg.YangLocator(),
			Features:      ycfg.FeaturesChecker(),
			Filter:        compile.IsConfig},
		nil)
}

func getInterfaceRunning(client *ifmgrd.Client, st schema.Node, intf string) (*data.Node, error) {
	//get ifmgrd version for configuration for interface
	run, err := client.Running(intf)
	if err != nil {
		return nil, err
	}
	if run == "" {
		run = "{}"
	}
	rt, err := union.UnmarshalJSONWithoutValidation(st, []byte(run))
	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}
	rtree := rt.Merge()
	rn := findCommitRoot(intf, rtree)
	return rn, nil

}

func configdTreeGet(st schema.Node) (*data.Node, error) {
	configdClient, err := configd_client.Dial(
		"unix",
		"/run/vyatta/configd/main.sock",
		os.Getenv("VYATTA_CONFIGD_SID"))
	if err != nil {
		return nil, err
	}
	defer configdClient.Close()

	cfg, err := configdClient.TreeGet(rpc.CANDIDATE, "", "json")
	if err != nil {
		return nil, err
	}

	ut, err := union.UnmarshalJSONWithoutValidation(st, []byte(cfg))
	if err != nil {
		return nil, err
	}
	return ut.Merge(), nil

}

func configdMatchesIfmgrd(client *ifmgrd.Client, st schema.Node, cfgTree *data.Node, intf string) (bool, error) {
	// get configd version of configuration for interface
	c := findCommitRoot(intf, cfgTree)

	rn, err := getInterfaceRunning(client, st, intf)
	if err != nil {
		//not present in ifmgrd
		return true, nil
	}

	// Compare configd and ifmgrd view of interface config
	differ := diff.NewNode(c, rn, st, nil)
	if differ == nil {
		// not present in ifmgr or configd
		// both agree
		return true, nil
	}

	if differ.Added() || differ.Deleted() || differ.Updated() {
		if waitInput.verbose {
			fmt.Printf("\nInterface %s pending changes:\n%s\n", intf, diff.NewNode(c, rn, st, nil).Serialize(true))
		}
		return false, err
	}
	return true, nil
}

func waitForMatch(wi *WaitInput) error {

	st, err := schemaGet()
	if err != nil {
		return err
	}

	configdtree, err := configdTreeGet(st)
	if err != nil {
		return err
	}

	vciClient, err := vci.Dial()
	if err != nil {
		return err
	}

	// Listen for configuration-updated notification
	// Recheck all config when received, so can ignore
	// interface name.
	update := make(chan bool, 1)
	sub := vciClient.Subscribe("vyatta-ifmgr-v1", "configuration-updated",
		func(data string) {
			update <- true
		}).Coalesce()
	sub.Run()
	defer sub.Cancel()

	client, err := ifmgrd.Dial("unix", "/run/ifmgrd/main.sock")
	if err != nil {
		return err
	}

	timeout := make(chan error, 1)
	go func() {
		time.Sleep(time.Duration(wi.timeout) * time.Second)
		timeout <- fmt.Errorf("Timeout expired")
	}()

	for {
		sets := false
		b, err := isSet(client, st, wi)
		if err != nil {
			return err
		}
		if b == true {
			sets = true
		}

		for _, iface := range wi.intf {
			r, _ := configdMatchesIfmgrd(client, st, configdtree, iface)
			if r != true {
				sets = false
			}
		}
		if sets == true {
			if wi.verbose {
				fmt.Printf("\nNo changes pending\n")
			}
			return nil
		}
		select {
		case <-update:
			if wi.verbose {
				fmt.Printf("\nReceived configuration_update notification:\n")
			}
		case err = <-timeout:
			return err
		}
	}
}

func getArgs(args []string) *WaitInput {
	var nxtset, nxtdel, nxttm, verbose bool
	timeout := uint32(15)

	set := make([]string, 0)
	delete := make([]string, 0)
	intf := make([]string, 0)
	for _, b := range args {
		switch {
		case nxtset == true:
			nxtset = false
			s := strings.Join(strings.Fields(b), " ")
			set = append(set, s)
		case nxtdel == true:
			nxtdel = false
			s := strings.Join(strings.Fields(b), " ")
			delete = append(delete, s)
		case nxttm == true:
			nxttm = false
			t, _ := strconv.Atoi(b)
			timeout = uint32(t)
		default:
			switch b {
			case "set":
				nxtset = true
			case "delete":
				nxtdel = true
			case "timeout":
				nxttm = true
			case "verbose":
				verbose = true
			default:
				intf = append(intf, b)
			}
		}
	}

	return &WaitInput{set: set, delete: delete, intf: intf, timeout: timeout, verbose: verbose}
}

func main() {
	flag.Parse()
	args := flag.Args()

	waitInput = getArgs(args)
	if err := waitForMatch(waitInput); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}
