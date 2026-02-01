package evaluator

import (
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestEvaluator is "end-to-end" tests of the evaluator, asserting expected
// output given an environment
func TestEvaluator(t *testing.T) {

	env := map[string]Value{}

	// TODO: this would be nicer if we had testdata files we could read in
	g1 := NewGoroutineDump()
	g1.Add(&Goroutine{
		ID:     1,
		Header: "goroutine 1 [chan receive 5 min]",
		Trace: `goroutine 1 [chan receive 5 min]:
main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`,
		Lines:    3,
		Duration: 5,
		State:    "chan receive",
	})
	g1.Add(&Goroutine{
		ID:     2,
		Header: "goroutine 2 [syscall]",
		Trace: `goroutine 2 [syscall]:
os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
os/signal.loop()
	/usr/local/go/src/os/signal/signal_unix.go:23 +0x13
created by os/signal.Notify.func1.1 in goroutine 1
	/usr/local/go/src/os/signal/signal.go:151 +0x1f
`,
		Lines: 7,
		State: "syscall",
	})
	g1.Add(&Goroutine{
		ID:     3,
		Header: "goroutine 3 [select 1 min]",
		Trace: `main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`,
		Lines:    3,
		Duration: 1,
		State:    "select",
	})

	g2 := NewGoroutineDump()
	g2.Add(&Goroutine{
		ID:     20,
		Header: "goroutine 20 [runnable]",
		Trace: `goroutine 20 [runnable]:
net/http.(*connReader).startBackgroundRead.gowrap2()
	/usr/local/go/src/net/http/server.go:677
runtime.goexit({})
	/usr/local/go/src/runtime/asm_amd64.s:1695 +0x1
created by net/http.(*connReader).startBackgroundRead in goroutine 37
	/usr/local/go/src/net/http/server.go:677 +0xba
`,
		Lines:    7,
		Duration: 0,
		State:    "runnable",
	})

	env["g1"] = Value{Tag: TagDump, Data: g1}
	env["g2"] = Value{Tag: TagDump, Data: g2}

	testCases := []struct {
		name         string
		src          string
		expect       func(*testing.T, *GoroutineDump)
		expectErrMsg string

		notImplemented bool // temporary until
	}{
		{
			name:         "empty src",
			src:          ``,
			expectErrMsg: "EOF",
		},
		{
			name: "simple where string comparison",
			src:  `g1.where(.state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where broken across newline",
			src: `g1.where(
				 .state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where numeric comparison",
			src:  `g1.where(.duration > 0)`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 2, dump.Len())
				must.Eq(t, 1, dump.Next().ID)
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where with binding",
			src:  `g3 = g1.where(.state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "compound where",
			src:  `g1.where(.duration > 0 and .state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name:           "diffed expressions",
			notImplemented: true, // TODO
			src: `l, c, r = diff(
				g1.where(.duration > 0),
				g1.delete(.state == "select"))`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				// TODO: this should accept multiple dumps
				must.Eq(t, 1, dump.Len())
			},
		},
		{
			name:           "unioned expressions",
			notImplemented: true, // TODO
			src: `union(
				g1.where(.duration > 0 and .lines > 0),
				g2.where(.state == "runnable"))`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 3, dump.Len())
				must.Eq(t, 1, dump.Next().ID)
				must.Eq(t, 3, dump.Next().ID)
				must.Eq(t, 20, dump.Next().ID)
			},
		},
		{
			name: "intersecting expressions with binding",
			src: `g3 = intersect(
				g1.where(.duration > 0 and .lines > 0),
				g1.where(.state == "chan receive"))`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 1, dump.Next().ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.notImplemented {
				t.Skip("TODO not yet implemented")
			}
			c := newCompiler()
			got, err := Evaluate(c, tc.src, env, t.TempDir())
			if tc.expectErrMsg != "" {
				test.Eq(t, NoValue, got)
				test.EqError(t, err, tc.expectErrMsg)
			} else {
				must.NoError(t, err)
				must.NotNil(t, got)
				dump, ok := got.Data.(*GoroutineDump)
				must.True(t, ok, must.Sprintf("did not return dump: %v", dump))
				tc.expect(t, dump)
			}
		})
	}
}

func BenchmarkEvaluator(b *testing.B) {

	env := map[string]Value{}

	// TODO: this would be nicer if we had testdata files we could read in
	g1 := NewGoroutineDump()
	g1.Add(&Goroutine{
		ID:     1,
		Header: "goroutine 1 [chan receive 5 min]",
		Trace: `goroutine 1 [chan receive 5 min]:
main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`,
		Lines:    3,
		Duration: 5,
		State:    "chan receive",
	})
	g1.Add(&Goroutine{
		ID:     2,
		Header: "goroutine 2 [syscall]",
		Trace: `goroutine 2 [syscall]:
os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
os/signal.loop()
	/usr/local/go/src/os/signal/signal_unix.go:23 +0x13
created by os/signal.Notify.func1.1 in goroutine 1
	/usr/local/go/src/os/signal/signal.go:151 +0x1f
`,
		Lines: 7,
		State: "syscall",
	})
	g1.Add(&Goroutine{
		ID:     3,
		Header: "goroutine 3 [select 1 min]",
		Trace: `main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`,
		Lines:    3,
		Duration: 1,
		State:    "select",
	})

	env["g1"] = Value{Tag: TagDump, Data: g1}

	src := `g1 where .duration > 0 and .state == "select"`

	length := 0
	c := newCompiler()
	cwd := b.TempDir()
	for b.Loop() {
		got, err := Evaluate(c, src, env, cwd)
		must.NoError(b, err)
		dump, ok := got.Data.(*GoroutineDump)
		must.True(b, ok, must.Sprintf("did not return dump: %v", dump))
		length += dump.Len()
	}
}
