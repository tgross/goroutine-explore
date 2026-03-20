// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

// Copyright (c) 2017-2021 linuxerwang and goroutine-inspect contributors
// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/peterh/liner"
	"github.com/tgross/goroutine-explore/internal"
)

func main() {
	var version bool
	var expr string
	flag.BoolVar(&version, "version", false, "display the version")
	flag.StringVar(&expr, "expression", "", "execute an expression")
	flag.Parse()
	if version {
		fmt.Println(Version())
		os.Exit(0)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("could not read working directory: %v\n", err)
		os.Exit(1)
	}

	useColor := os.Getenv("NO_COLOR") != "1" &&
		isatty.IsTerminal(os.Stdout.Fd())

	// TODO: would be nice if we could pass pragmas from args too
	e := internal.NewEvaluator(&internal.Config{
		WorkDir: wd,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Color:   useColor,
	})
	if expr != "" {
		os.Exit(evalOnce(e, expr))
	}

	os.Exit(repl(e))
}

func evalOnce(e *internal.Evaluator, src string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := e.Eval(ctx, src)
	if err != nil {
		switch {
		case errors.Is(err, internal.ErrCommandQuit):
			return 0
		default:
			return 1
		}
	}
	return 0
}

func repl(e *internal.Evaluator) int {
	var err error
	lines, err := createLiner(e)
	if err != nil {
		fmt.Printf("could not read history file: %v\n", err)
	}

	defer func() {
		err := saveLiner(lines)
		if err != nil {
			fmt.Printf("could not save history file: %v\n", err)
		}
		_ = lines.Close()
	}()

	previousCtrlC := false

	for {
		err := replOnce(e, lines)
		if err != nil {
			switch {
			case errors.Is(err, internal.ErrCommandConfirm):
				var confirm internal.ConfirmationAction
				errors.As(err, &confirm)
				confirmPrompt := confirm.Error()
				confirmation, _ := lines.Prompt(confirmPrompt)
				if strings.ToLower(confirmation) == "y" {
					confirm.Run()
				}
				continue
			case errors.Is(err, internal.ErrCommandQuit):
				return 0
			case errors.Is(err, internal.ErrNoSuchOpCode):
				return 129
			case errors.Is(err, liner.ErrPromptAborted):
				if previousCtrlC {
					return 0 // 2 Ctrl-C in a row quits
				}
				previousCtrlC = true
				continue // Ctrl-C
			case errors.Is(err, io.EOF):
				return 0 // Ctrl-D quits
			}
		}
		previousCtrlC = false
	}
}

func replOnce(e *internal.Evaluator, lines *liner.State) error {
	// note: there's a race between here and the next loop iteration where
	// Ctrl-C is unhandled
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	src, err := nextSrc(lines)
	if err != nil {
		return err
	}

	// append to history even if it won't compile so that users can up-arrow to
	// edit their mistake
	lines.AppendHistory(src)

	err = e.Eval(ctx, src)
	if err != nil {
		return err
	}
	return nil
}

// nextSrc grabs the next line off the liner, and handles concatenating
// multi-line input together because peterh/liner doesn't support multiline
// input (except by wrapping)
func nextSrc(lines *liner.State) (string, error) {
	var prompt = ">> "
	var src = ""

	for {
		srcLine, err := lines.Prompt(prompt)
		if err != nil {
			return "", err
		}
		if srcLine == "" {
			return src, nil
		}
		src += srcLine
		src = strings.TrimSpace(src)
		if strings.HasSuffix(src, "\\") || strings.HasSuffix(src, "(") ||
			strings.HasSuffix(src, "|") {
			prompt = ".. "
			src = strings.TrimSuffix(src, "\\")
			src += "\n" // gives us whitespace when we append the line
			continue
		}
		return src, nil
	}
}

func Version() string {
	info, _ := debug.ReadBuildInfo()
	version := info.Main.Version
	out := "goroutine-explore " + version

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			out += " (" + setting.Value + ")"
			break
		}
	}

	out += `

Copyright (c) 2021-2026 The goroutine-explore authors
Licensed Blue Oak Model License 1.0.0

This software contains open source dependencies.
See NOTICES.md in https://github.com/tgross/goroutine-explore
for copyright information.`

	return out
}
