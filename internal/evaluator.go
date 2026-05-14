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
	body := strings.NewReader(src)
	e.tokenizer.Reset(ctx, body)
	code, err := e.compiler.Compile(e.tokenizer)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		var cerr CompilerError
		if errors.As(err, &cerr) {
			fmt.Fprintln(e.stderr.red(), err.Error())

			errLine := cerr.tok.Pos.Line
			errCol := max(1, cerr.tok.Pos.Column-1)

			lines := strings.Split(src, "\n")
			for i, line := range lines {
				fmt.Fprintln(e.stderr.yellow(), ""+line)
				if i == errLine-1 {
					fmt.Fprintf(e.stderr.red(),
						"%s▲\n",
						strings.Repeat(" ", errCol),
					)
					fmt.Fprintf(e.stderr.red(),
						"%s╰%s\n",
						strings.Repeat(" ", errCol),
						strings.Repeat("─", max(0, len(line)-errCol-1)),
					)

				}
			}
			return err
		}

		// if it's not a compiler error we don't have a token position to print
		fmt.Fprintln(e.stderr.red(), err.Error())
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
		default:
			// TODO: we want this to include rich diagnostic feedback
			fmt.Fprintln(e.stderr.red(), err.Error()+"\n")
		}
		e.vm.env = oldEnv
		return err
	}

	return nil
}
