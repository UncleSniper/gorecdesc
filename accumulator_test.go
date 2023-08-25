package gorecdesc

import (
	tst "testing"
	. "github.com/UncleSniper/gotest"
)

func TestThe(t *tst.T) {
	c := Use(t)
	the := The(42)
	AssertThat(c, the()).Is(EqualTo(42))
}

func TestBypassAccu(t *tst.T) {
	c := Use(t)
	var theString string
	sink := func(s string) {
		theString = s
	}
	bypasser := BypassAccu[int, string](sink)
	accu := bypasser(5, "foo")
	AssertThat(c, accu).Is(EqualTo(5))
	AssertThat(c, theString).Is(EqualTo("foo"))
}

func TestBypassBiAccu(t *tst.T) {
	c := Use(t)
	var theLeft int
	var theRight uint64
	sink := func(left int, right uint64) {
		theLeft = left
		theRight = right
	}
	bypasser := BypassBiAccu[string, int, uint64](sink)
	accu := bypasser("foo", 42, 123)
	AssertThat(c, accu).Is(EqualTo("foo"))
	AssertThat(c, theLeft).Is(EqualTo(42))
	AssertThat(c, theRight).Is(EqualTo[uint64](123))
}
