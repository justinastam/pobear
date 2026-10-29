package sinks

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// csvEnvelopeColumns lead every row, ahead of the data columns.
var csvEnvelopeColumns = []string{"url", "status", "tier"}

// CSV streams one row per item to Path as items complete. The column set is
// fixed up front from DataKeys (the spec's stage output keys, in stage order)
// because a streamed file can't go back and add a column; a data key not in
// DataKeys is dropped. With no DataKeys, a single body column stands in.
type CSV struct {
	Path     string
	DataKeys []string

	file   *os.File
	w      *csv.Writer
	closed bool
}

func (c *CSV) Write(_ context.Context, item Item) error {
	if c.closed {
		return errClosed
	}
	if err := c.open(); err != nil {
		return err
	}
	row := []string{item.URL, strconv.Itoa(item.Status), item.Tier}
	if len(c.DataKeys) == 0 {
		row = append(row, item.Body)
	}
	for _, key := range c.DataKeys {
		cell, err := csvCell(item.Data[key])
		if err != nil {
			return err
		}
		row = append(row, cell)
	}
	return c.writeRow(row)
}

func (c *CSV) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if err := c.open(); err != nil {
		return err
	}
	return c.file.Close()
}

func (c *CSV) open() error {
	if c.file != nil {
		return nil
	}
	f, err := os.Create(c.Path) //nolint:gosec // Path is the caller-supplied sink target, not user-controlled input
	if err != nil {
		return err
	}
	c.file = f
	c.w = csv.NewWriter(f)
	if err := c.writeRow(c.header()); err != nil {
		_ = f.Close()
		return err
	}
	return nil
}

func (c *CSV) header() []string {
	header := append([]string{}, csvEnvelopeColumns...)
	if len(c.DataKeys) == 0 {
		return append(header, "body")
	}
	return append(header, c.DataKeys...)
}

func (c *CSV) writeRow(row []string) error {
	if err := c.w.Write(row); err != nil {
		return err
	}
	c.w.Flush()
	return c.w.Error()
}

// csvCell flattens one data value into a cell: a list (a css field's
// matches) becomes its values joined with a newline, so a single match reads
// as a plain value and no match as an empty cell; anything that isn't a
// string or a list of strings is JSON-encoded.
func csvCell(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "", nil
	case string:
		return val, nil
	case []string:
		return strings.Join(val, "\n"), nil
	case []any:
		parts := make([]string, len(val))
		for i, el := range val {
			s, err := csvCell(el)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, "\n"), nil
	case bool, int, int64, float64:
		return fmt.Sprint(val), nil
	default:
		data, err := marshalJSONNoEscape(val)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}
