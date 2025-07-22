# goroutine-explore

An interactive tool for analyzing Golang goroutine dumps.

_Note: this is a fork of [linuxerwang/goroutine-inspect][] and is undergoing
significant rewrite to have a new expression language and more flexible
goroutine dump ingest. Expect frequent breaking behavior changes until 1.0.0._

## Quick Start

Run `goroutine-explore` in your terminal to start a shell, and load a goroutine
dump from a file.

```sh
>>> g1 = load "goroutine-dump.txt"
# of goroutines: 2217

        running: 1
        IO wait: 533
        syscall: 2
   chan receive: 50
         select: 1504
       runnable: 38
     semacquire: 85
      chan send: 4
```

Filter the goroutine dump by an expression.

```
>>> g2 = g1 where .state == "select" \
              and .duration > 10 \
              and .trace contains "keepAlive"
# of goroutines: 5

         select: 5
```

Print the details of the result and save them to a file.

```
>>> show g2
goroutine 72 [select, 25 minutes]: 5 times: [72, 54755, 76757, 299, 201]
google.golang.org/grpc/transport.(*http2Server).keepalive(0xc4202f0420)
        google.golang.org/grpc/transport/http2_server.go:919 +0x488
created by google.golang.org/grpc/transport.newHTTP2Server
        google.golang.org/grpc/transport/http2_server.go:226 +0x97c

>>> save g2 "goroutines-filtered.txt"
```

### Install or Build

Install with:

```bash
go install github.com/tgross/goroutine-explore@latest
```

Or build from a source checkout and install it into `~/go/bin` with:

```bash
make install
```

### The Shell

Run `goroutine-explore` in your terminal to start an interactive shell. Shell
history is saved in your `XDG_CACHE_HOME` directory, in `$HOME/.cache`, or your
platform-specific cache directory such as `$HOME/Library/Caches` or
`%LocalAppData%`. The shell supports fairly typical line editing
shortcuts. Refer to the [`liner`][] docs for details.

You can write two kinds of instructions in the shell: commands and
expressions. Both instructions are executed when you press `<enter>`, unless the
line ends in a pipeline character `|` or backslash `\` to indicate that the
instruction will extend to multiple lines.

## Commands

Commands change the working environment of the `goroutine-inspect`
shell. Commands must always come at the start of a line and cannot be part of a
pipeline or filter expression.

| Command | Summary                                                         |
|---------|-----------------------------------------------------------------|
| cd      | Change working directory.                                       |
| empty   | Clears all variables in the workspace, with confirmation (y/N). |
| exit    | Exit the shell.                                                 |
| help    | Show available commands and expression functions.               |
| ls      | Show files in the working directory.                            |
| pwd     | Show the path to the working directory.                         |
| pragma  | Change the behavior of the shell.                               |
| quit    | Exit the shell (aliased to exit).                               |
| vars    | Show all variables in the workspace.                            |

* `cd`: Takes a single path argument, which must be quoted if it includes
  spaces. This command will expand environment variables like `$HOME` and path
  traveral expressions like `..` and returns to the previous working directory
  if the argument is `-`

* `empty`: Erases all variables in the workspace, with confirmation (y/N). You
  can bypass confirmation by setting `pragma empty:confirm false`.

* `exit`: Exits the shell, with confirmation (y/N). You can bypass confirmation
  by exiting via `Ctrl-D` or by setting `pragma exit:confirm false`.

* `help`: Show a summary of available commands and functions.

* `ls`: List all files in the working directory.

* `pwd`: Print the path to the working directory.

* `pragma`: Sets a `goroutine-explore` behavior from its defaults, or prints its
  current value. Using `pragma` without any arguments prints a summary of the
  available configurations and their current values.

  * `empty:confirm` (default value: `true`): If set to `false`, disable the
    confirmation prompt on the `empty` command.

  * `exit:confirm` (default value: `true`): If set to `false`, disable the
    confirmation prompt on the `exit` command.

  * `ls:format` (default value: `none`): Format flags to pass to the `ls`
    command. If set, the `ls` command will invoke the parent shell's `ls`
    command with these flags instead of listing the directory itself.

  * `show:dedup` (default value: `ids`): Controls how the `show` command
    deduplicates goroutines. The default behavior lists the IDs of duplicates
    with each goroutine stack. You can set this to `number` to show only the
    number of duplicates without the IDs. Or you can set this to `none` to stop
    deduplication entirely.

  * `show:color` (default value: `true`): Controls whether the `show` command
    adds color to the output. If you have the `NOCOLOR` environment variable
    set, this pragma defaults to `false` instead.

  * `vars:display` (default value: `count`): Controls the output of the `vars`
    command. By default, `vars` shows the total number of goroutines in each
    dump. If set to `summary`, the `vars` command will print a summary
    instead. If set to `none`, the `vars` command will only print the names of
    the dumps.

* `quit`: Exits the shell. Alias for `exit`.

* `vars`: Show all the variables in the workspace, along with a count of
  goroutines in the dump. This behavior can be modified by the `vars:display`
  pragma.

```
>>> vars

g0  10
g1  127
```

## Types

The `goroutine-inspect` shell understands the following types:

* goroutine: The stack trace of a single goroutine, along with its metadata such as ID and state.
* goroutine dump: A collection of goroutines.
* string: An UTF8 encoded string literal, wrapped in double quotes. Ex. `"running"`.
* pattern: A [`regexp`](https://pkg.go.dev/regexp)-compatible regex expression,
  wrapped in forward slashes. Ex. `/^foo.*bar$/`
* number: An unsigned integer between 0 and 2147483647.
* boolean: True or false as the literal `true` or `false` in the shell.
* field accessor: The name of a goroutine field, prefixed with a period. Ex. `.duration`.

## Variables

Variables can be any valid Go identifier. A variable can only store a goroutine
dump and not any other value (such as a number or individual goroutine).

## Expressions

All expressions return one or more goroutine dumps. The last expression on a
line will print a summary of those goroutine dumps.

### Show a summary

A variable by itself is an expression, so by typing the name of a variable you
can see its summary.

```
>>> g
# of goroutines: 2217

        running: 1
        IO wait: 533
        syscall: 2
   chan receive: 50
         select: 1504
       runnable: 38
     semacquire: 85
      chan send: 4
```

### Assignment

Bind a goroutine dump to a variable with the `=` sign. The left side of the
assignment is the variable you're assigning to, and the right side of the
assignment is the expression you're assigning from. Assignment always copies its
inputs.

```
>>> g2 = g1
# of goroutines: 2217

        running: 1
        IO wait: 533
        syscall: 2
   chan receive: 50
         select: 1504
       runnable: 38
     semacquire: 85
      chan send: 4

>>> vars
g1  g2
```

Some functions return multiple goroutine dumps. You can assign these results to
multiple variables separated by a `,`. For example, using the `diff` function
(described below):

```
>>> left, common, right = diff g1 g2
```

When a function returns multiple goroutine dumps and you only want to assign one
of them, you can use `_` to discard that dump, similar to assignment in Go.

```
>>> left, _, _ = diff g1 g2
```

### Pipelines

You can use `|` to pipeline multiple expressions. Assignment takes precedence
over pipe operators. This means the value of `g3` at the end of these two
expressions:

```
>>> g2 = g1 where .state == "select" \
              and .duration > 10 \
              and .trace contains "keepAlive"
>>> g3 = g2 delete .trace contains "gRPC"
```

Would be the same as the value of `g3` at the end of this expression, without
having to define an intermediate variable.

```
>>> g3 = g1 where .state == "select" |
            where .duration > 10 |
            where .trace contains "keepAlive" |
            delete .trace contains "gRPC"
```

## Functions

Functions read in a goroutine dump and return one or more goroutine dumps. The
general syntax for functions is:

```
<input> <function> [arguments]
```

Where `input` is a previous expression or pipeline that returns a goroutine
dump. (The `load` function is the sole exception; it will ignore any input.)
Arguments may be variables, strings, or other expressions. For example, the
following lines show equivalent `where` function calls:

```
>>> g2 = g1 where .state == "select"

>>> g2 = g1 | where .state == "select"
```

| Name        | Output  | Arguments                                       |
|-------------|---------|-------------------------------------------------|
| `as`        | dump    | new variable name                               |
| `delete`    | dump    | filter expression                               |
| `diff`      | 3 dumps | variable pointing to dump                       |
| `intersect` | dump    | variable pointing to dump                       |
| `limit`     | dump    | variable pointing to dump                       |
| `load`      | dump    | string path (note: load ignores any input dump) |
| `save`      | dump    | string path                                     |
| `show`      | dump    | [offset, limit]                                 |
| `union`     | dump    | variable pointing to dump                       |
| `where`     | dump    | filter expression                               |

### Assign mid-pipeline

The `as` function allows you to make an assignment to a variable with an
intermediate result from the middle of a pipeline. For example, the following
assigns the query result to `g2` before deleting the requested trace and
assigning that to `g3`.

```
>>> g3 = g1 where .state == "select" |
            where .duration > 10 |
            as g2 |
            delete .trace contains "gRPC"
```

If an error occurs while evaluating the pipeline, the target variable will not
be defined or updated.

### Load a goroutine dump from file

The `load` function returns a goroutine dump loaded from a file path. `load`
accepts absolute paths or paths relative to the working directory. Typically
you'll want to assign the result to a variable.

```
>>> g = load "goroutine-dump.txt"
# of goroutines: 2217

        running: 1
        IO wait: 533
        syscall: 2
   chan receive: 50
         select: 1504
       runnable: 38
     semacquire: 85
      chan send: 4
```

### Save a goroutine dump to a file

The `save` function takes a dump and a file path, and saves the dump in text
format equivalent to that written by the Go runtime's
[pprof](https://pkg.go.dev/runtime/pprof#Profile) with the `debug=2` flag. The
`save` accepts absolute paths or paths relative to the working directory.

```
>>> g save "goroutine-dump.txt"
```

The `save` function returns the dump that was saved. This allows you to save
intermediate results of a pipeline or assign a dump and save it in the same
command.

```
>>> g2 = g1 where .state == "select" |
            where .duration > 10 |
            where .trace contains "keepAlive" |
            save "./including-gRPC.txt"
            delete .trace contains "gRPC" |
            save "./without-gRPC.txt"
```

### Show the goroutines of a dump

The `show` function takes a dump and prints the goroutine stack for every
goroutine in the dump in a format equivalent to that written by the Go runtime's
[pprof](https://pkg.go.dev/runtime/pprof#Profile) with the `debug=2` flag.

By default, `show` will deduplicate goroutines with the same header and stack
and list the duplicate IDs. You can adjust this behavior with the `show:dedup`
pragma. The default value (`ids`) lists the IDs of duplicates with each
goroutine stack. You can set this to `number` to show only the number of
duplicates without the IDs. Or you can set this to `none` to stop deduplication
entirely.

The `show` function takes the following optional number arguments: `offset` and
`limit`. These allow you to page through a goroutine dump. The `offset` argument
is first but if only one of the two arguments is passed, `show` treats it as a
`limit` with no offset.

```
# show 10 goroutines starting at offset 100
>>> g1 show 100 10
```

Paging via `offset` and `limit` respects the `show:dedup` pragma such that only
the displayed goroutine stacks count towards the offset and limit. For example,
a stack with 100 duplicates would only count 1 towards a limit of 10 if
`show:dedup ids` or `show:dedup number`, but only 10 of the goroutines would be
shown if `show:dedup none`.

### Filter expressions

The `where` and `delete` functions return a dump where goroutines have been kept
or removed based on the outcome of a conditional filter expression. For example,
to filter a dump down to goroutines that have been in `select` for more than 10
minutes:

```
>>> g2 = g1 where .state == "select" and .duration > 10
>>> g2 show

goroutine 72 [select, 25 minutes]: 10 times: [72, 54755, 76757, 299, 201, 286, 283, 296, 204, 302]
google.golang.org/grpc/transport.(*http2Server).keepalive(0xc4202f0420)
        google.golang.org/grpc/transport/http2_server.go:919 +0x488
created by google.golang.org/grpc/transport.newHTTP2Server
        google.golang.org/grpc/transport/http2_server.go:226 +0x97c
```

The filter expression is applied to each goroutine, and field accessors in the
expression are implicitly for the current goroutine being examined. Filter
expressions can contain any of the following:

* Numbers or string literals
* Grouping (`(`, `)`): Parentheses can be used in combination with logical
  operators to create subexpressions.
* Logical operators (`and`, `or`): Short-circuiting operators.
* Numeric comparison (`>`, `>=`, `<`, `<=`, `==`, `!=`): can be applied to the numeric
  fields `.id`, `.dups`, `.lines`, and `.duration`.
* String comparison (`==`, `!=`): can be applied to string fields `.state` and `.trace`.
* Regex comparison (`=~`, `!~`): These use Go's standard
  [`regexp`](https://pkg.go.dev/regexp) flavor of regex. The left side is a
  string field like `.trace` and the right side is the literal pattern.
* `contains`: is a binary operator. The left side is a string field like
  `.trace` or `state` and the right side of the literal to match.

Filter expressions can also use the following helper functions.

| Function   | Args                             | Returns |
|------------|----------------------------------|---------|
| `lower`    | string or field accessor         | string  |
| `upper`    | string or field accessor         | string  |

Example:

```
>>> g2 = g1 where lower .trace contains "handleStream"
```

#### Properties of a Goroutine Dump Item

Each dump item has 5 properties which can be used in conditionals:

| property    | type   | meaning                                             |
|-------------|--------|-----------------------------------------------------|
| `.id`       | number | The goroutine ID.                                   |
| `.dups`     | number | The number of duplicate traces.                     |
| `.duration` | number | The waiting duration (in minutes) of a goroutine.   |
| `.lines`    | number | The number of lines of the goroutine's stack trace. |
| `.state`    | string | The running state of the goroutine.                 |
| `.trace`    | string | The concatenated text of the goroutine stack trace. |


### Diff

The `diff` expression takes two goroutine dumps and returns three goroutine dumps: a dump containing goroutines that only appear in the left side, a dump containing goroutines that appear in both the left and right side, and a dump containing goroutines that only appear in the right side.

```bash
>> l, c, r = g1 diff g2
>> l
# of goroutines: 574

        IO wait: 147
   chan receive: 1
       runnable: 3
         select: 421
        syscall: 2

>> c
# of goroutines: 651

        IO wait: 157
       runnable: 4
         select: 489
     semacquire: 1

>> r
# of goroutines: 992

        IO wait: 229
   chan receive: 49
      chan send: 4
       runnable: 31
        running: 1
         select: 594
     semacquire: 84
```


### Union

The `union` expression takes two goroutine dumps and returns a goroutine dump that combines them. Goroutines with the same ID in both dumps will be de-duplicated if they are identical. If they are not identical, this expression will return an error.

```
>> g3 = g1 union g2
# of goroutines: 574

        IO wait: 147
   chan receive: 1
       runnable: 3
         select: 421
        syscall: 2
```

### Intersect

The `intersect` expression takes two goroutine dumps and returns a goroutine dump that includes only goroutines that are identical between them. Goroutines with the same ID in both dumps will not be included if they are not identical.

```
>> g3 = g1 intersect g2
# of goroutines: 14

        IO wait: 7
   chan receive: 1
       runnable: 3
         select: 1
        syscall: 2
```

[linuxerwang/goroutine-inspect]: https://github.com/linuxerwang/goroutine-inspect
[`liner`]: (https://github.com/peterh/liner?tab=readme-ov-file#line-editing)
