package evaluator

import (
	"fmt"
	"slices"
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

func (gd *GoroutineDump) String() string {
	ids := make([]int, 0, len(gd.goroutines))
	for _, g := range gd.goroutines {
		ids = append(ids, g.ID)
	}
	return fmt.Sprintf("%v", ids)
}

type Goroutine struct {
	ID       int
	Header   string
	Trace    string
	Lines    int
	Duration int    // In minutes.
	State    string // from Meta
}

func (g *Goroutine) Debug() string {
	if g == nil {
		return "<nil>"
	}
	return g.Header // TODO
}
