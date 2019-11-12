// Copyright (c) 2018-2019, AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"net"
	"sync"

	"github.com/danos/config/data"
)

/*
 * Given the structure of the data model right now we can only
 * register for the top level interface names. This is good enough for
 * the current use case.
 */
func listConfigInterfaces(config *data.Node) []string {
	out := make([]string, 0)
	intfTree := config.Child("interfaces")
	for _, ifType := range intfTree.Children() {
		for _, intf := range ifType.ChildNames() {
			out = append(out, intf)
		}
	}
	return out
}

type IntfManager struct {
	sync.Mutex
	config     *data.Node
	interfaces map[string]*IntfMachine
}

func NewIntfManager() *IntfManager {
	return &IntfManager{
		interfaces: make(map[string]*IntfMachine),
	}
}

func (mgr *IntfManager) Register(intfName string) {
	mgr.Lock()
	defer mgr.Unlock()

	_, registered := mgr.interfaces[intfName]
	if registered {
		return
	}
	intf := NewIntfMachine(intfName)
	mgr.interfaces[intfName] = intf

	intf.Apply(mgr.config)
	_, err := net.InterfaceByName(intfName)
	if err == nil {
		intf.Plug()
	}
}

func (mgr *IntfManager) Unregister(intfName string) {
	mgr.Lock()
	defer mgr.Unlock()

	intf, managed := mgr.interfaces[intfName]
	if !managed {
		return
	}
	delete(mgr.interfaces, intfName)
	intf.Kill()
}

func (mgr *IntfManager) Apply(config *data.Node) {
	mgr.Lock()
	defer mgr.Unlock()
	mgr.config = config
	//update managed interfaces
	configInterfaces := make(map[string]struct{})
	for _, name := range listConfigInterfaces(config) {
		intf, managed := mgr.interfaces[name]
		if !managed {
			continue
		}
		intf.Apply(config)
		configInterfaces[name] = struct{}{}
	}

	//reset any interface that isn't in the config
	for name, intf := range mgr.interfaces {
		if _, inConfig := configInterfaces[name]; inConfig {
			continue
		}
		intf.Reset(config)
	}
}

func (mgr *IntfManager) newSession(intfName string) string {
	mgr.Lock()
	defer mgr.Unlock()
	intf, managed := mgr.interfaces[intfName]
	if !managed {
		return ""
	}
	return intf.newSession()
}

func (mgr *IntfManager) Plug(intfName string) {
	mgr.Lock()
	defer mgr.Unlock()
	intf, managed := mgr.interfaces[intfName]
	if !managed {
		return
	}
	intf.Plug()
}

func (mgr *IntfManager) Unplug(intfName string) {
	mgr.Lock()
	defer mgr.Unlock()
	intf, managed := mgr.interfaces[intfName]
	if !managed {
		return
	}
	intf.Unplug()
}
