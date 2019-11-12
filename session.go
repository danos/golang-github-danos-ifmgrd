// Copyright (c) 2019, AT&T Intellectual Property. All rights reserved.
//
// Copyright (c) 2015-2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only
package ifmgrd

import (
	"sync"

	"github.com/danos/config/data"
	"github.com/danos/config/schema"
	"github.com/danos/mgmterror"
)

type Session struct {
	candidate *data.Node
	running   *data.Node
	schema    schema.Node
}

type Sessions struct {
	sync.RWMutex
	sessions map[string]*Session
}

func NewSessionMap() *Sessions {
	return &Sessions{
		sessions: make(map[string]*Session),
	}
}

func (s *Sessions) New(
	sid string,
	candidate, running *data.Node,
	schema schema.Node,
) (*Session, error) {
	s.Lock()
	defer s.Unlock()
	sess, ok := s.sessions[sid]
	if ok {
		err := mgmterror.NewOperationFailedApplicationError()
		err.Message = "session exists"
		return nil, err
	}
	sess = &Session{
		candidate: candidate,
		running:   running,
		schema:    schema,
	}
	s.sessions[sid] = sess
	return sess, nil
}

func (s *Sessions) Delete(sid string) {
	s.Lock()
	defer s.Unlock()
	_, exists := s.sessions[sid]
	if !exists {
		return
	}
	delete(s.sessions, sid)
}

func (s *Sessions) Get(sid string) *Session {
	s.RLock()
	defer s.RUnlock()
	return s.sessions[sid]
}
