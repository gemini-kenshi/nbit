# nbit

[![Go Reference](https://pkg.go.dev/badge/github.com/gemini-kenshi/nbit.svg)](https://pkg.go.dev/github.com/gemini-kenshi/nbit)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**nbit** is a zero-dependency, N-bit bitmask library for Go. It represents arbitrarily wide bitmasks as a `[]uint64` slice (64 bits per word), keeping single-bit operations O(1) and memory access cache-friendly.

The library's defining feature is two **explicitly named OR operations** that prevent a common API ambiguity in bitmask libraries:

| Operation | Behaviour | Commutative? |
|-----------|-----------|:---:|
| `Union` | Expands result to hold all bits from both operands | ✓ |
| `Apply` | Overlays bits into a **fixed-width** receiver; extra bits are discarded | ✗ |

---

## Install

```bash
go get github.com/gemini-kenshi/nbit
```

Requires **Go 1.21** or later (uses built-in `min`/`max`).

---

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/gemini-kenshi/nbit"
)

func main() {
    // Construct from bit positions or a raw uint64.
    x := nb.FromBit(0, 4)   // bits 0 and 4 → 0x11
    y := nb.FromValue(17)   // 17 == 0x11

    fmt.Println(x.Equal(y)) // true

    // Subset check: is y a subset of x?
    if x.Mask(y).Equal(y) {
        fmt.Println("y ⊆ x")
    }

    // Multi-word mask (bit 64 starts word[1]).
    wide := nb.FromBit(0, 4, 64)
    fmt.Println(wide) // 0x11, 0x01
}
```

---

## API Reference

### Construction

#### `FromValue(v uint64) NB`

Creates a single-word (64-bit) bitmask from a raw `uint64`.

```go
flags := nb.FromValue(0b00010011) // bits 0, 1, 4 set
```

#### `FromBit(bits ...int) NB`

Creates a bitmask with exactly the named bit positions set. The backing slice is sized just large enough to hold the highest bit.

```go
a := nb.FromBit(0, 4)       // 1 word  — "0x11"
b := nb.FromBit(0, 4, 64)   // 2 words — "0x11, 0x01"
```

---

### Single-Bit Mutation

These methods use **pointer receivers** and mutate the value in-place.

#### `(n *NB) Set(bit int)`

Sets the bit at position `bit`. The backing slice grows automatically if `bit` is beyond the current capacity.

```go
n := nb.FromValue(0)
n.Set(5)    // n is now 0x20
n.Set(128)  // n grows to 3 words; bit 128 set in word[2]
```

#### `(n *NB) Clear(bit int)`

Clears the bit at position `bit`. A no-op if `bit` is beyond the current capacity (the bit is already zero).

```go
n := nb.FromValue(0xFF)
n.Clear(0)  // n is now 0xfe
n.Clear(99) // no-op — bit 99 is outside n's single word
```

#### `(n NB) Test(bit int) bool`

Reports whether the bit at position `bit` is set. Returns `false` if `bit` is beyond capacity.

```go
n := nb.FromBit(3, 7)
n.Test(3)   // true
n.Test(200) // false — safely out of bounds
```

---

### Comparison

These methods use **value receivers** and never mutate.

#### `(n NB) Equal(other NB) bool`

Reports whether two bitmasks represent the same set of bits. Operands of different backing lengths are handled safely: missing words are treated as zero.

```go
a := nb.FromBit(0, 4)
b := nb.FromValue(0x11)
a.Equal(b) // true — same logical value, possibly different backing length
```

#### `(n NB) IsZero() bool`

Reports whether all bits are zero. Short-circuits on the first non-zero word.

```go
nb.FromValue(0).IsZero()  // true
nb.FromBit(63).IsZero()   // false
NB{}.IsZero()             // true — zero value is all-zero
```

---

### Intersection

#### `(n NB) Mask(other NB) NB`

Returns the bitwise AND of `n` and `other`. The result has `min(len(n), len(other))` words — an AND can only produce bits present in both operands, so no data is silently dropped.

```go
a := nb.FromValue(0xFF)
b := nb.FromValue(0x0F)
a.Mask(b).String() // "0x0f"
```

**Classic subset test** — is `b` a subset of `a`?

```go
if a.Mask(b).Equal(b) {
    // every bit in b is also in a
}
```

**Disjoint test** — do `a` and `b` share any bits?

```go
if a.Mask(b).IsZero() {
    // no overlap
}
```

---

### OR Operations

This is where `nbit` departs from libraries that expose a single `Or` or `|` method. Standard bitwise OR is overloaded to mean two different things depending on context:

1. **Set union** — merge two independent sets; no data should be lost.
2. **State masking** — apply flags into a fixed-width register; overflow is discarded by design.

Conflating these two semantics under one method name causes subtle bugs. `nbit` names them explicitly.

---

#### `(n NB) Union(other NB) NB` — Mathematical Set Union

Commutative OR. The result is a **new** `NB` expanded to `max(len(n), len(other))` words so that every bit from both operands is preserved.

```go
a := nb.FromValue(0x11)      // 1 word
b := nb.FromBit(4, 8, 64)    // 2 words

ab := a.Union(b)
ba := b.Union(a)

ab.Equal(ba) // true — always commutative
len(ab.words) // 2 — expanded to hold b's word[1]
```

Use `Union` when you are combining two independent flag sets and neither operand is authoritative about the final width.

---

#### `(n *NB) Apply(other NB)` — Fixed-Width State Masking

Non-commutative OR. Mutates the receiver by OR-ing bits from `other`, but **only within the receiver's existing width**. Bits in `other` that fall beyond the receiver's last word are silently discarded.

```go
register := nb.FromValue(0x01) // 1-word "status register"
incoming := nb.FromBit(1, 64)  // 2 words: 0x02, 0x01

register.Apply(incoming)
// register.words[0] == 0x03  (bit 0 | bit 1)
// register has NOT grown — bit 64 from incoming was discarded
```

Use `Apply` when the receiver is a **fixed-width state container** (a hardware register, a status bitmap, a PCS group state) that must not grow. The left operand always defines the valid bit space.

**Illustrating the non-commutativity:**

```go
a := nb.FromValue(0x11)      // 1 word
b := nb.FromBit(4, 8, 64)    // 2 words

aCopy := nb.FromValue(0x11)
aCopy.Apply(b)               // aCopy: 1 word (unchanged width)

bCopy := nb.FromBit(4, 8, 64)
bCopy.Apply(a)               // bCopy: 2 words (unchanged width)

aCopy.Equal(bCopy)           // false — different widths, different semantics
```

---

### String Representation

#### `(n NB) String() string`

Implements `fmt.Stringer`. Returns an **LSB-first** hexadecimal representation: word[0] (bits 0–63) is printed first, word[1] (bits 64–127) second, and so on. Each word is formatted as `0x%02x` and separated by `, `.

```go
nb.FromValue(0x11).String()       // "0x11"
nb.FromBit(0, 4, 64).String()     // "0x11, 0x01"
nb.FromBit(0, 4, 128).String()    // "0x11, 0x00, 0x01"
NB{}.String()                     // "0x00"
```

The LSB-first ordering matches how embedded systems and protocol specifications typically lay out multi-byte flag fields (byte 0 = least significant).

---

## Design Notes

### Internal Layout

```
NB.words = [ word[0] | word[1] | word[2] | ... ]
              bits      bits      bits
              0–63      64–127    128–191
```

- **One allocation** per `NB` value (a single slice header + backing array).
- **No pointer indirection** beyond the slice itself — iteration is a linear scan over contiguous `uint64` values.
- The zero value `NB{}` is valid and represents an empty, all-zero mask.

### Receiver Discipline

The receiver type encodes the mutation contract directly in the type system:

| Receiver | Methods | Meaning |
|----------|---------|---------|
| `*NB` | `Set`, `Clear`, `Apply` | Mutates the value |
| `NB` | Everything else | Returns a new value or a pure read |

You never need documentation to know whether a method mutates — the type checker tells you at the call site.

### Why Not `[]bool` or `map[int]bool`?

| Representation | Bytes per 64 flags | Allocation per flag |
|----------------|--------------------|---------------------|
| `[]bool` | 64 | 1 byte |
| `map[int]bool` | ~300+ | hash entry + pointer |
| `[]uint64` (nbit) | **8** | none (bit packing) |

The `[]uint64` layout is 8× more compact than `[]bool` and keeps all flag data in a single contiguous region, maximising CPU cache utilisation during bulk operations like `Mask` and `Union`.

---

## Use Cases

| Scenario | Recommended method |
|----------|--------------------|
| Merging two independent flag sets | `Union` |
| Writing event flags into a fixed status register | `Apply` |
| Checking if a flag set is active | `Mask` + `Equal` |
| Checking if two flag sets are disjoint | `Mask` + `IsZero` |
| Querying a single feature flag | `Test` |
| Logging / debugging a bitmask | `String` (via `fmt`) |

---

## Contributing

1. Fork and clone the repository.
2. Create a feature branch: `git checkout -b feat/your-feature`.
3. Write tests alongside your changes; aim for >90% coverage.
4. Run `go test ./...` and `go vet ./...` before opening a PR.
5. Open a pull request describing the motivation and design.

---

## License

MIT — see [LICENSE](LICENSE).
