// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
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

type Diff struct {
	Left   *GoroutineDump
	Right  *GoroutineDump
	Common *GoroutineDump
}

type GoroutineDump struct {
	goroutines []*Goroutine

	// index is a duplicated sorted list of the goroutines in the dump
	index []*Goroutine

	// duplicates is a map of hash to duplicate IDs for that same hash,
	// excluding the goroutine that's in the index
	duplicates map[string][]int

	// isIndexed is a flag that we set to avoid reindexing a dump
	// repeatedly. Dump contents should be immutable once returned from the VM
	isIndexed bool

	// iterIndex is the index to the goroutines field for iterating through the
	// dump
	iterIndex int
}

func NewGoroutineDump() *GoroutineDump {
	gd := &GoroutineDump{
		goroutines: []*Goroutine{},
		index:      []*Goroutine{},
		duplicates: map[string][]int{},
	}
	return gd
}

func (gd *GoroutineDump) Add(g *Goroutine) {
	gd.goroutines = append(gd.goroutines, g)
	if gd.isIndexed {
		panic("indexed goroutine dumps should never be mutated")
	}
}

// Sort sorts the goroutines by ID. Generally speaking we only need this once
// we've unioned two dumps
func (gd *GoroutineDump) Sort() {
	sort.Slice(gd.goroutines, func(i, j int) bool {
		return gd.goroutines[i].ID < gd.goroutines[j].ID
	})
}

// Index creates an lookup table for duplicates. It assumes the dump is already
// sorted.
func (gd *GoroutineDump) Index() {
	if gd.isIndexed {
		return
	}
	gd.isIndexed = true
	gd.index = []*Goroutine{}
	gd.duplicates = map[string][]int{}

	for _, g := range gd.goroutines {
		if dupe, ok := gd.duplicates[g.hash]; ok {
			dupe = append(dupe, g.ID)
			gd.duplicates[g.hash] = dupe
		} else {
			gd.duplicates[g.hash] = []int{}
			gd.index = append(gd.index, g)
		}
	}
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
		index:      []*Goroutine{},
		duplicates: map[string][]int{},
	}
}

func (gd *GoroutineDump) Has(p *Goroutine) bool {
	for _, g := range gd.goroutines {
		if g.ID == p.ID && g.hash == p.hash {
			return true
		}
	}
	return false
}

func (gd *GoroutineDump) StartIter() {
	// note: we could implement a iter.Seq here but because we pull values we
	// end up having to keep pointers to the (next, stop) functions in object
	// state anyways, so until we need it for something else we'll use this
	// simpler approach
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
func (gd *GoroutineDump) Show(w *Writer, pragma PragmaDedup, limit, offset int) {
	if pragma == PragmaDedupNone {
		limit := safeLimit(gd.Len(), limit, offset)
		for i := offset; i < offset+limit; i++ {
			gd.goroutines[i].Print(w, nil, PragmaDedupNone)
		}
		return
	}

	if !gd.isIndexed {
		gd.Index()
	}
	limit = safeLimit(len(gd.index), limit, offset)
	for i := offset; i < offset+limit; i++ {
		g := gd.index[i]
		g.Print(w, gd.duplicates[g.hash], pragma)
	}
}

func safeLimit(size, limit, offset int) int {
	if limit == 0 {
		return size - offset
	}
	return min(limit, size-offset)
}

func (gd *GoroutineDump) Summary(w *Writer, name string) {
	if name != "" {
		fmt.Fprintf(w, "# of goroutines in %q: %d\n", name, len(gd.goroutines))
	} else {
		fmt.Fprintf(w, "# of goroutines: %d\n", len(gd.goroutines))
	}

	stats := map[string]int{}
	for _, g := range gd.goroutines {
		stats[g.State]++
	}
	if len(stats) > 0 {
		states := slices.Collect(maps.Keys(stats))
		sort.Strings(states)
		for _, k := range states {
			fmt.Fprintf(w, "%15s: %d\n", k, stats[k])
		}
	}
	fmt.Fprintln(w, "")
}

// Save saves the goroutine dump to the given file.
func (gd GoroutineDump) Save(fn string) error {
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close()

	w := NewWriter(f)
	for _, g := range gd.goroutines {
		g.Print(w, nil, PragmaDedupNone)
	}
	return f.Sync()
}

type Goroutine struct {
	ID        int    `json:"id"`
	Duration  int    `json:"duration"` // In minutes, from meta in header
	State     string `json:"state"`    // From meta in header
	CreatedBy int    `json:"createdBy"`
	Trace     string `json:"trace"`
	LineCount int    `json:"lines"`

	header     string
	duplicates []int // duplicate IDs

	buf      *bytes.Buffer // a copy of the original text from the dump
	isFrozen bool          // once a goroutine is frozen, we never add to it again
	hash     string        // set from location specs of all lines in the stack
	hasher   hash.Hash
}

var durationPattern = regexp.MustCompile(`^\d+ minutes$`)
var createdByPattern = regexp.MustCompile(`^created by .* in goroutine (\d+)`)

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
		header:     header,
		buf:        &bytes.Buffer{},
		Duration:   duration,
		State:      state,
		hasher:     sha256.New(),
		duplicates: []int{},
	}, nil
}

func (g *Goroutine) Debug() string {
	if g == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s (%s)", g.header, g.hash[:20])
}

// AddLine appends a line to the goroutine info.
func (g *Goroutine) AddLine(l string) {
	if !g.isFrozen {
		g.LineCount++
		g.buf.WriteString(l)
		g.buf.WriteString("\n")

		createdByMatches := createdByPattern.FindStringSubmatch(l)
		if len(createdByMatches) == 2 {
			createdBy, _ := strconv.ParseInt(createdByMatches[1], 10, 64)
			g.CreatedBy = int(createdBy)
		} else if strings.HasPrefix(l, "\t") || strings.HasPrefix(l, " ") {
			// sigquit dumps include fp, sp, and pc for each line, so we only
			// want the location spec to add to the hash
			l = strings.TrimSpace(l)
			parts := strings.Split(l, " ")
			fl := parts[0]
			fmt.Fprint(g.hasher, fl)
		}
	}
}

// Freeze freezes the goroutine info.
func (g *Goroutine) Freeze() {
	if !g.isFrozen {
		g.isFrozen = true
		g.Trace = g.buf.String()
		g.buf = nil
		g.hash = base64.StdEncoding.EncodeToString(
			[]byte(g.hasher.Sum(nil)))
	}
}

// Print outputs the goroutine details to stdout with color.
func (g *Goroutine) Print(w *Writer, duplicateIDs []int, pragma PragmaDedup) {
	fmt.Fprint(w.blue(), g.header)
	switch pragma {
	case PragmaDedupNone:
	case PragmaDedupIDs:
		if len(duplicateIDs) > 0 {
			fmt.Fprintf(w.green(), " %d times [%d, ", len(duplicateIDs)+1, g.ID)

			for i, id := range duplicateIDs {
				if i > 0 {
					fmt.Fprint(w, ", ")
				}
				fmt.Fprintf(w.green(), "%d", id)
			}
			fmt.Fprint(w.green(), "]")
		}
	case PragmaDedupNumber:
		if len(duplicateIDs) > 0 {
			fmt.Fprintf(w.green(), " %d times", len(duplicateIDs)+1)
		}
	}
	fmt.Fprintf(w, "\n%s\n", g.Trace)
}
