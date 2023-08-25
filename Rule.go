package gorecdesc

type ResultChannel[ReadT any, OutT any, ExpectT any] chan<- *Result[ReadT, OutT, ExpectT]

type Rule[ReadT any, OutT any, ExpectT any] func(*Reader[ReadT], ResultChannel[ReadT, OutT, ExpectT])

func RunRule[ReadT any, OutT any, ExpectT any](
	rule Rule[ReadT, OutT, ExpectT],
	reader *Reader[ReadT],
) *Result[ReadT, OutT, ExpectT] {
	resultChannel := make(chan *Result[ReadT, OutT, ExpectT])
	go rule(reader, resultChannel)
	return <-resultChannel
}
