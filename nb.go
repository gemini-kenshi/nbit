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
	"encoding/json"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// NB is an N-bit bitmask. The zero value (NB{}) behaves as an empty,
// all-zero mask. Callers should not modify the words slice directly.
//
// Aliasing: NB carries a []uint64 by reference. Copying an NB value
// (assignment, function argument, return value) copies the slice header
// but shares the backing array. Mutating one copy via Set, Clear, or Apply
// is therefore visible through every other copy of the same NB. Call Clone
// before mutating if independence is required.
type NB struct {
	words []uint64
}

// Clone returns a deep copy of n with an independent backing array.
// Use it before mutating an NB that was received from a caller, returned
// from a constructor the caller still holds, or otherwise might be aliased.
func (n NB) Clone() NB {
	if len(n.words) == 0 {
		return NB{}
	}
	dup := make([]uint64, len(n.words))
	copy(dup, n.words)
	return NB{words: dup}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// wordsNeeded returns the number of uint64 words required to hold maxBit+1 bits.
// maxBit/64 gives the zero-based word index; +1 converts to a count.
func wordsNeeded(maxBit int) int {
	return maxBit/64 + 1
}

// wordAt returns the word index and bit position within that word for a given
// bit index. Both results are computed with a single shift and mask, O(1).
// Panics if bit is negative — bit indices are always non-negative.
func wordAt(bit int) (w int, pos uint) {
	if bit < 0 {
		panic(fmt.Sprintf("nb: negative bit index %d", bit))
	}
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
// Panics if any bit is negative.
//
//	nb.FromBit(0, 4)     // single word: 0x11
//	nb.FromBit(0, 4, 64) // two words:  0x11, 0x01  (bit 64 starts word[1])
//
// FromBit() with no arguments returns the empty NB{} (no backing slice).
func FromBit(bits ...int) NB {
	if len(bits) == 0 {
		return NB{}
	}

	maxBit := 0
	for _, b := range bits {
		if b < 0 {
			panic(fmt.Sprintf("nb: negative bit index %d", b))
		}
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

// Set sets bit at position bit. The backing slice is grown in a single
// allocation if necessary. Panics if bit is negative.
func (n *NB) Set(bit int) {
	w, pos := wordAt(bit)
	if w >= len(n.words) {
		grown := make([]uint64, wordsNeeded(bit))
		copy(grown, n.words)
		n.words = grown
	}
	n.words[w] |= 1 << pos
}

// Clear clears the bit at position bit. If bit is beyond the current
// capacity it is a no-op (it is already zero). Panics if bit is negative.
func (n *NB) Clear(bit int) {
	w, pos := wordAt(bit)
	if w >= len(n.words) {
		return
	}
	n.words[w] &^= 1 << pos
}

// IsSet reports whether the bit at position bit is set.
// Returns false if bit is beyond the current capacity.
// Panics if bit is negative.
func (n NB) IsSet(bit int) bool {
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
	for i := range minLen {
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
//	a = FromValue(0x11)   // 1 word:  0x11
//	b = FromBit(4, 64)    // 2 words: 0x10, 0x01
//	a.Apply(b)            // a == 0x11 | 0x10 = 0x11 (b's word[1] is dropped)
func (n *NB) Apply(other NB) {
	for i := range min(len(n.words), len(other.words)) {
		n.words[i] |= other.words[i]
	}
}

// ── membership test ───────────────────────────────────────────────────────────

// HasAll reports whether every bit set in mask is also set in n (subset test).
//
// Semantics: "all errors in this event mask are active."
// Returns true if mask is empty (vacuously true). Allocation-free; O(max(len(n), len(mask))).
//
// Typical use — require all bits in a multi-bit condition mask to be active:
//
//	if errBitmap.HasAll(nb.FromValue(errcodes.ErrCriticalPair)) { ... }
func (n NB) HasAll(mask NB) bool {
	for i := range min(len(n.words), len(mask.words)) {
		if n.words[i]&mask.words[i] != mask.words[i] {
			return false
		}
	}
	// Mask bits beyond n's length cannot be set in n.
	for i := len(n.words); i < len(mask.words); i++ {
		if mask.words[i] != 0 {
			return false
		}
	}
	return true
}

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

// setBits returns the positions of all set bits in ascending order.
// O(k) where k = number of set bits. Result is sorted by construction.
func (n NB) setBits() []int {
	var out []int
	for i, w := range n.words {
		for w != 0 {
			pos := bits.TrailingZeros64(w)
			out = append(out, i*64+pos)
			w &= w - 1
		}
	}
	return out
}

// String returns the set bit positions as a sorted, space-separated list
// enclosed in square brackets.
//
//	FromBit(0, 4).String()       →  "[0 4]"
//	FromBit(0, 4, 64).String()   →  "[0 4 64]"
//	NB{}.String()                →  "[]"
func (n NB) String() string {
	b := n.setBits()
	if len(b) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, pos := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(strconv.Itoa(pos))
	}
	sb.WriteByte(']')
	return sb.String()
}

// ── JSON serialization ────────────────────────────────────────────────────────

// MarshalJSON encodes n as a JSON array of set bit positions in ascending order.
// Both NB{} and FromValue(0) marshal to "[]".
//
//	FromBit(0, 4).MarshalJSON()       →  [0,4]
//	FromBit(0, 4, 64).MarshalJSON()   →  [0,4,64]
//
// Round-trip preserves Equal: parsed.Equal(original) is always true.
func (n NB) MarshalJSON() ([]byte, error) {
	b := n.setBits()
	if len(b) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(b)
}

// UnmarshalJSON decodes a JSON array of bit positions into n.
// JSON null and [] both produce the zero-value NB. Replaces any prior content.
// Returns an error for negative bit positions.
func (n *NB) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = NB{}
		return nil
	}
	var positions []int
	if err := json.Unmarshal(data, &positions); err != nil {
		return fmt.Errorf("nb: unmarshal: %w", err)
	}
	*n = NB{}
	for _, pos := range positions {
		if pos < 0 {
			return fmt.Errorf("nb: unmarshal: negative bit position %d", pos)
		}
		n.Set(pos)
	}
	return nil
}
