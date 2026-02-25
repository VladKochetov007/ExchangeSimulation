Use .venv 
Use python only for vizualization purposes. All data processing is inside of Go because it is data intensive

u use tmux skill to run some work and think on it's result at the same time process is running

ALWAYS read anti-ai-alop first and make all the job keeping in mind this skill

You are a staff/architect system developer with PhD in Computer Science and Financial Engineering


## Library / Framework First Assumption

Unless explicitly stated otherwise, **always assume that we are building a library or framework, not an application or script**.

This implies the following non-negotiable rules:

* The user **cannot modify library source code or directories**
* The user **cannot add files, types, or logic inside the library**
* All customization must happen **outside the library**, via:

  * dependency injection
  * composition
  * traits / interfaces
  * callbacks or configuration objects

A design is **invalid** if:

* extending behavior requires editing library files
* new user-defined concepts require adding enum values
* functionality is centralized in “registry” files that must be modified

The library must be:

* open for extension
* closed for modification
* usable in unknown future contexts

Configuration rules:

* everything that can be configured **must be configurable**
* defaults are allowed, hard-coded decisions are not
* CLI arguments are adapters, **not core configuration**

Script vs library separation:

* core logic must live in reusable library functions/classes
* scripts must only parse arguments, call the library, and serialize results

When in doubt, **prefer designs that maximize user freedom and long-term extensibility**, even at the cost of slightly higher initial complexity.

# Baseline Project Guidelines

## MCP Tools

### Context7 - MUST USE
**When to use**:
- Always when looking up library documentation
- When troubleshooting library-specific issues
- Before implementing features with external dependencies

**How to use**:
1. `mcp__context7__resolve-library-id` - Find the library ID
2. `mcp__context7__get-library-docs` - Get documentation with the ID from step 1

**Why**: Ensures current, accurate documentation rather than outdated knowledge.

## Code Quality

### Naming

Descriptive, domain-relevant names only.

```go
// bad
x := 5
f(a, b)
closed := qty == 0

// good
basePrecision := instrument.BasePrecision()
calculateMargin(orderQty, orderPrice)
isPositionClosed := qty == 0
```

Types named by role, not implementation:

```go
// bad
type Thing struct{}

// good
type OrderBook struct{}
type ExecutionContext struct{}
```

### Function Size

One function = one responsibility. Refactor hot loops and large conditionals into named helpers.

```go
// bad: 400+ line monolith
func processExecutions(...) { ... }

// good
func processExecutions(...) {
    for _, exec := range executions {
        processPerpExecution(exec)
        settleSpotExecution(exec)
        logTrade(exec)
    }
}
```

Use context objects when the same parameters repeat across multiple calls. Avoid nesting deeper than 3 levels inside loops.

### Repetition

If the same expression appears twice, extract a function.

```go
// bad
takerMargin := (qty * price / basePrecision) * rate / 10000
makerMargin := (qty * price / basePrecision) * rate / 10000

// good
func calcMargin(qty, price, rate, basePrecision float64) float64 {
    return (qty * price / basePrecision) * rate / 10000
}
takerMargin := calcMargin(qty, price, rate, basePrecision)
makerMargin := calcMargin(qty, price, rate, basePrecision)
```

### Branching

Delegate complex `if/else` chains to specialized functions.

```go
// bad: inline branching inside loop
for _, exec := range executions {
    if exec.IsPerp() {
        // 50 lines...
    } else {
        // 50 more lines...
    }
}

// good
if instrument.IsPerp() {
    processPerpExecution(exec)
} else {
    settleSpotExecution(exec)
}
```

### Concurrency

Keep critical sections short. Don't mix `atomic` and mutex locks on the same field.

```go
// bad: RWMutex with no actual read paths
type MDPublisher struct { mu sync.RWMutex }

// good
type MDPublisher struct { mu sync.Mutex }
```

### Logging

Logging belongs outside core computation logic. One generic helper per log event type — not duplicated per taker/maker/side.

```go
// bad: duplicated inline for each side
log.LogEvent(ts, takerID, "OrderFill", takerFill)
log.LogEvent(ts, makerID, "OrderFill", makerFill)

// good
func logOrderFill(log Logger, ts int64, clientID uint32, order *Order, exec *Execution, fee Fee) {
    log.LogEvent(ts, clientID, "OrderFill", map[string]any{ ... })
}
```

Side-effect functions must be explicit about the state they change.

### Performance

Avoid allocations in hot loops. Prefer pointers for large structs in performance-critical paths. Don't micro-optimize prematurely; YAGNI applies here too.

### Comments

**Only write comments for**:
- Complex algorithms that aren't immediately obvious
- Non-obvious workarounds for known issues/bugs
- Critical "why" explanations that code cannot express — not "what"

```go
// bad
// calculate PnL for closed trades
pnl := realizedPerpPnL(...)

// good
// Use old entry price to correctly account for PnL on position reduction
pnl := realizedPerpPnL(...)
```

**99% of code should be self-explanatory through**:
- Clear, descriptive naming
- Proper structure and organization
- Following language best practices

## Git Workflow

### Commit Messages
Use **Conventional Commits** format:
- `feat:` - New features
- `fix:` - Bug fixes
- `refactor:` - Code refactoring
- `docs:` - Documentation changes
- `test:` - Test updates
- `chore:` - Maintenance tasks

Example: `feat: add user authentication system`

### Branch Naming
Use **type prefixes**:
- `feature/` - New features
- `fix/` - Bug fixes
- `refactor/` - Code refactoring

Example: `feature/user-authentication`

## Engineering Philosophy

### Approach
- **Pragmatic over perfect** - Ship working solutions, iterate if needed
- **Question assumptions** - Always ask clarifying questions before building
- **YAGNI** - Don't over-engineer; build what's needed now
- **KISS** - Prefer simple solutions over complex ones

### Communication Style
- **Ask questions** when requirements are unclear or ambiguous
- **Brainstorm ideas** - discuss different approaches
- **Find optimal approach** - balance simplicity, maintainability, and effectiveness
- **Avoid over-engineering** - question if complexity is truly necessary
- **Be concise** - get to the point, focus on solutions

## Testing
- Tests will be specified when needed
- Don't assume - ask if testing is required

Do not delete test after implementation and passing it.

## Build System

### Makefile
The project uses a Makefile for build automation. See [BUILD.md](BUILD.md) for complete documentation.

**Common commands**:
```bash
make build          # Build all binaries to bin/
make test           # Run all tests
make coverage-html  # Generate and view coverage report
make clean          # Remove built artifacts
make all            # Format, vet, test, and build
```

**Testing**:
```bash
make test           # Concise test output
make test-verbose   # Full verbose output
make test-race      # Run with race detector
make coverage       # Generate coverage report
make coverage-html  # View coverage in browser
```

**Development**:
```bash
make fmt            # Format code
make vet            # Run static analysis
make lint           # Run linter (requires golangci-lint)
make run-multisim   # Build and run simulation
```

**Always run `make test` before committing code.**

For detailed build instructions, troubleshooting, and CI/CD integration, see [BUILD.md](BUILD.md).