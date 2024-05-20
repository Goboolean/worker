package fetcher

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Goboolean/core-system.worker/internal/infrastructure/mongo"
	"github.com/Goboolean/core-system.worker/internal/job"
	"github.com/Goboolean/core-system.worker/internal/model"
	"github.com/Goboolean/core-system.worker/internal/util"
)

var (
	ErrInvalidStockId       = errors.New("fetch: can't parse stockId")
	ErrDocumentTypeMismatch = errors.New("fetch: mongo: document type mismatch")
)

type PastStock struct {
	Fetcher

	timeSlice           string
	isFetchingFullRange bool
	startTimestamp      int64 // Unix timestamp of start time
	stockId             string
	pastRepo            mongo.StockClient

	out chan model.Packet `type:"*StockAggregate"` //Job은 자신의 Output 채널에 대해 소유권을 가진다.

	wg   sync.WaitGroup
	stop *util.StopNotifier
}

func NewPastStock(mongo mongo.StockClient, parmas *job.UserParams) (*PastStock, error) {
	//여기에 기본값 입력 아웃풋 채널은 job이 소유권을 가져야 한다.

	instance := &PastStock{
		timeSlice:           DefaultTimeSlice,
		isFetchingFullRange: DefaultIsFetchingFullRange,
		pastRepo:            mongo,
		stop:                util.NewStopNotifier(),
		out:                 make(chan model.Packet),
	}

	if !parmas.IsKeyNullOrEmpty("productId") {

		val, ok := (*parmas)["productId"]
		if !ok {
			return nil, fmt.Errorf("create past stock fetch job: %w", ErrInvalidStockId)
		}

		instance.stockId = val
	}

	if !parmas.IsKeyNullOrEmpty("startDate") {
		instance.isFetchingFullRange = false

		val, err := strconv.ParseInt((*parmas)["startDate"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("create past stock fetch job: %w", err)
		}

		instance.startTimestamp = val

	}

	return instance, nil
}

func (ps *PastStock) Execute() {
	ps.wg.Add(1)
	go func() {
		defer ps.wg.Done()
		defer ps.stop.NotifyStop()
		defer close(ps.out)

		ctx, cancel := context.WithCancel(context.Background())

		//stop sig를 받았을 때 하던 작업을 멈추고 강제종료 하기 위한 부분.
		//graceful shutdown을 원하면 이 부분이 없어도 됩니다.
		go func() {
			<-ps.stop.Done()
			cancel()
		}()

		ps.pastRepo.SetTarget(ps.stockId, ps.timeSlice)
		//가져올 데이터의 개수
		var quantity int
		//처음 가져올 데이터의 Index
		var index int
		var err error
		var count int = ps.pastRepo.GetCount(ctx)

		if ps.isFetchingFullRange {
			index = 0
			quantity = count
		} else {

			index, err = ps.pastRepo.FindLatestIndexBy(ctx, ps.startTimestamp)
			if err != nil {
				panic(err)
			}

			quantity = count - index
		}
		var sequnce int64 = 0

		duration, _ := time.ParseDuration(ps.timeSlice)
		err = ps.pastRepo.ForEachDocument(ctx, index, quantity, func(doc mongo.StockDocument) {
			ps.out <- model.Packet{
				Sequnce: sequnce,
				Data: &model.StockAggregate{
					OpenTime:   doc.Timestamp,
					ClosedTime: doc.Timestamp + (duration.Milliseconds() / 1000),
					Open:       doc.Open,
					Closed:     doc.Close,
					High:       doc.High,
					Low:        doc.Low,
					Volume:     float32(doc.Volume),
				},
			}

			sequnce++
		})
		if err != nil {
			panic(err)
		}

	}()
}

func (ps *PastStock) Output() chan model.Packet {
	return ps.out
}

func (ps *PastStock) Close() error {
	ps.stop.NotifyStop()
	ps.wg.Wait()
	return nil
}
