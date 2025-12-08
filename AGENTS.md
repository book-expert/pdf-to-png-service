# Development Guide for LLM Agents

**These are not suggestions but strict directives that must be followed precisely. The guide is the North Star for all
actions.**

This guide serves as a comprehensive reference for LLM agents acting as craftsmen and craftswomen, building beautiful
software with love and care. It integrates foundational design principles, coding standards, and a recursive
problem-solving framework based on Design Driven Testing (DDT) and Golden Rules guidance. This document replaces
traditional Test-Driven Development (TDD) with an approach that explores implementation paths dynamically, tests designs
as they're built, and validates continuously against the Golden Rules—the North Star for all development decisions.

## Core Principles

These foundational principles apply universally across all languages and projects, forming the bedrock for elegant,
robust, maintainable systems.

### 1. Simplicity is Non-Negotiable

Strive for the most straightforward solution that fulfills all requirements. Complexity is the primary source of bugs,
security vulnerabilities, and maintenance overhead. A complex system is difficult to reason about and impossible to
modify with confidence. If a design is not simple, it is wrong—re-evaluate the approach with love and care.

### 2. Methodical Problem Decomposition

Before writing any implementation code:

1. **Identify and Isolate the Core Problem:** Articulate the precise problem, separating it from surrounding concerns
2. **Deconstruct into Subproblems:** Break down into the smallest possible, independent, verifiable subproblems
3. **Analyze Constraints and Environment:** Consider language, framework, and environmental limitations; identify edge
   cases, failure modes, and performance constraints upfront
4. **Solve Incrementally:** Address one subproblem at a time, using DDT to verify each piece before integration

### 3. Explicit Over Implicit

All intentions, dependencies, and behaviors must be clear and explicit. Code should never rely on "magic" or hidden
mechanisms. Make all assumptions visible and auditable—ambiguity and side effects create unpredictable, fragile systems.

### 4. Self-Documenting Code and Ultimate Readability

Code is read far more than written—optimize for the reader:

- **Intention-Revealing Names:** Use whole-word, descriptive names for all variables, functions, and classes.
  Single-letter or cryptic abbreviations are strictly forbidden. For LLM-generated code, explicit names (e.g.,
  `user_recency_score` not `urs`) maximize clarity
- **Purpose of Comments:** Explain WHY, not WHAT. Clarify complex logic, business rules, or implementation reasoning. If
  code needs comments to explain WHAT it does, refactor for simplicity
- **Comment Hygiene:** Every source file begins with a comment explaining its purpose within the system

### 5. Single Responsibility and Low Complexity

Every function, class, or module has one, and only one, reason to change. It does one thing with precision and
efficiency. Keep cognitive and cyclomatic complexity minimal (functions ≤60 lines, complexity ≤10) for easy testing,
reuse, and refactoring.

### 6. Acyclic Dependencies

The dependency graph for modules, packages, or services must be a Directed Acyclic Graph (DAG). **Circular dependencies
are strictly forbidden**—they create architectural flaws making components impossible to isolate, test, or deploy
independently.

### 7. Composition Over Inheritance

Favor composition and interfaces over implementation inheritance. Deep inheritance hierarchies lead to the "fragile base
class" problem where parent class changes cause unforeseen breaking consequences.

### 8. Error Handling Excellence

Handle every error explicitly and immediately where it occurs. Never ignore or swallow exceptions. All error messages
must be explicit, provide clear context, and avoid ambiguity. The system must fail fast and loudly, preventing corrupt
states.

### 9. Design Driven Testing (DDT)

Replace isolated TDD with DDT: explore multiple implementation paths (e.g., list vs. dict, sync vs. async), prototype
alternatives, test designs holistically as built. Tests verify system integrity, not isolated pieces in vacuum. Coverage
must exceed 80%.

### 10. Verifiable Truth and No Deception

All claims about functionality must be backed by demonstrable proof through comprehensive passing tests. Deceptive
placeholders or hardcoded mocks are strictly forbidden.

### 11. Automated Quality Enforcement

Extensive use of linters, formatters, and static analysis tools is mandatory. **Suppressing linter warnings is strictly
forbidden**—warnings indicate issues that must be fixed by redesigning code, not silencing tools.

### 12. Immutability By Default

Design components to be immutable whenever possible. This eliminates entire classes of bugs related to side effects and
unpredictable state changes.

### 13. Efficient Memory Management

Be deliberate about memory allocation and resource lifetimes. Avoid unnecessary allocations and ensure all resources are
explicitly released.

### 14. Consistency Reduces Cognitive Load

Follow established style guides and project conventions rigorously to create predictable, easily understood codebases.

### 15. No Premature Optimization

Write correct, clean, and simple code first. Only apply targeted optimizations after identifying significant, measured
bottlenecks with profiling tools.

### 16. Remove What Isn't Used

Immediately delete dead code, unused variables, stale files, or unnecessary abstractions. This includes stale
comments—if a comment no longer accurately describes code, update or delete it immediately.

## Recursive Problem-Solving Framework

This framework replaces traditional sequential planning and isolated TDD with a dynamic, recursive approach guided by
the Golden Rules (the North Star). Agents explore implementation paths via DDT, test designs as built, and validate
continuously—avoiding upfront over-prediction which is error-prone.

### Graph-Based Problem Analysis

Create a dynamic problem graph with:

- **Problem Nodes:** Each distinct problem or concern in the system
- **Execution Paths:** Vectors showing how problems connect and influence each other
- **Data Flows:** Movement of information through the system
- **Golden Rule Compliance:** Every decision validated against principles
- **Recursive Depth:** Understanding that solving one problem reveals new problems

For each node, address:

1. **Data Structures Needed:** What structures solve this problem? Explore alternatives (e.g., array vs. linked list)
   —prototype both via DDT
2. **Foundations and Assumptions:** What are the foundations? List assumptions explicitly
3. **Mathematical Rigor:** Trace implementations with logic and proofs; compute Big O (storage, retrieval, update)
4. **Edge Cases:** Define precisely—boundaries, concurrency issues, type mismatches, resource exhaustion, temporal
   anomalies
5. **Communication:** How do components communicate? Use composition over inheritance; avoid OOP over-complication
6. **Algorithms and Bottlenecks:** What algorithms fit? Where are bottlenecks? Identify cheap improvements (async,
   caching)

### Elegance Assessment

Before implementing, ask with love and care:

1. Does the design need hacks?
2. Can I explain it simply?
3. Does every component have single responsibility?
4. Are there circular dependencies?
5. Is the code self-documenting?

**If you answer "YES" to needing hacks, the design is flawed**—redesign with elegance. If you constantly hack a design,
it's a crap design.

### The Iterative DDT Cycle (NOT Sequential Planning)

This is recursive, not a serialized todo list. There's no way to anticipate all issues from one change upfront—like LLM
next-token prediction, pragmatic development analyzes current context, implements small, and re-evaluates.

**Process:**

```
ANALYZE → EXPLORE PATHS → IMPLEMENT (small) → EVALUATE → ANALYZE (refined)
    ↑                                                             ↓
    └─────────────────────── RECURSE ──────────────────────────┘
```

**Concrete Steps for Each Iteration:**

1. **Assess Alignment:** Before any action, confirm: Are my actions aligned with the principles, standards, and Golden
   Rules? If no, fix compliance. If yes, proceed.

2. **Determine Information and Approach:** Do I know what information I need? What approach should I use? Explore 2-3
   implementation paths (don't guess)—prototype minimally to understand trade-offs.

3. **Minimal Implementation:** Code the chosen path with:
    - Whole-word variable names (explicit for LLM clarity)
    - Type hints (Python) or complete types (Go)
    - Single responsibility
    - No hard-coded values (use constants/config)
    - Comments explaining WHY
    - Error messages explicit and contextual

4. **Test Design as Built (DDT):** Run holistic tests verifying system behavior (not vacuums). Test edges, performance.
   Confirm incrementally with love and care—tests that actually test the system.

5. **Evaluate Against North Star:** Check implementation against all Golden Rules. Does it uphold simplicity? Elegance?
   KISS? Profile Big O, trace logic. If issues emerge, pivot to another explored path or redesign.

6. **Refine and Recurse:** Update problem graph with new revelations, repeat. Each cycle deepens understanding—solutions
   reveal new problems naturally.

### Anti-Patterns to Avoid

1. **Todo-List Planning:** Sequential checklists assume all problems known upfront—reality is recursive
2. **TDD Without Context:** Isolated unit tests ignore system wholeness; use DDT for holistic design validation
3. **Circular Dependencies:** Architectural flaw creating unmaintainable tangles
4. **Ignoring Elegance:** Hacks signal poor design—always redesign for minimal viable elegance
5. **Guessing Paths:** Never guess implementation choices—always explore via DDT prototypes

## Golden Rules (North Star)

Agents continuously check alignment, building beautiful software incrementally.

1. **Write what you mean, mean what you write** — Explicit intent, no ambiguity
2. **Smaller is faster** — Concise functions, modules, systems
3. **Simple is efficient** — Avoid over-engineering (KISS)
4. **Explicit > Implicit** — No magic; document assumptions
5. **Use whole words** — LLM-generated code benefits from clarity: `recency_score` not `rs`
6. **Related code lives together** — Modular, co-located
7. **No hard-coded values** — Everything in config, environment, or named constants
8. **Fresh info/specs** — Latest libraries, specifications, research (e.g., Qdrant 1.10+)
9. **Clean trash code** — Refactor on sight; no dead variables or functions
10. **Read problem 3x** — Define what you know before coding
11. **Plan first** — Architecture before implementation (via problem graph)
12. **DRY** — Don't Repeat Yourself
13. **KISS** — Keep It Simple, Stupid
14. **Accurate comments** — Explain WHY, not WHAT
15. **Put love and care in your work** — This is a craft; you are a craftsman/craftswoman building beauty
16. **No baseless assumptions** — Guessing is like the lottery; base decisions on science and prior art
17. **If you don't know, ask** — Reference science, experts, and prior art
18. **Set expectations early** — Clear scope and goals
19. **Lean, clean, mean solutions** — Minimal viable elegance
20. **Truthful, kind, helpful** — Advance humanity to a better state
21. **Elegance is non-negotiable** — If you constantly hack a design, redesign it
22. **Recursive analysis** — Iteratively refine as you build, not sequential planning
23. **Mathematical rigor** — Big O analysis, proofs of correctness
24. **Dynamic guidance** — Problem graphs and DDT, not checklists
25. **Golden Rules are the North Star** — Every decision validated against these 25 rules
26. **Design Driven Iteration** — Explore paths via DDT, test as built—never guess
27. **ALWAYS return from a function**
28. **Program non-blocking code, because who likes being blocked**
29. **BE HONEST, KIND AND FRIENDLY**
30. **If you put love and care in your work, you will build something deserving love and care**
31. **Design it to be extendable**

## Language-Specific Standards

Extend core principles with language-specific rules. All examples demonstrate whole-word naming, DDT exploration, and
tools passing with zero warnings.

### Python

**Tools:** black (format), ruff (lint), mypy (type check) — zero warnings required

**Type Hints:** Every function signature includes explicit type hints for all arguments and return values

**Immutability:** Prefer tuples over lists for unchanging collections; use generators for large data

**Error Handling:** Always chain exceptions to preserve root cause context

**Testing:** Use pytest with fixtures; pytest.raises() for exceptions; 80%+ coverage

**Example (DDT Exploration: Prototyped list vs. deque for stack, chose deque for O(1) operations):**

**Avoid using try...except. Use if else, return and exit. `With open` does not need `try`**

**Keep docstrings short**

**NEVER add imports inside the code. All imports are at the top of the source**

**NO empty classes doing nothing. It is more confusing than not having anything at all.**

```python
from collections import deque
from typing import Deque, TypeVar, Generic
import pytest

ElementType = TypeVar('ElementType')


class Stack(Generic[ElementType]):
    """
    Implements LIFO stack using deque.
    
    Why deque: O(1) append/pop from both ends, unlike list which is O(n) for left operations.
    DDT explored list vs. deque; chose deque for efficiency and elegance.
    """

    def __init__(self) -> None:
        self.stack_elements: Deque[ElementType] = deque()

    def push_element(self, new_element: ElementType) -> None:
        """
        Adds element to top of stack.
        
        Why: Append right maintains LIFO; O(1) operation.
        """
        self.stack_elements.append(new_element)

    def pop_element(self) -> ElementType:
        """
        Removes and returns top element.
        
        Why: Pop right for LIFO; raises IndexError on empty for explicit failure.
        """
        if not self.stack_elements:
            raise IndexError("Cannot pop from empty stack")
        return self.stack_elements.pop()

    def peek_top_element(self) -> ElementType:
        """
        Returns top element without removing.
        
        Why: Non-destructive view; fails explicitly if empty.
        """
        if not self.stack_elements:
            raise IndexError("Cannot peek at empty stack")
        return self.stack_elements[-1]


# DDT: Tests design holistically, exploring edges like empty stack
def test_stack_push_and_pop() -> None:
    integer_stack = Stack[int]()
    integer_stack.push_element(10)
    integer_stack.push_element(20)
    assert integer_stack.pop_element() == 20  # LIFO verified
    assert integer_stack.peek_top_element() == 10


def test_stack_pop_from_empty() -> None:
    string_stack = Stack[str]()
    with pytest.raises(IndexError, match="Cannot pop from empty stack"):
        string_stack.pop_element()


def test_stack_peek_at_empty() -> None:
    string_stack = Stack[str]()
    with pytest.raises(IndexError, match="Cannot peek at empty stack"):
        string_stack.peek_top_element()
```

### Go

**Tools:** gofmt/gofumpt (format), golangci-lint (lint) — zero warnings required

**Naming:** camelCase unexported, PascalCase exported; godoc comments for all exported identifiers

**Complexity Limits:** Functions ≤60 lines or 40 statements; cyclomatic complexity ≤10; cognitive complexity ≤5

**Error Handling:** Each error gets unique variable name; wrap with fmt.Errorf and %w

**Concurrency:** Pass context.Context as first argument; use channels for communication

**Testing:** Standard testing package with testify; table-driven tests

**Example (DDT Exploration: Prototyped quicksort vs. bubble sort, chose bubble for simplicity/clarity):**

```go
package main

import (
    "fmt"
    "testing"
    "github.com/stretchr/testify/assert"
)

// BubbleSort sorts integer slice in ascending order.
//
// Why bubble sort: Simple adjacent-element swaps for educational clarity.
// O(n²) complexity acceptable for small datasets or teaching contexts.
// DDT explored quicksort vs. bubble; chose bubble for code simplicity.
func BubbleSort(input_numbers []int) []int {
    // Copy to avoid mutation (why: immutability principle)
    sorted_numbers := make([]int, len(input_numbers))
    copy(sorted_numbers, input_numbers)
    
    numbers_length := len(sorted_numbers)
    for outer_iteration := 0; outer_iteration < numbers_length-1; outer_iteration++ {
        for inner_iteration := 0; inner_iteration < numbers_length-outer_iteration-1; inner_iteration++ {
            if sorted_numbers[inner_iteration] > sorted_numbers[inner_iteration+1] {
                // Swap adjacent elements for ordering
                sorted_numbers[inner_iteration], sorted_numbers[inner_iteration+1] = 
                    sorted_numbers[inner_iteration+1], sorted_numbers[inner_iteration]
            }
        }
    }
    return sorted_numbers
}

// DDT: Table-driven tests explore paths (sorted, unsorted, reverse, empty, single)
func TestBubbleSort(t *testing.T) {
    test_cases := []struct {
        test_name       string
        input_numbers   []int
        expected_output []int
    }{
        {
            test_name:       "already_sorted",
            input_numbers:   []int{1, 2, 3, 4},
            expected_output: []int{1, 2, 3, 4},
        },
        {
            test_name:       "unsorted_numbers",
            input_numbers:   []int{4, 2, 7, 1, 3},
            expected_output: []int{1, 2, 3, 4, 7},
        },
        {
            test_name:       "reverse_order",
            input_numbers:   []int{5, 4, 3, 2, 1},
            expected_output: []int{1, 2, 3, 4, 5},
        },
        {
            test_name:       "single_element",
            input_numbers:   []int{42},
            expected_output: []int{42},
        },
        {
            test_name:       "empty_slice",
            input_numbers:   []int{},
            expected_output: []int{},
        },
    }

    for _, test_case := range test_cases {
        t.Run(test_case.test_name, func(t *testing.T) {
            result := BubbleSort(test_case.input_numbers)
            assert.Equal(t, test_case.expected_output, result)
        })
    }
}

func ExampleBubbleSort() {
    numbers := []int{64, 34, 25, 12, 22, 11, 90}
    sorted := BubbleSort(numbers)
    fmt.Println(sorted)
    // Output: [11 12 22 25 34 64 90]
}
```

### Bash

**Tools:** shellcheck (lint) — zero warnings required

**Strict Mode:** Every script begins with `set -euo pipefail`

**Naming:** `readonly UPPER_CASE_SNAKE` for constants; `local lower_case_snake` for variables; `lower_case_snake` for
functions

**Safety:** Quote ALL variables; use printf over echo; direct redirection over cat pipelines

**Error Handling:** Never redirect to /dev/null; capture output/exit codes explicitly

**Scope:** Bash for simple automation/glue only; use Go/Python for complex logic

**Example (DDT Exploration: Prototyped regex vs. loop validation, chose loop for portability):**

```bash
#!/usr/bin/env bash
# Purpose: Validates numeric inputs within acceptable range
# Why: Simple loop-based validation is portable and clear

set -euo pipefail  # Strict mode for safety

readonly MINIMUM_ACCEPTABLE_VALUE=0
readonly MAXIMUM_ACCEPTABLE_VALUE=100

validate_numeric_inputs() {
    local input_values=("$@")
    local valid_count=0
    local invalid_count=0

    # DDT: Explored regex vs. loop; loop more portable and readable
    for input_value in "${input_values[@]}"; do
        # Check if numeric
        if ! [[ "$input_value" =~ ^[0-9]+$ ]]; then
            printf 'Error: "%s" is not numeric\n' "$input_value" >&2
            (( ++invalid_count ))
            continue
        fi
        
        # Check range
        local numeric_value="$input_value"
        if (( numeric_value < MINIMUM_ACCEPTABLE_VALUE || numeric_value > MAXIMUM_ACCEPTABLE_VALUE )); then
            printf 'Error: %d out of range [%d-%d]\n' \
                "$numeric_value" "$MINIMUM_ACCEPTABLE_VALUE" "$MAXIMUM_ACCEPTABLE_VALUE" >&2
            (( ++invalid_count ))
        else
            (( ++valid_count ))
        fi
    done

    if (( invalid_count > 0 )); then
        printf 'Validation failed: %d invalid of %d total inputs\n' \
            "$invalid_count" "${#input_values[@]}" >&2
        return 1
    fi
    
    printf 'Success: All %d inputs valid\n' "$valid_count"
    return 0
}

# DDT: Test design paths (valid, invalid range, non-numeric)
test_validate_numeric_inputs_all_valid() {
    local test_inputs=(10 25 75 100)
    local output
    if output=$(validate_numeric_inputs "${test_inputs[@]}"); then
        if [[ "$output" == *"All 4 inputs valid"* ]]; then
            printf 'DDT PASS: Valid inputs path verified\n'
            return 0
        fi
    fi
    printf 'DDT FAIL: Valid inputs path broken\n' >&2
    return 1
}

test_validate_numeric_inputs_out_of_range() {
    local test_inputs=(-5 150)
    local output
    if ! output=$(validate_numeric_inputs "${test_inputs[@]}" 2>&1); then
        if [[ "$output" == *"out of range"* ]]; then
            printf 'DDT PASS: Range check path verified\n'
            return 0
        fi
    fi
    printf 'DDT FAIL: Range check path broken\n' >&2
    return 1
}

test_validate_numeric_inputs_non_numeric() {
    local test_inputs=(abc 123)
    local output
    if ! output=$(validate_numeric_inputs "${test_inputs[@]}" 2>&1); then
        if [[ "$output" == *"not numeric"* ]]; then
            printf 'DDT PASS: Non-numeric check path verified\n'
            return 0
        fi
    fi
    printf 'DDT FAIL: Non-numeric check path broken\n' >&2
    return 1
}
```

## Implementation Checklist (Per Iteration, Not Linear Plan)

Use this to validate each DDT cycle—not as a serialized todo list.

### Pre-Implementation

- [ ] Alignment confirmed: Actions comply with Golden Rules and principles
- [ ] Problem clearly stated and mapped to graph
- [ ] Data structures identified with Big O analysis
- [ ] Paths explored via DDT prototypes (2-3 alternatives)
- [ ] Edge cases documented explicitly
- [ ] Mathematical rigor confirmed (formulas, proofs if applicable)
- [ ] Design elegance validated (no hacks needed)
- [ ] Dependencies mapped (DAG confirmed, no circles)

### During Implementation

- [ ] Whole-word variable names used (LLM-compatible)
- [ ] Every function has single responsibility
- [ ] Type hints (Python) or complete types (Go) on all functions
- [ ] Comments explain WHY, not WHAT
- [ ] No hard-coded values (constants or config)
- [ ] Error messages explicit and contextual
- [ ] No dead code, unused variables, stale comments

### After Implementation

- [ ] Code passes linters without warnings (ruff/mypy, golangci-lint, shellcheck)
- [ ] Code formatted (black, gofmt)
- [ ] DDT tests validate design holistically (not just happy path)
- [ ] Edge cases covered in tests
- [ ] Big O verified (profiling if needed)
- [ ] Integration with problem graph confirmed
- [ ] New problems revealed? (→ next recursive iteration)

## Operational Mandates

### Pre-Generation

- Confirm necessity lovingly; justify new code
- Explore design paths via DDT prototypes

### Implementation

- Declare variables at scope top
- Parameterize everything (decouple logic from config)
- Validate at boundaries
- Least privilege principle
- Never log secrets

### Verification

- DDT recursive loop: explore → implement → evaluate → refine
- Lint, format, refactor with care
- Test holistically (≥80% coverage)

### Security

- Sanitize all external inputs
- Immutable defaults where possible
- Explicit resource management

## Conclusion

This guide is your North Star as a craftsman or craftswoman building beautiful software:

- **Start with understanding:** Read problem 3x, create graph, explore paths via DDT
- **Build incrementally:** Small, validated steps with love and care
- **Check constantly:** Against Golden Rules, elegance, mathematical rigor
- **Adapt dynamically:** Solutions reveal new problems—recurse naturally
- **Value elegance:** If you hack, redesign; beauty sustains systems over time
- **Use whole words:** LLM-generated code deserves explicit clarity
- **Trust mathematics:** Big O, proofs, not guesses or cargo-cult practices

The goal is systems that are clear, robust, maintainable, and beautiful—not from perfectionism, but because elegance is
how good systems stay good, advancing humanity with every thoughtful line of code.

