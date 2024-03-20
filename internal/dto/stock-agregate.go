package dto

// stockAggreate 우리
type StockAggregate struct {
	OpenTime   int64
	ClosedTime int64
	Open       float32
	Closed     float32
	High       float32
	Low        float32
	Volume     float32
}
