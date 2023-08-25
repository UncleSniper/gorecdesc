package gorecdesc

type ResultChannel[ReadT any, OutT any, ExpectT any] chan<- *Result[ReadT, OutT, ExpectT]

type Rule[ReadT any, OutT any, ExpectT any] func(*Reader[ReadT], ResultChannel[ReadT, OutT, ExpectT])

func RunRule[ReadT any, OutT any, ExpectT any](
	rule Rule[ReadT, OutT, ExpectT],
	reader *Reader[ReadT],
) *Result[ReadT, OutT, ExpectT] {
	resultChannel := make(chan *Result[ReadT, OutT, ExpectT])
	go rule(reader, resultChannel)
	return <-resultChannel
}

func MapRule[ReadT any, FromT any, ToT any, ExpectT any](
	formatPacket func(*Packet[ReadT]) string,
	unmapped ExpectT,
	formatUnmapped func(ExpectT) string,
	innerRule Rule[ReadT, FromT, ExpectT],
	mapping func(FromT) ToT,
) Rule[ReadT, ToT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, ToT, ExpectT]) {
		if debugOn {
			debugf("Entering MapRule with Reader %s\n", debugReader(reader))
		}
		var outValue ToT
		var result *Result[ReadT, ToT, ExpectT]
		if innerRule == nil {
			if debugOn {
				debugf("[MapRule with Reader %s] Missing inner rule\n", debugReader(reader))
			}
			if formatUnmapped == nil {
				formatUnmapped = func(ExpectT) string {
					return "<missing mapped rule not formattable>"
				}
			}
			current := reader.Current()
			result = &Result[ReadT, ToT, ExpectT] {
				Offset: current.Offset,
				Result: outValue,
				Error: &SyntaxError[ReadT, ExpectT] {
					Found: current,
					Expected: []ExpectT {unmapped},
					FormatFound: formatPacket,
					FormatExpected: formatUnmapped,
				},
				Reader: reader,
			}
		} else {
			if debugOn {
				debugf("[MapRule with Reader %s] Deletating to inner rule\n", debugReader(reader))
			}
			result = MapResult(RunRule(innerRule, reader), mapping)
		}
		if debugOn {
			debugf("[MapRule with Reader %s] Issuing %s\n", debugReader(reader), debugResult(result))
		}
		resultChannel <- result
		if debugOn {
			debugf("Leaving MapRule with Reader %s\n", debugReader(reader))
		}
	}
}
