// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestGoroutineDump_ShowOffset(t *testing.T) {
	t.Parallel()
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

	// use a pattern un-anchored at the tail so we can tolerate duplicates list
	patt := regexp.MustCompile(
		`^` +
			headerStartPatt +
			headerGPPatt +
			headerMPatt +
			headerMPPatt +
			headerStatusPatt +
			headerLabelPatt +
			`:`,
	)

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
			t.Parallel()
			dump := dump.Copy()
			recorder := new(bytes.Buffer)
			w := NewWriter(recorder)
			dump.Show(w, tc.pragma, tc.limit, tc.offset)
			must.Eq(t, tc.expect, getIDs(t, recorder))
		})
	}
}

func TestGoroutine_Indexing(t *testing.T) {
	t.Parallel()
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

func TestGoroutine_ParseLabels(t *testing.T) {
	t.Parallel()
	test.Eq(t, map[string]string{"foo": "bar"},
		parseLabels(`foo: bar`))

	test.Eq(t, map[string]string{"foo": "bar", "baz": "qux"},
		parseLabels(`"foo": "bar", "baz": "qux"`))

	test.Eq(t, map[string]string{"foo": "bar baz", "baz": "qux"},
		parseLabels(` foo : "bar baz", "baz": "qux"`))

	test.Eq(t, map[string]string{"foo baz": "bar", "baz": "qux"},
		parseLabels(` "foo baz" : bar, "baz": "qux"`))

	test.Eq(t, map[string]string{"foo:baz": "bar", "baz": "qux"},
		parseLabels(` "foo:baz" : bar , "baz": "qux"`))
}

func TestGoroutine_LoadFromHeader(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		header string
		expect *Goroutine
	}{
		{
			name:   "all fields",
			header: `goroutine 10 gp=0x3dd90b57a5a0 m=3 mp=0x3dd90b4e9008 [wait, 10 minutes] {"foo": bar}:`,
			expect: &Goroutine{ID: 10, State: "wait", Duration: 10,
				Labels: map[string]string{"foo": "bar"}},
		},
		{
			name:   "quoted labels with spaces",
			header: `goroutine 11 [wait, 10 minutes] {"foo bar": baz}:`,
			expect: &Goroutine{ID: 11, State: "wait", Duration: 10,
				Labels: map[string]string{"foo bar": "baz"}},
		},
		{
			name:   "unquoted labels and no duration",
			header: `goroutine 12 [running] {foobar: baz}:`,
			expect: &Goroutine{ID: 12, State: "running",
				Labels: map[string]string{"foobar": "baz"}},
		},
		{
			name:   "state with spaces and duration",
			header: `goroutine 13 [IO wait, 1 minute]:`,
			expect: &Goroutine{ID: 13, State: "IO wait", Duration: 1},
		},
		{
			name:   "state with parens",
			header: `goroutine 14 [force gc (idle)]:`,
			expect: &Goroutine{ID: 14, State: "force gc (idle)"},
		},
		{
			name:   "state with comma",
			header: `goroutine 15 [select, locked to thread]:`,
			expect: &Goroutine{ID: 15, State: "select, locked to thread"},
		},
		{
			name:   "state with comma and duration",
			header: `goroutine 16 [select, locked to thread, 4 minutes]:`,
			expect: &Goroutine{ID: 16, State: "select, locked to thread", Duration: 4},
		},
		{
			name:   "state with duration",
			header: `goroutine 17 [select, 1 minute]:`,
			expect: &Goroutine{ID: 17, State: "select", Duration: 1},
		},
		{
			name:   "state only",
			header: `goroutine 18 [select]:`,
			expect: &Goroutine{ID: 18, State: "select"},
		},
	}

	for _, tc := range testCases {
		g, err := NewGoroutine(tc.header)
		must.NoError(t, err)
		must.NotNil(t, g)
		must.Eq(t, tc.expect.ID, g.ID, must.Sprint(tc.name))
		must.Eq(t, tc.expect.State, g.State, must.Sprint(tc.name))
		must.Eq(t, tc.expect.Duration, g.Duration, must.Sprint(tc.name))
		must.Eq(t, tc.expect.Labels, g.Labels, must.Sprint(tc.name))
	}
}
