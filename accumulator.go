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
		if sink != nil {
			sink(piece)
		}
		return accumulator
	}
}

type CombineBiAccu[
	AccumulatorT any,
	LeftPieceT any,
	RightPieceT any,
] func(AccumulatorT, LeftPieceT, RightPieceT) AccumulatorT

func BypassBiAccu[
	AccumulatorT any,
	LeftPieceT any,
	RightPieceT any,
](sink func(LeftPieceT, RightPieceT)) CombineBiAccu[AccumulatorT, LeftPieceT, RightPieceT] {
	return func(accumulator AccumulatorT, left LeftPieceT, right RightPieceT) AccumulatorT {
		if sink != nil {
			sink(left, right)
		}
		return accumulator
	}
}
