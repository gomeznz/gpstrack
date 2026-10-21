// Package store writes append-only JSON Lines files, one per UTC day.
// Phase 0 keeps everything in flat files so captures are easy to copy off
// the Pi and inspect; the real database comes in Phase 1.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyJSONL appends records to <dir>/<prefix>-YYYY-MM-DD.jsonl.
type DailyJSONL struct {
	dir, prefix string

	mu   sync.Mutex
	day  string
	file *os.File
}

func NewDailyJSONL(dir, prefix string) (*DailyJSONL, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &DailyJSONL{dir: dir, prefix: prefix}, nil
}

// Write appends v as one JSON line, rotating the file at UTC midnight.
func (d *DailyJSONL) Write(at time.Time, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	day := at.UTC().Format("2006-01-02")
	if d.file == nil || day != d.day {
		if d.file != nil {
			d.file.Close()
		}
		name := filepath.Join(d.dir, fmt.Sprintf("%s-%s.jsonl", d.prefix, day))
		f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			d.file = nil
			return err
		}
		d.file, d.day = f, day
	}
	_, err = d.file.Write(append(line, '\n'))
	return err
}

func (d *DailyJSONL) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.file == nil {
		return nil
	}
	err := d.file.Close()
	d.file = nil
	return err
}
