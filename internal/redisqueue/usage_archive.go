package redisqueue

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

var (
	archivePath atomic.Value
	archiveMu   sync.Mutex
)

// SetUsageArchivePath configures a local JSONL file used by the management usage API.
func SetUsageArchivePath(path string) {
	archivePath.Store(strings.TrimSpace(path))
}

// UsageArchivePath returns the configured local usage archive path.
func UsageArchivePath() string {
	value, _ := archivePath.Load().(string)
	return strings.TrimSpace(value)
}

func appendUsageArchive(payload []byte) {
	path := UsageArchivePath()
	if path == "" || len(bytes.TrimSpace(payload)) == 0 {
		return
	}

	line, err := marshalUsageArchiveLine(payload)
	if err != nil {
		log.WithError(err).Warn("failed to encode usage archive record")
		return
	}

	archiveMu.Lock()
	defer archiveMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.WithError(err).Warn("failed to create usage archive directory")
		return
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.WithError(err).Warn("failed to open usage archive")
		return
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			log.WithError(errClose).Warn("failed to close usage archive")
		}
	}()

	if _, errWrite := file.Write(append(line, '\n')); errWrite != nil {
		log.WithError(errWrite).Warn("failed to write usage archive")
	}
}

func marshalUsageArchiveLine(payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if json.Valid(trimmed) {
		return append([]byte(nil), trimmed...), nil
	}
	return json.Marshal(string(trimmed))
}

// ArchivedUsageRecords returns locally persisted usage records as JSON fragments.
func ArchivedUsageRecords() [][]byte {
	path := UsageArchivePath()
	if path == "" {
		return nil
	}

	archiveMu.Lock()
	defer archiveMu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.WithError(err).Warn("failed to open usage archive for reading")
		}
		return nil
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			log.WithError(errClose).Warn("failed to close usage archive reader")
		}
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	records := make([][]byte, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		records = append(records, append([]byte(nil), line...))
	}
	if errScan := scanner.Err(); errScan != nil {
		log.WithError(errScan).Warn("failed to scan usage archive")
	}
	return records
}
