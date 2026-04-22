// Package sessionlog stores merged session lines on disk using an unlinked
// temp file so line text is not retained in RAM. A companion offset file
// (also unlinked) stores int64 byte offsets for random access by line index.
package sessionlog

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Kind classifies a log line for filtering and display.
type Kind string

const (
	KindStdout Kind = "o"
	KindStderr Kind = "e"
	// KindMeta is supervisor/imux text (not child stream bytes).
	KindMeta Kind = "m"
)

// Record is one JSON line in the backing store.
type Record struct {
	T    time.Time `json:"t"`
	K    Kind      `json:"k"`
	ID   string    `json:"id,omitempty"`
	Name string    `json:"name,omitempty"`
	Msg  string    `json:"m"`
}

// SessionLog is a line-oriented append-only log with per-line read by index.
type SessionLog struct {
	mu   sync.Mutex
	data *os.File
	off  *os.File
	tee  *os.File
}

// Open creates unlinked temp files for data and offsets. If teePath is non-empty,
// opens that path for append (plain text, no ANSI) and mirrors each line.
func Open(teePath string) (*SessionLog, error) {
	data, err := os.CreateTemp("", "imux-log-data-*.jsonl")
	if err != nil {
		return nil, fmt.Errorf("sessionlog data temp: %w", err)
	}
	dataName := data.Name()
	off, err := os.CreateTemp("", "imux-log-off-*.bin")
	if err != nil {
		_ = data.Close()
		_ = os.Remove(dataName)
		return nil, fmt.Errorf("sessionlog offset temp: %w", err)
	}
	offName := off.Name()
	if err := os.Remove(dataName); err != nil {
		_ = data.Close()
		_ = off.Close()
		_ = os.Remove(offName)
		return nil, fmt.Errorf("sessionlog unlink data: %w", err)
	}
	if err := os.Remove(offName); err != nil {
		_ = data.Close()
		_ = off.Close()
		return nil, fmt.Errorf("sessionlog unlink offsets: %w", err)
	}

	s := &SessionLog{data: data, off: off}
	if teePath != "" {
		f, err := os.OpenFile(teePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("sessionlog tee %q: %w", teePath, err)
		}
		s.tee = f
	}
	return s, nil
}

// Close releases backing files.
func (s *SessionLog) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	if s.data != nil {
		errs = append(errs, s.data.Close())
		s.data = nil
	}
	if s.off != nil {
		errs = append(errs, s.off.Close())
		s.off = nil
	}
	if s.tee != nil {
		errs = append(errs, s.tee.Close())
		s.tee = nil
	}
	return errors.Join(errs...)
}

// Append writes one record and appends its start offset to the sidecar.
func (s *SessionLog) Append(rec Record) error {
	if rec.T.IsZero() {
		rec.T = time.Now()
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil || s.off == nil {
		return errors.New("sessionlog: closed")
	}

	pos, err := s.data.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := s.data.Write(b); err != nil {
		return err
	}
	if err := binary.Write(s.off, binary.LittleEndian, pos); err != nil {
		return err
	}

	if s.tee != nil {
		tag := string(rec.K)
		if tag == "" {
			tag = "?"
		}
		who := rec.Name
		if who == "" {
			who = rec.ID
		}
		if who == "" {
			who = "?"
		}
		line := fmt.Sprintf("%s [%s|%s] %s\n", rec.T.Format(time.RFC3339Nano), tag, who, rec.Msg)
		if _, err := io.WriteString(s.tee, line); err != nil {
			return err
		}
	}
	return nil
}

// LineCount returns the number of lines stored.
func (s *SessionLog) LineCount() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.off == nil {
		return 0, errors.New("sessionlog: closed")
	}
	st, err := s.off.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size() / 8, nil
}

func readOffsetAt(f *os.File, lineIdx int64) (int64, error) {
	var buf [8]byte
	_, err := f.ReadAt(buf[:], lineIdx*8)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(buf[:])), nil
}

// ReadLine returns the record at line index i (0-based).
func (s *SessionLog) ReadLine(i int64) (Record, error) {
	var rec Record
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil || s.off == nil {
		return rec, errors.New("sessionlog: closed")
	}
	st, err := s.off.Stat()
	if err != nil {
		return rec, err
	}
	n := st.Size() / 8
	if i < 0 || i >= n {
		return rec, fmt.Errorf("sessionlog: line index %d out of range [0,%d)", i, n)
	}
	pos, err := readOffsetAt(s.off, i)
	if err != nil {
		return rec, err
	}

	var chunk [65536]byte
	var buf bytes.Buffer
	for {
		nr, err := s.data.ReadAt(chunk[:], pos+int64(buf.Len()))
		if nr > 0 {
			buf.Write(chunk[:nr])
			if idx := bytes.IndexByte(buf.Bytes(), '\n'); idx >= 0 {
				line := buf.Bytes()[:idx]
				if err := json.Unmarshal(line, &rec); err != nil {
					return rec, err
				}
				return rec, nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if buf.Len() == 0 {
					return rec, io.ErrUnexpectedEOF
				}
				if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
					return rec, err
				}
				return rec, nil
			}
			return rec, err
		}
		if buf.Len() > 16<<20 {
			return rec, errors.New("sessionlog: line exceeds 16MiB")
		}
	}
}
