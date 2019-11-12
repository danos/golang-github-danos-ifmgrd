// Copyright (c) 2018-2019, AT&T Intellectual Property.
// All rights reserved.
//
// SPDX-License-Identifier: GPL-2.0-only

package ifmgrd

import (
	"runtime"

	"github.com/danos/config/commit"
	"github.com/danos/mgmterror"
	"github.com/danos/utils/exec"
)

var commitWorkers = newCommitPool()

func init() {
	exec.NewExecError = func(path []string, err string) error {
		return mgmterror.NewExecError(path, err)
	}
}

type commitRequest struct {
	committer *Committer
	resp      chan commitResponse
}

type commitResponse struct {
	outs []*exec.Output
	errs []error
}

type commitWorker struct {
	requests chan commitRequest
}

func (w *commitWorker) work() {
	for {
		req := <-w.requests
		outs, errs, _, _ := commit.Commit(req.committer)
		req.resp <- commitResponse{outs: outs, errs: errs}
	}
}

type commitPool struct {
	work chan commitRequest
}

// A commit pool starts up NumCPU workers to handle commit requests.
// NumCPU is used as an arbitrary heuristic as to how many parallel
// requests the system can handle at once.
//
// Commits are distributed to these workers for processing.
func newCommitPool() *commitPool {
	var nWorker = runtime.NumCPU()
	b := &commitPool{
		work: make(chan commitRequest, 100),
	}

	for i := 0; i < nWorker; i++ {
		w := &commitWorker{
			requests: b.work,
		}
		go w.work()
	}
	return b
}

func (b *commitPool) Commit(committer *Committer) (outs []*exec.Output, errs []error) {
	respCh := make(chan commitResponse, 1)
	b.work <- commitRequest{
		committer: committer,
		resp:      respCh,
	}
	resp := <-respCh
	return resp.outs, resp.errs
}
