package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Filter specifies predicates for ReadFiltered. Zero values are ignored.
type Filter struct {
	Type     string    // match events with this Type
	Actor    string    // match events with this Actor
	Since    time.Time // match events at or after this time
	AfterSeq uint64    // match events with Seq > AfterSeq (0 = no filter)
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
		if filter.AfterSeq > 0 && e.Seq <= filter.AfterSeq {
			continue
		}
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

// ReadLatestSeq returns the highest Seq in the events file, or 0 if
// the file is missing or empty.
func ReadLatestSeq(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading latest seq: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	var maxSeq uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}
	if err := scanner.Err(); err != nil {
		return maxSeq, fmt.Errorf("scanning events: %w", err)
	}
	return maxSeq, nil
}

// ReadFrom reads events starting at the given byte offset in the file.
// Returns the events read, the byte offset after the last complete line,
// and any error. Returns (nil, offset, nil) if no new data is available
// or the file doesn't exist yet. Skips malformed lines (partial writes).
func ReadFrom(path string, offset int64) ([]Event, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}
		return nil, offset, fmt.Errorf("reading events: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seeking events: %w", err)
	}

	var result []Event
	scanner := bufio.NewScanner(f)
	bytesRead := int64(0)
	for scanner.Scan() {
		line := scanner.Bytes()
		bytesRead += int64(len(line)) + 1 // +1 for newline
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines (partial writes)
		}
		result = append(result, e)
	}
	if err := scanner.Err(); err != nil {
		return result, offset + bytesRead, fmt.Errorf("scanning events: %w", err)
	}
	return result, offset + bytesRead, nil
}
