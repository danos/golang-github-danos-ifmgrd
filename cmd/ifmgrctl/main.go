// Copyright (c) 2019, AT&T Intellectual Property. All rights reserved.
//
// Copyright (c) 2015-2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	configd_client "github.com/danos/configd/client"
	"github.com/danos/configd/rpc"
	"github.com/danos/ifmgrd"
)

type action struct {
	name        string
	description string
	run         func(*ifmgrd.Client, ...string) error
	nargs       int
}

var actions = map[string]*action{
	"apply": &action{
		"apply",
		"apply latest config to managed interfaces",
		apply,
		0,
	},
	"register": &action{
		"register",
		"register a new device to be managed",
		register,
		1,
	},
	"unregister": &action{
		"unregister",
		"stop managing a device",
		unregister,
		1,
	},
	"plug": &action{
		"plug",
		"send plug event for device",
		plug,
		0,
	},
	"unplug": &action{
		"unplug",
		"send unplug event for device",
		unplug,
		0,
	},
}

func apply(client *ifmgrd.Client, args ...string) error {
	configdClient, err := configd_client.Dial(
		"unix",
		"/run/vyatta/configd/main.sock",
		os.Getenv("VYATTA_CONFIG_SID"))
	defer configdClient.Close()
	if err != nil {
		return err
	}
	cfg, err := configdClient.TreeGet(rpc.CANDIDATE, "", "json")
	if err != nil {
		return err
	}
	err = client.Apply(cfg)
	return err
}

func register(client *ifmgrd.Client, args ...string) error {
	return client.Register(args[0])
}

func unregister(client *ifmgrd.Client, args ...string) error {
	return client.Unregister(args[0])
}

func getIntfName(args ...string) (string, error) {
	var ifname string
	if len(args) == 0 {
		ifname = os.Getenv("INTERFACE")
	} else {
		ifname = args[0]
	}
	if ifname == "" {
		return "", fmt.Errorf("must supply interface name")
	}
	return ifname, nil
}

func plug(client *ifmgrd.Client, args ...string) error {
	ifname, err := getIntfName(args...)
	if err != nil {
		return err
	}
	return client.Plug(ifname)
}

func unplug(client *ifmgrd.Client, args ...string) error {
	ifname, err := getIntfName(args...)
	if err != nil {
		return err
	}
	return client.Unplug(ifname)
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <action> <args>\n", os.Args[0])
	w := tabwriter.NewWriter(os.Stderr, 0, 8, 2, '\t', 0)
	fmt.Fprintln(w, "Available actions:")
	actionnames := make([]string, 0, len(actions))
	for name, _ := range actions {
		actionnames = append(actionnames, name)
	}
	sort.Sort(sort.StringSlice(actionnames))
	for _, name := range actionnames {
		fmt.Fprintf(w, "  %s\t%s\n", name, actions[name].description)
	}
	w.Flush()
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Must supply action")
		usage()
	}

	actionname, args := args[0], args[1:]
	action, exists := actions[actionname]
	if !exists {
		fmt.Fprintln(os.Stderr, "Invalid action:", actionname)
		usage()
	}

	if len(args) < action.nargs {
		fmt.Fprintln(os.Stderr,
			"Invalid number of arguments to",
			actionname,
			"needs",
			action.nargs)
		os.Exit(1)
	}

	client, err := ifmgrd.Dial("unix", "/run/ifmgrd/main.sock")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	err = action.run(client, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
