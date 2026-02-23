package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Filter specifies predicates for ReadFiltered. Zero values are ignored.
type Filter struct {
	Type  string    // match events with this Type
	Actor string    // match events with this Actor
	Since time.Time // match events at or after this time
}

// ReadAll reads all events from the JSONL file at path.
// Returns (nil, nil) if the file is missing or empty.
func ReadAll(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading events: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("scanning events: %w", err)
	}
	return events, nil
}

// ReadFiltered reads events from path and returns only those matching
// all non-zero fields in filter. Returns (nil, nil) if the file is
// missing or empty.
func ReadFiltered(path string, filter Filter) ([]Event, error) {
	all, err := ReadAll(path)
	if err != nil {
		return nil, err
	}

	var result []Event
	for _, e := range all {
		if filter.Type != "" && e.Type != filter.Type {
			continue
		}
		if filter.Actor != "" && e.Actor != filter.Actor {
			continue
		}
		if !filter.Since.IsZero() && e.Ts.Before(filter.Since) {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}
