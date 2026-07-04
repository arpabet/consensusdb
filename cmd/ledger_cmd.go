/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.arpabet.com/cligo"
	"go.arpabet.com/consensusdb/pkg/ledger"
	"go.arpabet.com/consensusdb/pkg/verify"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
Ledger CLI (plan S6): manage the ledger CA and node signing keys, and verify the
consensus offline.

	consensusdb ledger ca-init      ca.key ca.pub      # once: create the CA
	consensusdb ledger keygen       node.key node.pub  # per node: BLS key pair
	consensusdb ledger issue        ca.key <id> node.pub node.cert   # CA vouches
	consensusdb ledger verify       ca.pub qc.bin node-a.cert node-b.cert …

Keys/certs are raw bytes (BLS keys, value-encoded certs). The CA private key stays
offline; only ca.pub and the node certs are needed to verify a quorum.
*/

type LedgerGroup struct {
	Parent cligo.CliGroup `cli:"group=cli"`
}

func (LedgerGroup) Group() string { return "ledger" }

func (LedgerGroup) Help() (string, string) {
	return "verifiable ledger: CA, node keys, and offline verification", ""
}

func writeFile(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }

// LedgerCAInitCommand creates a new ledger CA (Ed25519), writing the private seed
// and the public key.
type LedgerCAInitCommand struct {
	Parent cligo.CliGroup `cli:"group=ledger"`
	KeyOut string         `cli:"argument=ca_key_out,required"`
	PubOut string         `cli:"argument=ca_pub_out,required"`
	Log    *zap.Logger    `inject:""`
}

func (t *LedgerCAInitCommand) Command() string { return "ca-init" }
func (t *LedgerCAInitCommand) Help() (string, string) {
	return "create the ledger certificate authority", "Writes the CA private seed and public key. Keep the private seed offline."
}

func (t *LedgerCAInitCommand) Run(ctx context.Context) error {
	ca, err := ledger.GenerateCA()
	if err != nil {
		return err
	}
	seed, _ := ca.MarshalBinary()
	pub, _ := ca.Public().MarshalBinary()
	if err := writeFile(t.KeyOut, seed); err != nil {
		return err
	}
	if err := writeFile(t.PubOut, pub); err != nil {
		return err
	}
	fmt.Printf("ledger CA created: private %s (keep offline), public %s\n", t.KeyOut, t.PubOut)
	return nil
}

// LedgerKeygenCommand generates a node BLS key pair.
type LedgerKeygenCommand struct {
	Parent cligo.CliGroup `cli:"group=ledger"`
	KeyOut string         `cli:"argument=node_key_out,required"`
	PubOut string         `cli:"argument=node_pub_out,required"`
	Log    *zap.Logger    `inject:""`
}

func (t *LedgerKeygenCommand) Command() string { return "keygen" }
func (t *LedgerKeygenCommand) Help() (string, string) {
	return "generate a node BLS signing key pair", "BLS12-381; signatures are 48 bytes and aggregate into one across the quorum."
}

func (t *LedgerKeygenCommand) Run(ctx context.Context) error {
	key, err := ledger.GenerateNodeKey()
	if err != nil {
		return err
	}
	priv, _ := key.MarshalBinary()
	pub, _ := key.Public().MarshalBinary()
	if err := writeFile(t.KeyOut, priv); err != nil {
		return err
	}
	if err := writeFile(t.PubOut, pub); err != nil {
		return err
	}
	fmt.Printf("node key created: private %s, public %s\n", t.KeyOut, t.PubOut)
	fmt.Println("next: get it certified — `consensusdb ledger issue <ca.key> <node-id> <node.pub> <node.cert>`")
	return nil
}

// LedgerIssueCommand certifies a node public key with the CA. The proof-of-
// possession is produced from the node private key (so this runs where the node
// key is available, or the operator supplies it); here we require the node key to
// prove possession, keeping the CA honest.
type LedgerIssueCommand struct {
	Parent  cligo.CliGroup `cli:"group=ledger"`
	CAKey   string         `cli:"argument=ca_key,required"`
	NodeID  string         `cli:"argument=node_id,required"`
	NodeKey string         `cli:"argument=node_key,required"`
	CertOut string         `cli:"argument=cert_out,required"`
	Days    int            `cli:"option=days,default=0,help=validity in days (0 = no expiry)"`
	Log     *zap.Logger    `inject:""`
}

func (t *LedgerIssueCommand) Command() string { return "issue" }
func (t *LedgerIssueCommand) Help() (string, string) {
	return "CA-certify a node key for a node id",
		"Binds <node_id> to the node's public key, signed by the CA, after verifying a proof-of-possession from the node private key."
}

func (t *LedgerIssueCommand) Run(ctx context.Context) error {
	caSeed, err := os.ReadFile(t.CAKey)
	if err != nil {
		return err
	}
	ca, err := ledger.ParseCA(caSeed)
	if err != nil {
		return err
	}
	keyBytes, err := os.ReadFile(t.NodeKey)
	if err != nil {
		return err
	}
	nodeKey, err := ledger.ParseNodePrivateKey(keyBytes)
	if err != nil {
		return err
	}
	pop, err := ledger.ProofOfPossession(nodeKey, t.NodeID)
	if err != nil {
		return err
	}
	var notAfter int64
	if t.Days > 0 {
		notAfter = time.Now().AddDate(0, 0, t.Days).Unix()
	}
	cert, err := ca.Issue(t.NodeID, nodeKey.Public(), pop, notAfter)
	if err != nil {
		return err
	}
	raw, err := ledger.EncodeCert(cert)
	if err != nil {
		return err
	}
	if err := writeFile(t.CertOut, raw); err != nil {
		return err
	}
	fmt.Printf("issued cert for %q → %s\n", t.NodeID, t.CertOut)
	return nil
}

// LedgerVerifyCommand verifies a quorum certificate offline against the CA public
// key and the node certs — no running cluster, the anti-vendor-lock property.
type LedgerVerifyCommand struct {
	Parent    cligo.CliGroup `cli:"group=ledger"`
	CAPub     string         `cli:"argument=ca_pub,required"`
	QuorumIn  string         `cli:"argument=quorum_cert,required"`
	CertPaths []string       `cli:"argument=node_certs,required"`
	Threshold int            `cli:"option=threshold,default=0,help=minimum signers required (0 = any signer count present)"`
	Log       *zap.Logger    `inject:""`
}

func (t *LedgerVerifyCommand) Command() string { return "verify" }
func (t *LedgerVerifyCommand) Help() (string, string) {
	return "verify a quorum certificate offline against the CA + node certs", ""
}

func (t *LedgerVerifyCommand) Run(ctx context.Context) error {
	caPubBytes, err := os.ReadFile(t.CAPub)
	if err != nil {
		return err
	}
	caPub, err := ledger.ParseCAPublicKey(caPubBytes)
	if err != nil {
		return err
	}
	qcBytes, err := os.ReadFile(t.QuorumIn)
	if err != nil {
		return err
	}
	qc, err := ledger.DecodeQuorum(qcBytes)
	if err != nil {
		return err
	}
	certs := map[string]*ledger.NodeCert{}
	for _, path := range t.CertPaths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cert, err := ledger.DecodeCert(raw)
		if err != nil {
			return xerrors.Errorf("%s: %w", path, err)
		}
		certs[cert.NodeID] = cert
	}
	threshold := t.Threshold
	if threshold == 0 {
		threshold = len(qc.Signers)
	}
	if err := ledger.VerifyQuorum(caPub, qc, certs, threshold, time.Now().Unix()); err != nil {
		return err
	}
	fmt.Printf("VERIFIED ✓  height=%d digest=%s signers=%d (threshold %d)\n",
		qc.Checkpoint.Height, ledger.DigestHex(digest32(qc.Checkpoint.Digest)), len(qc.Signers), threshold)
	return nil
}

func digest32(b []byte) [32]byte {
	var d [32]byte
	copy(d[:], b)
	return d
}

// LedgerVerifyBackupCommand verifies OFFLINE that a backup dump corresponds to a
// quorum-certified checkpoint: it loads the dump into a throwaway store, reads the
// persisted chain head, and confirms the quorum certificate attests exactly that
// head. No running cluster is needed.
type LedgerVerifyBackupCommand struct {
	Parent    cligo.CliGroup `cli:"group=ledger"`
	Backup    string         `cli:"argument=backup,required"`
	CAPub     string         `cli:"argument=ca_pub,required"`
	QuorumIn  string         `cli:"argument=quorum_cert,required"`
	CertPaths []string       `cli:"argument=node_certs,required"`
	Password  string         `cli:"option=password,default=,help=dump password (or BACKUP_PASSWORD)"`
	Threshold int            `cli:"option=threshold,default=0,help=minimum signers (0 = the certificate's signer count)"`
	s3Options
	Log *zap.Logger `inject:""`
}

func (t *LedgerVerifyBackupCommand) Command() string { return "verify-backup" }
func (t *LedgerVerifyBackupCommand) Help() (string, string) {
	return "verify a backup file against a quorum certificate (offline)",
		"Confirms the backup is exactly the state a quorum certified at a height: loads the dump, reads the chain head, and checks it matches the quorum certificate's checkpoint."
}

func (t *LedgerVerifyBackupCommand) Run(ctx context.Context) error {
	caCert, err := os.ReadFile(t.CAPub)
	if err != nil {
		return err
	}
	quorum, err := os.ReadFile(t.QuorumIn)
	if err != nil {
		return err
	}
	var nodeCerts [][]byte
	for _, p := range t.CertPaths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		nodeCerts = append(nodeCerts, raw)
	}
	opts := verify.Options{
		Source:     t.Backup,
		Password:   t.s3Options.resolvePassword(t.Password),
		S3:         t.config(),
		CACert:     caCert,
		QuorumCert: quorum,
		NodeCerts:  nodeCerts,
		Threshold:  t.Threshold,
	}
	lastPct := -1
	res, err := verify.VerifyBackup(ctx, opts, func(pct int) {
		if pct/10 != lastPct/10 { // print at 10% steps
			lastPct = pct
			fmt.Printf("\rverifying… %3d%%", pct)
		}
	})
	fmt.Println()
	if err != nil {
		return err
	}
	if !res.Verified {
		return xerrors.Errorf("NOT VERIFIED: %s", res.Message)
	}
	fmt.Printf("VERIFIED ✓  height=%d digest=%s signers=%d\n%s\n",
		res.Height, res.Digest, res.Signers, res.Message)
	return nil
}
