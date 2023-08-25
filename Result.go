package gorecdesc

type Result[ReadT any, OutT any, ExpectT any] struct {
	Offset uint64
	Result OutT
	Structure string
	Error ParseError[ReadT, ExpectT]
	Reader *Reader[ReadT]
}

func SubstResult[ReadT any, OldOutT any, NewOutT any, ExpectT any](
	result *Result[ReadT, OldOutT, ExpectT],
	newValue NewOutT,
) *Result[ReadT, NewOutT, ExpectT] {
	return &Result[ReadT, NewOutT, ExpectT] {
		Offset: result.Offset,
		Result: newValue,
		Structure: result.Structure,
		Error: result.Error,
		Reader: result.Reader,
	}
}

func MapResult[ReadT any, OldOutT any, NewOutT any, ExpectT any](
	result *Result[ReadT, OldOutT, ExpectT],
	transform func(OldOutT) NewOutT,
) *Result[ReadT, NewOutT, ExpectT] {
	clone := &Result[ReadT, NewOutT, ExpectT] {
		Offset: result.Offset,
		Structure: result.Structure,
		Error: result.Error,
		Reader: result.Reader,
	}
	if transform != nil {
		clone.Result = transform(result.Result)
	}
	return clone
}
