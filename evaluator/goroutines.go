package evaluator

/*
The types in this file are placeholders until the evaluator gets wired up into
the main application
*/

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

func (gd *GoroutineDump) StartIter() {
	gd.iterIndex = 0
}

func (gd *GoroutineDump) Len() int {
	return len(gd.goroutines)
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
