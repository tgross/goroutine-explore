// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"io"
	"os"
)

const (
	fgRed    = "\x1b[38;05;1m"
	fgGreen  = "\x1b[38;05;2m"
	fgYellow = "\x1b[38;05;3m"
	fgBlue   = "\x1b[38;05;4m"
	reset    = "\x1b[0m"
)

// Writer wraps a io.Writer and lets us add color to output
type Writer struct {
	inner    io.Writer
	useColor bool
	color    string
}

// NewWritersFrom returns a "stdout" and a "stderr" Writer based on the
// configuration
func NewWritersFrom(cfg *Config) (*Writer, *Writer) {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	wOut := &Writer{
		inner:    cfg.Stdout,
		useColor: cfg.Color,
	}
	wErr := &Writer{
		inner:    cfg.Stderr,
		useColor: cfg.Color,
	}
	return wOut, wErr
}

// NewWriter is the default constructor without color (typically for tests)
func NewWriter(inner io.Writer) *Writer {
	w := &Writer{
		inner: inner,
	}
	return w
}

// blue returns a shallow copy of the Writer with the color set
func (w *Writer) blue() *Writer {
	return &Writer{
		inner:    w.inner,
		useColor: w.useColor,
		color:    fgBlue,
	}
}

// red returns a shallow copy of the Writer with the color set
func (w *Writer) red() *Writer {
	return &Writer{
		inner:    w.inner,
		useColor: w.useColor,
		color:    fgRed,
	}
}

// yellow returns a shallow copy of the Writer with the color set
func (w *Writer) yellow() *Writer {
	return &Writer{
		inner:    w.inner,
		useColor: w.useColor,
		color:    fgYellow,
	}
}

// green returns a shallow copy of the Writer with the color set
func (w *Writer) green() *Writer {
	return &Writer{
		inner:    w.inner,
		useColor: w.useColor,
		color:    fgGreen,
	}
}

// Write implements io.Writer. It tries to always write the entire contents to
// the inner io.Writer before returning. If configured for color, it'll emit an
// ANSI color code byte before writing and then reset when it'd done.
func (w *Writer) Write(p []byte) (int, error) {
	if w.useColor && w.color != "" {
		defer w.writeImpl([]byte(reset)) //nolint:errcheck
		w.writeImpl([]byte(w.color))     //nolint:errcheck
	}
	return w.writeImpl(p)
}

func (w *Writer) writeImpl(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		nn, err := w.inner.Write(p[n:])
		if err != nil {
			return nn, err
		}
		n += nn
	}
	return n, nil
}

// writeToFile writes the entire buffer provided to the file path, taking care
// of short reads.
func writeToFile(path string, buf []byte) error {
	path = expandPath(path)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fw := NewWriter(f)
	_, err = fw.Write(buf)
	if err != nil {
		return err
	}
	_, err = fw.Write([]byte("\n"))
	if err != nil {
		return err
	}
	return f.Sync()
}
