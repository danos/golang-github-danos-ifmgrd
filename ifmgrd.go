// Copyright (c) 2019, AT&T Intellectual Property. All rights reserved.
//
// Copyright (c) 2015-2016 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"sync/atomic"

	"github.com/danos/config/schema"
)

type atomicSchemaNode struct {
	atomic.Value
}

// atomic.Values need to be consistently store a concrete type.
// This wrapper allows us to store anything that meets the
// interface we care about.
type nodeWrapper struct {
	node schema.Node
}

func newAtomicSchemaNode() *atomicSchemaNode {
	a := &atomicSchemaNode{}
	tree, _ := schema.NewTree(nil)
	a.Store(tree)
	return a
}

func (t *atomicSchemaNode) Store(n schema.Node) {
	t.Value.Store(&nodeWrapper{n})
}

func (t *atomicSchemaNode) Load() schema.Node {
	v := t.Value.Load().(*nodeWrapper)
	return v.node
}

var intfmgr *IntfManager
var sessionmgr *Sessions
var SchemaTree *atomicSchemaNode

func init() {
	sessionmgr = NewSessionMap()
	intfmgr = NewIntfManager()
	SchemaTree = newAtomicSchemaNode()
}

type Config struct {
	Yangdir       string
	Socket        string
	Capabilities  string
	ConfigdSocket string
}
