// Copyright (c) 2019, AT&T Intellectual Property. All rights reserved.
//
// Copyright (c) 2015 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
	"unicode"
)

type Srv struct {
	*net.UnixListener
	m      map[string]reflect.Method
	Config *Config
}

func NewSrv(l *net.UnixListener, config *Config) *Srv {
	s := &Srv{
		UnixListener: l,
		m:            make(map[string]reflect.Method),
		Config:       config,
	}

	t := reflect.TypeOf(new(Disp))
	for m := 0; m < t.NumMethod(); m++ {
		meth := t.Method(m)
		ftype := meth.Func.Type()
		if unicode.IsLower(rune(meth.Name[0])) {
			//only exported methods
			continue
		}
		if ftype.NumOut() != 2 {
			//with 2 return values
			continue
		}
		if ftype.Out(1).Name() != "error" {
			//whose second return value is an error
			continue
		}

		s.m[meth.Name] = meth
	}
	return s
}

//Serve is the server main loop.
//It accepts connections and spawns a goroutine to handle that connection.
func (s *Srv) Serve() error {
	var err error
	for {
		conn, err := s.AcceptUnix()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			s.LogError(err)
			break
		}
		sconn := s.NewConn(conn)

		go sconn.Handle()
	}
	return err
}

//NewConn creates a new SrvConn and returns a reference to it.
func (s *Srv) NewConn(conn *net.UnixConn) *SrvConn {
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	c := &SrvConn{
		UnixConn: conn,
		srv:      s,
		enc:      enc,
		dec:      dec,
		sending:  new(sync.Mutex),
	}
	return c
}

//Log is a common place to do logging so that the
//implementation may change in the future.
func (d *Srv) Log(format string, v ...interface{}) {
	fmt.Printf(format, v...)
}

//LogError logs an error if the passed in value is non nil
func (d *Srv) LogError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
	}
}

func (d *Srv) LogFatal(err error) {
	if err != nil {
		panic(err)
	}
}
