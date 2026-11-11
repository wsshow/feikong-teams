package eventlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	storageport "fkteams/internal/ports/storage"
	"fkteams/internal/runtime/atomicfile"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func selectedDocuments(stores []storageport.Documents) storageport.Documents {
	if len(stores) > 0 {
		return stores[0]
	}
	return nil
}
func readDocument(path string, stores ...storageport.Documents) (io.ReadCloser, error) {
	if store := selectedDocuments(stores); store != nil {
		data, err := store.Read(path)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return os.Open(path)
}
func writeDocument(path string, data []byte, stores ...storageport.Documents) error {
	if store := selectedDocuments(stores); store != nil {
		return store.Write(path, data)
	}
	return atomicfile.WriteFile(path, data, 0644)
}
func documentTranscripts(sessionDir string, stores ...storageport.Documents) ([]string, error) {
	if store := selectedDocuments(stores); store != nil {
		paths, err := store.List(filepath.Join(sessionDir, subagentsDirName))
		if err != nil {
			return nil, err
		}
		filtered := []string{}
		for _, path := range paths {
			if filepath.Base(path) == TranscriptFileName {
				filtered = append(filtered, path)
			}
		}
		return filtered, nil
	}
	return filepath.Glob(filepath.Join(sessionDir, subagentsDirName, "*", TranscriptFileName))
}
func readDatabaseTranscript(path string, store storageport.Documents) ([]TranscriptEvent, error) {
	data, err := store.Read(path)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	events := []TranscriptEvent{}
	for {
		record, readErr := readTranscriptRecord(reader, maxTranscriptRecordBytes)
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if trimmed := bytes.TrimSpace(record); len(trimmed) > 0 {
			var event TranscriptEvent
			if err := json.Unmarshal(trimmed, &event); err != nil {
				return nil, fmt.Errorf("decode database transcript: %w", err)
			}
			events = append(events, event.Normalize())
		}
		if readErr == io.EOF {
			return events, nil
		}
	}
}
