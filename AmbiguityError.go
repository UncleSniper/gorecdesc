package gorecdesc

import (
	"strings"
)

type AmbiguityChoice[ReadT any, ExpectT any] struct {
	Structure string
	EndBefore *Packet[ReadT]
}

type AmbiguityError[ReadT any, ExpectT any] struct {
	Structure string
	StartsAt *Packet[ReadT]
	FormatPacket func(*Packet[ReadT]) string
	Choices []AmbiguityChoice[ReadT, ExpectT]
}

func(err *AmbiguityError[ReadT, ExpectT]) Start() *Packet[ReadT] {
	return err.StartsAt
}

func(err *AmbiguityError[ReadT, ExpectT]) Near() *Packet[ReadT] {
	return nil
}

func(err *AmbiguityError[ReadT, ExpectT]) Expectation() []ExpectT {
	return nil
}

func(err *AmbiguityError[ReadT, ExpectT]) OfferStructure(committed Commission, structure string) {
	if len(err.Structure) == 0 {
		err.Structure = structure
	}
}

func(err *AmbiguityError[ReadT, ExpectT]) CommisionAndStructure() (Commission, string) {
	return COM_UNKNOWN, err.Structure
}

func(err *AmbiguityError[ReadT, ExpectT]) SubErrors() []ParseError[ReadT, ExpectT] {
	return nil
}

func(err *AmbiguityError[ReadT, ExpectT]) Error() string {
	var builder strings.Builder
	builder.WriteString("Ambiguity in ")
	if len(err.Structure) == 0 {
		builder.WriteString("grammar")
	} else {
		builder.WriteString(err.Structure)
	}
	var rendition string
	var haveStartingAt bool
	if err.StartsAt != nil && err.FormatPacket != nil {
		rendition = err.FormatPacket(err.StartsAt)
		if len(rendition) > 0 {
			builder.WriteString(" starting at ")
			builder.WriteString(rendition)
			haveStartingAt = true
		}
	}
	allSamePacket := len(err.Choices) > 0
	var samePacket *Packet[ReadT]
	if allSamePacket {
		for _, choice := range err.Choices {
			if choice.EndBefore == nil {
				allSamePacket = false
				break
			}
			if samePacket == nil {
				samePacket = choice.EndBefore
			} else if choice.EndBefore != samePacket {
				allSamePacket = false
				break
			}
		}
	}
	if allSamePacket && err.FormatPacket != nil {
		rendition = err.FormatPacket(samePacket)
		if len(rendition) > 0 {
			if haveStartingAt {
				builder.WriteString(" and")
			}
			builder.WriteString(" ending at ")
			builder.WriteString(rendition)
		}
	}
	if len(err.Choices) > 0 {
		builder.WriteString(": Could be any of: ")
		for index, choice := range err.Choices {
			if index == len(err.Choices) - 1 && len(err.Choices) > 1 {
				builder.WriteString(", or ")
			} else if index > 0 {
				builder.WriteString(", ")
			}
			if len(choice.Structure) > 0 {
				builder.WriteString(choice.Structure)
			} else {
				builder.WriteString("something")
			}
			if !allSamePacket && choice.EndBefore != nil && err.FormatPacket != nil {
				rendition = err.FormatPacket(choice.EndBefore)
				if len(rendition) > 0 {
					builder.WriteString(" ending before ")
					builder.WriteString(rendition)
				}
			}
		}
	}
	return builder.String()
}

var _ ParseError[int, byte] = &AmbiguityError[int, byte]{}
