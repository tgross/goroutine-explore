// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash"
	"maps"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

/*
The types in this file are placeholders until the evaluator gets wired up into
the main application
*/

type Diff struct {
	Left   *GoroutineDump
	Right  *GoroutineDump
	Common *GoroutineDump
}

type GoroutineDump struct {
	goroutines []*Goroutine
	iterIndex  int
}

func NewGoroutineDump() *GoroutineDump {
	gd := &GoroutineDump{
		goroutines: []*Goroutine{},
	}
	return gd
}

func (gd *GoroutineDump) Add(g *Goroutine) {
	gd.goroutines = append(gd.goroutines, g)
}

func (gd *GoroutineDump) Next() *Goroutine {
	if gd.iterIndex >= len(gd.goroutines) {
		return nil
	}
	g := gd.goroutines[gd.iterIndex]
	gd.iterIndex++
	return g
}

// Copy duplicates the goroutine dump with a shallow copy of the goroutines and
// a reset iterator
func (gd *GoroutineDump) Copy() *GoroutineDump {
	return &GoroutineDump{
		goroutines: slices.Clone(gd.goroutines),
	}
}

func (gd *GoroutineDump) Has(p *Goroutine) bool {
	for _, g := range gd.goroutines {
		if g.ID == p.ID {
			return true // TODO: want an equality hash here
		}
	}
	return false
}

func (gd *GoroutineDump) StartIter() {
	gd.iterIndex = 0
}

func (gd *GoroutineDump) Len() int {
	if gd == nil {
		return 0
	}
	return len(gd.goroutines)
}

// String is a placeholder until we wire this all up to the REPL writer
func (gd *GoroutineDump) String() string {
	ids := make([]int, 0, len(gd.goroutines))
	for _, g := range gd.goroutines {
		ids = append(ids, g.ID)
	}
	return fmt.Sprintf("%v", ids)
}

// Show displays the goroutines with the given limit and offset
func (gd *GoroutineDump) Show(w *Writer, limit, offset int) {
	if limit == 0 {
		limit = gd.Len() - offset
	} else {
		limit = min(limit, gd.Len()-offset)
	}
	for i := offset; i < offset+limit; i++ {
		gd.goroutines[i].Print(w)
	}
}

func (gd *GoroutineDump) Summary(w *Writer, name string) {
	if name != "" {
		fmt.Fprintf(w, "# of goroutines in %q: %d\n", name, len(gd.goroutines)) //nolint:errcheck
	} else {
		fmt.Fprintf(w, "# of goroutines: %d\n", len(gd.goroutines)) //nolint:errcheck
	}

	stats := map[string]int{}
	for _, g := range gd.goroutines {
		stats[g.State]++
	}
	if len(stats) > 0 {
		states := slices.Collect(maps.Keys(stats))
		sort.Strings(states)
		for _, k := range states {
			fmt.Fprintf(w, "%15s: %d\n", k, stats[k]) //nolint:errcheck
		}
	}
	fmt.Fprintln(w, "") //nolint:errcheck
}

// Save saves the goroutine dump to the given file.
func (gd GoroutineDump) Save(fn string) error {
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	w := NewWriter(f)
	for _, g := range gd.goroutines {
		g.Print(w)
	}

	return nil
}

type Goroutine struct {
	ID        int
	Header    string
	Trace     string
	LineCount int
	Duration  int    // In minutes, from meta in header
	State     string // From meta in header

	lineMd5    []string
	fullMd5    string
	fullHasher hash.Hash
	Duplicates []int

	isFrozen bool
	buf      *bytes.Buffer
}

var durationPattern = regexp.MustCompile(`^\d+ minutes$`)

// NewGoroutine creates and returns a new Goroutine.
func NewGoroutine(header string) (*Goroutine, error) {
	idx := strings.Index(header, "[")
	parts := strings.Split(header[idx+1:len(header)-2], ",")
	state := strings.TrimSpace(parts[0])

	duration := 0
	if len(parts) > 1 {
		value := strings.TrimSpace(parts[1])
		if durationPattern.MatchString(value) {
			if d, err := strconv.Atoi(value[:len(value)-8]); err == nil {
				duration = d
			}
		}
	}

	// TODO: this throws out the "gp=", "m=", and "mp=" fields we see on a
	// SIGQUIT. We should have searchable fields for these as well.
	idxParts := strings.Split(strings.TrimSpace(header[9:idx]), " ")
	idstr := strings.TrimSpace(idxParts[0])
	id, err := strconv.Atoi(idstr)
	if err != nil {
		return nil, err
	}

	return &Goroutine{
		ID:         id,
		LineCount:  1,
		Header:     header,
		buf:        &bytes.Buffer{},
		Duration:   duration,
		State:      state,
		fullHasher: md5.New(),
		Duplicates: []int{},
	}, nil
}

func (g *Goroutine) Debug() string {
	if g == nil {
		return "<nil>"
	}
	return g.Header // TODO
}

// AddLine appends a line to the goroutine info.
func (g *Goroutine) AddLine(l string) {
	if !g.isFrozen {
		g.LineCount++
		g.buf.WriteString(l)
		g.buf.WriteString("\n")

		if strings.HasPrefix(l, "\t") || strings.HasPrefix(l, " ") {

			// sigquit dumps include fp, sp, and pc for each line, so we only
			// want the line itself here
			l = strings.TrimSpace(l)
			parts := strings.Split(l, " ")
			fl := parts[0]

			h := md5.New()
			fmt.Fprint(h, fl) //nolint:errcheck
			g.lineMd5 = append(g.lineMd5, string(h.Sum(nil)))

			fmt.Fprint(g.fullHasher, fl) //nolint:errcheck
		}
	}
}

// Freeze freezes the goroutine info.
func (g *Goroutine) Freeze() {
	if !g.isFrozen {
		g.isFrozen = true
		g.Trace = g.buf.String()
		g.buf = nil

		g.fullMd5 = string(g.fullHasher.Sum(nil))
	}
}

// PrintWithColor outputs the goroutine details to stdout with color.
//
//nolint:errcheck
func (g Goroutine) Print(w *Writer) {
	fmt.Fprint(w.blue(), g.Header)
	if len(g.Duplicates) > 0 {
		fmt.Fprintf(w.red(), "%d times [", len(g.Duplicates))
		for i, id := range g.Duplicates {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w.green(), "%d", id)
		}
		fmt.Fprint(w.red(), "]")
	}
	fmt.Fprintf(w, "\n%s\n", g.Trace)
}
