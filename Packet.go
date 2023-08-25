package gorecdesc

type Packet[ReadT any] struct {
	Offset uint64
	Item ReadT
	EOF bool
}

type Locatable[SymbolT any] struct {
	Symbol SymbolT
	Location Location
}

type RangeLocatable[SymbolT any] struct {
	Symbol SymbolT
	Start Location
	End Location
}
