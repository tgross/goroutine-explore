// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

type Tag uint

//go:generate stringer -type Tag -linecomment
const (
	TagNone          Tag = iota
	TagBool              // bool
	TagNumber            // number
	TagString            // string
	TagIdentifier        // identifier
	TagFieldAccessor     // field accessor
	TagCommand           //command
	TagDump              // dump
	TagGoroutine         // goroutine
	TagAddress           // address
	TagDiff              // diff
)

type Value struct {
	Tag  Tag
	Data any
}

var NoValue = Value{Tag: TagNone}
