package gorecdesc

import (
	"fmt"
)

func EmptySequence[ReadT any, OutT any, ExpectT any](returnValue OutT) Rule[ReadT, OutT, ExpectT] {
	return func(reader *Reader[ReadT], resultChannel ResultChannel[ReadT, OutT, ExpectT]) {
		result := &Result[ReadT, OutT, ExpectT] {
			Offset: reader.current.Offset,
			Result: returnValue,
			Reader: reader,
		}
		if debugOn {
			debugf("Entering EmptySequence with Reader %s\n", debugReader(reader))
			debugf("[EmptySequence with Reader %s] Issuing %s\n", debugResult(result))
			debugf("Leaving EmptySequence with Reader %s\n", debugReader(reader))
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
				if debugOn {
					debugf("Leaving Sequence with Reader %s\n", debugReader(reader))
				}
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
			if debugOn {
				debugf(
					"[Sequence with Reader %s] Changing Reader to %s\n",
					debugReader(reader),
					debugReader(childResult.Reader),
				)
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
		if debugOn {
			debugf("Leaving Sequence with Reader %s\n", debugReader(reader))
		}
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
						"[Choice with Reader %s] Adding split Reader %s to Parallel for choice %d\n",
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
			if debugOn {
				debugf("Leaving Choice for structure '%s' with Reader %s\n", structure, debugReader(reader))
			}
			return
		}
		if debugOn {
			debugf(
				"[Choice with Reader %s] Adding original Reader to Parallel for choice %d\n",
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
										"ACK_UNSUBSCRIBE_ON_SUCCESS to Reader %s of result %d " +
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
						"[Choice with Reader %s] Issuing %s of one true choice %d\n",
						debugReader(reader),
						debugResult(positiveResults[0]),
						maxPositiveIndex,
					)
				}
				resultChannel <- positiveResults[0]
				if debugOn {
					debugf(
						"Leaving Choice for structure '%s' with Reader %s\n",
						structure,
						positiveResults[0].Reader,
					)
				}
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
				if debugOn {
					debugf("Leaving Choice for structure '%s' with Reader %s\n", structure, result.Reader)
				}
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
			reader = negativeResults[0].Reader
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
			reader = result.Reader
		}
		if debugOn {
			debugf("Leaving Choice for structure '%s' with Reader %s\n", structure, debugReader(reader))
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
		if debugOn {
			debugf("Entering Repetition with Reader %s\n", debugReader(reader))
		}
		if itemRule == nil {
			if debugOn {
				debugf(
					"[Repetition with Reader %s] No item rule provided, issuing ACK_UNSUBSCRIBE_ON_ERROR\n",
					debugReader(reader),
				)
			}
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
			if debugOn {
				debugf(
					"[Repetition with Reader %s] No item rule provided, issuing %s\n",
					debugReader(reader),
					debugResult(result),
				)
			}
			resultChannel <- result
			return
		}
		var accumulator AccumulatorT
		if initAccu != nil {
			accumulator = initAccu()
		}
		if debugOn {
			debugf("[Repetition with Reader %s] Initial accumulator = %+v\n", debugReader(reader), accumulator)
		}
		var haveItemCount uint64
		separatorConsumed := false
		var separatorValue SeparatorT
		var emptyItem ItemT
		var emptySeparator SeparatorT
		for {
			if debugOn {
				debugf("[Repetition with Reader %s] Trying for item %d\n", debugReader(reader), haveItemCount)
			}
			if haveItemCount == maxItems {
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Hit max item count %d before reading item\n",
						debugReader(reader),
						maxItems,
					)
				}
				break
			}
			offsetBeforeItem := reader.Current().Offset
			var itemParallel Parallel[ReadT, ItemT, ExpectT]
			split := reader.Split()
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Adding split Reader %s to Parallel for no item\n",
					debugReader(reader),
					debugReader(split),
				)
			}
			itemParallel.Add(split, EmptySequence[ReadT, ItemT, ExpectT](emptyItem))
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Adding original Reader to Parallel for item\n",
					debugReader(reader),
				)
			}
			itemParallel.Add(reader, itemRule)
			itemResults := itemParallel.Await()
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Parallel.Await() for item returned results: %s\n",
					debugReader(reader),
					debugResultList(itemResults),
				)
			}
			itemResult := itemResults[1]
			if itemResult.Error != nil {
				// we might actually get away with this
				if haveItemCount >= minItems && (allowTrailingSeparator || !separatorConsumed) {
					if haveItemCount > 0 && allowTrailingSeparator && separatorRule != nil && combineAccu != nil {
						accumulator = combineAccu(accumulator, separatorValue, emptyItem)
						if debugOn {
							debugf(
								"[Repetition with Reader %s] Updated accumulator to %+v\n",
								debugReader(reader),
								accumulator,
							)
						}
					}
					outResult := SubstResult[ReadT, ItemT, AccumulatorT, ExpectT](itemResults[0], accumulator)
					if debugOn {
						var reason string
						if allowTrailingSeparator {
							reason = "allowTrailingSeparator = true"
						} else {
							reason = "separatorConsumed = false"
						}
						debugf(
							"[Repetition with Reader %s] Accepting failed item since " +
									"haveItemCount %d >= minItems %d and %s\n",
							debugReader(reader),
							haveItemCount,
							minItems,
							reason,
						)
						debugf(
							"[Repetition with Reader %s] Issuing result %s\n",
							debugReader(reader),
							debugResult(outResult),
						)
					}
					resultChannel <- outResult
				} else {
					// nope, the error has it
					if debugOn {
						var reason string
						if haveItemCount < minItems {
							reason = fmt.Sprintf("haveItemCount %d < minItems %d", haveItemCount, minItems)
						} else {
							reason = "allowTrailingSeparator = false and separatorConsumed = true"
						}
						debugf(
							"[Repetition with Reader %s] Rejecting failed item since %s, sending " +
									"ACK_UNSUBSCRIBE_ON_SUCCESS on channel of split (empty sequence) Reader %s\n",
							debugReader(reader),
							reason,
							debugReader(split),
						)
					}
					split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
					errResult := SubstResult[ReadT, ItemT, AccumulatorT, ExpectT](itemResult, accumulator)
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Issuing result %s due to failed item\n",
							debugReader(reader),
							debugResult(errResult),
						)
					}
					resultChannel <- errResult
				}
				return
			}
			// we successfully read an item
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Successfully read item #%d, sending " +
							"ACK_UNSUBSCRIBE_ON_SUCCESS on channel of split (empty sequence) Reader %s\n",
					debugReader(reader),
					haveItemCount,
					debugReader(split),
				)
			}
			split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Changing Reader to %s\n",
					debugReader(reader),
					debugReader(itemResult.Reader),
				)
			}
			reader = itemResult.Reader
			if combineAccu != nil {
				accumulator = combineAccu(accumulator, separatorValue, itemResult.Result)
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Updated accumulator to %+v\n",
						debugReader(reader),
						accumulator,
					)
				}
			}
			haveItemCount++
			separatorConsumed = false
			if haveItemCount == maxItems && (!allowTrailingSeparator || separatorRule == nil) {
				if debugOn {
					var reason string
					if !allowTrailingSeparator {
						reason = "allowTrailingSeparator = false"
					} else {
						reason = "separatorRule == nil"
					}
					debugf(
						"[Repetition with Reader %s] Stopping because haveItemCount == maxItems %d and %s",
						debugReader(reader),
						maxItems,
						reason,
					)
				}
				break
			}
			if separatorRule == nil {
				if reader.Current().Offset == offsetBeforeItem {
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Item rule did not consume any packets " +
									"(still at packet %d) and no separator rule given: Issuing " +
									"ACK_UNSUBSCRIBE_ON_ERROR \n",
							debugReader(reader),
							offsetBeforeItem,
						)
					}
					reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_ERROR)
					errResult := &Result[ReadT, AccumulatorT, ExpectT] {
						Offset: offsetBeforeItem,
						Result: accumulator,
						Error: &InfiniteRepetitionError[ReadT, ExpectT] {
							Found: reader.Current(),
							FormatFound: formatPacket,
						},
						Reader: reader,
					}
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Item rule did not consume any packets " +
									"(still at packet %d) and no separator rule given: Issuing %s\n",
							debugReader(reader),
							offsetBeforeItem,
							debugResult(errResult),
						)
					}
					resultChannel <- errResult
					return
				}
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Skipping reading separator after item %d: No rule given\n",
						debugReader(reader),
						haveItemCount - 1,
					)
				}
				continue
			}
			// now we need to read a separator
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Trying for separator following item %d\n",
					debugReader(reader),
					haveItemCount - 1,
				)
			}
			offsetBeforeSeparator := reader.Current().Offset
			var separatorParallel Parallel[ReadT, SeparatorT, ExpectT]
			split = reader.Split()
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Adding split Reader %s to Parallel for no separator\n",
					debugReader(reader),
					debugReader(split),
				)
			}
			separatorParallel.Add(split, EmptySequence[ReadT, SeparatorT, ExpectT](emptySeparator))
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Adding original Reader to Parallel for separator\n",
					debugReader(reader),
				)
			}
			separatorParallel.Add(reader, separatorRule)
			separatorResults := separatorParallel.Await()
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Parallel.Await() for separator returned results: %s\n",
					debugReader(reader),
					debugResultList(separatorResults),
				)
			}
			separatorResult := separatorResults[1]
			if separatorResult.Error != nil {
				// we might actually get away with this
				if haveItemCount >= minItems {
					outResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](
						separatorResults[0],
						accumulator,
					)
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Accepting failed separator since " +
									"haveItemCount %d >= minItems %d\n",
							debugReader(reader),
							haveItemCount,
							minItems,
						)
						debugf(
							"[Repetition with Reader %s] Issuing result %s\n",
							debugReader(reader),
							debugResult(outResult),
						)
					}
					resultChannel <- outResult
				} else {
					// nope, the error has it
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Rejecting failed separator since " +
									"haveItemCount %d > minItems %d, sending ACK_UNSUBSCRIBE_ON_SUCCESS " +
									"on channel of split (empty sequence) Reader %s\n",
							debugReader(reader),
							haveItemCount,
							minItems,
							debugReader(split),
						)
					}
					split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
					errResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](separatorResult, accumulator)
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Issuing result %s\n",
							debugReader(reader),
							debugResult(errResult),
						)
					}
					resultChannel <- errResult
				}
				return
			}
			// we successfully read a separator
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Successfully read separator following item #%d\n",
					debugReader(reader),
					haveItemCount - 1,
				)
				debugf(
					"[Repetition with Reader %s] Changing Reader to %s\n",
					debugReader(reader),
					debugReader(separatorResult.Reader),
				)
			}
			reader = separatorResult.Reader
			if reader.Current().Offset == offsetBeforeItem {
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Neither item role not separator rule consumed any packets " +
								"(still at packet %d): Issuing ACK_UNSUBSCRIBE_ON_ERROR\n",
						debugReader(reader),
						offsetBeforeItem,
					)
				}
				reader.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_ERROR)
				errResult := &Result[ReadT, AccumulatorT, ExpectT] {
					Offset: offsetBeforeItem,
					Result: accumulator,
					Error: &InfiniteRepetitionError[ReadT, ExpectT] {
						Found: reader.Current(),
						FormatFound: formatPacket,
					},
					Reader: reader,
				}
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Neither item role not separator rule consumed any packets " +
								"(still at packet %d): Issuing %s\n",
						debugReader(reader),
						offsetBeforeItem,
						debugResult(errResult),
					)
				}
				resultChannel <- errResult
				return
			}
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Sending ACK_UNSUBSCRIBE_ON_SUCCESS on channel of split " +
							"(empty sequence) Reader %s\n",
					debugReader(reader),
					debugReader(split),
				)
			}
			split.AcknowledgeOnChannel(ACK_UNSUBSCRIBE_ON_SUCCESS)
			separatorValue = separatorResult.Result
			if haveItemCount == maxItems {
				if debugOn {
					debugf(
						"[Repetition with Reader %s] Hit max item count %d after reading separator\n",
						debugReader(reader),
						maxItems,
					)
				}
				if combineAccu != nil {
					accumulator = combineAccu(accumulator, separatorValue, emptyItem)
					if debugOn {
						debugf(
							"[Repetition with Reader %s] Updated accumulator to %+v\n",
							debugReader(reader),
							accumulator,
						)
					}
				}
				outResult := SubstResult[ReadT, SeparatorT, AccumulatorT, ExpectT](separatorResult, accumulator)
				if debugOn {
					debugf("[Repetition with Reader %s] Issuing %s\n", debugReader(reader), debugResult(outResult))
				}
				resultChannel <- outResult
				return
			}
			separatorConsumed = reader.Current().Offset > offsetBeforeSeparator
			if debugOn {
				debugf(
					"[Repetition with Reader %s] Setting separatorConsumed = %v after separator\n",
					debugReader(reader),
					separatorConsumed,
				)
			}
		}
		result := &Result[ReadT, AccumulatorT, ExpectT] {
			Offset: reader.Current().Offset,
			Result: accumulator,
			Reader: reader,
		}
		if debugOn {
			debugf(
				"[Repetition with Reader %s] Stopped iterating; issuing %s\n",
				debugReader(reader),
				debugResult(result),
			)
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
