// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/shoenig/test/must"
)

func TestGoroutineDump_ShowOffset(t *testing.T) {
	const dummyGoroutineMetaTmpl = `goroutine %d [%s]:`

	dump := NewGoroutineDump()
	for i := 0; i < 20; i++ {
		gr, err := NewGoroutine(fmt.Sprintf(dummyGoroutineMetaTmpl, i, "running"))
		must.NoError(t, err)
		dump.goroutines = append(dump.goroutines, gr)
	}

	// round-trip everything we Show() back thru load()
	getIDs := func(t *testing.T, buf *bytes.Buffer) []int {
		t.Helper()
		got := []int{}
		out, err := loadFrom(buf)
		must.NoError(t, err)
		for _, goroutine := range out.goroutines {
			got = append(got, goroutine.ID)
		}
		return got
	}

	recorder := new(bytes.Buffer)
	w := NewWriter(recorder)

	recorder.Reset()
	dump.Show(w, PragmaDedupNone, 25, 0)
	must.Eq(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	}, getIDs(t, recorder))

	recorder.Reset()
	dump.Show(w, PragmaDedupNone, 5, 10)
	must.Eq(t, []int{10, 11, 12, 13, 14}, getIDs(t, recorder))

	recorder.Reset()
	dump.Show(w, PragmaDedupNone, 20, 10)
	must.Eq(t, []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}, getIDs(t, recorder))
}

func TestGoroutine_Indexing(t *testing.T) {

	gd := NewGoroutineDump()
	gd.Add(testGoroutine(`goroutine 20 [IO wait]:`))

	for i := 1; i < 5; i++ {
		gd.Add(testGoroutineFromStack(fmt.Sprintf(`goroutine %d [runnable]:
net/http.(*connReader).startBackgroundRead.gowrap2()
	/usr/local/go/src/net/http/server.go:677`, i)))
	}

	for i := 10; i > 5; i-- {
		gd.Add(testGoroutineFromStack(fmt.Sprintf(
			`goroutine %d [select, 1 minutes]:
		main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5`, i)))
	}

	gd.Sort()
	gd.Index()
	must.Len(t, 3, gd.index)
	must.Eq(t, 1, gd.index[0].ID)
	must.Eq(t, []int{2, 3, 4}, gd.duplicates[gd.index[0].hash])

	must.Eq(t, 6, gd.index[1].ID)
	must.Eq(t, []int{7, 8, 9, 10}, gd.duplicates[gd.index[1].hash])

	must.Eq(t, 20, gd.index[2].ID)
	must.Eq(t, []int{}, gd.duplicates[gd.index[2].hash])
}
