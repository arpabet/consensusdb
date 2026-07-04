/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package console is the HTTP/JSON admin surface behind the web console: a small
REST API on the http-server that the browser can call (the value-rpc data plane
has no JS client). It runs long operations — notably backup ledger verification —
as background jobs the UI polls for progress.
*/
package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"go.arpabet.com/consensusdb/pkg/verify"
)

// JobState is the lifecycle of a background job.
type JobState string

const (
	JobRunning JobState = "running"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

// Job is the pollable status of a background verification.
type Job struct {
	ID        string         `json:"id"`
	State     JobState       `json:"state"`
	Progress  int            `json:"progress"` // 0..100
	Result    *verify.Result `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	StartedAt time.Time      `json:"startedAt"`
	EndedAt   *time.Time     `json:"endedAt,omitempty"`
}

// JobManager runs and tracks background verification jobs.
type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewJobManager() *JobManager { return &JobManager{jobs: map[string]*Job{}} }

func newJobID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// StartVerifyBackup kicks off a backup verification and returns its job id
// immediately; the caller polls Get for progress and the result.
func (m *JobManager) StartVerifyBackup(opts verify.Options) string {
	job := &Job{ID: newJobID(), State: JobRunning, StartedAt: time.Now()}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	go func() {
		res, err := verify.VerifyBackup(context.Background(), opts, func(pct int) {
			m.mu.Lock()
			if j := m.jobs[job.ID]; j != nil {
				j.Progress = pct
			}
			m.mu.Unlock()
		})
		end := time.Now()
		m.mu.Lock()
		defer m.mu.Unlock()
		j := m.jobs[job.ID]
		j.EndedAt = &end
		if err != nil {
			j.State = JobFailed
			j.Error = err.Error()
			return
		}
		j.Progress = 100
		j.State = JobDone
		j.Result = res
	}()
	return job.ID
}

// Get returns a snapshot of a job by id.
func (m *JobManager) Get(id string) (Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true // copy
}

// List returns snapshots of all jobs (most-recent jobs are small in number).
func (m *JobManager) List() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	return out
}
