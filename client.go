// Copyright (c) 2018-2019, AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
)

//GetFuncName() returns the unqualified name of the caller
func GetFuncName() string {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return "invalid"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "invalid"
	}
	name := fn.Name()
	i := strings.LastIndex(name, ".")
	return name[i+1:]
}

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
	id   int
}

func Dial(network, address string) (*Client, error) {
	c, e := net.Dial(network, address)
	if e != nil {
		return nil, e
	}

	client := &Client{
		conn: c,
		enc:  json.NewEncoder(c),
		dec:  json.NewDecoder(c),
		id:   0,
	}

	return client, nil
}

func (c *Client) call(method string, args ...interface{}) (interface{}, error) {
	var rep Response
	c.id++
	c.enc.Encode(&Request{Method: method, Args: args, Id: c.id})
	c.dec.Decode(&rep)
	//fmt.Printf("%#v\n", &rpc.Request{Method: method, Args: args, Id: c.id})
	//fmt.Printf("%#v\n", rep)
	if err, ok := rep.Error.(string); ok {
		return rep.Result, errors.New(err)
	}
	return rep.Result, nil
}

//Per JSON RPC spec we must return a value upon success. This is not
//idomatic for go, so if the method will only return an error just
//ignore the bool.
func (c *Client) callBoolIgnore(method string, args ...interface{}) error {
	i, err := c.call(method, args...)
	if err != nil {
		return err
	}
	if _, ok := i.(bool); ok {
		return nil
	} else {
		return fmt.Errorf("Wrong return type for %s got %T expecting bool", method, i)
	}
}

func (c *Client) callString(method string, args ...interface{}) (string, error) {
	s, err := c.call(method, args...)
	if err != nil {
		return "", err
	}
	if st, ok := s.(string); ok {
		return st, nil
	}

	return "", fmt.Errorf("Wrong return type for %s got %T expecting string", method, s)
}

func (c *Client) Running(intf string) (string, error) {
	return c.callString(GetFuncName(), intf)
}

func (c *Client) Apply(config string) error {
	return c.callBoolIgnore(GetFuncName(), config)
}

func (c *Client) Register(intfName string) error {
	return c.callBoolIgnore(GetFuncName(), intfName)
}

func (c *Client) Unregister(intfName string) error {
	return c.callBoolIgnore(GetFuncName(), intfName)
}

func (c *Client) Plug(intfName string) error {
	return c.callBoolIgnore(GetFuncName(), intfName)
}

func (c *Client) Unplug(intfName string) error {
	return c.callBoolIgnore(GetFuncName(), intfName)
}
