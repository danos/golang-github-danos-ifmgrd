// Copyright (c) 2019, AT&T Intellectual Property. All rights reserved.
//
// Copyright (c) 2015 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"fmt"
)

//Request represents an RPC request
type Request struct {
	//Op is the method that was called via json rpc
	Method string `json:"method"`
	//Args is a list of arguments to that method
	Args []interface{} `json:"params"`
	//Id is the unique request identifier
	Id int `json:"id"`
}

//Response represents an RPC response
type Response struct {
	//Result is any value returned by the handler
	//The client library uses reflection to ensure it received the appropriate type.
	Result interface{} `json:"result"`
	//Error contains a message describing a problem
	Error interface{} `json:"error"`
	//Id is the unique request identifier
	Id int `json:"id"`
}

type MethErr struct {
	Name string
}

func (e *MethErr) Error() string {
	return fmt.Sprintf("unknown method %s", e.Name)
}

type ArgErr struct {
	Method string
	Farg   interface{}
	Typ    string
	Etyp   string
}

func (e *ArgErr) Error() string {
	if e.Typ == "" {
		return fmt.Sprintf("cannot use %v (type %T) as type %s in call to %s",
			e.Farg, e.Farg, e.Etyp, e.Method)
	}
	return fmt.Sprintf("cannot use %v (type %s) as type %s in call to %s",
		e.Farg, e.Typ, e.Etyp, e.Method)
}

type ArgNErr struct {
	Method string
	Len    int
	Elen   int
}

func (e *ArgNErr) Error() string {
	if e.Len > e.Elen {
		return fmt.Sprintf("too many arguments in call to %s expected %d got %d",
			e.Method, e.Elen, e.Len)
	}
	return fmt.Sprintf("too few arguments in call to %s expected %d got %d",
		e.Method, e.Elen, e.Len)
}
