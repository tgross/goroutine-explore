// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

// Copyright (c) 2017-2021 linuxerwang and goroutine-inspect contributors
// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/peterh/liner"
	"github.com/tgross/goroutine-explore/internal"
)

func getConfDir() string {
	userDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatal(err)
	}

	dir := filepath.Join(userDir, "goroutine-explore")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

	return dir
}

func getHistoryFile() string {
	return filepath.Join(getConfDir(), "history")
}

func createLiner(e *internal.Evaluator) (*liner.State, error) {
	lines := liner.NewLiner()
	lines.SetCtrlCAborts(true)
	lines.SetMultiLineMode(true)
	lines.SetTabCompletionStyle(liner.TabPrints)
	lines.SetCompleter(func(line string) []string {
		tokens := strings.Split(line, " ")
		if len(tokens) == 0 {
			return []string{}
		}
		lastToken := tokens[len(tokens)-1]
		prefix := strings.ToLower(lastToken)

		completions := []string{}
		options := e.Completions()
		for _, option := range options {
			if strings.HasPrefix(option, prefix) {
				completion := strings.TrimSuffix(line, lastToken)
				completion += option
				completions = append(completions, completion)
			}
		}

		return completions
	})

	if f, err := os.Open(getHistoryFile()); err == nil {
		defer f.Close()
		_, err := lines.ReadHistory(f)
		if err != nil {
			return nil, err
		}
	}

	return lines, nil
}

func saveLiner(lines *liner.State) error {
	f, err := os.Create(getHistoryFile())
	if err != nil {
		log.Fatal("Error writing history file: ", err)
	}
	defer f.Close()

	_, err = lines.WriteHistory(f)
	return err
}
