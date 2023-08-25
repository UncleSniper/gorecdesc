package gorecdesc

import (
	"strings"
)

type SyntaxError[ReadT any, ExpectT any] struct {
	Found *Packet[ReadT]
	Expected []ExpectT
	FormatFound func(*Packet[ReadT]) string
	FormatExpected func(ExpectT) string
	Committed Commission
	Structure string
	ChoiceErrors []ParseError[ReadT, ExpectT]
}

func(err *SyntaxError[ReadT, ExpectT]) Start() *Packet[ReadT] {
	return err.Found
}

func(err *SyntaxError[ReadT, ExpectT]) Near() *Packet[ReadT] {
	return err.Found
}

func(err *SyntaxError[ReadT, ExpectT]) Expectation() []ExpectT {
	return err.Expected
}

func(err *SyntaxError[ReadT, ExpectT]) OfferStructure(committed Commission, structure string) {
	if len(err.Structure) == 0 {
		err.Committed = committed
		err.Structure = structure
	}
}

func(err *SyntaxError[ReadT, ExpectT]) CommisionAndStructure() (Commission, string) {
	return err.Committed, err.Structure
}

func(err *SyntaxError[ReadT, ExpectT]) SubErrors() []ParseError[ReadT, ExpectT] {
	return err.ChoiceErrors
}

func(err *SyntaxError[ReadT, ExpectT]) Error() string {
	var builder strings.Builder
	builder.WriteString("Expected")
	if len(err.Expected) == 0 || err.FormatExpected == nil {
		builder.WriteString("... something")
	} else {
		had := false
		var previousRendition string
		for _, expectation := range err.Expected {
			rendition := err.FormatExpected(expectation)
			if len(rendition) == 0 {
				continue
			}
			if len(previousRendition) > 0 {
				if had {
					builder.WriteString(", ")
				} else {
					builder.WriteString(" ")
					had = true
				}
				builder.WriteString(previousRendition)
			}
			previousRendition = rendition
		}
		if len(previousRendition) > 0 {
			if had {
				builder.WriteString(", or ")
			} else {
				builder.WriteString(" ")
			}
			builder.WriteString(previousRendition)
		}
	}
	if err.Found != nil && err.FormatFound != nil {
		rendition := err.FormatFound(err.Found)
		if len(rendition) > 0 {
			builder.WriteString(" near ")
			builder.WriteString(rendition)
		}
	}
	if len(err.Structure) > 0 {
		switch err.Committed {
			case COM_START:
				builder.WriteString(" to start ")
			case COM_CONTINUE:
				builder.WriteString(" to continue ")
			case COM_COMPLETE:
				builder.WriteString(" to complete ")
			default:
				builder.WriteString(" for ")
		}
		builder.WriteString(err.Structure)
	}
	return builder.String()
}

var _ ParseError[int, byte] = &SyntaxError[int, byte]{}
