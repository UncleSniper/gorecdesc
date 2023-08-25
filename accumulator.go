package gorecdesc

type InitAccu[AccumulatorT any] func() AccumulatorT

type CombineAccu[AccumulatorT any, PieceT any] func(AccumulatorT, PieceT) AccumulatorT

func The[T any](value T) InitAccu[T] {
	return func() T {
		return value
	}
}

func BypassAccu[AccumulatorT any, PieceT any](sink func(PieceT)) CombineAccu[AccumulatorT, PieceT] {
	return func(accumulator AccumulatorT, piece PieceT) AccumulatorT {
		sink(piece)
		return accumulator
	}
}
