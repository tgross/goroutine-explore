// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

// Pragma represents all the VM's configuration values.
type Pragma struct {
	EmptyConfirm bool
	ExitConfirm  bool
	ListFormat   string
	ShowColor    bool
	ShowCount    int
	ShowDedup    string
	VarsDisplay  string
	Gas          int
	StackSize    int
}

func NewPragma() *Pragma {
	return &Pragma{
		EmptyConfirm: true,
		ExitConfirm:  true,
		ListFormat:   "",
		ShowColor:    true,
		ShowCount:    0,
		ShowDedup:    PragmaDedupIDs,
		VarsDisplay:  PragmaDisplayCount,
		Gas:          defaultGas,
		StackSize:    defaultStackLimit,
	}
}

const defaultStackLimit = 1024
const defaultGas = 1024 * 1024 * 1024

type PragmaDisplay string

const (
	PragmaDisplayCount   = "count"
	PragmaDisplaySummary = "summary"
	PragmaDisplayNone    = "none"
)

type PragmaDedup string

const (
	PragmaDedupIDs    = "ids"
	PragmaDedupNumber = "number"
	PragmaDedupNone   = "none"
)
