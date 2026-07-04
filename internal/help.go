// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import "fmt"

func opCommandHelp(vm *VM, _ OpCode, index uint) error {
	var topic string
	con, err := vm.fetchConstant(index)
	if err != nil {
		return fmt.Errorf("%w: expected help topic or empty", ErrInvalidArg)
	} else {
		var ok bool
		topic, ok = con.(string)
		if !ok {
			return fmt.Errorf("help topics must be strings")
		}
		if topic == "" {
			topic = "all"
		}
	}
	msg, ok := helpText[topic]
	if !ok {
		msg = fmt.Sprintf("no help for topic %q\n", topic)
	}

	fmt.Fprint(vm.wOut, msg)
	return ErrCommandOk
}

var helpText = map[string]string{
	"all":        helpTopics,
	"commands":   helpCommands,
	"filters":    helpFilters,
	"functions":  helpFunctions,
	"pragmas":    helpPragmas,
	"properties": helpProperties,
	"types":      helpTypes,
}

const (
	helpTopics = `Help topics

commands    Help for all commands
filters     Help for filter expressions
functions   Help for all functions
pragmas     Help for all configuration options
properties  Help for goroutine properties
types       Describe all types

Use help("<topic>") to get detailed help for the topic.
`

	helpCommands = `Commands control the goroutine-explore shell.

cd      Change working directory.
empty   Clears all variables in the workspace, with confirmation (y/N).
exit    Exit the shell.
help    Show available commands and expression functions.
ls      Show files in the working directory.
pwd     Show the path to the working directory.
quit    Exit the shell (aliased to exit).
vars    Show all variables in the workspace.
`

	helpFilters = `Filter expressions can contain any of the following.

* Numbers or string literals
* Grouping ((, )): Parentheses can be used in combination with logical
  operators to create subexpressions.
* Logical operators (and, or): Short-circuiting operators.
* Numeric comparison (>, >=, <, <=, ==, !=): can be applied to the
  numeric fields .id, .dups, .lines, and .duration.
* String comparison (==, !=): can be applied to string fields .state and .trace.
* Regular expression comparison (=~ or matches, !~): These use Go's standard
  flavor of regex. The left side is a string field like .trace and the right
  side is the literal pattern.
* contains: is a binary operator. The left side is a string field like
  .trace or state and the right side of the literal to match.
* in: is a binary operator and the opposite of contains. The left side is a
  literal to match and the right side is a string field like .trace or
  .state.
`

	helpFunctions = `Functions take arguments and product goroutine dumps as outputs.

function   arguments                output
as         dump, new variable name  dump
delete     dump, filter expression  dump
diff       dump, dump               3 dumps
intersect  dump, dump               dump
load       string path              dump
save       dump, string path        dump
show       dump [limit, offset]     dump
union      dump, dump               dump
where      dump, filter expression  dump
`

	helpPragmas = `Pragmas allow configuring the shell behavior.

option                default     description
pragma.empty.confirm  true        Require confirmation for empty command.
pragma.exit.confirm   true        Require confirmation for exit/quit command.
pragma.limits.steps   1073741824  Maximum virtual machine steps per invocation.
pragma.limits.stack   1024        Maximum virtual machine stack.
pragma.ls.format      none        Format flags to pass to the ls command.
pragma.show.color     true        Show color in output.
pragma.show.count     0           Default value of show function count argument.
pragma.show.dedup     ids         Controls show function goroutine
                                  deduplication. One of (ids, none, number)
pragma.vars.display   count       Controls display of vars command. One of
                                  (count, summary, none)
`

	helpProperties = `Goroutines have properties that you can filter on.

property    type    meaning
.id         number  The goroutine ID.
.createdby  number  The goroutine ID of the parent goroutine
.dups       number  The number of duplicate traces.
.duration   number  The waiting duration (in minutes) of a goroutine.
.lines      number  The number of lines of the goroutine's stack trace.
.state      string  The running state of the goroutine.
.trace      string  The concatenated text of the goroutine stack trace.
`

	helpTypes = `Types

goroutine        The stack trace of a single goroutine, along with its metadata
                 such as ID and state.
goroutine dump   A collection of goroutines.
string           An UTF8 encoded string literal, wrapped in double quotes or
                 backticks. Ex. "running".
pattern:         A regular expression, wrapped in double quotes or backticks.
                 Ex. "^foo.*bar$"
number           An unsigned integer between 0 and 2147483647.
boolean          True or false as the literal true or false in the shell.
field accessor   The name of a goroutine field, prefixed with a period.
                 Ex. .duration.
`
)
