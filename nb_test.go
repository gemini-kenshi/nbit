package nb

import (
	"testing"
)

// ── construction ──────────────────────────────────────────────────────────────

func TestFromValue(t *testing.T) {
	n := FromValue(0x11)
	if !n.Test(0) {
		t.Error("bit 0 should be set")
	}
	if !n.Test(4) {
		t.Error("bit 4 should be set")
	}
	if n.Test(1) {
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
		{"empty", []int{}, 0},
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

func TestSetClearTest(t *testing.T) {
	n := FromValue(0)
	n.Set(3)
	if !n.Test(3) {
		t.Error("bit 3 should be set after Set(3)")
	}
	n.Clear(3)
	if n.Test(3) {
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
	if !n.Test(64) {
		t.Error("bit 64 should be set")
	}
}

func TestClearOutOfBoundsIsNoop(t *testing.T) {
	n := FromValue(0x11)
	n.Clear(200) // far beyond capacity — must not panic
	if !n.Test(0) || !n.Test(4) {
		t.Error("existing bits should be unchanged after out-of-bounds Clear")
	}
}

func TestTestOutOfBoundsReturnsFalse(t *testing.T) {
	n := FromValue(0x11)
	if n.Test(200) {
		t.Error("Test(200) should return false for a single-word NB")
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
	n := FromBit(0, 4) // 0x11
	got := n.String()
	if got != "0x11" {
		t.Errorf("String() = %q, want %q", got, "0x11")
	}
}

func TestStringMultiWord(t *testing.T) {
	// bit 64 → word[1]; bits 0,4 → word[0]
	n := FromBit(0, 4, 64)
	got := n.String()
	want := "0x11, 0x01"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStringZeroNB(t *testing.T) {
	n := NB{}
	if n.String() != "0x00" {
		t.Errorf("String() of empty NB = %q, want %q", n.String(), "0x00")
	}
}

// ── Union vs Apply demonstration ──────────────────────────────────────────────

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
