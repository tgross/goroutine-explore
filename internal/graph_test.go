// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"testing"

	"github.com/shoenig/test/must"
)

func Test_inGraphOf(t *testing.T) {
	gd := mockDumpForGraph()
	predicate := NewGoroutineDump()
	predicate.Add(gd.byID(6))
	predicate.Add(gd.byID(12))

	out := inGraphOf(gd, predicate)
	must.Eq(t, "[1 2 4 6 10 12 14]", out.String())
}

func Test_FuncToDot(t *testing.T) {
	gd := mockDumpForGraph()
	predicate := NewGoroutineDump()
	predicate.Add(gd.byID(6))
	predicate.Add(gd.byID(12))

	vm := NewVM(&Config{WorkDir: t.TempDir()})
	recorder := new(bytes.Buffer)
	vm.wOut = NewWriter(recorder)
	vm.pushDump(gd)
	must.NoError(t, opFuncToDot(vm, OpCodeNoop, 0))

	expect := `digraph G {
rankdir="LR"
node[shape=record style=filled color="lightgreen"]
1 [label="{{<id>1 |running|0 min}|}"]
2 [label="{{<id>2 |running|0 min}|}"]
3 [label="{{<id>3 |running|0 min}|}"]
4 [label="{{<id>4 |running|0 min}|}"]
5 [label="{{<id>5 |running|0 min}|}"]
6 [label="{{<id>6 |running|0 min}|}"]
7 [label="{{<id>7 |running|0 min}|}"]
8 [label="{{<id>8 |running|0 min}|}"]
9 [label="{{<id>9 |running|0 min}|}"]
10 [label="{{<id>10 |running|0 min}|}"]
11 [label="{{<id>11 |running|0 min}|}"]
12 [label="{{<id>12 |running|0 min}|}"]
13 [label="{{<id>13 |running|0 min}|}"]
14 [label="{{<id>14 |running|0 min}|}"]
15 [label="{{<id>15 |running|0 min}|}"]
16 [label="{{<id>16 |running|0 min}|}"]
17 [label="{{<id>17 |running|0 min}|}"]
18 [label="{{<id>18 |running|0 min}|}"]
19 [label="{{<id>19 |running|0 min}|}"]
20 [label="{{<id>20 |running|0 min}|}"]
21 [label="{{<id>21 |running|0 min}|}"]
22 [label="{{<id>22 |running|0 min}|}"]
23 [label="{{<id>23 |running|0 min}|}"]
24 [label="{{<id>24 |running|0 min}|}"]
25 [label="{{<id>25 |running|0 min}|}"]
26 [label="{{<id>26 |running|0 min}|}"]
27 [label="{{<id>27 |running|0 min}|}"]
28 [label="{{<id>28 |running|0 min}|}"]
29 [label="{{<id>29 |running|0 min}|}"]
node[shape=oval color=lightgray]
1:id -> 2:id
1:id -> 3:id
2:id -> 4:id
3:id -> 5:id
4:id -> 6:id
5:id -> 8:id
6:id -> 10:id
10:id -> 12:id
8:id -> 13:id
12:id -> 14:id
7:id -> 15:id
13:id -> 21:id
15:id -> 27:id
}
`
	must.Eq(t, expect, recorder.String())
}
