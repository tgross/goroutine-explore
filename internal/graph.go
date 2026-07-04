// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"fmt"
	"strings"
)

func opFuncGraph(vm *VM, _ OpCode, _ uint) error {
	predicate, err := vm.popDump()
	if err != nil {
		return err
	}
	src, err := vm.popDump()
	if err != nil {
		return err
	}
	out := inGraphOf(src, predicate)
	vm.pushDump(out)
	return nil
}

// inGraphOf finds all parents and descendants of the goroutines in the
// predicate dump. The dump it returns is sorted and indexed.
func inGraphOf(sourceDump *GoroutineDump, predicate *GoroutineDump) *GoroutineDump {
	result := NewGoroutineDump()
	for _, g := range predicate.goroutines {
		if !result.Has(g) {
			result.Add(g)
		}
		graphAncestors(sourceDump, g, result)
		graphDescendants(sourceDump, g, result)
	}
	result.Sort()
	result.Index()
	return result
}

// graphAncestors traces the parent of the goroutine, then its parent,
// etc. until we reach the a goroutine with no parent
func graphAncestors(dump *GoroutineDump, g *Goroutine, result *GoroutineDump) {
	stack := []*Goroutine{g}
	for len(stack) > 0 {
		g = stack[0]
		stack = stack[1:]
		if parent := dump.byID(g.CreatedBy); parent != nil {
			if !result.Has(parent) {
				result.Add(parent)
				stack = append(stack, parent)
			}
		}
	}
}

// graphDescendants traces all children of a goroutine, all their children,
// etc. fanning out until there are no more children.
func graphDescendants(dump *GoroutineDump, parent *Goroutine, result *GoroutineDump) {
	stack := []*Goroutine{parent}
	for len(stack) > 0 {
		parent = stack[0]
		stack = stack[1:]
		for _, g := range dump.goroutines {
			if parent.ID == g.CreatedBy {
				if !result.Has(g) {
					result.Add(g)
					stack = append(stack, g)
				}
			}
		}
	}
}

type edge struct {
	from int
	to   int
}

func opFuncToDot(vm *VM, _ OpCode, _ uint) error {
	path, err := vm.popString()
	if err != nil {
		return err
	}
	dump, err := vm.peekDump()
	if err != nil {
		return err
	}

	w := bytes.NewBuffer([]byte{})

	// header
	_, _ = w.Write([]byte("digraph G {\nrankdir=\"LR\"\n"))
	_, _ = w.Write([]byte("node[shape=record style=filled color=\"lightgreen\"]\n"))

	// a given goroutine's parent may not exist in the dump, so in order to show
	// these with differently styled nodes, we need to track all the nodes we've
	// seen and not
	seenIDs := map[int]struct{}{}
	unknownIDs := map[int]struct{}{}
	edges := []edge{}

	for _, g := range dump.goroutines {
		seenIDs[g.ID] = struct{}{}
		delete(unknownIDs, g.ID)

		trace := g.Trace
		if g.LineCount > 10 {
			trace = strings.Join(strings.Split(g.Trace, "\n")[:10], "\n")
			trace += "\n(truncated)\n"
		}

		trace = strings.ReplaceAll(trace, "{", `\{`)
		trace = strings.ReplaceAll(trace, "}", `\}`)
		// TODO: at least some dotviz implementations don't align the indent
		// correctly, so until we can figure that out put a leading . here
		trace = strings.ReplaceAll(trace, "\t", `.\ \ \ `)
		trace = strings.ReplaceAll(trace, "\n", `\l`)

		fmt.Fprintf(w,
			"%d [label=\"{{<id>%d |%s|%d min}|%s}\"]\n",
			g.ID, g.ID, g.State, g.Duration, trace)

		if g.CreatedBy > 0 {
			if _, ok := seenIDs[g.CreatedBy]; !ok {
				unknownIDs[g.CreatedBy] = struct{}{}
			}
			edges = append(edges, edge{from: g.CreatedBy, to: g.ID})
		}
	}

	// write all the unknown nodes
	fmt.Fprintln(w, "node[shape=oval color=lightgray]")
	for g := range unknownIDs {
		fmt.Fprintf(w, "%d [label=\"%d\"]\n", g, g)
	}

	// write all the edges
	for _, e := range edges {
		_, unknownFrom := unknownIDs[e.from]
		if unknownFrom {
			fmt.Fprintf(w, "%d -> %d:id\n", e.from, e.to)
		} else {
			fmt.Fprintf(w, "%d:id -> %d:id\n", e.from, e.to)
		}
	}

	// trailer
	fmt.Fprint(w, "}")

	vm.didShow = true
	fmt.Fprintln(vm.wOut, w.String())
	if path == "" {
		return nil
	}
	return writeToFile(path, w.Bytes())
}
