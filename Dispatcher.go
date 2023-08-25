package gorecdesc

import (
	"io"
	"sync"
)

type Cookie uint64

type channelPair[ReadT any] struct {
	packetChannel chan *Packet[ReadT]
	ackChannel chan Acknowledgement
}

type queuedChannel struct {
	cookie Cookie
	ackChannel chan Acknowledgement
}

type Dispatcher[ReadT any] struct {
	nextCookie Cookie
	channels map[Cookie]channelPair[ReadT]
	nextOffset uint64
	mapLock sync.Mutex
	sendLock sync.Mutex
}

func(disp *Dispatcher[ReadT]) Send(item ReadT, eof bool) {
	if debugOn {
		debugf("[Dispatcher] Entering send for %v (eof = %v) with %d channels\n", item, eof, len(disp.channels))
	}
	disp.sendLock.Lock()
	// send packets
	disp.mapLock.Lock()
	packet := &Packet[ReadT] {
		Offset: disp.nextOffset,
		Item: item,
		EOF: eof,
	}
	if debugOn {
		debugf("[Dispatcher] Packet offset in send for %v (eof = %v) is %d\n", item, eof, packet.Offset)
	}
	disp.nextOffset++
	if disp.channels == nil {
		disp.channels = make(map[Cookie]channelPair[ReadT])
	}
	channelQueue := make([]queuedChannel, len(disp.channels))
	index := 0
	for cookie, pair := range disp.channels {
		if debugOn {
			debugf("[Dispatcher] Sending packet %d to Reader with cookie %d\n", packet.Offset, cookie)
		}
		pair.packetChannel <- packet
		channelQueue[index] = queuedChannel {
			cookie: cookie,
			ackChannel: pair.ackChannel,
		}
		index++
	}
	disp.mapLock.Unlock()
	// await acknowledgements
	toUnsubscribe := make([]Cookie, len(channelQueue))
	unsubscribeCount := 0
	for _, queued := range channelQueue {
		if debugOn {
			debugf(
				"[Dispatcher] Awaiting ack to packet %d from Reader with cookie %d\n",
				packet.Offset,
				queued.cookie,
			)
		}
		ack := <-queued.ackChannel
		if debugOn {
			debugf(
				"[Dispatcher] Ack to packet %d from Reader with cookie %d was %s\n",
				packet.Offset,
				queued.cookie,
				debugAck(ack),
			)
		}
		if ack != ACK_KEEP_SUBSCRIPTION {
			toUnsubscribe[unsubscribeCount] = queued.cookie
			unsubscribeCount++
		}
	}
	// process unsubscribes
	if unsubscribeCount > 0 {
		disp.mapLock.Lock()
		for index = 0; index < unsubscribeCount; index++ {
			if debugOn {
				debugf(
					"[Dispatcher] Unsubscribing Reader with cookie %d in response to ack to packet %d\n",
					toUnsubscribe[index],
					packet.Offset,
				)
			}
			delete(disp.channels, toUnsubscribe[index])
		}
		disp.mapLock.Unlock()
	}
	disp.sendLock.Unlock()
	if debugOn {
		debugf(
			"[Dispatcher] Leaving send for %v (eof = %v, offset = %d) with %d channels\n",
			item,
			eof,
			packet.Offset,
			len(disp.channels),
		)
	}
}

func(disp *Dispatcher[ReadT]) Subscribe() *Reader[ReadT] {
	disp.mapLock.Lock()
	packetChannel := make(chan *Packet[ReadT])
	ackChannel := make(chan Acknowledgement)
	cookie := disp.nextCookie
	disp.channels[cookie] = channelPair[ReadT] {
		packetChannel: packetChannel,
		ackChannel: ackChannel,
	}
	disp.nextCookie++
	disp.mapLock.Unlock()
	reader := &Reader[ReadT] {
		dispatcher: disp,
		packetChannel: packetChannel,
		ackChannel: ackChannel,
	}
	if debugOn {
		debugf("[Dispatcher] Issuing new Reader %08X with cookie %d\n", reader, cookie)
	}
	return reader
}

func SendRunes(disp *Dispatcher[Locatable[rune]], reader io.RuneReader, location Location) error {
	for {
		r, _, err := reader.ReadRune()
		if err == nil {
			disp.Send(Locatable[rune] {
				Symbol: r,
				Location: location,
			}, false)
			if r == '\n' {
				location.NextLine()
			} else {
				location.NextColumn()
			}
		} else if err == io.EOF {
			disp.Send(Locatable[rune] {
				Symbol: '\x00',
				Location: location,
			}, true)
			break
		} else {
			return err
		}
	}
	return nil
}

func SendBytes(disp *Dispatcher[Locatable[byte]], reader io.Reader, location Location) error {
	buffer := make([]byte, 128)
	for {
		count, err := reader.Read(buffer)
		for i := 0; i < count; i++ {
			disp.Send(Locatable[byte] {
				Symbol: buffer[i],
				Location: location,
			}, false)
			if buffer[i] == byte('\n') {
				location.NextLine()
			} else {
				location.NextColumn()
			}
		}
		if err != nil {
			if err != io.EOF {
				return err
			}
			disp.Send(Locatable[byte] {
				Symbol: byte(0),
				Location: location,
			}, true)
			break
		}
	}
	return nil
}
