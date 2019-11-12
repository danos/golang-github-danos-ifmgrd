// Copyright (c) 2017-2019, AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015, 2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"fmt"
	"os"
	"time"

	"github.com/danos/vci"
	"github.com/danos/config/commit"
	"github.com/danos/config/data"
	"github.com/danos/config/diff"
)

type ConfigurationUpdated struct {
	Interface struct {
		Name string `rfc7951:"name"`
	} `rfc7951:"vyatta-ifmgr-v1:interface"`
}

func (mach *IntfMachine) notifyConfigUpdated() {
	var cu ConfigurationUpdated
	cu.Interface.Name = mach.ifname
	vci.EmitNotification("vyatta-ifmgr-v1", "configuration-updated", &cu)
}

type InterfaceState struct {
	Interface struct {
		Name  string `rfc7951:"name"`
		State string `rfc7951:"state"`
	} `rfc7951:"vyatta-ifmgr-v1:interface"`
}

func (mach *IntfMachine) notifyInterfaceState(state string) {
	var s InterfaceState
	s.Interface.Name = mach.ifname
	s.Interface.State = state
	vci.EmitNotification("vyatta-ifmgr-v1", "interface-state", &s)
}

type State uint32

const (
	unplugged State = iota
	plugged
	applying
	unapplying
	shuttingdown
	shutdown
)

func (s State) String() string {
	switch s {
	case unplugged:
		return "Unplugged"
	case plugged:
		return "Plugged"
	case applying:
		return "Applying"
	case unapplying:
		return "Unapplying"
	case shuttingdown:
		return "Shuttingdown"
	case shutdown:
		return "Shutdown"
	}
	return "Unknown"
}

type messageType uint32

const (
	apply messageType = iota
	unapply
	reset
	plug
	unplug
	isShutdown
	kill
	done
)

func (t messageType) String() string {
	switch t {
	case apply:
		return "Apply"
	case unapply:
		return "Unapply"
	case reset:
		return "Reset"
	case plug:
		return "Plug"
	case unplug:
		return "Unplug"
	case isShutdown:
		return "Shutdown"
	case kill:
		return "Kill"
	case done:
		return "Done"
	}
	return "Unknown"
}

type message struct {
	typ  messageType
	data interface{}
}

type TransFn func(*IntfMachine, interface{}) State

// find 'interfaces <type> <name>' and create a dummy path
// to only that node.
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

func (mach *IntfMachine) newSession() string {
	schema := SchemaTree.Load()
	sid := "INTF_" + mach.ifname + "_" + time.Now().String()
	candidate := mach.candidate.Load()
	running := mach.running.Load()
	/*
	 * The session needs the whole tree for reference, but
	 * we only apply the interface nodes.
	 */

	intfCandidate := findCommitRoot(mach.ifname, candidate)
	intfRunning := findCommitRoot(mach.ifname, running)
	sessionmgr.New(sid, intfCandidate, intfRunning, schema)
	return sid
}

func applyIntf(name string, candidate, running *data.Node) bool {
	schema := SchemaTree.Load()
	sid := "INTF_" + name + "_" + time.Now().String()
	/*
	 * The session needs the whole tree for reference, but
	 * we only apply the interface nodes.
	 */
	sessionmgr.New(sid, candidate, running, schema)
	defer sessionmgr.Delete(sid)

	intfCandidate := findCommitRoot(name, candidate)
	intfRunning := findCommitRoot(name, running)

	fmt.Println(name, "config differences:",
		diff.NewNode(intfCandidate, intfRunning, schema, nil).Serialize(true))
	if intfCandidate == intfRunning {
		return false
	}

	committer := NewCommitter(intfCandidate, intfRunning, schema, sid)
	if !commit.Changed(committer) {
		return false
	}
	outs, errs := commitWorkers.Commit(committer)
	for _, out := range outs {
		fmt.Println(out)
	}
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err)
	}
	return true
}

type IntfMachine struct {
	ifname          string
	curState        State
	messages        chan *message
	done            chan struct{}
	transitionTable map[State]map[messageType]TransFn
	candidate       *data.AtomicNode
	running         *data.AtomicNode
	plugged         bool
	killReq         bool
}

func (mach *IntfMachine) applyUnplugged(cfg interface{}) State {
	fmt.Println("Staging new configuration for interface", mach.ifname)
	//swap candidate
	config := cfg.(*data.Node)
	mach.candidate.Store(config)
	return unplugged
}

func (mach *IntfMachine) resetUnplugged(cfg interface{}) State {
	fmt.Println("Removing configuration for interface", mach.ifname)
	config := cfg.(*data.Node)
	mach.candidate.Store(config)
	return unplugged
}

func (mach *IntfMachine) apply(cfg interface{}) State {
	fmt.Println("Applying new configuration for interface", mach.ifname)
	config := cfg.(*data.Node)
	return mach.applyconfig(config)
}

func (mach *IntfMachine) unapply(cfg interface{}) State {
	fmt.Println("Unapplying configuration for interface", mach.ifname)
	config := cfg.(*data.Node)
	return mach.applyconfig(config)
}

func (mach *IntfMachine) applyconfig(candidate *data.Node) State {
	//swap candidate
	mach.candidate.Store(candidate)

	candidate = mach.candidate.Load()
	running := mach.running.Load()

	//start commit actions
	go func() {
		changes := applyIntf(mach.ifname, candidate, running)
		mach.running.Store(candidate)
		if changes {
			mach.notifyConfigUpdated()
		}

		mach.send(&message{typ: done, data: nil})
	}()
	return applying
}

func (mach *IntfMachine) unapplyconfig(newState State) State {
	//start commit actions
	go func() {
		// clear up any running configuration
		changes := applyIntf(mach.ifname, nil, mach.running.Load())
		mach.running.Store(nil)
		if changes {
			mach.notifyConfigUpdated()
		}

		mach.send(&message{typ: done, data: nil})
	}()
	return newState
}

func (mach *IntfMachine) reset(cfg interface{}) State {
	fmt.Println("Removing configuration for interface", mach.ifname)
	config := cfg.(*data.Node)
	return mach.applyconfig(config)
}

func (mach *IntfMachine) plug(_ interface{}) State {
	fmt.Println("Interface", mach.ifname, "became active")
	mach.notifyInterfaceState("plugged")
	mach.plugged = true
	return mach.applyconfig(mach.candidate.Load())
}

func (mach *IntfMachine) plugUnapplying(_ interface{}) State {
	fmt.Println("Interface", mach.ifname, "became active")
	mach.notifyInterfaceState("plugged")
	mach.plugged = true
	return unapplying
}

func (mach *IntfMachine) unplug(_ interface{}) State {
	fmt.Println("Interface", mach.ifname, "became inactive")
	mach.notifyInterfaceState("unplugged")
	mach.plugged = false
	// Cleanup the existing config
	return mach.unapplyconfig(unapplying)
}

func (mach *IntfMachine) unplugApplying(_ interface{}) State {
	// Note that interface is unplugged, so that cleanup
	// can happen once apply is complete
	fmt.Println("Interface", mach.ifname, "became inactive during apply")
	mach.notifyInterfaceState("unplugged")
	mach.plugged = false
	return applying
}

func (mach *IntfMachine) unplugUnapplying(_ interface{}) State {
	// Unplug seen while cleaning up a previous unplug.
	// Interface like flip-flopping
	fmt.Println("Interface", mach.ifname, "became inactive during unapply")
	mach.notifyInterfaceState("unplugged")
	mach.plugged = false
	return unapplying
}

func (mach *IntfMachine) resetApplying(cfg interface{}) State {
	fmt.Println("Removing configuration for interface", mach.ifname,
		"during previous application")
	config := cfg.(*data.Node)
	mach.candidate.Store(config)
	return applying
}

func (mach *IntfMachine) resetUnapplying(cfg interface{}) State {
	fmt.Println("Removing configuration for interface", mach.ifname,
		"during previous application")
	config := cfg.(*data.Node)
	mach.candidate.Store(config)
	return unapplying
}

func (mach *IntfMachine) swapApplying(cfg interface{}) State {
	//coalesce the changes that occur while we are running scripts.
	fmt.Println("Staging new configuration for interface", mach.ifname,
		"during previous application")
	config := cfg.(*data.Node)
	//swap candidate
	mach.candidate.Store(config)
	return applying
}

func (mach *IntfMachine) swapUnapplying(cfg interface{}) State {
	// Simply update the candidate
	fmt.Println("Staging new configuration for interface", mach.ifname,
		"during unapply")
	config := cfg.(*data.Node)
	//swap candidate
	mach.candidate.Store(config)
	return unapplying
}

func (mach *IntfMachine) doneApplying(_ interface{}) State {
	if mach.killReq {
		return mach.unapplyconfig(shuttingdown)
	}
	if !mach.plugged {
		// interface has been unplugged
		return mach.unapplyconfig(unapplying)
	}
	candidate := mach.candidate.Load()
	running := mach.running.Load()
	if running != candidate {
		fmt.Println("Configuration for interface", mach.ifname,
			"changed while previous application was working;",
			"applying new changeset.")
		//loop so we apply any coalesced updates we may have missed while
		//running previous transaction
		return mach.applyconfig(candidate)
	}
	fmt.Println("Configuration for interface", mach.ifname, "completed")
	return plugged
}

func (mach *IntfMachine) doneUnapplying(_ interface{}) State {
	fmt.Println("Unapply for interface", mach.ifname, "completed")
	if mach.killReq {
		return mach.unapplyconfig(shuttingdown)
	}
	if !mach.plugged {
		return unplugged
	}
	return mach.applyconfig(mach.candidate.Load())
}

func (mach *IntfMachine) kill(_ interface{}) State {
	fmt.Println("Stopping interface manager for", mach.ifname)
	return shutdown
}

func (mach *IntfMachine) killPlugged(_ interface{}) State {
	fmt.Println("Stopping interface manager for", mach.ifname)
	return mach.unapplyconfig(shuttingdown)
}

func (mach *IntfMachine) killApplying(_ interface{}) State {
	fmt.Println("Stopping interface manager for", mach.ifname)
	mach.killReq = true
	return applying
}

func (mach *IntfMachine) killUnapplying(_ interface{}) State {
	fmt.Println("Stopping interface manager for", mach.ifname)
	mach.killReq = true
	return unapplying
}

func (mach *IntfMachine) send(msg *message) bool {
	select {
	case mach.messages <- msg:
		return true
	case <-mach.done:
		return false
	}
}

func (mach *IntfMachine) Apply(cfg *data.Node) {
	mach.send(&message{typ: apply, data: cfg})
}

func (mach *IntfMachine) Reset(cfg *data.Node) {
	mach.send(&message{typ: reset, data: cfg})
}

func (mach *IntfMachine) Plug() {
	mach.send(&message{typ: plug, data: nil})
}

func (mach *IntfMachine) Unplug() {
	mach.send(&message{typ: unplug, data: nil})
}

func (mach *IntfMachine) Kill() {
	mach.send(&message{typ: kill, data: nil})
}

func (mach *IntfMachine) IsShutdown() bool {
	return !mach.send(&message{typ: isShutdown, data: nil})
}

func NewIntfMachine(ifname string) *IntfMachine {
	mach := &IntfMachine{
		ifname:    ifname,
		curState:  unplugged,
		messages:  make(chan *message),
		done:      make(chan struct{}),
		candidate: data.NewAtomicNode(nil),
		running:   data.NewAtomicNode(nil),
		transitionTable: map[State]map[messageType]TransFn{
			unplugged: {
				apply: (*IntfMachine).applyUnplugged,
				reset: (*IntfMachine).resetUnplugged,
				plug:  (*IntfMachine).plug,
				kill:  (*IntfMachine).kill,
			},
			plugged: {
				apply:  (*IntfMachine).apply,
				reset:  (*IntfMachine).reset,
				unplug: (*IntfMachine).unplug,
				kill:   (*IntfMachine).killPlugged,
			},
			applying: {
				apply:  (*IntfMachine).swapApplying,
				reset:  (*IntfMachine).resetApplying,
				unplug: (*IntfMachine).unplugApplying,
				done:   (*IntfMachine).doneApplying,
				kill:   (*IntfMachine).killApplying,
			},
			unapplying: {
				apply:  (*IntfMachine).swapUnapplying,
				reset:  (*IntfMachine).resetUnapplying,
				plug:   (*IntfMachine).plugUnapplying,
				unplug: (*IntfMachine).unplugUnapplying,
				done:   (*IntfMachine).doneUnapplying,
				kill:   (*IntfMachine).killUnapplying,
			},
			shuttingdown: {
				done: (*IntfMachine).kill,
			},
		},
	}
	go mach.run()
	return mach
}

func (mach *IntfMachine) run() {
	state := mach.curState
	for {
		msg := <-mach.messages
		trans := mach.transitionTable[state][msg.typ]
		if trans == nil {
			fmt.Println("No transition for", msg.typ, "in state", state)
			continue
		}
		state = trans(mach, msg.data)
		mach.curState = state
		if state == shutdown {
			break
		}
	}
	close(mach.done)
}
