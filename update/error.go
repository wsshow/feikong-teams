package update

import "errors"

var (
	// ErrUnsupportedChecksumAlgorithm Unsupported checksum algorithm
	ErrUnsupportedChecksumAlgorithm = errors.New("unsupported checksum algorithm")
	// ErrChecksumNotMatched File checksum does not match the computed checksum
	ErrChecksumNotMatched = errors.New("file checksum does not match the computed checksum")
	// ErrChecksumFileNotFound Checksum file not found
	ErrChecksumFileNotFound = errors.New("checksum file not found")
	// ErrAssetNotFound Asset not found
	ErrAssetNotFound = errors.New("asset not found")
	// ErrCollectorNotFound Collector not found
	ErrCollectorNotFound = errors.New("collector not found")
	// ErrEmptyURL URL is empty
	ErrEmptyURL = errors.New("empty url")
)
