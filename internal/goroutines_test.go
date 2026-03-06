// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/shoenig/test/must"
)

func TestGoroutineDump_ShowOffset(t *testing.T) {
	dump := NewGoroutineDump()

	stack1 := mockStack(4)
	stack2 := mockStack(5)
	stack3 := mockStack(2)

	for i := 1; i <= 5; i++ {
		dump.Add(mockGoroutine(i, "runnable", stack1))
	}
	for i := 6; i <= 15; i++ {
		dump.Add(mockGoroutine(i, "select, 5 minutes", stack2))
	}
	for i := 16; i <= 20; i++ {
		dump.Add(mockGoroutine(i, "IO wait", stack3))
	}

	// use an un-anchored pattern so we can tolerate duplicates list
	patt := regexp.MustCompile(`^goroutine\s+(\d+)\s+.*\[(.*)\]:`)

	// round-trip everything we Show() back thru load()
	getIDs := func(t *testing.T, buf *bytes.Buffer) []int {
		t.Helper()
		got := []int{}
		out, err := loadFrom(buf, patt)
		must.NoError(t, err)
		for _, goroutine := range out.goroutines {
			got = append(got, goroutine.ID)
		}
		return got
	}

	recorder := new(bytes.Buffer)
	w := NewWriter(recorder)

	testCases := []struct {
		name   string
		pragma PragmaDedup
		limit  int
		offset int
		expect []int
	}{
		{
			name:   "no dedup limit more than total",
			pragma: PragmaDedupNone,
			limit:  25,
			offset: 0,
			expect: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
				11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		},
		{
			name:   "no dedup offset pushes limit past end",
			pragma: PragmaDedupNone,
			limit:  25,
			offset: 10,
			expect: []int{11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		},
		{
			name:   "no dedup offset more than total",
			pragma: PragmaDedupNone,
			limit:  5,
			offset: 25,
			expect: []int{},
		},
		{
			name:   "with dedup limit more than total",
			pragma: PragmaDedupIDs,
			limit:  25,
			offset: 0,
			expect: []int{1, 6, 16},
		},
		{
			name:   "with dedup offset pushes limit past end",
			pragma: PragmaDedupIDs,
			limit:  5,
			offset: 1,
			expect: []int{6, 16},
		},
		{
			name:   "with dedup offset more than total",
			pragma: PragmaDedupIDs,
			limit:  5,
			offset: 25,
			expect: []int{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder.Reset()
			dump.Show(w, tc.pragma, tc.limit, tc.offset)
			must.Eq(t, tc.expect, getIDs(t, recorder))
		})
	}
}

func TestGoroutine_Indexing(t *testing.T) {

	gd := NewGoroutineDump()
	gd.Add(mockGoroutine(20, "IO wait"))

	stack1 := mockStack(4)
	for i := 1; i < 5; i++ {
		gd.Add(mockGoroutine(i, "runnable", stack1))
	}

	stack2 := mockStack(5)
	for i := 10; i > 5; i-- {
		gd.Add(mockGoroutine(i, "runnable", stack2))
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
