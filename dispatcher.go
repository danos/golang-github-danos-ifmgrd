// Copyright (c) 2018-2019, AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015-2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"github.com/danos/config/diff"
	"github.com/danos/config/schema"
	"github.com/danos/config/union"
	client "github.com/danos/configd/client"
	"github.com/danos/configd/rpc"
	"github.com/danos/mgmterror"
	"github.com/danos/utils/pathutil"
)

type Disp struct {
	client  *client.Client
	secrets bool
}

func (d *Disp) validatePath(ps []string) error {
	var sn schema.Node = SchemaTree.Load()
	for i, v := range ps {
		sn = sn.SchemaChild(v)
		if sn == nil {
			err := mgmterror.NewUnknownElementApplicationError(v)
			err.Path = pathutil.Pathstr(ps[:i])
			return err
		}
	}

	return nil
}

//ifmgrd specific
func (d *Disp) Apply(config string) (bool, error) {
	st := SchemaTree.Load()
	ut, err := union.UnmarshalJSONWithoutValidation(st, []byte(config))
	if err != nil {
		return false, err
	}
	dtree := ut.Merge()
	intfmgr.Apply(dtree)
	return true, nil
}

func (d *Disp) Register(intfName string) (bool, error) {
	intfmgr.Register(intfName)
	return true, nil
}

func (d *Disp) Unregister(intfName string) (bool, error) {
	intfmgr.Unregister(intfName)
	return true, nil
}

func (d *Disp) Plug(intfName string) (bool, error) {
	intfmgr.Plug(intfName)
	return true, nil
}

func (d *Disp) Unplug(intfName string) (bool, error) {
	intfmgr.Unplug(intfName)
	return true, nil
}

//Pretend to be configd for anything started in this session.
//For this to work we need to start in a new mount namespace.
func (d *Disp) getTree(db rpc.DB, sid string) union.Node {
	session := sessionmgr.Get(sid)
	switch db {
	case rpc.EFFECTIVE, rpc.AUTO, rpc.CANDIDATE:
		return union.NewNode(
			session.candidate, nil, SchemaTree.Load(), nil, 0)
	}
	return union.NewNode(
		session.running, nil, SchemaTree.Load(), nil, 0)
}

func (d *Disp) Get(db rpc.DB, sid string, path string) ([]string, error) {
	return d.getTree(db, sid).Get(nil, pathutil.Makepath(path))
}

// Get an interfaces running configuration
func (d *Disp) Running(intf string) (string, error) {
	sid := intfmgr.newSession(intf)
	if sid == "" {
		// interface not currently managed by ifmgr
		// pending configuration changes may change that.
		err := mgmterror.NewDataMissingError()
		err.Message = "Interface not managed by ifmgrd"
		return "", err
	}
	defer sessionmgr.Delete(sid)

	var opts map[string]interface{}

	if d.secrets {
		opts = make(map[string]interface{})
		opts["Secrets"] = true
	}

	return d.TreeGet(rpc.RUNNING, sid, "/", "json", opts)
}

func (d *Disp) Exists(db rpc.DB, sid string, path string) (bool, error) {
	ps := pathutil.Makepath(path)
	if err := d.validatePath(ps); err != nil {
		return false, err
	}

	ut := d.getTree(db, sid)
	exists := ut.Exists(nil, ps)
	return exists == nil, nil
}

func (d *Disp) NodeGetStatus(
	db rpc.DB,
	sid string,
	path string,
) (rpc.NodeStatus, error) {
	session := sessionmgr.Get(sid)
	diffTree := diff.NewNode(session.candidate,
		session.running, SchemaTree.Load(), nil)

	ps := pathutil.Makepath(path)
	diffNode := diffTree.Descendant(ps)

	if diffNode == nil {
		//TODO: I'd rather we not return an error at all for unknown nodes,
		//      IIRC the upper layer throws away the information anyway
		err := mgmterror.NewDataMissingError()
		err.Message = "Node does not exist"
		return rpc.UNCHANGED, err
	}

	//This is gross, but the old API clients expects exactly this behavior
	//ideally we could use the simple diff output as it actually reflects
	//the useful state of the node.
	_, isLeafVal := diffNode.Schema().(schema.LeafValue)
	parent := diffNode.Parent()
	var parentIsLeaf, parentIsLeafList bool
	if parent != nil {
		_, parentIsLeaf = parent.Schema().(schema.Leaf)
		_, parentIsLeafList = parent.Schema().(schema.LeafList)
	}
	switch {
	case diffNode.Deleted():
		return rpc.DELETED, nil
	case isLeafVal && parentIsLeaf:
		return rpc.CHANGED, nil
	case diffNode.Added():
		return rpc.ADDED, nil
	case diffNode.Changed():
		return rpc.CHANGED, nil
	case isLeafVal && parentIsLeafList && diffNode.Parent().Changed():
		return rpc.CHANGED, nil
	default:
		return rpc.UNCHANGED, nil
	}
}

func (d *Disp) NodeIsDefault(
	db rpc.DB,
	sid string,
	path string,
) (bool, error) {
	return d.getTree(db, sid).IsDefault(nil, pathutil.Makepath(path))
}

func (d *Disp) TreeGet(
	db rpc.DB,
	sid, path, encoding string,
	flags map[string]interface{},
) (string, error) {
	ps := pathutil.Makepath(path)
	ut, _ := d.getTree(db, sid).Descendant(nil, ps)
	if ut == nil {
		err := mgmterror.NewUnknownElementApplicationError(ps[len(ps)-1])
		err.Path = pathutil.Pathstr(ps[:len(ps)-1])
		return "", err
	}

	var options []union.UnionOption
	if f, exists := flags["Defaults"]; exists {
		defaults, _ := f.(bool)
		if defaults {
			options = append(options, union.IncludeDefaults)
		}
	}
	var secrets bool
	if f, exists := flags["Secrets"]; exists {
		secrets, _ = f.(bool)
	}
	if !secrets {
		options = append(options, union.HideSecrets)
	}
	return ut.Marshal("data", encoding, options...)
}

func (d *Disp) SessionExists(sid string) (bool, error) {
	sess := sessionmgr.Get(sid)
	return sess != nil, nil
}

//Pretend to be configd, proxy safe requests as needed
func (d *Disp) NodeGetType(sid string, path string) (rpc.NodeType, error) {
	return d.client.NodeGetType(path)
}
func (d *Disp) TmplGet(path string) (map[string]string, error) {
	return d.client.TmplGet(path)
}
func (d *Disp) TmplGetChildren(path string) ([]string, error) {
	return d.client.TmplGetChildren(path)
}
func (d *Disp) TmplValidatePath(path string) (bool, error) {
	return d.client.TmplValidatePath(path)
}
func (d *Disp) TmplValidateValues(path string) (bool, error) {
	return d.client.TmplValidateValues(path)
}

func (d *Disp) SchemaGet(module string, format string) (string, error) {
	return d.client.SchemaGet(module, format)
}
func (d *Disp) GetSchemas() (string, error) {
	return d.client.GetSchemas()
}
func (d *Disp) AuthAuthorize(path string, perm int) (bool, error) {
	return true, nil
}

func (d *Disp) ReadConfigFile(filename string) (string, error) {
	return d.client.ReadConfigFile(filename)
}

func (d *Disp) CallRpc(namespace, name, args, encoding string) (string, error) {
	return d.client.CallRpc(namespace, name, args, encoding)
}

func (d *Disp) CallRpcXml(namespace, name, args string) (string, error) {
	return d.client.CallRpcXml(namespace, name, args)
}

func (d *Disp) MigrateConfigFile(filename string) (string, error) {
	return d.client.MigrateConfigFile(filename)
}

func (d *Disp) Expand(path string) (string, error) {
	return d.client.Expand(path)
}
