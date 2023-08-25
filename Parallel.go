package gorecdesc

type Parallel[ReadT any, OutT any, ExpectT any] struct {
	states []*parallelState[ReadT, OutT, ExpectT]
}

type parallelState[ReadT any, OutT any, ExpectT any] struct {
	reader *Reader[ReadT]
	outerAckChannel AckChannel
	innerAckChannel chan Acknowledgement
	innerResultChannel chan *Result[ReadT, OutT, ExpectT]
	result *Result[ReadT, OutT, ExpectT]
	skipped []*Packet[ReadT]
}

func(par *Parallel[ReadT, OutT, ExpectT]) Add(
	reader *Reader[ReadT],
	rule Rule[ReadT, OutT, ExpectT],
) {
	state := &parallelState[ReadT, OutT, ExpectT] {
		reader: reader,
		outerAckChannel: reader.ackChannel,
		innerAckChannel: make(chan Acknowledgement),
		innerResultChannel: make(chan *Result[ReadT, OutT, ExpectT]),
	}
	if debugOn {
		debugf(
			"[Parallel.Add] Starting goroutine for child %d with reader %s\n",
			len(par.states),
			debugReader(reader),
		)
	}
	par.states = append(par.states, state)
	reader.ackChannel = state.innerAckChannel
	go rule(reader, state.innerResultChannel)
}

func(par *Parallel[ReadT, OutT, ExpectT]) Await() []*Result[ReadT, OutT, ExpectT] {
	stateCount := len(par.states)
	if stateCount == 0 {
		if debugOn {
			debugln("[Parallel.Await] Returning early because there are zero states")
		}
		return nil
	}
	acks := make([]Acknowledgement, stateCount)
	var roundCount uint64
	for {
		alive := false
		// collect all acknowledgements
		for stateIndex, state := range par.states {
			if state.result != nil {
				if debugOn {
					debugf(
						"[Parallel.Await] Collecting acks for round %d: Skipping state %d with reader %s; " +
								"it already has result = %s\n",
						roundCount,
						stateIndex,
						debugReader(state.reader),
						state.result,
					)
				}
				continue
			}
			ack := <-state.innerAckChannel
			if debugOn {
				debugf(
					"[Parallel.Await] Collecting acks for round %d: State %d with reader %s sent %s\n",
					roundCount,
					stateIndex,
					debugReader(state.reader),
					debugAck(ack),
				)
			}
			acks[stateIndex] = ack
			if ack == ACK_KEEP_SUBSCRIPTION {
				alive = true
			}
		}
		if debugOn {
			debugf(
				"[Parallel.Await] Round %d (for reader #0 = %s) has alive = %v\n",
				roundCount,
				debugReader(par.states[0].reader),
				alive,
			)
		}
		// prepare for next round
		for stateIndex, state := range par.states {
			if state.result == nil {
				if debugOn {
					debugf(
						"[Parallel.Await] Preparing for next round in round %d: " +
								"State %d with reader %s has no result\n",
						roundCount,
						stateIndex,
						debugReader(state.reader),
					)
				}
				if acks[stateIndex] == ACK_KEEP_SUBSCRIPTION {
					// child wants to keep going, so we simply propagate the acknowledgement
					if debugOn {
						debugf(
							"[Parallel.Await] Preparing for next round in round %d: " +
									"State %d with reader %s continuing, propagating ACK_KEEP_SUBSCRIPTION\n",
							roundCount,
							stateIndex,
							debugReader(state.reader),
						)
					}
					state.outerAckChannel <- ACK_KEEP_SUBSCRIPTION
					continue
				} else {
					// child is unsubscribing
					state.result = <-state.innerResultChannel
					if debugOn {
						debugf(
							"[Parallel.Await] Preparing for next round in round %d: " +
									"State %d with reader %s unsubscribing with %s, fetched result = %s\n",
							roundCount,
							stateIndex,
							debugReader(state.reader),
							debugAck(acks[stateIndex]),
							debugResult(state.result),
						)
					}
				}
			}
			if state.result.Error == nil {
				// child is done, but others must be able to keep going
				reader := state.result.Reader
				current := reader.Current()
				state.skipped = append(state.skipped, current)
				state.outerAckChannel <- ACK_KEEP_SUBSCRIPTION
				next := reader.NextFromChannel()
				if debugOn {
					debugf(
						"[Parallel.Await] Preparing for next round in round %d: " +
								"State %d with reader %s has no error, thus skipping it ahead: " +
								"Registering skipped packet = %s, issuing ACK_KEEP_SUBSCRIPTION outward " +
								"-> next packet = %s\n",
						roundCount,
						stateIndex,
						debugReader(state.reader),
						debugPacket(current),
						debugPacket(next),
					)
				}
			} else {
				// child is in error, therefore its reader is already unsubscribed
				if debugOn {
					debugf(
						"[Parallel.Await] Preparing for next round in round %d: " +
								"State %d with reader %s has error %+v, doing nothing\n",
						roundCount,
						stateIndex,
						debugReader(state.reader),
						state.result.Error,
					)
				}
			}
		}
		// bail out
		if !alive {
			if debugOn {
				debugf(
					"[Parallel.Await] Round %d (for reader #0 = %s): Bailing out (nothing alive)\n",
					roundCount,
					debugReader(par.states[0].reader),
				)
			}
			results := make([]*Result[ReadT, OutT, ExpectT], stateCount)
			for stateIndex, state := range par.states {
				results[stateIndex] = state.result
				// restore reader
				reader := state.result.Reader
				reader.ackChannel = state.outerAckChannel
				reader.Reprovide(state.skipped, true)
			}
			if debugOn {
				debugf(
					"[Parallel.Await] Round %d (for reader #0 = %s): Completing bailout with results = %s\n",
					roundCount,
					debugReader(par.states[0].reader),
					debugResultList(results),
				)
			}
			return results
		}
		roundCount++
	}
}

func(par *Parallel[ReadT, OutT, ExpectT]) Reset() {
	par.states = nil
}
