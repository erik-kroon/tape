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

type sessionFile struct {
	path   string
	codecs sessionFileCodecRegistry
}

type sessionFileCodecRegistry struct {
	byType map[string]EventCodec
}

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

type sessionFileWriter struct {
	session      sessionFile
	file         *os.File
	writer       *bufio.Writer
	bytesWritten int64
	indexEntries []sessionIndexEntry
	closed       bool
}

type sessionFileStream struct {
	session sessionFile
	file    *os.File
	scanner *bufio.Scanner
	index   *sessionIndex
}

func newSessionFile(path string, codecs []EventCodec) (sessionFile, error) {
	registry, err := newSessionFileCodecRegistry(codecs)
	if err != nil {
		return sessionFile{}, err
	}

	return sessionFile{
		path:   path,
		codecs: registry,
	}, nil
}

func newSessionFileCodecRegistry(custom []EventCodec) (sessionFileCodecRegistry, error) {
	registry := sessionFileCodecRegistry{
		byType: map[string]EventCodec{},
	}

	for _, codec := range builtinEventCodecs() {
		registry.byType[codec.Type] = codec
	}

	for _, codec := range custom {
		if err := validateEventCodec(codec); err != nil {
			return sessionFileCodecRegistry{}, err
		}
		if _, exists := registry.byType[codec.Type]; exists {
			return sessionFileCodecRegistry{}, fmt.Errorf("codec error: duplicate event codec %q", codec.Type)
		}
		registry.byType[codec.Type] = codec
	}

	return registry, nil
}

func (r sessionFileCodecRegistry) Marshal(index int, event Event) ([]byte, error) {
	codec, ok := r.byType[event.Type()]
	if !ok {
		return nil, fmt.Errorf("sink error: unsupported event type %T", event)
	}

	payload, err := codec.Encode(event)
	if err != nil {
		return nil, err
	}

	return json.Marshal(sessionRecord{
		Type:    codec.Type,
		Payload: payload,
		Index:   index,
	})
}

func (r sessionFileCodecRegistry) Decode(record sessionRecord) (Event, error) {
	codec, ok := r.byType[record.Type]
	if !ok {
		return nil, fmt.Errorf("decode error: unknown event type %q", record.Type)
	}

	payload, err := record.payload()
	if err != nil {
		return nil, err
	}

	event, err := codec.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("decode error: invalid %s payload: %w", record.Type, err)
	}
	return event, nil
}

func (s sessionFile) buildIndex() error {
	if err := s.validateIndexSource(); err != nil {
		return err
	}

	entries, err := s.scanIndexEntries()
	if err != nil {
		return err
	}

	return s.writeIndex(entries)
}

func (s sessionFile) newWriter() (*sessionFileWriter, error) {
	file, err := os.Create(s.path)
	if err != nil {
		return nil, err
	}

	return &sessionFileWriter{
		session: s,
		file:    file,
		writer:  bufio.NewWriter(file),
	}, nil
}

func (s sessionFile) openStream() (*sessionFileStream, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}

	return &sessionFileStream{
		session: s,
		file:    file,
		scanner: bufio.NewScanner(file),
		index:   s.loadIndex(),
	}, nil
}

func (s sessionFile) validateIndexSource() error {
	ext := strings.ToLower(filepath.Ext(s.path))
	switch ext {
	case ".tape", ".jsonl":
		return nil
	default:
		return fmt.Errorf("unsupported index source %q", ext)
	}
}

func (s sessionFile) scanIndexEntries() ([]sessionIndexEntry, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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
			record, event, decodeErr := s.decodeLine(line)
			if decodeErr != nil {
				return nil, decodeErr
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

func (s sessionFile) writeIndex(entries []sessionIndexEntry) error {
	info, err := os.Stat(s.path)
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

	indexPath := sessionIndexPath(s.path)
	tempPath := indexPath + ".tmp"
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, indexPath)
}

func (s sessionFile) loadIndex() *sessionIndex {
	data, err := os.ReadFile(sessionIndexPath(s.path))
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

	info, err := os.Stat(s.path)
	if err != nil {
		return nil
	}
	if info.Size() != index.SourceSize || info.ModTime().UnixNano() != index.SourceModTimeUnixNano {
		return nil
	}

	return &index
}

func (s sessionFile) decodeLine(line []byte) (sessionRecord, Event, error) {
	record, err := decodeSessionRecord(line)
	if err != nil {
		return sessionRecord{}, nil, err
	}

	event, err := s.codecs.Decode(record)
	if err != nil {
		return sessionRecord{}, nil, err
	}

	return record, event, nil
}

func (w *sessionFileWriter) Write(index int, event Event) error {
	if w.closed {
		return fmt.Errorf("sink error: recorder closed")
	}

	record, err := w.session.codecs.Marshal(index, event)
	if err != nil {
		return err
	}
	w.indexEntries = append(w.indexEntries, newSessionIndexEntry(w.bytesWritten, index, event))
	if _, err := w.writer.Write(record); err != nil {
		return err
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return err
	}
	w.bytesWritten += int64(len(record) + 1)
	return w.writer.Flush()
}

func (w *sessionFileWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if w.writer != nil {
		if err := w.writer.Flush(); err != nil {
			w.file.Close()
			return err
		}
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	return w.session.writeIndex(w.indexEntries)
}

func (s *sessionFileStream) Next() (Event, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	_, event, err := s.session.decodeLine(s.scanner.Bytes())
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (s *sessionFileStream) SeekStartAt(startAt StartAt) (bool, error) {
	if s.index == nil || !startAt.Active() {
		return false, nil
	}

	if _, err := s.file.Seek(s.index.offsetFor(startAt), io.SeekStart); err != nil {
		return false, err
	}
	s.scanner = bufio.NewScanner(s.file)
	return true, nil
}

func (s *sessionFileStream) Close() error {
	return s.file.Close()
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
