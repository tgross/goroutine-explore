# Security Policy

If you encounter a security bug, please report via a GitHub issue. Please keep
in mind the security model described below.

You must _never_ use `goroutine-explore` to run untrusted expressions, because
expressions can overwrite arbitrary files owned by the user with the `save`
function. For example, you should never use untrusted data as the input to the
`--expression` parameter.

However, it should be safe to `load` arbitrary files as goroutine dumps without
those files being able to do anything worse than return an error or hang or
crash `goroutine-explore`. It should be impossible for a loaded file to force
unexpected instructions in the bytecode VM, in particular running the `save`
function when not requested by the user.

This repository does not use Dependabot updates, as the dependency tree is very
small and consists of fairly stable libraries. The repository is configured with
Dependabot alerts so that any critical vulnerabilities are flagged.
