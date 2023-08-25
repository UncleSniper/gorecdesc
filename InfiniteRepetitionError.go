package gorecdesc

import (
	"strings"
)

type InfiniteRepetitionError[ReadT any, ExpectT any] struct {
	Found *Packet[ReadT]
	FormatFound func(*Packet[ReadT]) string
	Structure string
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) Start() *Packet[ReadT] {
	return err.Found
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) Near() *Packet[ReadT] {
	return err.Found
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) Expectation() []ExpectT {
	return nil
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) OfferStructure(committed Commission, structure string) {
	if len(err.Structure) == 0 {
		err.Structure = structure
	}
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) CommisionAndStructure() (Commission, string) {
	return COM_UNKNOWN, err.Structure
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) SubErrors() []ParseError[ReadT, ExpectT] {
	return nil
}

func(err *InfiniteRepetitionError[ReadT, ExpectT]) Error() string {
	var builder strings.Builder
	builder.WriteString("Repetition")
	if len(err.Structure) > 0 {
		builder.WriteString(" in ")
		builder.WriteString(err.Structure)
	}
	if err.Found != nil && err.FormatFound != nil {
		rendition := err.FormatFound(err.Found)
		if len(rendition) > 0 {
			builder.WriteString(" near ")
			builder.WriteString(rendition)
		}
	}
	builder.WriteString(" would be infinite: Iteration consumed no packets but did not fail, either")
	return builder.String()
}

var _ ParseError[int, byte] = &InfiniteRepetitionError[int, byte]{}
