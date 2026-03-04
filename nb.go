// Package nb provides an N-bit bitmask type backed by a []uint64 slice.
//
// Each uint64 word holds 64 bits; word[0] covers bits 0–63, word[1] covers
// bits 64–127, and so on. This layout is cache-friendly and keeps single-bit
// operations at O(1).
//
// Mutation vs. immutability contract:
//   - Set, Clear, Apply use pointer receivers (*NB) — they mutate the receiver.
//   - All other methods use value receivers (NB) — they return new values or
//     pure read results, leaving the receiver unchanged.
package nb

import (
	"fmt"
	"strings"
)

// NB is an N-bit bitmask. The zero value (NB{}) behaves as an empty,
// all-zero mask. Callers should not modify the words slice directly.
type NB struct {
	words []uint64
}

// ── internal helpers ──────────────────────────────────────────────────────────

// wordsNeeded returns the number of uint64 words required to hold maxBit+1 bits.
// Uses the round-up division idiom: ceil((maxBit + 1) / 64).
func wordsNeeded(maxBit int) int {
	return (maxBit + 64) >> 6
}

// wordAt returns the word index and bit position within that word for a given
// bit index. Both results are computed with a single shift and mask, O(1).
func wordAt(bit int) (w int, pos uint) {
	return bit >> 6, uint(bit & 63)
}

// ── construction ──────────────────────────────────────────────────────────────

// FromValue creates a single-word NB from a uint64 value.
// Bit n of v becomes bit n of the returned NB.
//
//	nb.FromValue(0x11)  // bits 0 and 4 set
func FromValue(v uint64) NB {
	return NB{words: []uint64{v}}
}

// FromBit creates an NB with exactly the specified bits set.
// The backing slice is sized to hold the highest requested bit.
//
//	nb.FromBit(0, 4)     // 0x11
//	nb.FromBit(0, 4, 32) // 0x11, 0x01  (two words)
func FromBit(bits ...int) NB {
	if len(bits) == 0 {
		return NB{words: []uint64{0}}
	}

	maxBit := 0
	for _, b := range bits {
		if b > maxBit {
			maxBit = b
		}
	}

	words := make([]uint64, wordsNeeded(maxBit))
	for _, b := range bits {
		w, pos := wordAt(b)
		words[w] |= 1 << pos
	}
	return NB{words: words}
}

// ── single-bit operations (pointer receivers — mutate in-place) ───────────────

// Set sets bit at position bit. The backing slice is grown if necessary.
func (n *NB) Set(bit int) {
	w, pos := wordAt(bit)
	// Grow if needed.
	for len(n.words) <= w {
		n.words = append(n.words, 0)
	}
	n.words[w] |= 1 << pos
}

// Clear clears the bit at position bit. If bit is beyond the current
// capacity it is a no-op (it is already zero).
func (n *NB) Clear(bit int) {
	w, pos := wordAt(bit)
	if w >= len(n.words) {
		return
	}
	n.words[w] &^= 1 << pos
}

// Test reports whether the bit at position bit is set.
// Returns false if bit is beyond the current capacity.
func (n NB) Test(bit int) bool {
	w, pos := wordAt(bit)
	if w >= len(n.words) {
		return false
	}
	return n.words[w]>>pos&1 == 1
}

// ── comparison (value receivers — pure reads) ─────────────────────────────────

// Equal reports whether n and other represent the same bitset.
// Words beyond the shorter operand are compared against 0.
func (n NB) Equal(other NB) bool {
	for i := 0; i < max(len(n.words), len(other.words)); i++ {
		var a, b uint64
		if i < len(n.words) {
			a = n.words[i]
		}
		if i < len(other.words) {
			b = other.words[i]
		}
		if a != b {
			return false
		}
	}
	return true
}

// IsZero reports whether all bits are zero.
func (n NB) IsZero() bool {
	for _, w := range n.words {
		if w != 0 {
			return false
		}
	}
	return true
}

// ── intersection ──────────────────────────────────────────────────────────────

// Mask returns the bitwise AND of n and other (set intersection).
// The result is bounded to min(len(n), len(other)) words — bits that exist
// in only one operand cannot survive an AND, so no truncation occurs.
func (n NB) Mask(other NB) NB {
	minLen := min(len(n.words), len(other.words))
	res := make([]uint64, minLen)
	for i := 0; i < minLen; i++ {
		res[i] = n.words[i] & other.words[i]
	}
	return NB{words: res}
}

// ── union (commutative OR, returns new NB) ────────────────────────────────────

// Union returns the bitwise OR of n and other (set union).
// Commutative: a.Union(b).Equal(b.Union(a)) is always true.
// The result is expanded to max(len(n), len(other)) words so that no
// bits from either operand are lost.
func (n NB) Union(other NB) NB {
	maxLen := max(len(n.words), len(other.words))
	res := make([]uint64, maxLen)
	for i := range maxLen {
		var a, b uint64
		if i < len(n.words) {
			a = n.words[i]
		}
		if i < len(other.words) {
			b = other.words[i]
		}
		res[i] = a | b
	}
	return NB{words: res}
}

// ── fixed-width overlay (non-commutative, pointer receiver — mutates) ─────────

// Apply overlays the bits of other onto the receiver using bitwise OR, but
// strictly within the receiver's current width. Bits in other that fall
// beyond len(n.words) are silently ignored.
//
// This is intentionally non-commutative: the left operand (receiver) defines
// the valid bit space. Think of it as writing external flags into a fixed-width
// status register — overflow is not an error; it is discarded.
//
//	a = FromValue(0x11)   // 1 word
//	b = FromBit(4, 8)     // 2 words: 0x10, 0x01
//	a.Apply(b)            // a == 0x11 | 0x10 = 0x11 (only word 0 of b used)
func (n *NB) Apply(other NB) {
	for i := 0; i < len(n.words) && i < len(other.words); i++ {
		n.words[i] |= other.words[i]
	}
}

// ── membership test ───────────────────────────────────────────────────────────

// HasAny reports whether any bit set in mask overlaps with n across all words.
//
// Semantics: "at least one error in this event mask is active."
// Returns false if either n or mask is empty. O(min(len(n), len(mask))),
// allocation-free.
//
// Typical use — evaluating a multi-bit error mask against an aggregated error bitmap:
//
//	if errBitmap.HasAny(nb.FromValue(errcodes.ErrOvpWarn1)) { ... }
func (n NB) HasAny(mask NB) bool {
	for i := range min(len(n.words), len(mask.words)) {
		if n.words[i]&mask.words[i] != 0 {
			return true
		}
	}
	return false
}

// ── string representation ─────────────────────────────────────────────────────

// String returns an LSB-first hexadecimal representation of the bitmask.
// Each 64-bit word is formatted as "0x%02x" and joined by ", ".
// word[0] holds bits 0–63 (least significant), word[1] bits 64–127, etc.
//
//	FromBit(0, 4, 32).String()  →  "0x11, 0x01"
//
// Note: bit 32 falls in word 0 (bits 0–63), so the example above produces
// a single-word result 0x11 | (1<<32) = "0x100000011".
//
//	FromBit(0, 4, 64).String()  →  "0x11, 0x01"  (bit 64 starts word[1])
func (n NB) String() string {
	if len(n.words) == 0 {
		return "0x00"
	}
	parts := make([]string, len(n.words))
	for i, w := range n.words {
		parts[i] = fmt.Sprintf("0x%02x", w)
	}
	return strings.Join(parts, ", ")
}
