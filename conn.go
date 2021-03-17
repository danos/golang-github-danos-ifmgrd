// Copyright (c) 2019,2021 AT&T Intellectual Property.
// All rights reserved.
//
// Copyright (c) 2015-2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"sync"
	"syscall"

	client "github.com/danos/configd/client"
	"github.com/danos/utils/audit"
	"github.com/danos/utils/os/group"
)

type LoginPidError struct {
	pid int32
}

func (e *LoginPidError) Error() string {
	return fmt.Sprintf("Login User Id is not set for PID %d", e.pid)
}

func newLoginPidError(pid int32) error {
	return &LoginPidError{pid: pid}
}

func IsLoginPidError(err error) bool {
	_, ok := err.(*LoginPidError)

	return ok
}

func newResponse(result interface{}, err error, id int) *Response {
	var resp Response
	if err != nil {
		resp = Response{Error: err.Error(), Id: id}
	} else {
		resp = Response{Result: result, Id: id}
	}
	return &resp
}

// Get User ID for connecting process
func getLoginUid(pid int32) (uint32, error) {

	u, e := audit.GetPidLoginuid(pid)
	if e != nil {
		fmt.Printf("Error getting Login User Id: %s\n", e.Error())
		return 0, e
	}

	// The special value of -1 is used when login ID is not set. This
	// is the case for daemons and boot processes. Since we're using
	// unsigned numbers we take the bitwise complement of 0.
	if u == ^uint32(0) {
		return 0, newLoginPidError(pid)
	}

	return u, nil
}

type SrvConn struct {
	*net.UnixConn
	cred    *syscall.Ucred
	srv     *Srv
	enc     *json.Encoder
	dec     *json.Decoder
	sending *sync.Mutex
}

//Send an rpc response with appropriate data or an error
func (conn *SrvConn) sendResponse(resp *Response) error {
	conn.sending.Lock()
	err := conn.enc.Encode(&resp)
	conn.sending.Unlock()
	return err

}

//Receive an rpc request and do some preprocessing.
func (conn *SrvConn) readRequest() (*Request, error) {
	var req = new(Request)
	err := conn.dec.Decode(req)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (conn *SrvConn) getCreds() (*syscall.Ucred, error) {
	uf, err := conn.File()
	if err != nil {
		return nil, err
	}
	cred, err := syscall.GetsockoptUcred(
		int(uf.Fd()),
		syscall.SOL_SOCKET,
		syscall.SO_PEERCRED)
	if err != nil {
		conn.srv.LogError(err)
		return nil, err
	}
	uf.Close()

	cred.Uid, _ = getLoginUid(cred.Pid)

	return cred, nil
}

// Handle is the main loop for a connection.
// It receives the requests, calls the request method
//and returns the response to the client.
func (conn *SrvConn) Handle() {

	var secrets bool

	cred, err := conn.getCreds()
	if err != nil {
		if !IsLoginPidError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
	} else {
		groups, err := group.LookupUid(strconv.Itoa(int(cred.Uid)))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			for _, gr := range groups {
				if gr.Name == "secrets" {
					secrets = true
				}
			}
		}
	}

	client, err := client.Dial("unix", conn.srv.Config.ConfigdSocket, "RUNNING")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	defer client.Close()
	disp := &Disp{
		client:  client,
		secrets: secrets,
	}

	for {
		req, err := conn.readRequest()
		if err != nil {
			if err != io.EOF {
				conn.srv.LogError(err)
			}
			break
		}

		result, err := conn.Call(disp, req.Method, req.Args)
		err = conn.sendResponse(newResponse(result, err, req.Id))
		if err != nil {
			break
		}
	}
	conn.Close()
	return
}

func (conn *SrvConn) Call(
	disp *Disp,
	method string,
	args []interface{},
) (interface{}, error) {
	m, ok := conn.srv.m[method]
	if !ok {
		return nil, &MethErr{Name: method}
	}

	typ := m.Func.Type()

	//Number of args are equal?
	if len(args) != typ.NumIn()-1 {
		return nil, &ArgNErr{
			Method: method,
			Len:    len(args),
			Elen:   typ.NumIn() - 1,
		}
	}

	//validate arguments
	//prepending the first argument *Disp
	vals := make([]reflect.Value, len(args)+1)
	vals[0] = reflect.ValueOf(disp)
	for i, v := range args {
		t1 := reflect.TypeOf(v)
		t2 := typ.In(i + 1)
		if t1 != t2 {
			if !t1.ConvertibleTo(t2) {
				return nil, &ArgErr{
					Method: method,
					Farg:   v,
					Typ:    t1.Name(),
					Etyp:   t2.Name(),
				}
			}
			vals[i+1] = reflect.ValueOf(v).Convert(t2)
		} else {
			vals[i+1] = reflect.ValueOf(v)
		}
	}

	//call the function
	rets := m.Func.Call(vals)
	err, ok := rets[1].Interface().(error)
	if ok {
		return rets[0].Interface(), err
	} else {
		return rets[0].Interface(), nil
	}
}
