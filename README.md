# goroutine-explore

An interactive tool for analyzing Golang goroutine dumps.

```
>> load("goroutine-dump.txt").where(.state == "select" \
        and .duration > 10 \
        and .trace contains "keepalive").show()

goroutine 72 [select, 25 minutes]: 5 times: [72, 54755, 76757, 299, 201]
google.golang.org/grpc/transport.(*http2Server).keepalive(0xc4202f0420)
        google.golang.org/grpc/transport/http2_server.go:919 +0x488
created by google.golang.org/grpc/transport.newHTTP2Server
        google.golang.org/grpc/transport/http2_server.go:226 +0x97c
```

* [Quick Start](#quick-start)
  * [Install or Build](#install-or-build)
  * [The Shell](#the-shell)
  * [The CLI](#the-cli)
* [Commands](#commands)
* [Types](#types)
  * [Variables](#variables)
  * [Pragma](#pragma)
* [Expressions](#expressions)
  * [Show a summary](#show-a-summary)
  * [Assignment](#assignment)
  * [Pipelines](#pipelines)
  * [Functions](#functions)
  * [Assign mid-pipeline](#assign-mid-pipeline)
  * [Load a goroutine dump from file](#load-a-goroutine-dump-from-file)
  * [Save a goroutine dump to a file](#save-a-goroutine-dump-to-a-file)
  * [Show the goroutines of a dump](#show-the-goroutines-of-a-dump)
  * [Filter expressions](#filter-expressions)
    * [Properties of a Goroutine Dump Item](#properties-of-a-goroutine-dump-item)
  * [Diff](#diff)
  * [Union](#union)
  * [Intersect](#intersect)
  * [JSON](#json)
  * [dot](#dot)
* [Contributing](#contributing)
  * [Maintenance](#maintenance)
  * [AI Policy](#ai-policy)
* [FAQ](#faq)

## Quick Start

Run `goroutine-explore` in your terminal to start a shell, and load a goroutine
dump from a file.

```
>> g1 = load("goroutine-dump.txt")
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
>> g2 = g1.where(.state == "select" \
              and .duration > 10 \
              and .trace contains "keepAlive")
# of goroutines: 5

         select: 5
```

Print the details of the result and save them to a file.

```
>> g2.show()
goroutine 72 [select, 25 minutes]: 5 times: [72, 54755, 76757, 299, 201]
google.golang.org/grpc/transport.(*http2Server).keepalive(0xc4202f0420)
        google.golang.org/grpc/transport/http2_server.go:919 +0x488
created by google.golang.org/grpc/transport.newHTTP2Server
        google.golang.org/grpc/transport/http2_server.go:226 +0x97c

>> g2.save("goroutines-filtered.txt")
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

### The CLI

You can run `goroutine-explore` with the `-e`/`-expression` option to run an
expression without starting the REPL shell. When using this option,
`goroutine-explore` will load a goroutine dump from stdin and treat it as the
left hand side of a pipe expression.

```bash
$ cat goroutine-dump.txt | goroutine-explore -e 'show()'
```

## Commands

Commands change the working environment of the `goroutine-explore`
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
| quit    | Exit the shell (aliased to exit).                               |
| vars    | Show all variables in the workspace.                            |

* `cd`: Takes a single path argument, which must be quoted if it includes
  spaces. This command will expand environment variables like `$HOME` and path
  traversal expressions like `..` and returns to the previous working directory
  if the argument is `-`

* `empty`: Erases all variables in the workspace, with confirmation (y/N). You
  can bypass confirmation by setting `pragma.empty.confirm false`.

* `exit`: Exits the shell, with confirmation (y/N). You can bypass confirmation
  by exiting via `Ctrl-D` or by setting `pragma.exit.confirm false`.

* `help`: Show a summary of available commands and functions. Calling `help`
  without arguments shows a list of help topics. You can call help with a topic
  (ex. `help(pragmas)` to get help on a particular topic.

* `ls`: List all files in the working directory.

* `pwd`: Print the path to the working directory.

* `quit`: Exits the shell. Alias for `exit`.

* `vars`: Show all the variables in the workspace, along with a count of
  goroutines in the dump. This behavior can be modified by the `vars.display`
  pragma.

```
>> vars

g0  10
g1  127
```

## Types

The `goroutine-explore` shell understands the following types:

* goroutine: The stack trace of a single goroutine, along with its metadata such as ID and state.
* goroutine dump: A collection of goroutines.
* string: An UTF8 encoded string literal, wrapped in double quotes or backticks. Ex. `"running"`.
* pattern: A [`regexp`](https://pkg.go.dev/regexp)-compatible regex expression,
  wrapped in double quotes or backticks. Ex. `"^foo.*bar$"`
* number: An unsigned integer between 0 and 2147483647.
* boolean: True or false as the literal `true` or `false` in the shell.
* field accessor: The name of a goroutine field, prefixed with a period. Ex. `.duration`
* label map: A map of goroutine labels added via the `runtime/pprof`
  package. You can access a specific key with a field accessor on the `.labels`
  field. Ex. `.labels.worker_id`.

### Variables

Variables can be any valid Go identifier. A variable can only store a goroutine
dump and not any other value (such as a number or individual goroutine). The
value a variable points to is immutable but the variable can be rebound by
assignment.

### Pragma

You can change the behavior of `goroutine-explore` from its defaults by setting
pragmas. Setting a pragma has similar syntax to setting a variable:

```
>> pragma.show.color = false
```

You can get the current values by using `pragma` or any subset of dot-separated
keys For example, to show all the pragmas under `pragma.show`:

```
>> pragma.show
show.color = true
show.count = 0
show.dedup = "ids"
```

The available pragmas are as follows:

* `pragma.debug.disassemble` (default value: `none`): If set to `error`, emit a
  debug representation of the byte code and the virtual machine state whenever
  it returns an error after your expression is successfully compiled. If set to
  `return`, emit the debug representation after every expression, even on
  success. This pragma is generally used for development of `goroutine-explore`
  but may also be useful if you're reporting a bug.

* `pragma.empty.confirm` (default value: `true`): If set to `false`, disable the
  confirmation prompt on the `empty` command.

* `pragma.exit.confirm` (default value: `true`): If set to `false`, disable the
  confirmation prompt on the `exit` command.

* `pragma.limits.steps` (default value: `1073741824`): Maximum number of virtual
  machine steps (op codes retired) per invocation. This limit is designed to
  prevent accidental infinite loops in case of a bug. Iterating over a goroutine
  dump with a simple filter query takes roughly 5 steps per goroutine plus a
  little overhead for the query, so you should be able to iterate many millions
  of goroutines in a single expression without hitting this limit. The limit is
  reset each time the `goroutine-explore` REPL returns for more input.

* `pragma.limits.stack` (default value: `1024`): Maximum size of the virtual
  machine stack. `goroutine-explore` uses the stack for intermediate results of
  nested calls an single expression. Each time the `goroutine-explore` REPL
  returns for more input, the stack is cleared, so you should only need to
  adjust this if you are creating unusually large expressions.

* `pragma.ls.format` (default value: `none`): Format flags to pass to the `ls`
  command. If set, the `ls` command will invoke the parent shell's `ls` command
  with these flags instead of listing the directory itself.

* `pragma.show.color` (default value: `true`): Controls whether the `show`
  command adds color to the output. If you have the `NO_COLOR` environment
  variable set, this pragma defaults to `false` instead.

* `pragma.show.count` (default value: `0`): Controls the default value of the
  `show` command's `count` argument. The default value of `0` shows all
  goroutines in the dump.

* `pragma.show.dedup` (default value: `ids`): Controls how the `show` command
  deduplicates goroutines. The default behavior lists the IDs of duplicates with
  each goroutine stack. You can set this to `number` to show only the number of
  duplicates without the IDs. Or you can set this to `none` to stop
  deduplication entirely.

* `pragma.vars.display` (default value: `count`): Controls the output of the
  `vars` command. By default, `vars` shows the total number of goroutines in
  each dump. If set to `summary`, the `vars` command will print a summary
  instead. If set to `none`, the `vars` command will only print the names of the
  dumps.

## Expressions

All expressions return one or more goroutine dumps. The last expression on a
line will print a summary of those goroutine dumps, unless you have used the
`show` or `json` functions.

### Show a summary

A variable by itself is an expression, so by typing the name of a variable you
can see its summary.

```
>> g
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
>> g2 = g1
# of goroutines: 2217

        running: 1
        IO wait: 533
        syscall: 2
   chan receive: 50
         select: 1504
       runnable: 38
     semacquire: 85
      chan send: 4

>> vars
g1  g2
```

Some functions return multiple goroutine dumps. You can assign these results to
multiple variables separated by a `,`. For example, using the `diff` function
(described below):

```
>> left, common, right = diff(g1, g2)
```

When a function returns multiple goroutine dumps and you only want to assign one
of them, you can use `_` to discard that dump, similar to assignment in Go.

```
>> left, _, _ = diff(g1, g2)
```

### Pipelines

You can use `|` to pipeline multiple expressions. Assignment takes precedence
over pipe operators. This means the value of `g3` at the end of these two
expressions:

```
>> g2 = g1.where(.state == "select"
              and .duration > 10
              and .trace contains "keepAlive"))
>> g3 = g2.delete(.trace contains "gRPC")
```

Would be the same as the value of `g3` at the end of this expression, without
having to define an intermediate variable.

```
>> g3 = g1.where(.state == "select") |
            where(.duration > 10) |
            where(.trace contains "keepAlive") |
            delete(.trace contains "gRPC")
```

### Functions

Functions read in a goroutine dump and return one or more goroutine dumps. The
general syntax for functions is similar to calling a function in Go:

```
f(arg, arg)
```

Most functions expect a goroutine dump as their first argument (the `load`
function is the sole exception), or an expression that returns a goroutine
dump. Additional arguments may be variables, strings, or other expressions.

Functions can be called as though they were methods on the goroutine dump
object, omitting their first argument. Or the function form can be used without
the first argument if it immediately follows a pipe. For example, the following
lines show equivalent `where` function calls that compile to identical bytecode:

```
>> g2 = where(g1, .state == "select")

>> g2 = g1.where(.state == "select")

>> g2 = g1 | where(.state == "select")
```

You can chain function calls as well. For example, the following lines show
equivalent compound or chained `where` function calls which will evaluate to
identical values.

```
>> g2 = g1.where(.state == "select" and .duration > 1)

>> g2 = g1 | where(.state == "select") | where(.duration > 1)

>> g2 = g1.where(.state == "select").where(.duration > 1)
```

The compiler optimizes pipelined or chained `where` and `delete` expressions
into a single loop, so if you have a very large goroutine dump to process it
will not iterate it multiple times unnecessarily. That said, if you're planning
on repeatedly querying the results of a `where` on a large set of goroutines,
you may find you can improve performance by assigning that value to a variable
so that subsequent queries are working on the smaller set.

| Name        | Arguments               | Output  |
|-------------|-------------------------|---------|
| `as`        | dump, new variable name | dump    |
| `delete`    | dump, filter expression | dump    |
| `diff`      | dump, dump              | 3 dumps |
| `dot`       | dump, [string path]     | dump    |
| `graph`     | dump, dump              | dump    |
| `intersect` | dump, dump              | dump    |
| `json`      | dump, [string path]     | dump    |
| `load`      | string path             | dump    |
| `save`      | dump, string path       | dump    |
| `show`      | dump, [limit, offset]   | dump    |
| `union`     | dump, dump              | dump    |
| `where`     | dump, filter expression | dump    |

### Assign mid-pipeline

The `as` function allows you to make an assignment to a variable with an
intermediate result from the middle of a pipeline. For example, the following
assigns the query result to `g2` before deleting the requested trace and
assigning that to `g3`.

```
>> g3 = g1.where(.state == "select") |
            where(.duration > 10) |
            as(g2) |
            delete(.trace contains "gRPC")
```

If an error occurs while evaluating the pipeline, the target variable will not
be defined or updated.

### Load a goroutine dump from file

The `load` function returns a goroutine dump loaded from a file path. `load`
accepts absolute paths or paths relative to the working directory. Loading a
goroutine dump is the most costly operation in `goroutine-explore`, so typically
you'll want to assign the result to a variable so it can be reused.

```
>> g = load("goroutine-dump.txt")
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
format equivalent to that written by the Go runtime's [`pprof`][] with the
`debug=2` flag. The `save` accepts absolute paths or paths relative to the
working directory.

```
>> g.save("goroutine-dump.txt")
```

The `save` function returns the dump that was saved. This allows you to save
intermediate results of a pipeline or assign a dump and save it in the same
command.

```
>> g2 = g1.where(.state == "select") |
            where(.duration > 10) |
            where(.trace contains "keepAlive") |
            save("./including-gRPC.txt")
            delete(.trace contains "gRPC")) |
            save("./without-gRPC.txt")
```

### Show the goroutines of a dump

The `show` function takes a dump and prints the goroutine stack for every
goroutine in the dump in a format equivalent to that written by the Go runtime's
[`pprof`][] with the `debug=2` flag.

By default, `show` will deduplicate goroutines with the same header and stack
and list the duplicate IDs. You can adjust this behavior with the `show.dedup`
pragma. The default value (`ids`) lists the IDs of duplicates with each
goroutine stack. You can set this to `number` to show only the number of
duplicates without the IDs. Or you can set this to `none` to stop deduplication
entirely.

The `show` function takes the following optional number arguments: `limit` and
`offset`. These allow you to page through a goroutine dump. If `limit` is zero,
then all goroutines starting at the `offset` will be shown. If `offset` is zero
or missing, it is ignored and show starts from the beginning of the dump.

```
# show 10 goroutines starting at offset 100
>> g1.show(10, 100)
```

Paging via `offset` and `limit` respects the `show.dedup` pragma such that only
the displayed goroutine stacks count towards the offset and limit. For example,
a stack with 100 duplicates would only count 1 towards a limit of 10 if
`show.dedup ids` or `show.dedup number`, but only 10 of the goroutines would be
shown if `show.dedup none`.

### Filter expressions

The `where`, `delete`, and `graph` functions return a dump where goroutines have
been kept or removed based on the outcome of a conditional filter expression.

The `where` function includes goroutines that match the filter. The `delete`
function excludes goroutines that match the filter. The `graph` function
includes goroutines that match the filter (similar to `where`), but also all
goroutines that are ancestors or descendants of those goroutines, by following
the graph implied by the "created by" lines of their traces.

For example, to filter a dump down to goroutines that have been in `select` for
more than 10 minutes:

```
>> g2 = g1.where(.state == "select" and .duration > 10)
>> g2.show()

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
* Numeric comparison (`>`, `>=`, `<`, `<=`, `==`, `!=`): can be applied to the
  numeric fields `.id`, `.dups`, `.lines`, and `.duration`.
* String comparison (`==`, `!=`): can be applied to string fields `.state` and
  `.trace`, and to the values in a `.labels` map.
* Regex comparison (`=~` or `matches`, `!~`): These use Go's standard
  [`regexp`](https://pkg.go.dev/regexp) flavor of regex. The left side is a
  string field like `.trace` and the right side is the literal pattern.
* `contains`: is a binary operator. The left side either a string field like
  `.trace` or `state`, or a map of `.labels`. The right side of the literal to match.
* `in`: is a binary operator and the opposite of `contains`. The left side is a
  literal to match and the right side is a string field like `.trace` or
  `.state`, or a map of `.labels`.

#### Properties of a Goroutine Dump Item

Each dump item has 5 properties which can be used in conditionals:

| property     | type   | meaning                                                |
|--------------|--------|--------------------------------------------------------|
| `.id`        | number | The goroutine ID.                                      |
| `.createdby` | number | The goroutine ID of the parent goroutine               |
| `.duration`  | number | The waiting duration (in minutes) of a goroutine.      |
| `.lines`     | number | The number of lines of the goroutine's stack trace.    |
| `.state`     | string | The running state of the goroutine.                    |
| `.trace`     | string | The concatenated text of the goroutine stack trace.    |
| `.labels`    | map    | A map of goroutine labels with string keys and values. |

### Diff

The `diff` function takes two goroutine dumps and returns three goroutine dumps:
a dump containing goroutines that only appear in the left side, a dump
containing goroutines that appear in both the left and right side, and a dump
containing goroutines that only appear in the right side.

```
>> l, c, r = diff(g1, g2)
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

The `union` function takes two goroutine dumps and returns a goroutine dump that
combines them. Goroutines with the same ID in both dumps will be deduplicated
if they are identical. If they are not identical, this expression will return an
error.

```
>> g3 = union(g1, g2)
# of goroutines: 574

        IO wait: 147
   chan receive: 1
       runnable: 3
         select: 421
        syscall: 2
```

### Intersect

The `intersect` function takes two goroutine dumps and returns a goroutine dump
that includes only goroutines that are identical between them. Goroutines with
the same ID in both dumps will not be included if they are not identical.

```
>> g3 = intersect(g1, g2)
# of goroutines: 14

        IO wait: 7
   chan receive: 1
       runnable: 3
         select: 1
        syscall: 2
```

### JSON

The `json` function takes a goroutine dump and outputs a pretty-printed JSON
array of all the goroutines in the dump. Unlike the `show` function, no
deduplication of goroutines happens.

```
>> g2 = g1.where(.state == "select" and .duration > 10)
>> g2.json()

[
 {
   "id": 72,
   "header": "goroutine 72 [select, 25 minutes]",
   "createdBy": 10,
   "duration": 25,
   "state: "select",
   "lines": 4,
   "duplicates": [72, 54755, 76757, 299, 201],
   "trace": "google.golang.org/grpc/transport.(*http2Server).keepalive(0xc4202f0420)\n\tgoogle.golang.org/grpc/transport/http2_server.go:919 +0x488\ncreated by google.golang.org/grpc/transport.newHTTP2Server\n\tgoogle.golang.org/grpc/transport/http2_server.go:226 +0x97c"
 }
 ...
]
```

You can use the `load` and `json` function together to turn a goroutine dump
file into JSON for processing with other tools like [`jq`][].

```bash
$ goroutine-explore -e 'load("goroutine-dump.txt").json()' | jq .
```

The `json` function takes an optional path parameter to write a copy of the JSON
to disk, in addition to printing it to the shell.

```
>> g.json("./dump.json")
```

### dot

The `dot` function takes a goroutine dump and outputs a dot-syntax directed
graph of all the goroutines in the dump. This is primarily useful for writing to
a file and then using [graphviz][] tools like `dot` to turn it into an image.

```bash
$ goroutine-explore \
    -e 'load("goroutine-dump.txt").where(id > 200 and id < 300).dot()' \
    > ./graph.dot
$ dot -Tsvg ./graph.dot > ./graph.svg
$ xdg-open ./graph.svg

```

The `dot` function takes an optional path parameter to write a copy of the graph
to disk, in addition to printing it to the shell.

```
>> g.dot("./dump.dot")
```

## Contributing

If you have a small bug fix or documentation fix, please make a pull request!

Generally speaking I'm going to try to resist scope creep. That means I'm not
going to add large new features to `goroutine-explore` unless they feel
compellingly useful or especially fun to work on. I'm unlikely to accept PRs for
major new features rather than working on these features myself. You should
definitely feel free to fork, remix, and have fun with new features
yourself. But if you'd like to suggest a feature, please open an issue for
discussion.

Please follow the [Code of Conduct][] when contributing to this project.

### Maintenance

Generally speaking I'll try to keep on top of bugs, but I'm not going to burn
myself out over it and will tend to batch up work unless it's
critical. Dependencies will be updated as needed, not automatically. The
toolchain will be updated as older toolchains become unsupported, but I'm not
making a commitment to stay on the oldest stable toolchain.

Features will get worked on as I feel like it and have time to set aside. Please
do not leave comments asking for a schedule. If the project becomes
unmaintained, it will be archived. Please don't open issues to ask things like
"is this project dead?" just because you haven't seen a stream of new commits.

### AI Policy

Pull requests and issues that are substantially machine generated rather than
human authored will not be accepted. Any pull request to this project will ask
you to confirm that you are the author and that you are contributing your
changes under the project's license.

Please do not use LLMs to generate bug reports or in discussions (with the sole
exception of using them for translation if you cannot communicate in English). I
want to talk to you, not a machine.

## FAQ

* **How does this project relate to goroutine-inspect?** This project started as
  a fork of [linuxerwang/goroutine-inspect][] where I fixed some bugs and small
  papercuts. But I wanted to make a much larger set of changes like an entirely
  new expression language, and this involved a near total rewrite. Rather than
  keeping the name and creating potential confusion between the two projects, I
  changed the name of the new project to `goroutine-explore`. The remaining code
  that belongs to `goroutine-inspect` is properly attributed in the repository
  and the [NOTICES][] file.

* **Where can I download a binary?** I'm not going to provide pre-built
  binaries. I currently only run Linux, and I don't want to have to deal with
  code signing for macOS and Windows. This tool is only useful for Go
  developers, who I expect can easily install via `go install`.

* **Can I package a binary for my distribution?** Of course, feel free! Just
  keep in mind that the license requires attribution for distribution. Also,
  your distribution's policies are your problem to solve, not mine. If you patch
  `goroutine-explore` in any way before distributing, please ensure your users
  know to contact you first for bug reports.

* **What's the security model?** You must _never_ use `goroutine-explore` to run
  untrusted expressions, because expressions can overwrite arbitrary files owned
  by the user with the `save` function. For example, you should never use
  untrusted data as the input to the `--expression` parameter. However, it
  should be safe to `load` arbitrary files as goroutine dumps without those
  files being able to do anything worse than return an error or hang or crash
  `goroutine-explore`. It should be impossible for a loaded file to force
  unexpected instructions in the bytecode VM, in particular running the `save`
  function when not requested by the user. Please report any such behavior as a
  bug.


[`liner`]: https://github.com/peterh/liner?tab=readme-ov-file#line-editing
[`pprof`]: https://pkg.go.dev/runtime/pprof#Profile
[`jq`]: https://jqlang.org/
[graphviz]: https://graphviz.org/
[Code of Conduct]: ./.github/CODE_OF_CONDUCT.md
[NOTICES]: ./NOTICES.md
[linuxerwang/goroutine-inspect]: https://github.com/linuxerwang/goroutine-inspect
