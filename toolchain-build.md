What your toolchain should support

Your toolchain should support four goals:

Build quality
Behavioral equivalence
Performance
Developer productivity
Language
Go 1.25

go.mod

module github.com/<org>/picomatch-go

go 1.25

toolchain go1.25.0
Formatter

Go standard formatter

gofmt

or

goimports
Static Analysis

Built-in

go vet
Linter

Use

golangci-lint

Enable

govet
staticcheck
errcheck
revive
ineffassign
unused
gosimple
Testing

Native Go tests

go test ./...
Coverage
go test -cover ./...

Coverage profile

go test -coverprofile=coverage.out ./...
Benchmarks ⭐⭐⭐⭐⭐

The README emphasizes speed and benchmark comparisons with minimatch, showing performance is a core project goal.

Implement Go benchmarks:

go test -bench=.

Benchmark:

simple *
**
braces
extglobs
POSIX classes
Fuzz Testing ⭐⭐⭐⭐⭐

This is almost mandatory.

Picomatch handles many edge cases:

nested extglobs
escaped characters
brackets
braces
Unicode
malformed input
deeply nested expressions

Native Go fuzzing:

go test -fuzz=.
Differential Testing ⭐⭐⭐⭐⭐

This is probably your biggest competitive advantage.

Random Glob
      │
      ▼
 JavaScript picomatch
      │
      ▼
Expected Result
      ▲
      │
 Go picomatch

Compare

boolean result
compiled regex behavior
scan output (where applicable)
parse output (where practical)

This directly supports the Behavioral Equivalence (30%) judging criterion.

Repository structure

The README exposes these API entry points:

scan
parse
compileRe
makeRe
toRegex
isMatch
matchBase
test

These naturally suggest the following package layout:

picomatch-go/

go.mod
go.sum

cmd/

internal/
    lexer/
    scanner/
    parser/
    regex/
    matcher/
    ast/
    options/

pkg/
    picomatch/

testdata/

bench/

fuzz/

scripts/

.github/
    workflows/
GitHub Workflows
ci.yml

Every push:

Checkout

↓

Setup Go

↓

go fmt

↓

go vet

↓

golangci-lint

↓

go test

↓

coverage
benchmark.yml
go test -bench=.

Publish benchmark results as artifacts.

fuzz.yml

Manual or scheduled:

go test -fuzz
parity.yml ⭐⭐⭐⭐⭐

This should be your custom workflow.

Run

Node.js implementation

Generate expected outputs.

Then

Go implementation

Compare outputs.

Fail if any mismatch exists.

This workflow is unique to a porting project and provides strong evidence of correctness.

Makefile
fmt:
	go fmt ./...

lint:
	golangci-lint run

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

cover:
	go test -cover ./...

bench:
	go test -bench=.

fuzz:
	go test -fuzz=Fuzz -fuzztime=30s
Development tools
Purpose	Tool
Dependency management	go mod
Formatting	gofmt / goimports
Linting	golangci-lint
Static analysis	go vet
Advanced analysis	staticcheck
Security	govulncheck
Testing	go test
Coverage	go test -cover
Benchmarking	go test -bench
Fuzzing	go test -fuzz
Language server	gopls
One more thing I'd add

From the README, picomatch's public API consists of functions like scan, parse, compileRe, makeRe, toRegex, isMatch, matchBase, and test, along with a rich set of options (dot, nocase, windows, posix, noextglob, etc.).

Instead of implementing everything at once, mirror this API incrementally:

Core parser (scan, parse)
Regex generation (makeRe, compileRe, toRegex)
Matcher APIs (isMatch, main matcher function)
Compatibility options (dot, nocase, windows, posix, etc.)

This phased approach lets you deliver a working subset early, continuously validate it against the original behavior, and expand feature coverage throughout the 72-hour hackathon.