// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/scanner"
)

type Config struct {
	WorkDir string
	Stdout  io.Writer
	Stderr  io.Writer
	Color   bool
}

type Evaluator struct {
	compiler  *Compiler
	tokenizer *Tokenizer
	vm        *VM

	stdout *Writer
	stderr *Writer
}

func NewEvaluator(cfg *Config) *Evaluator {
	compiler := NewCompiler()
	tokenizer := NewTokenizer()
	vm := NewVM(cfg)
	wOut, wErr := NewWritersFrom(cfg)

	return &Evaluator{
		compiler:  compiler,
		tokenizer: tokenizer,
		vm:        vm,
		stdout:    wOut,
		stderr:    wErr,
	}
}

var baseCompletions = []string{
	"where", "delete", "as", "save", "load",
	".where", ".delete", ".as", ".load", ".save",
	"show", "diff", "intersect", "union",
	".show", ".diff", ".intersect", ".union",
	"and", "or", "contains",
	"cd", "empty", "exit", "help", "ls", "pwd", "quit", "vars", "pragma",
}

// Completions returns all the known functions, commands, and keywords, plus all
// the tokens in the environment
func (e *Evaluator) Completions() []string {
	completions := slices.Clone(baseCompletions)
	if e.vm != nil {
		for k := range e.vm.env {
			completions = append(completions, k)
		}
	}
	return completions
}

func (e *Evaluator) Eval(ctx context.Context, src string) error {
	var errPos ErrorWithPosition

	body := strings.NewReader(src)
	e.tokenizer.Reset(ctx, body)
	code, err := e.compiler.Compile(e.tokenizer)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			err = nil
		case errors.As(err, &errPos):
			errPos.Print(e.stderr, src)
		default:
			// unexpected / non-compiler errors won't have a position
			fmt.Fprintln(e.stderr.red(), err.Error())
		}
		return err
	}

	// capture the previous environment so we can roll it back
	oldEnv := maps.Clone(e.vm.env)
	e.vm.Reset(code)
	err = e.vm.Run(ctx)
	if err != nil {
		switch {
		case errors.Is(err, ErrCommandQuit),
			errors.Is(err, ErrCommandConfirm):
		case errors.Is(err, ErrCommandOk),
			errors.Is(err, context.Canceled):
			err = nil
		case errors.As(err, &errPos):
			errPos.Print(e.stderr, src)
		default:
			// unexpected errors won't have a position
			fmt.Fprintln(e.stderr.red(), err.Error()+"\n")
		}
		e.vm.env = oldEnv
		return err
	}

	return nil
}

type ErrorWithPosition struct {
	pos   scanner.Position
	inner error
}

func (e ErrorWithPosition) Unwrap() error {
	return e.inner
}

func (e ErrorWithPosition) Error() string {
	return e.inner.Error()
}

func (e ErrorWithPosition) Print(w *Writer, src string) {
	fmt.Fprintln(w.red(), e.inner.Error())

	errLine := e.pos.Line
	errCol := max(1, e.pos.Column-1)

	lines := strings.Split(src, "\n")
	for i, line := range lines {
		fmt.Fprintln(w.yellow(), ""+line)
		if i == errLine-1 {
			fmt.Fprintf(w.red(),
				"%s▲\n",
				strings.Repeat(" ", errCol),
			)
			fmt.Fprintf(w.red(),
				"%s╰%s\n",
				strings.Repeat(" ", errCol),
				strings.Repeat("─", max(0, len(line)-errCol-1)),
			)
		}
	}
}
