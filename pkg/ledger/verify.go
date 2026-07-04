/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package ledger

import (
	"bytes"

	"golang.org/x/xerrors"
)

/*
Verification helpers shared by the CLI, the ledger.verify API, and the
backup-verifier. They take only the trust material — CA public key, node certs,
quorum certificate — so verification always works offline, without a running
cluster (the anti-vendor-lock property).
*/

// CertBundle collects node certs by node id (built from files, digests, or a
// registry).
type CertBundle map[string]*NodeCert

// AddCert indexes a cert by its node id.
func (b CertBundle) AddCert(cert *NodeCert) { b[cert.NodeID] = cert }

// Verify checks a quorum certificate against the CA and this bundle at the given
// threshold. It returns nil when the certificate is a valid quorum proof.
func Verify(ca *CAPublicKey, qc *QuorumCertificate, certs CertBundle, threshold int, now int64) error {
	return VerifyQuorum(ca, qc, map[string]*NodeCert(certs), threshold, now)
}

// MatchesHead reports whether a verified quorum certificate attests exactly the
// chain head (index, digest) found in some state — the link that ties a backup's
// data to a quorum-agreed checkpoint. Verify the quorum certificate first.
func (qc *QuorumCertificate) MatchesHead(index uint64, digest [32]byte) error {
	if qc.Checkpoint.Height != index {
		return xerrors.Errorf("ledger: checkpoint height %d != state head %d", qc.Checkpoint.Height, index)
	}
	if !bytes.Equal(qc.Checkpoint.Digest, digest[:]) {
		return xerrors.Errorf("ledger: checkpoint digest %s != state head %s",
			DigestHex(digest32(qc.Checkpoint.Digest)), DigestHex(digest))
	}
	return nil
}

func digest32(b []byte) [32]byte {
	var d [32]byte
	copy(d[:], b)
	return d
}
