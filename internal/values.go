// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

type Tag uint

//go:generate stringer -type Tag
const (
	TagNone Tag = iota
	TagBool
	TagNumber
	TagString
	TagIdentifier
	TagFieldAccessor
	TagCommand
	TagDump
	TagGoroutine
	TagAddress
	TagDiff
)

type Value struct {
	Tag  Tag
	Data any
}

var NoValue = Value{Tag: TagNone}
