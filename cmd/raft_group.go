/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import "go.arpabet.com/cligo"

/*
RaftGroup roots the raftvrpc cluster-management commands (`raft
config|join|bootstrap`) under the application's root cli group. The published
raftvrpc.RaftGroup declares no parent, which cligo rejects at registration; the
commands themselves attach to whichever registered group is named "raft", so this
local group stands in for it.
*/
type RaftGroup struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (RaftGroup) Group() string { return "raft" }

func (RaftGroup) Help() (string, string) {
	return "raft cluster management over value-rpc", ""
}
