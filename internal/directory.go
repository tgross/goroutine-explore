// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

// Copyright (c) 2017-2021 linuxerwang and goroutine-inspect contributors
// SPDX-License-Identifier: BSD-2-Clause

package internal

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
)

func opCommandListDir(vm *VM, _ OpCode, _ uint) error {
	w := vm.wOut

	if vm.pragma.ListFormat != "" {
		cmd := exec.Command("ls", vm.pragma.ListFormat)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(out))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	f, err := os.Open(wd)
	if err != nil {
		return err
	}
	defer f.Close()

	fis, err := f.Readdir(-1)
	if err != nil {
		return err
	}

	sort.Slice(fis, func(i, j int) bool {
		return fis[i].Name() < fis[j].Name()
	})

	for _, fi := range fis {
		if fi.IsDir() {
			fmt.Fprintln(w.blue(), fi.Name())
		} else {
			fmt.Fprintln(w, fi.Name())
		}
	}
	return nil
}
