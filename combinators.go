package gorecdesc

func Sequence[ReadT any, AccumulatorT any, PieceT any, ExpectT any](
	initAccu InitAccu[AccumulatorT],
	combineAccu CombineAccu[AccumulatorT, PieceT],
	children ...Rule[ReadT, PieceT, ExpectT],
) Rule[ReadT, AccumulatorT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, AccumulatorT, ExpectT]) {
		if debugOn {
			debugf("Entering Sequence with %d children with Reader %s\n", len(children), debugReader(reader))
		}
		var accumulator AccumulatorT
		if initAccu != nil {
			accumulator = initAccu()
		}
		if debugOn {
			debugf("[Sequence with Reader %s] Initial accumulator = %+v\n", debugReader(reader), accumulator)
		}
		childResultChannel := make(chan *Result[ReadT, PieceT, ExpectT])
		for childIndex, child := range children {
			if child == nil {
				if debugOn {
					debugf(
						"[Sequence with Reader %s] Skipping child %d since the Rule is nil\n",
						debugReader(reader),
						childIndex,
					)
				}
				continue
			}
			if debugOn {
				debugf(
					"[Sequence with Reader %s] Launching goroutine for child %d\n",
					debugReader(reader),
					childIndex,
				)
			}
			go child(reader, childResultChannel)
			childResult := <-childResultChannel
			if debugOn {
				debugf(
					"[Sequence with Reader %s] Received result = %s from child %d\n",
					debugReader(reader),
					debugResult(childResult),
					childIndex,
				)
			}
			if childResult.Error != nil {
				outResult := SubstResult[ReadT, PieceT, AccumulatorT, ExpectT](childResult, accumulator)
				if debugOn {
					debugf(
						"[Sequence with Reader %s] Child %d returned error, propagating result = %s\n",
						debugReader(reader),
						childIndex,
						debugResult(outResult),
					)
				}
				resultChannel <- outResult
				return
			}
			if combineAccu != nil {
				accumulator = combineAccu(accumulator, childResult.Result)
				if debugOn {
					debugf(
						"[Sequence with Reader %s] Updated accumulator to %+v\n",
						debugReader(reader),
						accumulator,
					)
				}
			}
			reader = childResult.Reader
		}
		result := &Result[ReadT, AccumulatorT, ExpectT] {
			Offset: reader.current.Offset,
			Result: accumulator,
			Reader: reader,
		}
		if debugOn {
			debugf("Leaving Sequence with Reader %s with result = %s\n", debugReader(reader), debugResult(result))
		}
		resultChannel <- result
	}
}

func Choice[ReadT any, OutT any, ExpectT any](
	structure string,
	formatPacket func(*Packet[ReadT]) string,
	noChoice ExpectT,
	formatNoChoice func(ExpectT) string,
	compareExpect func(ExpectT, ExpectT) bool,
	formatExpected func(ExpectT) string,
	choices ...Rule[ReadT, OutT, ExpectT],
) Rule[ReadT, OutT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, OutT, ExpectT]) {
		if debugOn {
			debugf(
				"Entering Choice for structure '%s' with %d children with Reader %s\n",
				structure,
				len(choices),
				debugReader(reader),
			)
		}
		choiceCount := 0
		var lowestChoiceIndex int
		startPacket := reader.Current()
		var parallel Parallel[ReadT, OutT, ExpectT]
		for index, choice := range choices {
			if choice == nil {
				if debugOn {
					debugf("[Choice with Reader %s] Choice %d is nil, skipping it\n", debugReader(reader), index)
				}
				continue
			}
			choiceCount++
			if choiceCount == 1 {
				lowestChoiceIndex = index
			} else {
				split := reader.Split()
				if debugOn {
					debugf(
						"[Choice with Reader %s] Adding split reader %s to Parallel for choice %d\n",
						debugReader(reader),
						debugReader(split),
						index,
					)
				}
				parallel.Add(split, choice)
			}
		}
		if debugOn {
			debugf("[Choice with Reader %s] Non-nil choice count = %d\n", debugReader(reader), choiceCount)
		}
		if choiceCount == 0 {
			reader.Acknowledge(ACK_UNSUBSCRIBE_ON_ERROR)
			if formatNoChoice == nil {
				formatNoChoice = func(ExpectT) string {
					return "one of zero choices"
				}
			}
			result := &Result[ReadT, OutT, ExpectT] {
				Offset: startPacket.Offset,
				Structure: structure,
				Error: &SyntaxError[ReadT, ExpectT] {
					Found: startPacket,
					Expected: []ExpectT {noChoice},
					FormatExpected: formatNoChoice,
					Structure: structure,
				},
				Reader: reader,
			}
			if debugOn {
				debugf(
					"[Choice with Reader %s] No choices given, issuing ACK_UNSUBSCRIBE_ON_ERROR with result = %s\n",
					debugReader(reader),
					debugResult(result),
				)
			}
			resultChannel <- result
			return
		}
		if debugOn {
			debugf(
				"[Choice with Reader %s] Adding original reader to Parallel for choice %d\n",
				debugReader(reader),
				lowestChoiceIndex,
			)
		}
		parallel.Add(reader, choices[lowestChoiceIndex])
		results := parallel.Await()
		if debugOn {
			debugf(
				"[Choice with Reader %s] Parallel.Await() returned results: %s\n",
				debugReader(reader),
				debugResultList(results),
			)
		}
		var positiveResults, negativeResults []*Result[ReadT, OutT, ExpectT]
		var maxPositiveOffset, maxNegativeOffset uint64
		var maxPositiveIndex, maxNegativeIndex int
		for resultIndex, result := range results {
			if result.Error == nil {
				if positiveResults == nil {
					maxPositiveOffset = result.Offset
					maxPositiveIndex = resultIndex
				} else if result.Offset > maxPositiveOffset {
					positiveResults = nil
					maxPositiveOffset = result.Offset
					maxPositiveIndex = resultIndex
				}
				if result.Offset >= maxPositiveOffset {
					positiveResults = append(positiveResults, result)
				}
			} else {
				if negativeResults == nil || result.Offset > maxNegativeOffset {
					maxNegativeOffset = result.Offset
					maxNegativeIndex = resultIndex
				}
				negativeResults = append(negativeResults, result)
			}
		}
		if debugOn {
			debugf(
				"[Choice with Reader %s] Categorized results: positiveResults = %s, negativeResults = %s, " +
						"maxPositiveOffset = %d, maxNegativeOffset = %d, " +
						"maxPositiveIndex = %d, maxNegativeIndex = %d\n",
				debugReader(reader),
				debugResultList(positiveResults),
				debugResultList(negativeResults),
				maxPositiveOffset,
				maxNegativeOffset,
				maxPositiveIndex,
				maxNegativeIndex,
			)
		}
		switch len(positiveResults) {
			case 0:
				// consider negative results below
				if debugOn {
					debugf(
						"[Choice with Reader %s] No positive results, yielding to negative result handling\n",
						debugReader(reader),
					)
				}
			case 1:
				// there is a one true result; close short readers
				for resultIndex, result := range positiveResults {
					if result.Offset < maxPositiveOffset {
						if debugOn {
							debugf(
								"[Choice with Reader %s] One true result detected; issuing " +
										"ACK_UNSUBSCRIBE_ON_SUCCESS to reader %s of positive result %d " +
										"because its offset %d < maximum %d\n",
								debugReader(reader),
								debugReader(result.Reader),
								resultIndex,
								result.Offset,
								maxPositiveOffset,
							)
						}
						result.Reader.Acknowledge(ACK_UNSUBSCRIBE_ON_SUCCESS)
					}
				}
				// the reader of that result is the only one still live
				if debugOn {
					debugf(
						"Leaving Choice with Reader %s with result %s of one true choice %d\n",
						debugReader(reader),
						debugResult(positiveResults[0]),
						maxPositiveIndex,
					)
				}
				resultChannel <- positiveResults[0]
				return
			default:
				// rule is ambiguous
				ambChoices := make([]AmbiguityChoice[ReadT, ExpectT], len(positiveResults))
				for resultIndex, result := range positiveResults {
					_, childStructure := result.Error.CommisionAndStructure()
					ambChoices[resultIndex] = AmbiguityChoice[ReadT, ExpectT] {
						Structure: childStructure,
						EndBefore: result.Reader.Current(),
					}
					result.Reader.Acknowledge(ACK_UNSUBSCRIBE_ON_SUCCESS)
				}
				if debugOn {
					debugf(
						"[Choice with Reader %s] Ambiguity detected: %s\n",
						debugReader(reader),
						debugAmbiguityChoiceList(ambChoices),
					)
				}
				result := &Result[ReadT, OutT, ExpectT] {
					Offset: maxPositiveOffset,
					Structure: structure,
					Error: &AmbiguityError[ReadT, ExpectT] {
						Structure: structure,
						StartsAt: startPacket,
						FormatPacket: formatPacket,
						Choices: ambChoices,
					},
					Reader: results[maxPositiveIndex].Reader,
				}
				if debugOn {
					debugf(
						"[Choice with Reader %s] Resolving ambiguity by issuing result: %s\n",
						debugReader(reader),
						debugResult(result),
					)
				}
				resultChannel <- result
				return
		}
		if len(negativeResults) == 1 {
			if debugOn {
				debugf(
					"[Choice with Reader %s] Propagating one true negative result %s of choice %d\n",
					debugReader(reader),
					debugResult(negativeResults[0]),
					maxNegativeIndex,
				)
			}
			resultChannel <- negativeResults[0]
		} else {
			errors := make([]ParseError[ReadT, ExpectT], len(negativeResults))
			var expectations [][]ExpectT
			for resultIndex, result := range positiveResults {
				errors[resultIndex] = result.Error
				expected := result.Error.Expectation()
				if len(expected) > 0 {
					expectations = append(expectations, expected)
				}
			}
			maxResult := results[maxNegativeIndex]
			result := &Result[ReadT, OutT, ExpectT] {
				Offset: maxNegativeOffset,
				Result: maxResult.Result,
				Structure: structure,
				Error: &SyntaxError[ReadT, ExpectT] {
					Found: maxResult.Reader.Current(),
					Expected: MergeExpectations[ExpectT](compareExpect, expectations...),
					FormatFound: formatPacket,
					FormatExpected: formatExpected,
					Structure: structure,
					ChoiceErrors: errors,
				},
				Reader: maxResult.Reader,
			}
			if debugOn {
				debugf(
					"[Choice with Reader %s] No positive but %d negative results; issuing overall result %s\n",
					debugReader(reader),
					len(negativeResults),
					debugResult(result),
				)
			}
			resultChannel <- result
		}
	}
}
