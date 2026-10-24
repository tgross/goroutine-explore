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

func opCommandGetWorkingDir(vm *VM, _ OpCode, _ uint) error {
	fmt.Fprint(vm.wOut, vm.cwd+"\n")
	return nil
}

func opCommandChangeDir(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	path, ok := con.(string)
	if !ok {
		return fmt.Errorf("cd requires a string argument")
	}
	if path == "-" {
		path = vm.pcwd
	}
	path = os.ExpandEnv(path)

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	err = os.Chdir(path)
	if err != nil {
		return fmt.Errorf("could not change working directory: %w", err)
	}
	vm.pcwd = vm.cwd
	vm.cwd = path
	return ErrCommandOk
}
