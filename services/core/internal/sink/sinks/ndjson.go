package sinks

import (
	"bytes"
	"context"
	"io"
	"os"
)

type NDJSON struct {
	Path   string
	Append bool

	file   *os.File
	closed bool
}

func (n *NDJSON) Write(_ context.Context, item Item) error {
	if n.closed {
		return errClosed
	}
	if n.file == nil {
		f, err := n.open()
		if err != nil {
			return err
		}
		n.file = f
	}
	return writeJSONLine(n.file, item)
}

func (n *NDJSON) Close() error {
	if n.closed {
		return nil
	}
	n.closed = true
	if n.file == nil {
		f, err := n.open()
		if err != nil {
			return err
		}
		return f.Close()
	}
	return n.file.Close()
}

func (n *NDJSON) open() (*os.File, error) {
	if !n.Append {
		return os.Create(n.Path) //nolint:gosec // Path is the caller-supplied sink target, not user-controlled input
	}
	f, err := os.OpenFile(n.Path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // Path is the caller-supplied sink target, not user-controlled input
	if err != nil {
		return nil, err
	}
	if err := trimPartialLine(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// trimPartialLine drops a trailing record with no newline — the half-written
// last line of a run that was killed mid-write — so appended records never
// get glued onto it. That record's URL was never fully recorded, so a
// skipSeen resume re-fetches it anyway.
func trimPartialLine(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	buf := make([]byte, 4096)
	for end := size; end > 0; {
		start := max(end-int64(len(buf)), 0)
		chunk := buf[:end-start]
		if _, err := f.ReadAt(chunk, start); err != nil && err != io.EOF {
			return err
		}
		if end == size && chunk[len(chunk)-1] == '\n' {
			return nil
		}
		if i := bytes.LastIndexByte(chunk, '\n'); i >= 0 {
			return f.Truncate(start + int64(i) + 1)
		}
		end = start
	}
	if size == 0 {
		return nil
	}
	return f.Truncate(0)
}
