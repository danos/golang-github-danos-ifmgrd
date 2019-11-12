// Copyright (c) 2017-2019, AT&T Intellectual Property.
// All rights reserved.
// Copyright (c) 2015,2017 by Brocade Communications Systems, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"fmt"
	"os"
	"time"

	"github.com/danos/config/commit"
	"github.com/danos/config/data"
	"github.com/danos/config/schema"
)

type Committer struct {
	candidate *data.Node
	running   *data.Node
	schema    schema.Node
	sid       string
	debug     bool
}

func NewCommitter(
	candidate, running *data.Node,
	schema schema.Node,
	sid string,
) *Committer {
	return &Committer{
		candidate: candidate,
		running:   running,
		schema:    schema,
		sid:       sid,
	}
}

//commit.Context
func (c *Committer) Log(msgs ...interface{}) {
	if c.Debug() {
		fmt.Println(msgs...)
	}
}
func (c *Committer) LogCommitMsg(string)             {}
func (c *Committer) LogCommitTime(string, time.Time) {}
func (c *Committer) LogError(msgs ...interface{}) {
	fmt.Fprintln(os.Stderr, msgs...)
}
func (c *Committer) LogAudit(_ string) {
	return
}
func (c *Committer) Debug() bool {
	return c.debug
}
func (c *Committer) Sid() string {
	return c.sid
}
func (c *Committer) Uid() uint32 {
	return 0
}
func (c *Committer) Running() *data.Node {
	return c.running
}
func (c *Committer) Candidate() *data.Node {
	return c.candidate
}
func (c *Committer) Schema() schema.Node {
	return c.schema
}
func (c *Committer) RunDeferred() bool {
	return true
}
func (c *Committer) Effective() commit.EffectiveDatabase {
	return c
}

//commit.EffectiveDatabase
func (c *Committer) Set(_ []string) error {
	return nil
}
func (c *Committer) Delete(_ []string) error {
	return nil
}
