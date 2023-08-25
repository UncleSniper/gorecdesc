package gorecdesc

type PacketChannel[ReadT any] <-chan *Packet[ReadT]

type Acknowledgement uint

const (
	ACK_KEEP_SUBSCRIPTION Acknowledgement = iota
	ACK_UNSUBSCRIBE_ON_SUCCESS
	ACK_UNSUBSCRIBE_ON_ERROR
)

type AckChannel chan<- Acknowledgement

type Reader[ReadT any] struct {
	dispatcher *Dispatcher[ReadT]
	packetChannel PacketChannel[ReadT]
	ackChannel AckChannel
	current *Packet[ReadT]
	prepended [][]*Packet[ReadT]
	inPrepended int
}

func(reader *Reader[ReadT]) Current() *Packet[ReadT] {
	return reader.current
}

func(reader *Reader[ReadT]) Next() *Packet[ReadT] {
	if len(reader.prepended) == 0 {
		if debugOn {
			debugf("[Reader %s] Retrieving next packet from channel\n", debugReader(reader))
		}
		reader.current = <-reader.packetChannel
	} else {
		if debugOn {
			debugf("[Reader %s] Unqueuing next packet from prepended list\n", debugReader(reader))
		}
		front := reader.prepended[0]
		reader.current = front[reader.inPrepended]
		reader.inPrepended++
		if reader.inPrepended >= len(front) {
			reader.prepended = reader.prepended[1:]
			reader.inPrepended = 0
		}
	}
	if debugOn {
		debugf("[Reader %s] Set current packet\n", debugReader(reader))
	}
	return reader.current
}

func(reader *Reader[ReadT]) NextFromChannel() *Packet[ReadT] {
	if debugOn {
		debugf("[Reader %s] Explicitly retrieving next packet from channel\n", debugReader(reader))
	}
	reader.current = <-reader.packetChannel
	if debugOn {
		debugf("[Reader %s] Set current packet\n", debugReader(reader))
	}
	return reader.current
}

func(reader *Reader[ReadT]) Split() *Reader[ReadT] {
	clone := reader.dispatcher.Subscribe()
	clone.current = reader.current
	if debugOn {
		debugf("[Reader %s] Splitting off new Reader %s\n", debugReader(reader), debugReader(clone))
	}
	return clone
}

func(reader *Reader[ReadT]) Acknowledge(unsubscribe Acknowledgement) {
	if len(reader.prepended) == 0 {
		if debugOn {
			debugf("[Reader %s] Sending ack %s\n", debugReader(reader), debugAck(unsubscribe))
		}
		reader.ackChannel <- unsubscribe
		if debugOn {
			debugf("[Reader %s] Sent ack %s\n", debugReader(reader), debugAck(unsubscribe))
		}
	} else if debugOn {
		debugf(
			"[Reader %s] Dropping ack %s since prepended list is non-empty\n",
			debugReader(reader),
			debugAck(unsubscribe),
		)
	}
}

func(reader *Reader[ReadT]) Reprovide(packets []*Packet[ReadT], andCurrent bool) {
	if len(packets) == 0 {
		return
	}
	if debugOn {
		debugf(
			"[Reader %s] Will reprovide %s (andCurrent = %v)\n",
			debugReader(reader),
			debugPacketList(packets),
			andCurrent,
		)
	}
	if len(reader.prepended) == 0 {
		if andCurrent {
			packets = append(packets, reader.current)
		}
		reader.current = packets[0]
		packets = packets[1:]
		if len(packets) == 0 {
			reader.prepended = nil
		} else {
			reader.prepended = [][]*Packet[ReadT] {packets}
		}
	} else {
		if reader.inPrepended > 0 {
			last := len(reader.prepended) - 1
			inPrepended := reader.inPrepended
			if andCurrent {
				inPrepended--
			}
			if inPrepended > 0 {
				reader.prepended[last] = reader.prepended[last][inPrepended:]
			}
		} else if andCurrent {
			packets = append(packets, reader.current)
		}
		reader.current = packets[0]
		if len(packets) > 1 {
			reader.prepended = append(reader.prepended, packets[1:])
		}
	}
	reader.inPrepended = 0
	if debugOn {
		debugf("[Reader %s] State after Reprovide\n", debugReader(reader))
	}
}

func Intercept[ReadT any, OutT any, ExpectT any](
	reader *Reader[ReadT],
	oldResultChannel ResultChannel[ReadT, OutT, ExpectT],
	interceptor func(Acknowledgement, *Result[ReadT, OutT, ExpectT]) *Result[ReadT, OutT, ExpectT],
) (
	newResultChannel ResultChannel[ReadT, OutT, ExpectT],
) {
	if debugOn {
		debugf("Intercept for Reader %s\n", debugReader(reader))
	}
	oldAckChannel := reader.ackChannel
	newAckChannel := make(chan Acknowledgement)
	nrc := make(chan *Result[ReadT, OutT, ExpectT])
	newResultChannel = nrc
	reader.ackChannel = newAckChannel
	go func() {
		for {
			if debugOn {
				debugf("[Intercept for Reader %08X] Awaiting ack on inner channel\n", reader)
			}
			ack := <-newAckChannel
			if debugOn {
				debugf("[Intercept for Reader %08X] Received %s on inner channel\n", reader, debugAck(ack))
			}
			if ack != ACK_KEEP_SUBSCRIPTION {
				if debugOn {
					debugf("[Intercept for Reader %08X] Awaiting result on inner channel\n", reader)
				}
				result := <-nrc
				if debugOn {
					debugf("[Intercept for Reader %08X] Received %s on inner channel\n", reader, debugResult(result))
					debugf("[Intercept for Reader %08X] Sending %s to outer channel\n", reader, debugAck(ack))
				}
				oldAckChannel <- ack
				if interceptor != nil {
					result = interceptor(ack, result)
					if debugOn {
						debugf(
							"[Intercept for Reader %08X] Interceptor turned result into %s\n",
							reader,
							debugResult(result),
						)
					}
				}
				if debugOn {
					debugf("[Intercept for Reader %08X] Sending result to outer channel\n", reader)
				}
				oldResultChannel <- result
				if debugOn {
					debugf("[Intercept for Reader %08X] Quitting\n", reader)
				}
				break
			}
			if debugOn {
				debugf("[Intercept for Reader %08X] Sending ACK_KEEP_SUBSCRIPTION to outer channel\n", reader)
			}
			oldAckChannel <- ACK_KEEP_SUBSCRIPTION
			if interceptor != nil {
				interceptor(ACK_KEEP_SUBSCRIPTION, nil)
			}
		}
	}()
	return
}
