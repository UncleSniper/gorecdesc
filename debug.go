package gorecdesc

import (
	"os"
	"fmt"
	"sync"
	"strings"
)

const debugPrefix string = "***DEBUG[github.com/UncleSniper/gorecdesc]: "

var debugOn bool = len(os.Getenv("GO_USDO_RECDESC_DEBUG")) > 0

var debugLock sync.Mutex

func debugf(format string, args ...any) {
	if debugOn {
		debugLock.Lock()
		fmt.Fprint(os.Stderr, debugPrefix)
		fmt.Fprintf(os.Stderr, format, args...)
		debugLock.Unlock()
	}
}

func debugln(args ...any) {
	if debugOn {
		debugLock.Lock()
		fmt.Fprint(os.Stderr, debugPrefix)
		fmt.Fprintln(os.Stderr, args...)
		debugLock.Unlock()
	}
}

func debugResult[ReadT any, OutT any, ExpectT any](result *Result[ReadT, OutT, ExpectT]) string {
	if result == nil {
		return "Result == nil"
	}
	var err string
	if result.Error == nil {
		err = "nil"
	} else {
		err = fmt.Sprintf("<<%s>>", result.Error.Error())
	}
	return fmt.Sprintf(
		"Result { Offset = %d, Result = %+v, Structure = %q, Error = %s, Reader = %s }",
		result.Offset,
		result.Result,
		result.Structure,
		err,
		debugReader(result.Reader),
	)
}

func debugAck(ack Acknowledgement) string {
	switch ack {
		case ACK_KEEP_SUBSCRIPTION:
			return "ACK_KEEP_SUBSCRIPTION"
		case ACK_UNSUBSCRIBE_ON_SUCCESS:
			return "ACK_UNSUBSCRIBE_ON_SUCCESS"
		case ACK_UNSUBSCRIBE_ON_ERROR:
			return "ACK_UNSUBSCRIBE_ON_ERROR"
		default:
			return fmt.Sprintf("<unknown Acknowledgement %d>", ack)
	}
}

func debugPacket[ReadT any](packet *Packet[ReadT]) string {
	return fmt.Sprintf("Packet { Offset = %d, Item = %+v, EOF = %v }", packet.Offset, packet.Item, packet.EOF)
}

func debugPacketList[ReadT any](packets []*Packet[ReadT]) string {
	var builder strings.Builder
	if packets == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteRune('[')
		for index, packet := range packets {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(debugPacket(packet))
		}
		builder.WriteRune(']')
	}
	return builder.String()
}

func debugPacketListList[ReadT any](prepended [][]*Packet[ReadT]) string {
	var builder strings.Builder
	if prepended == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteRune('[')
		for index, batch := range prepended {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(debugPacketList(batch))
		}
		builder.WriteRune(']')
	}
	return builder.String()
}

func debugReader[ReadT any](reader *Reader[ReadT]) string {
	return fmt.Sprintf(
		"%d (current = %s, prepended = %s, inPrepended = %d)",
		reader.id,
		debugPacket(reader.current),
		debugPacketListList(reader.prepended),
		reader.inPrepended,
	)
}

func debugResultList[ReadT any, OutT any, ExpectT any](results []*Result[ReadT, OutT, ExpectT]) string {
	var builder strings.Builder
	if results == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteRune('[')
		for index, result := range results {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(debugResult(result))
		}
		builder.WriteRune(']')
	}
	return builder.String()
}

func debugAmbiguityChoice[ReadT any, ExpectT any](choice AmbiguityChoice[ReadT, ExpectT]) string {
	return fmt.Sprintf(
		"AmbiguityChoice { Structure = %q, EndBefore = %s }",
		choice.Structure,
		debugPacket(choice.EndBefore),
	)
}

func debugAmbiguityChoiceList[ReadT any, ExpectT any](choices []AmbiguityChoice[ReadT, ExpectT]) string {
	var builder strings.Builder
	if choices == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteRune('[')
		for index, choice := range choices {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(debugAmbiguityChoice(choice))
		}
		builder.WriteRune(']')
	}
	return builder.String()
}
