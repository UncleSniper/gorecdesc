package gorecdesc

type Commission uint

const (
	COM_UNKNOWN Commission = iota
	COM_START
	COM_CONTINUE
	COM_COMPLETE
)

type ParseError[ReadT any, ExpectT any] interface {
	error
	Start() *Packet[ReadT]
	Near() *Packet[ReadT]
	Expectation() []ExpectT
	OfferStructure(Commission, string)
	CommisionAndStructure() (Commission, string)
	SubErrors() []ParseError[ReadT, ExpectT]
}

func MergeExpectations[ExpectT any](
	compareExpect func(ExpectT, ExpectT) bool,
	subsets ...[]ExpectT,
) []ExpectT {
	if len(subsets) == 0 {
		return nil
	}
	merged := make([]ExpectT, len(subsets[0]))
	for subsetIndex, subset := range subsets {
		if subsetIndex == 0 {
			for expectIndex, expect := range subset {
				merged[expectIndex] = expect
			}
		} else {
			if compareExpect == nil {
				merged = append(merged, subset...)
			} else {
				for _, expect := range subset {
					have := false
					for _, existing := range merged {
						if compareExpect(expect, existing) {
							have = true
							break
						}
					}
					if !have {
						merged = append(merged, expect)
					}
				}
			}
		}
	}
	return merged
}
