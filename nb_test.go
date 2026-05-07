// Package nb white-box tests. The test file lives in package nb (not nb_test)
// so it can inspect the unexported words field to verify internal layout.
// This is intentional: the []uint64 representation is stable and its layout
// is part of the documented contract (LSB-first, word[0] = bits 0–63).
package nb

import (
	"encoding/json"
	"testing"
)

// ── construction ──────────────────────────────────────────────────────────────

func TestFromValue(t *testing.T) {
	n := FromValue(0x11)
	if !n.IsSet(0) {
		t.Error("bit 0 should be set")
	}
	if !n.IsSet(4) {
		t.Error("bit 4 should be set")
	}
	if n.IsSet(1) {
		t.Error("bit 1 should not be set")
	}
}

func TestFromBit(t *testing.T) {
	tests := []struct {
		name string
		bits []int
		want uint64
	}{
		{"single bit 0", []int{0}, 1},
		{"bits 0 and 4", []int{0, 4}, 0x11},
		{"bit 63 (top of word 0)", []int{63}, 1 << 63},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := FromBit(tc.bits...)
			if n.words[0] != tc.want {
				t.Errorf("word[0] = 0x%x, want 0x%x", n.words[0], tc.want)
			}
		})
	}
}

func TestFromBitEmptyIsZeroValue(t *testing.T) {
	n := FromBit()
	if len(n.words) != 0 {
		t.Errorf("FromBit() should return zero-value NB, got %d words", len(n.words))
	}
	if !n.IsZero() {
		t.Error("FromBit() should be zero")
	}
	if !n.Equal(NB{}) {
		t.Error("FromBit() should equal NB{}")
	}
}

func TestFromBitMultiWord(t *testing.T) {
	// bit 64 starts word[1]; bit 0 is in word[0]
	n := FromBit(0, 4, 64)
	if len(n.words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(n.words))
	}
	if n.words[0] != 0x11 {
		t.Errorf("word[0] = 0x%x, want 0x11", n.words[0])
	}
	if n.words[1] != 1 {
		t.Errorf("word[1] = 0x%x, want 0x01", n.words[1])
	}
}

func TestFromValueAndFromBitEquivalence(t *testing.T) {
	x := FromBit(0, 4)
	y := FromValue(17) // 0x11 = bit 0 + bit 4
	if !x.Equal(y) {
		t.Errorf("FromBit(0,4) != FromValue(17): %s vs %s", x, y)
	}
}

// ── single-bit operations ─────────────────────────────────────────────────────

func TestSetClearIsSet(t *testing.T) {
	n := FromValue(0)
	n.Set(3)
	if !n.IsSet(3) {
		t.Error("bit 3 should be set after Set(3)")
	}
	n.Clear(3)
	if n.IsSet(3) {
		t.Error("bit 3 should be cleared after Clear(3)")
	}
}

func TestSetGrowsSlice(t *testing.T) {
	// Start with a single-word NB, Set a bit in word[1].
	n := FromValue(0)
	n.Set(64) // word[1], bit 0
	if len(n.words) != 2 {
		t.Fatalf("expected 2 words after Set(64), got %d", len(n.words))
	}
	if !n.IsSet(64) {
		t.Error("bit 64 should be set")
	}
}

func TestClearOutOfBoundsIsNoop(t *testing.T) {
	n := FromValue(0x11)
	n.Clear(200) // far beyond capacity — must not panic
	if !n.IsSet(0) || !n.IsSet(4) {
		t.Error("existing bits should be unchanged after out-of-bounds Clear")
	}
}

func TestIsSetOutOfBoundsReturnsFalse(t *testing.T) {
	n := FromValue(0x11)
	if n.IsSet(200) {
		t.Error("IsSet(200) should return false for a single-word NB")
	}
}

// ── comparison ────────────────────────────────────────────────────────────────

func TestIsZero(t *testing.T) {
	if !FromValue(0).IsZero() {
		t.Error("FromValue(0) should be zero")
	}
	if FromValue(1).IsZero() {
		t.Error("FromValue(1) should not be zero")
	}
	if !(NB{}).IsZero() {
		t.Error("zero-value NB{} should be zero")
	}
}

func TestEqualSameLength(t *testing.T) {
	a := FromValue(0x11)
	b := FromValue(0x11)
	if !a.Equal(b) {
		t.Error("identical values should be equal")
	}
	c := FromValue(0x12)
	if a.Equal(c) {
		t.Error("different values should not be equal")
	}
}

func TestEqualDifferentLength(t *testing.T) {
	// Two-word NB with word[1]==0 should equal a one-word NB of same value.
	a := FromBit(0, 4)       // 1 word: 0x11
	b := FromBit(0, 4, 64)   // 2 words: 0x11, 0x01
	if a.Equal(b) {
		t.Error("masks with different set bits should not be equal")
	}

	// A two-word NB where word[1] is zero should equal a one-word NB.
	n := FromValue(0x11)
	n.Set(0) // already set; no new words added
	m := FromBit(0, 4)
	if !n.Equal(m) {
		t.Errorf("same logical value with different backing lengths should be equal: %s vs %s", n, m)
	}
}

// ── Mask (AND) ────────────────────────────────────────────────────────────────

func TestMaskSameLength(t *testing.T) {
	a := FromValue(0xFF)
	b := FromValue(0x0F)
	got := a.Mask(b)
	if got.words[0] != 0x0F {
		t.Errorf("Mask result = 0x%x, want 0x0f", got.words[0])
	}
}

func TestMaskDifferentLength(t *testing.T) {
	// a is 1 word, b is 2 words; result should have min(1,2)=1 word.
	a := FromValue(0xFF)
	b := FromBit(0, 1, 64) // 2 words
	got := a.Mask(b)
	if len(got.words) != 1 {
		t.Errorf("Mask result has %d words, want 1", len(got.words))
	}
	if got.words[0] != 0x03 { // only bits 0,1 survive (both set in a and b's word[0])
		t.Errorf("Mask result = 0x%x, want 0x03", got.words[0])
	}
}

func TestMaskSubsetCheck(t *testing.T) {
	// Classic: if (x & y) == y then y ⊆ x.
	x := FromBit(0, 4, 8)
	y := FromBit(0, 4)
	if !x.Mask(y).Equal(y) {
		t.Error("y should be a subset of x")
	}
}

func TestMaskZeroOverlap(t *testing.T) {
	a := FromBit(0)
	b := FromBit(1)
	if !a.Mask(b).IsZero() {
		t.Error("disjoint masks should produce zero")
	}
}

// ── Union (commutative OR) ────────────────────────────────────────────────────

func TestUnionExpands(t *testing.T) {
	a := FromValue(0x11)        // 1 word
	b := FromBit(0, 4, 64)     // 2 words
	got := a.Union(b)
	if len(got.words) != 2 {
		t.Errorf("Union result has %d words, want 2", len(got.words))
	}
	if got.words[1] != 1 {
		t.Errorf("word[1] = 0x%x, want 0x01", got.words[1])
	}
}

func TestUnionCommutative(t *testing.T) {
	a := FromBit(0, 4)      // 1 word: 0x11
	b := FromBit(4, 8, 64)  // 2 words

	ab := a.Union(b)
	ba := b.Union(a)

	if !ab.Equal(ba) {
		t.Errorf("Union is not commutative: a|b=%s, b|a=%s", ab, ba)
	}
}

func TestUnionDoesNotMutateOperands(t *testing.T) {
	a := FromValue(0x01)
	b := FromValue(0x02)
	_ = a.Union(b)
	if a.words[0] != 0x01 {
		t.Error("Union should not mutate the receiver")
	}
	if b.words[0] != 0x02 {
		t.Error("Union should not mutate the argument")
	}
}

// ── Apply (non-commutative fixed-width OR) ────────────────────────────────────

func TestApplyWithinBounds(t *testing.T) {
	a := FromValue(0x01)
	b := FromValue(0x02)
	a.Apply(b)
	if a.words[0] != 0x03 {
		t.Errorf("Apply result = 0x%x, want 0x03", a.words[0])
	}
}

func TestApplyTruncatesOther(t *testing.T) {
	// a is 1 word; b is 2 words. Bits from b's word[1] must be ignored.
	a := FromValue(0x01)           // 1 word
	b := FromBit(1, 64)            // 2 words: 0x02, 0x01
	a.Apply(b)
	if len(a.words) != 1 {
		t.Errorf("Apply grew receiver to %d words, want 1", len(a.words))
	}
	if a.words[0] != 0x03 { // bit 0 | bit 1
		t.Errorf("Apply result = 0x%x, want 0x03", a.words[0])
	}
}

func TestApplyIsNonCommutative(t *testing.T) {
	// Demonstrate the documented non-commutativity.
	// a=0x11 (1 word), b=FromBit(4,8) (1 word, 2 words?)
	// Use different sizes to make the distinction clear.
	a := FromValue(0x11) // 1 word
	b := FromBit(4, 64)  // 2 words: 0x10, 0x01

	// a.Apply(b): a gets bit 4 OR'd in (from b's word[0]=0x10), bit 64 ignored.
	ac := FromValue(a.words[0]) // copy
	ac.Apply(b)
	// a result: 0x11 | 0x10 = 0x11 (bit 4 was already set)

	// b.Apply(a): b gets bits from a's word[0]=0x11, nothing from word[1].
	bc := FromBit(4, 64) // copy
	bc.Apply(a)
	// bc result word[0]: 0x10 | 0x11 = 0x11; word[1]: 0x01 (unchanged)

	// The two results have different lengths, so they cannot be equal.
	if ac.Equal(bc) {
		t.Error("Apply should not be commutative when operands differ in size")
	}
	if len(ac.words) != 1 {
		t.Errorf("ac should have 1 word (receiver width), got %d", len(ac.words))
	}
	if len(bc.words) != 2 {
		t.Errorf("bc should have 2 words (receiver width), got %d", len(bc.words))
	}
}

// ── String ────────────────────────────────────────────────────────────────────

func TestStringSingleWord(t *testing.T) {
	n := FromBit(0, 4)
	got := n.String()
	if got != "[0 4]" {
		t.Errorf("String() = %q, want %q", got, "[0 4]")
	}
}

func TestStringMultiWord(t *testing.T) {
	// bit 64 starts word[1]; bits 0 and 4 are in word[0]
	n := FromBit(0, 4, 64)
	got := n.String()
	want := "[0 4 64]"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	// bit 128 starts word[2]; word[1] is zero — no gap appears in output
	sparse := FromBit(0, 128)
	if got := sparse.String(); got != "[0 128]" {
		t.Errorf("String() sparse = %q, want %q", got, "[0 128]")
	}
}

func TestStringZeroNB(t *testing.T) {
	n := NB{}
	if n.String() != "[]" {
		t.Errorf("String() of empty NB = %q, want %q", n.String(), "[]")
	}
}

// ── Union vs Apply demonstration ──────────────────────────────────────────────

// ── HasAny ────────────────────────────────────────────────────────────────────

func TestHasAny(t *testing.T) {
	tests := []struct {
		name string
		n    NB
		mask NB
		want bool
	}{
		{"empty NB, any mask", NB{}, FromValue(0xFF), false},
		{"zero word, any mask", FromValue(0), FromValue(0xFF), false},
		{"mask zero always false", FromValue(0xFF), FromValue(0), false},
		{"single matching bit", FromBit(3), FromBit(3), true},
		{"no overlap", FromValue(0x0F), FromValue(0xF0), false},
		{"partial overlap", FromValue(0x11), FromValue(0x01), true},
		{"multi-bit mask, one matches", FromValue(0x10), FromValue(0x11), true},
		{"multi-bit mask, none match", FromValue(0x04), FromValue(0x11), false},
		{"two-word NB, mask hits word[0]", FromBit(0, 64), FromValue(0x01), true},
		{"two-word NB, mask hits word[1]", FromBit(0, 64), FromBit(64), true},
		{"two-word mask on one-word NB", FromValue(0x01), FromBit(0, 64), true},
		{"empty mask on non-empty NB", FromValue(0xFF), NB{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.n.HasAny(tc.mask)
			if got != tc.want {
				t.Errorf("HasAny(%s) = %v, want %v", tc.mask, got, tc.want)
			}
		})
	}
}

// ── HasAll ────────────────────────────────────────────────────────────────────

func TestHasAll(t *testing.T) {
	tests := []struct {
		name string
		n    NB
		mask NB
		want bool
	}{
		{"empty mask always true (vacuous)", FromValue(0xFF), NB{}, true},
		{"empty NB, non-empty mask", NB{}, FromValue(0xFF), false},
		{"exact match", FromBit(0, 4), FromBit(0, 4), true},
		{"superset: n has more bits", FromBit(0, 4, 8), FromBit(0, 4), true},
		{"subset: mask has extra bit", FromBit(0, 4), FromBit(0, 4, 8), false},
		{"no overlap", FromValue(0x0F), FromValue(0xF0), false},
		{"mask hits word[1], n is 1-word", FromValue(0xFF), FromBit(0, 64), false},
		{"both 2-word, all match", FromBit(0, 4, 64), FromBit(0, 64), true},
		{"both 2-word, word[1] mismatch", FromBit(0, 64), FromBit(0, 65), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.n.HasAll(tc.mask)
			if got != tc.want {
				t.Errorf("HasAll(%s) = %v, want %v", tc.mask, got, tc.want)
			}
		})
	}
}

// TestUnionVsApply is an executable demonstration of the conversation's
// key design decision: Union is commutative and expanding; Apply is
// non-commutative and fixed-width.
func TestUnionVsApply(t *testing.T) {
	// a = 0x11 (1 word), b = bits {4, 8} over 2 words
	a := FromValue(0x11)        // word[0]=0x11
	b := FromBit(4, 8, 64)     // word[0]=0x110, word[1]=0x01

	// Union: commutative, result has max(1,2)=2 words.
	ab := a.Union(b)
	ba := b.Union(a)
	if !ab.Equal(ba) {
		t.Errorf("Union must be commutative: %s != %s", ab, ba)
	}
	if len(ab.words) != 2 {
		t.Errorf("Union result should have 2 words, got %d", len(ab.words))
	}

	// Apply: non-commutative.
	// a.Apply(b) keeps a's 1-word width; ignores b's word[1].
	aCopy := FromValue(0x11)
	aCopy.Apply(b)
	if len(aCopy.words) != 1 {
		t.Errorf("a.Apply(b) must not grow a: got %d words", len(aCopy.words))
	}

	// b.Apply(a) keeps b's 2-word width.
	bCopy := FromBit(4, 8, 64)
	bCopy.Apply(a)
	if len(bCopy.words) != 2 {
		t.Errorf("b.Apply(a) must keep b's width: got %d words", len(bCopy.words))
	}
}

// ── Clone & aliasing ──────────────────────────────────────────────────────────

func TestCloneIndependence(t *testing.T) {
	a := FromValue(0x11)
	b := a.Clone()
	b.Set(8)
	if a.IsSet(8) {
		t.Error("Clone should not share backing storage with the source")
	}
	if !b.IsSet(8) {
		t.Error("Clone copy should reflect its own mutations")
	}
}

func TestCloneOfEmptyNB(t *testing.T) {
	if !(NB{}).Clone().Equal(NB{}) {
		t.Error("Clone of zero-value NB should equal NB{}")
	}
	if !FromBit().Clone().IsZero() {
		t.Error("Clone of empty FromBit() should be zero")
	}
}

func TestValueCopyAliasesBacking(t *testing.T) {
	// This test pins down the documented aliasing rule. If this ever
	// changes (e.g. NB switches to copy-on-write), update the docs.
	a := FromValue(0x01)
	b := a // shallow copy; shares backing array
	(&b).Set(1)
	if !a.IsSet(1) {
		t.Error("aliasing contract: mutation through a copy should be visible on the source until Clone is used")
	}

	// And Clone breaks the aliasing.
	c := a.Clone()
	(&c).Set(8)
	if a.IsSet(8) {
		t.Error("Clone must break aliasing; source must not see post-clone mutations")
	}
}

// ── negative-bit panics ───────────────────────────────────────────────────────

func TestNegativeBitPanics(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"FromBit", func() { FromBit(-1) }},
		{"Set", func() { n := FromValue(0); n.Set(-1) }},
		{"Clear", func() { n := FromValue(0); n.Clear(-1) }},
		{"IsSet", func() { _ = FromValue(0).IsSet(-1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s with negative bit should panic", tc.name)
				}
			}()
			tc.fn()
		})
	}
}

// ── Set growth single-allocation sanity ───────────────────────────────────────

func TestSetGrowsToExactWordIndex(t *testing.T) {
	// After Set on a far-away bit, len(words) must equal w+1 exactly.
	n := FromValue(0)
	n.Set(200) // word index 3 (200 >> 6 == 3)
	if len(n.words) != 4 {
		t.Errorf("expected len==4 after Set(200), got %d", len(n.words))
	}
	if !n.IsSet(200) {
		t.Error("bit 200 should be set")
	}
	// Earlier words must remain zero.
	for i := 0; i < 3; i++ {
		if n.words[i] != 0 {
			t.Errorf("word[%d] should be 0, got 0x%x", i, n.words[i])
		}
	}
}

// ── JSON marshaling ───────────────────────────────────────────────────────────

func TestJSONRoundTrip(t *testing.T) {
	trailing := FromBit(0, 64)
	trailing.Clear(64) // word[1] becomes zero; should be stripped on marshal

	cases := []struct {
		name string
		n    NB
	}{
		{"zero value", NB{}},
		{"FromValue(0)", FromValue(0)},
		{"FromValue(0x11)", FromValue(0x11)},
		{"two-word", FromBit(0, 4, 64)},
		{"high bit in word 0", FromBit(63)},
		{"three-word", func() NB { n := NB{}; n.Set(200); return n }()},
		{"trailing zero stripped", trailing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.n)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got NB
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !got.Equal(tc.n) {
				t.Errorf("round-trip: got %s, want %s", got, tc.n)
			}
		})
	}
}

func TestJSONMarshalShape(t *testing.T) {
	cases := []struct {
		n    NB
		want string
	}{
		{NB{}, "[]"},
		{FromBit(0, 4), "[0,4]"},
		{FromBit(0, 4, 64), "[0,4,64]"},
	}
	for _, tc := range cases {
		data, err := json.Marshal(tc.n)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", tc.n, err)
		}
		if string(data) != tc.want {
			t.Errorf("Marshal(%s) = %s, want %s", tc.n, data, tc.want)
		}
	}
}

func TestJSONUnmarshalNull(t *testing.T) {
	var n NB
	if err := json.Unmarshal([]byte("null"), &n); err != nil {
		t.Fatalf("Unmarshal(null): %v", err)
	}
	if !n.IsZero() {
		t.Errorf("Unmarshal(null) should produce zero NB, got %s", n)
	}
}

func TestJSONUnmarshalReplaces(t *testing.T) {
	n := FromValue(0xFF)
	if err := json.Unmarshal([]byte("[0]"), &n); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !n.Equal(FromBit(0)) {
		t.Errorf("Unmarshal should replace prior content, got %s", n)
	}
}

func TestJSONUnmarshalRejectsGarbage(t *testing.T) {
	cases := []string{`"abc"`, `{}`, `["x"]`, `[-1]`}
	for _, input := range cases {
		var n NB
		if err := json.Unmarshal([]byte(input), &n); err == nil {
			t.Errorf("Unmarshal(%s) should return error", input)
		}
	}
}

func TestJSONInsideStruct(t *testing.T) {
	type Report struct {
		ErrBitmap NB `json:"errBitmap"`
	}
	original := Report{ErrBitmap: FromBit(0, 4, 64)}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.ErrBitmap.Equal(original.ErrBitmap) {
		t.Errorf("struct round-trip: got %s, want %s", got.ErrBitmap, original.ErrBitmap)
	}
}
