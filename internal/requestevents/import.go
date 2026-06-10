package requestevents

import (
	"bufio"
	"bytes"
	"fmt"
)

func ParseImportJSONL(data []byte) ([]Event, int, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, 0, fmt.Errorf("empty import payload")
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	events := make([]Event, 0)
	failed := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		event, err := NormalizeRaw(line)
		if err != nil {
			failed++
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, failed, err
	}
	if len(events) == 0 {
		return nil, failed, fmt.Errorf("no valid request events found")
	}
	return events, failed, nil
}
