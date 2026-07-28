// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/shoenig/test/must"
)

type testShortWriter struct {
	w *bytes.Buffer
}

// Write makes writes of at most 8 bytes to exercise handling of short writes
func (w *testShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.w.Write(p[0:min(8, len(p))])
}

func TestWriter(t *testing.T) {
	t.Parallel()
	recorder := &testShortWriter{w: new(bytes.Buffer)}
	w := NewWriter(recorder)
	_, err := fmt.Fprintf(w, "0123456789")
	must.NoError(t, err)
	must.Eq(t, "0123456789", recorder.w.String())

	// blue with color disabled
	recorder.w.Reset()
	_, err = fmt.Fprintf(w.blue(), "0123456789")
	must.NoError(t, err)
	must.Eq(t, "0123456789", recorder.w.String())

	// blue with color enabled
	recorder.w.Reset()
	w.useColor = true
	_, err = fmt.Fprintf(w.blue(), "0123456789")
	must.NoError(t, err)
	must.Eq(t, fgBlue+"0123456789"+reset, recorder.w.String())

	// ensure we don't mutate color handlikng of the original
	recorder.w.Reset()
	_, err = fmt.Fprintf(w, "0123456789")
	must.NoError(t, err)
	must.Eq(t, "0123456789", recorder.w.String())
}
