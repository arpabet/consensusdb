/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package ledger makes the consensus verifiable (plan S6). Every committed raft
entry is folded into a hash chain in the FSM apply path — a purely deterministic
computation, so every replica derives the identical chain; divergence means
corruption or tampering, and it is detectable rather than silent. Periodically the
chain head is snapshotted into a Checkpoint that a quorum of nodes co-signs; the
aggregate of those signatures is a compact, third-party-checkable proof that a
majority agreed on exactly this history.

Signatures use BLS12-381 with keys in G2, so each signature lives in G1 and is
just 48 bytes — and N node signatures over one checkpoint aggregate into a single
48-byte signature (plus a signer set), the smallest practical footprint for a
multi-signer quorum certificate. Node keys are certified by a common Ed25519 CA
(see ca.go).
*/
package ledger

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// GenesisDigest is the chain's starting value (all zero), before any entry.
var GenesisDigest [32]byte

// NextDigest folds one committed raft entry into the chain:
//
//	chain[i] = SHA-256( chain[i-1] ‖ index ‖ term ‖ commandBytes )
//
// commandBytes is the exact bytes stored in the raft log entry (canonical after
// the value migration), so the result is identical on every replica.
func NextDigest(prev [32]byte, index, term uint64, command []byte) [32]byte {
	h := sha256.New()
	h.Write(prev[:])
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], index)
	h.Write(buf[:])
	binary.BigEndian.PutUint64(buf[:], term)
	h.Write(buf[:])
	h.Write(command)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// HashChain tracks the running chain head (the last folded entry's index and
// digest). It is not safe for concurrent use; the FSM advances it single-threaded
// on the apply path.
type HashChain struct {
	index  uint64
	digest [32]byte
}

// NewHashChain seeds a chain at a known head — genesis (index 0, zero digest) for
// a fresh node, or a persisted head after a restart/snapshot.
func NewHashChain(index uint64, digest [32]byte) *HashChain {
	return &HashChain{index: index, digest: digest}
}

// Advance folds an entry and moves the head forward. Entries must arrive in raft
// index order (the apply path guarantees this); out-of-order or replayed indices
// are ignored so re-applied entries do not corrupt the head.
func (c *HashChain) Advance(index, term uint64, command []byte) {
	if index <= c.index {
		return
	}
	c.digest = NextDigest(c.digest, index, term, command)
	c.index = index
}

// Head returns the current chain index and digest.
func (c *HashChain) Head() (index uint64, digest [32]byte) {
	return c.index, c.digest
}

// DigestHex renders a digest as lowercase hex.
func DigestHex(d [32]byte) string { return hex.EncodeToString(d[:]) }
