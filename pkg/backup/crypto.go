/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package backup writes and reads consensusdb dumps: an optionally password-
protected, streaming container around the raw badger backup bytes, plus sinks and
sources over local files and S3-compatible object storage (AWS S3, MinIO, GCS).

Container format — a self-describing header followed by the body:

	magic     [6]byte  "CDBBAK"
	version   byte      1
	mode      byte      0 = plain, 1 = argon2id + AES-256-GCM
	if mode==1:
	  memKiB    uint32   argon2id memory
	  timeCost  uint32   argon2id iterations
	  threads   uint8    argon2id parallelism
	  saltLen   uint8    then salt[saltLen]
	  noncePfx  [4]byte  random nonce prefix
	body:
	  plain: the raw backup bytes to EOF
	  enc:   a sequence of AEAD frames (below)

Encrypted body — the age-STREAM construction: the plaintext is split into 64 KiB
chunks, each sealed with AES-256-GCM under a 96-bit nonce
[noncePfx(4) ‖ counter(7, big-endian) ‖ lastFlag(1)]. The final chunk (possibly
empty) sets lastFlag=1, so a truncated stream is detected — the reader never sees
a valid last chunk. Each frame is [uint32 header: bit31=last, bits0..30=ciphertext
length] ‖ ciphertext. Any tamper fails the GCM tag. The random salt makes the
derived key unique per dump, so the counter safely starts at 0.
*/
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/xerrors"
)

const (
	magic       = "CDBBAK"
	formatVer   = 1
	modePlain   = 0
	modeEncrypt = 1

	chunkSize   = 64 * 1024
	saltLen     = 16
	noncePfxLen = 4
	keyLen      = 32 // AES-256
	lastFlagBit = 1 << 31

	argonMemKiB   = 64 * 1024 // 64 MiB — backups are infrequent, so a strong KDF is cheap
	argonTimeCost = 3
	argonThreads  = 4
)

// NewWriter returns a WriteCloser that wraps dst with the backup container. An
// empty password writes a plain dump; a non-empty password derives an
// argon2id key and encrypts. Close MUST be called — it flushes the final
// (finality-marked) chunk that makes the dump verifiable and complete.
func NewWriter(dst io.Writer, password string) (io.WriteCloser, error) {
	if password == "" {
		if _, err := dst.Write(header(modePlain, nil, nil, 0)); err != nil {
			return nil, err
		}
		return &plainWriter{dst: dst}, nil
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	noncePfx := make([]byte, noncePfxLen)
	if _, err := rand.Read(noncePfx); err != nil {
		return nil, err
	}
	aead, err := newAEAD(password, salt)
	if err != nil {
		return nil, err
	}
	if _, err := dst.Write(header(modeEncrypt, salt, noncePfx, 0)); err != nil {
		return nil, err
	}
	return &encWriter{dst: dst, aead: aead, noncePfx: noncePfx, buf: make([]byte, 0, chunkSize)}, nil
}

// NewReader reads the container header from src and returns a Reader over the
// original backup bytes. A password is required iff the dump is encrypted;
// supplying the wrong password (or a tampered/truncated stream) fails on Read.
func NewReader(src io.Reader, password string) (io.Reader, error) {
	head := make([]byte, len(magic)+2)
	if _, err := io.ReadFull(src, head); err != nil {
		return nil, xerrors.Errorf("backup: read header: %w", err)
	}
	if string(head[:len(magic)]) != magic {
		return nil, xerrors.New("backup: not a consensusdb dump (bad magic)")
	}
	if head[len(magic)] != formatVer {
		return nil, xerrors.Errorf("backup: unsupported format version %d", head[len(magic)])
	}
	switch head[len(magic)+1] {
	case modePlain:
		if password != "" {
			return nil, xerrors.New("backup: dump is not encrypted, no password needed")
		}
		return src, nil
	case modeEncrypt:
		if password == "" {
			return nil, xerrors.New("backup: dump is encrypted, a password is required")
		}
		return newEncReader(src, password)
	default:
		return nil, xerrors.Errorf("backup: unknown mode %d", head[len(magic)+1])
	}
}

// header builds the container header. salt/noncePfx are only used for modeEncrypt.
func header(mode byte, salt, noncePfx []byte, _ int) []byte {
	h := append([]byte(magic), formatVer, mode)
	if mode == modeEncrypt {
		h = binary.BigEndian.AppendUint32(h, argonMemKiB)
		h = binary.BigEndian.AppendUint32(h, argonTimeCost)
		h = append(h, argonThreads, byte(len(salt)))
		h = append(h, salt...)
		h = append(h, noncePfx...)
	}
	return h
}

func newAEAD(password string, salt []byte) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(password), salt, argonTimeCost, argonMemKiB, argonThreads, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// nonce composes the 96-bit nonce for chunk counter, with the finality flag.
func nonce(noncePfx []byte, counter uint64, last bool) []byte {
	n := make([]byte, 12)
	copy(n[:noncePfxLen], noncePfx)
	var c [8]byte
	binary.BigEndian.PutUint64(c[:], counter)
	copy(n[noncePfxLen:11], c[1:]) // low 7 bytes of the counter
	if last {
		n[11] = 1
	}
	return n
}

// --- plain ---------------------------------------------------------------------

type plainWriter struct{ dst io.Writer }

func (w *plainWriter) Write(p []byte) (int, error) { return w.dst.Write(p) }
func (w *plainWriter) Close() error                { return nil }

// --- encrypted writer ----------------------------------------------------------

type encWriter struct {
	dst      io.Writer
	aead     cipher.AEAD
	noncePfx []byte
	counter  uint64
	buf      []byte
	closed   bool
}

func (w *encWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, xerrors.New("backup: write after close")
	}
	total := len(p)
	for len(p) > 0 {
		space := chunkSize - len(w.buf)
		n := len(p)
		if n > space {
			n = space
		}
		w.buf = append(w.buf, p[:n]...)
		p = p[n:]
		if len(w.buf) == chunkSize {
			if err := w.seal(false); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

// Close seals the buffered remainder as the final chunk (empty is fine) so the
// dump is complete and truncation-detectable.
func (w *encWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.seal(true)
}

func (w *encWriter) seal(last bool) error {
	ct := w.aead.Seal(nil, nonce(w.noncePfx, w.counter, last), w.buf, nil)
	w.counter++
	w.buf = w.buf[:0]

	hdr := uint32(len(ct))
	if last {
		hdr |= lastFlagBit
	}
	if err := binary.Write(w.dst, binary.BigEndian, hdr); err != nil {
		return err
	}
	_, err := w.dst.Write(ct)
	return err
}

// --- encrypted reader ----------------------------------------------------------

type encReader struct {
	src      io.Reader
	aead     cipher.AEAD
	noncePfx []byte
	counter  uint64
	plain    []byte // decrypted, not yet consumed
	done     bool
}

func newEncReader(src io.Reader, password string) (*encReader, error) {
	head := make([]byte, 4+4+1+1)
	if _, err := io.ReadFull(src, head); err != nil {
		return nil, xerrors.Errorf("backup: read kdf params: %w", err)
	}
	// memKiB, timeCost read from the stream for forward-compat, but the AEAD is
	// derived with the parameters actually stored (below). We honor stored params.
	memKiB := binary.BigEndian.Uint32(head[0:4])
	timeCost := binary.BigEndian.Uint32(head[4:8])
	threads := head[8]
	sl := int(head[9])
	rest := make([]byte, sl+noncePfxLen)
	if _, err := io.ReadFull(src, rest); err != nil {
		return nil, xerrors.Errorf("backup: read salt: %w", err)
	}
	salt, noncePfx := rest[:sl], rest[sl:]

	key := argon2.IDKey([]byte(password), salt, timeCost, memKiB, threads, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &encReader{src: src, aead: aead, noncePfx: noncePfx}, nil
}

func (r *encReader) Read(p []byte) (int, error) {
	for len(r.plain) == 0 {
		if r.done {
			return 0, io.EOF
		}
		if err := r.next(); err != nil {
			return 0, err
		}
	}
	n := copy(p, r.plain)
	r.plain = r.plain[n:]
	return n, nil
}

// next reads, decrypts and buffers one frame; sets done on the final chunk.
func (r *encReader) next() error {
	var hdr uint32
	if err := binary.Read(r.src, binary.BigEndian, &hdr); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// EOF before a finality-marked chunk means the dump was truncated.
			return xerrors.New("backup: truncated dump (no final chunk)")
		}
		return err
	}
	last := hdr&lastFlagBit != 0
	ct := make([]byte, hdr&^lastFlagBit)
	if _, err := io.ReadFull(r.src, ct); err != nil {
		return xerrors.Errorf("backup: truncated frame: %w", err)
	}
	pt, err := r.aead.Open(nil, nonce(r.noncePfx, r.counter, last), ct, nil)
	if err != nil {
		return xerrors.New("backup: decryption failed (wrong password or corrupt dump)")
	}
	r.counter++
	r.plain = pt
	if last {
		r.done = true
	}
	return nil
}
