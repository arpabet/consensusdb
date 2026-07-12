/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package replication

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"go.uber.org/zap"
)

/*
The AddressReconciler heals the gap raft bootstrap leaves on Kubernetes: the
seed records itself under its resolved private IP, while its stable identity is
a DNS name. A leader whose recorded address differs from its advertise address
re-registers itself in place; a matching record reports healed; a node not in
the configuration (a joiner before enrollment) is left alone.
*/
func TestAddressReconcilerSelfHeal(t *testing.T) {
	seed := newTestNode(t, 0)
	if err := seed.raft.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{Suffrage: raft.Voter, ID: seed.id, Address: seed.addr}},
	}).Error(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	waitLeader(t, seed.raft)
	ctx := context.Background()

	advertise := raft.ServerAddress("node-0.stable.dns.internal:8300")
	rec := &AddressReconciler{Log: zap.NewNop(), selfID: seed.id, advertise: advertise}

	// Bootstrap recorded the transport address, so the first pass detects drift
	// and issues the in-place AddVoter (this node is the leader).
	healed, err := rec.reconcileOnce(ctx, seed.raft)
	if err != nil {
		t.Fatalf("reconcile (drift): %v", err)
	}
	if healed {
		t.Fatal("first pass reported healed before the fix was verified")
	}

	// The committed configuration now records the advertise address, same id.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, found := recordedAddress(seed.raft.GetConfiguration().Configuration(), seed.id)
		if found && got == advertise {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recorded address = %q, want %q", got, advertise)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(seed.raft.GetConfiguration().Configuration().Servers); n != 1 {
		t.Fatalf("server count = %d, want 1 (address updated in place, not duplicated)", n)
	}

	// The verification pass reports healed and the loop would stop.
	healed, err = rec.reconcileOnce(ctx, seed.raft)
	if err != nil {
		t.Fatalf("reconcile (verify): %v", err)
	}
	if !healed {
		t.Fatal("verify pass did not report healed")
	}

	// A node that is not (yet) a member is left alone: no error, not healed.
	stranger := &AddressReconciler{Log: zap.NewNop(), selfID: "not-a-member", advertise: advertise}
	healed, err = stranger.reconcileOnce(ctx, seed.raft)
	if err != nil || healed {
		t.Fatalf("non-member reconcile = (healed=%v, err=%v), want (false, nil)", healed, err)
	}
}
