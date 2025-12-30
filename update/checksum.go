package update

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

// Algorithm checksum algorithm type
type Algorithm string

const (
	// SHA256 algorithm using SHA-256 hash function.
	SHA256 Algorithm = "SHA256"
	// SHA1 algorithm using SHA-1 hash function.
	SHA1 Algorithm = "SHA1"
)

// VerifyFile validates file integrity against expected checksum.
func VerifyFile(algo Algorithm, expectedChecksum, filename string) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var h hash.Hash
	switch algo {
	case SHA256:
		h = sha256.New()
	case SHA1:
		h = sha1.New()
	default:
		return ErrUnsupportedChecksumAlgorithm
	}

	if _, err = io.Copy(h, f); err != nil {
		return err
	}

	if expectedChecksum != hex.EncodeToString(h.Sum(nil)) {
		return ErrChecksumNotMatched
	}
	return nil
}
