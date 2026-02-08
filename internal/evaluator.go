// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"strings"
)

func Evaluate(compiler *Compiler, src string, env map[string]Value, cwd string) (Value, error) {
	body := strings.NewReader(src)
	tokenizer := NewTokenizer(body)
	chunk, err := compiler.Compile(tokenizer)
	if err != nil {
		return NoValue, err
	}

	cfg := &vmConfig{cwd: cwd}

	vm, err := NewVM(cfg)
	if err != nil {
		return NoValue, err
	}
	vm.env = env
	vm.reset(chunk)
	val, err := vm.run()
	// vm.debug()

	return val, err
}
