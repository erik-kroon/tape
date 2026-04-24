package tape

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const sessionIndexVersion = 1

type sessionIndex struct {
	Version               int                 `json:"version"`
	SourceSize            int64               `json:"source_size"`
	SourceModTimeUnixNano int64               `json:"source_mod_time_unix_nano"`
	Entries               []sessionIndexEntry `json:"entries"`
}

type sessionIndexEntry struct {
	Offset   int64  `json:"offset"`
	Time     string `json:"time"`
	Sequence int64  `json:"sequence"`
	Index    int    `json:"index"`
}

func BuildSessionIndex(path string) error {
	return buildSessionIndexWithCodecs(path)
}

func buildSessionIndexWithCodecs(path string, codecs ...EventCodec) error {
	if err := validateSessionIndexSource(path); err != nil {
		return err
	}

	entries, err := scanSessionIndexEntries(path, codecs...)
	if err != nil {
		return err
	}

	return writeSessionIndex(path, entries)
}

func validateSessionIndexSource(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".tape", ".jsonl":
		return nil
	default:
		return fmt.Errorf("unsupported index source %q", ext)
	}
}

func scanSessionIndexEntries(path string, codecs ...EventCodec) ([]sessionIndexEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	registry, err := newEventCodecRegistry(codecs)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(file)
	var (
		offset  int64
		entries []sessionIndexEntry
	)
	for {
		lineOffset := offset
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			offset += int64(len(line))
			record, err := decodeSessionRecord(line)
			if err != nil {
				return nil, err
			}
			event, err := registry.Decode(record)
			if err != nil {
				return nil, err
			}
			entries = append(entries, newSessionIndexEntry(lineOffset, record.Index, event))
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}

	return entries, nil
}

func writeSessionIndex(path string, entries []sessionIndexEntry) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	index := sessionIndex{
		Version:               sessionIndexVersion,
		SourceSize:            info.Size(),
		SourceModTimeUnixNano: info.ModTime().UnixNano(),
		Entries:               entries,
	}

	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	indexPath := sessionIndexPath(path)
	tempPath := indexPath + ".tmp"
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, indexPath)
}

func loadSessionIndex(path string) *sessionIndex {
	data, err := os.ReadFile(sessionIndexPath(path))
	if err != nil {
		return nil
	}

	var index sessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil
	}
	if index.Version != sessionIndexVersion {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Size() != index.SourceSize || info.ModTime().UnixNano() != index.SourceModTimeUnixNano {
		return nil
	}

	return &index
}

func (i *sessionIndex) offsetFor(startAt StartAt) int64 {
	if len(i.Entries) == 0 {
		return i.SourceSize
	}

	if !startAt.Time.IsZero() {
		target := startAt.Time
		position := sort.Search(len(i.Entries), func(idx int) bool {
			entryTime, err := time.Parse(time.RFC3339Nano, i.Entries[idx].Time)
			if err != nil {
				return true
			}
			return !entryTime.Before(target)
		})
		if position >= len(i.Entries) {
			return i.SourceSize
		}
		return i.Entries[position].Offset
	}

	position := sort.Search(len(i.Entries), func(idx int) bool {
		return i.Entries[idx].Sequence >= startAt.Sequence
	})
	if position >= len(i.Entries) {
		return i.SourceSize
	}
	return i.Entries[position].Offset
}

func sessionIndexPath(path string) string {
	return path + ".idx"
}

func decodeSessionRecord(line []byte) (sessionRecord, error) {
	var record sessionRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return sessionRecord{}, fmt.Errorf("decode error: invalid session record: %w", err)
	}
	return record, nil
}

func newSessionIndexEntry(offset int64, index int, event Event) sessionIndexEntry {
	return sessionIndexEntry{
		Offset:   offset,
		Time:     event.Timestamp().UTC().Format(time.RFC3339Nano),
		Sequence: event.Sequence(),
		Index:    index,
	}
}
