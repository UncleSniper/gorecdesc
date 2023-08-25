package gorecdesc

func EmptySequence[ReadT any, OutT any, ExpectT any](returnValue OutT) Rule[ReadT, OutT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, OutT, ExpectT]) {
		result := &Result[ReadT, OutT, ExpectT] {
			Offset: reader.current.Offset,
			Result: returnValue,
			Reader: reader,
		}
		resultChannel <- result
	}
}

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
				for resultIndex, result := range results {
					if result.Error == nil && result.Offset < maxPositiveOffset {
						if debugOn {
							debugf(
								"[Choice with Reader %s] One true result detected; issuing " +
										"ACK_UNSUBSCRIBE_ON_SUCCESS to reader %s of result %d " +
										"because its offset %d < maximum %d\n",
								debugReader(reader),
								debugReader(result.Reader),
								resultIndex,
								result.Offset,
								maxPositiveOffset,
							)
						}
						result.Reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
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
				var ambChoices []AmbiguityChoice[ReadT, ExpectT]
				for _, result := range results {
					if result.Error != nil {
						continue
					}
					_, childStructure := result.Error.CommisionAndStructure()
					if result.Offset == maxPositiveOffset {
						ambChoices = append(ambChoices, AmbiguityChoice[ReadT, ExpectT] {
							Structure: childStructure,
							EndBefore: result.Reader.Current(),
						})
					}
					result.Reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
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

func Repetition[ReadT any, AccumulatorT any, ItemT any, SeparatorT any, ExpectT any](
	formatPacket func(*Packet[ReadT]) string,
	initAccu InitAccu[AccumulatorT],
	combineAccu CombineBiAccu[AccumulatorT, SeparatorT, ItemT],
	noItem ExpectT,
	formatNoItem func(ExpectT) string,
	itemRule Rule[ReadT, ItemT, ExpectT],
	separatorRule Rule[ReadT, SeparatorT, ExpectT],
	minItems uint64,
	maxItems uint64,
	allowTrailingSeparator bool,
) Rule[ReadT, AccumulatorT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, AccumulatorT, ExpectT]) {
		if itemRule == nil {
			reader.Acknowledge(ACK_UNSUBSCRIBE_ON_ERROR)
			if formatNoItem == nil {
				formatNoItem = func(ExpectT) string {
					return "repetition of unspecified item"
				}
			}
			current := reader.Current()
			result := &Result[ReadT, AccumulatorT, ExpectT] {
				Offset: current.Offset,
				Error: &SyntaxError[ReadT, ExpectT] {
					Found: current,
					Expected: []ExpectT {noItem},
					FormatExpected: formatNoItem,
				},
				Reader: reader,
			}
			resultChannel <- result
			return
		}
		var accumulator AccumulatorT
		if initAccu != nil {
			accumulator = initAccu()
		}
		var haveItemCount uint64
		separatorConsumed := false
		var separatorValue SeparatorT
		var emptyItem ItemT
		var emptySeparator SeparatorT
		for {
			if haveItemCount == maxItems {
				break
			}
			offsetBefore := reader.Current().Offset
			var itemParallel Parallel[ReadT, ItemT, ExpectT]
			split := reader.Split()
			itemParallel.Add(split, EmptySequence[ReadT, ItemT, ExpectT](emptyItem))
			itemParallel.Add(reader, itemRule)
			itemResults := itemParallel.Await()
			itemResult := itemResults[1]
			if itemResult.Error != nil {
				// we might actually get away with this
				if haveItemCount >= minItems && (allowTrailingSeparator || !separatorConsumed) {
					if haveItemCount > 0 && allowTrailingSeparator && separatorRule != nil && combineAccu != nil {
						accumulator = combineAccu(accumulator, separatorValue, emptyItem)
					}
					outResult := SubstResult[ReadT, ItemT, AccumulatorT, ExpectT](itemResults[0], accumulator)
					resultChannel <- outResult
				} else {
					// nope, the error has it
					split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
					errResult := SubstResult[ReadT, ItemT, AccumulatorT, ExpectT](itemResult, accumulator)
					resultChannel <- errResult
				}
				return
			}
			// we successfully read an item
			split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
			reader = itemResult.Reader
			if combineAccu != nil {
				accumulator = combineAccu(accumulator, separatorValue, itemResult.Result)
			}
			haveItemCount++
			if haveItemCount == maxItems && (!allowTrailingSeparator || separatorRule == nil) {
				break
			}
			if separatorRule == nil {
				if reader.Current().Offset == offsetBefore {
					reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_ERROR)
					errResult := &Result[ReadT, AccumulatorT, ExpectT] {
						Offset: offsetBefore,
						Result: accumulator,
						Error: &InfiniteRepetitionError[ReadT, ExpectT] {
							Found: reader.Current(),
							FormatFound: formatPacket,
						},
						Reader: reader,
					}
					resultChannel <- errResult
					return
				}
				continue
			}
			// now we need to read a separator
			var separatorParallel Parallel[ReadT, SeparatorT, ExpectT]
			split = reader.Split()
			separatorParallel.Add(split, EmptySequence[ReadT, SeparatorT, ExpectT](emptySeparator))
			separatorParallel.Add(reader, separatorRule)
			separatorResults := separatorParallel.Await()
			separatorResult := separatorResults[1]
			if separatorResult.Error != nil {
				// we might actually get away with this
				if haveItemCount == minItems {
					outResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](
						separatorResults[0],
						accumulator,
					)
					resultChannel <- outResult
				} else {
					// nope, the error has it
					split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
					errResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](separatorResult, accumulator)
					resultChannel <- errResult
				}
				return
			}
			// we successfully read a separator
			reader = separatorResult.Reader
			if reader.Current().Offset == offsetBefore {
				reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_ERROR)
				errResult := &Result[ReadT, AccumulatorT, ExpectT] {
					Offset: offsetBefore,
					Result: accumulator,
					Error: &InfiniteRepetitionError[ReadT, ExpectT] {
						Found: reader.Current(),
						FormatFound: formatPacket,
					},
					Reader: reader,
				}
				resultChannel <- errResult
				return
			}
			split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
			separatorValue = separatorResult.Result
			if haveItemCount == maxItems {
				if combineAccu != nil {
					accumulator = combineAccu(accumulator, separatorValue, emptyItem)
				}
				outResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](separatorResult, accumulator)
				resultChannel <- outResult
				return
			}
		}
		result := &Result[ReadT, AccumulatorT, ExpectT] {
			Offset: reader.Current().Offset,
			Result: accumulator,
			Reader: reader,
		}
		resultChannel <- result
	}
}

func Option[ReadT any, OutT any, ExpectT any](
	formatPacket func(*Packet[ReadT]) string,
	none OutT,
	subjectRule Rule[ReadT, OutT, ExpectT],
) Rule[ReadT, OutT, ExpectT] {
	if subjectRule == nil {
		return EmptySequence[ReadT, OutT, ExpectT](none)
	}
	var noItem ExpectT
	return Repetition(
		formatPacket,
		func() OutT {
			return none
		},
		func(accumulator OutT, separator int, item OutT) OutT {
			return item
		},
		noItem,
		nil,
		subjectRule,
		nil,
		0,
		1,
		false,
	)
}
