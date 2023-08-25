package gorecdesc

func TokenPredicate[ReadT any](subPredicate func(ReadT) bool) func(*Packet[ReadT]) bool {
	if subPredicate == nil {
		return nil
	}
	return func(packet *Packet[ReadT]) bool {
		return packet != nil && subPredicate(packet.Item)
	}
}

func TokenReturn[ReadT any]() func(*Packet[ReadT]) ReadT {
	return func(packet *Packet[ReadT]) ReadT {
		return packet.Item
	}
}
