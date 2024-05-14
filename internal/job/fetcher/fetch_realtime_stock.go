package fetcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Goboolean/core-system.worker/internal/infrastructure/mongo"
	"github.com/Goboolean/core-system.worker/internal/job"
	"github.com/Goboolean/core-system.worker/internal/model"
	"github.com/Goboolean/core-system.worker/internal/util"
)

type RealtimeStock struct {
	Fetcher

	pastRepo mongo.StockClient

	//미리 가져올 데이터의 개수
	prefetchNum int
	timeSlice   string
	stockId     string

	out  chan any `type:"*StockAggregate"` //Job은 자신의 Output 채널에 대해 소유권을 가진다.
	wg   sync.WaitGroup
	stop *util.StopNotifier
}

func NewRealtimeStock(mongo mongo.StockClient, params *job.UserParams) (*RealtimeStock, error) {
	//여기에 기본값 입력 아웃풋 채널은 job이 소유권을 가져야 한다.
	instance := &RealtimeStock{
		out:  make(chan any),
		stop: util.NewStopNotifier(),
	}

	if !params.IsKeyNullOrEmpty("productId") {

		val, ok := (*params)["productId"]
		if !ok {
			return nil, fmt.Errorf("create past stock fetch job: %w", ErrInvalidStockId)
		}

		instance.stockId = val
	}

	return instance, nil
}

func (rt *RealtimeStock) Execute() {
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		defer rt.stop.NotifyStop()
		defer close(rt.out)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-rt.stop.Done()
			cancel()
		}()

		rt.pastRepo.SetTarget(rt.stockId, rt.timeSlice)

		//prefetch past stock data
		count := rt.pastRepo.GetCount(ctx)
		duration, _ := time.ParseDuration(rt.timeSlice)

		err := rt.pastRepo.ForEachDocument(ctx, (count-1)-(rt.prefetchNum), rt.prefetchNum, func(doc mongo.StockDocument) {

			rt.out <- &model.StockAggregate{
				OpenTime:   doc.Timestamp,
				ClosedTime: doc.Timestamp + (duration.Milliseconds() / 1000),
				Open:       doc.Open,
				Closed:     doc.Close,
				High:       doc.High,
				Low:        doc.Low,
				Volume:     float32(doc.Volume),
			}
		})

		if err != nil {
			panic(err)
		}

		for {
			select {
			case <-rt.stop.Done():
				return
				//case <- karfka:

				// 알맞게 변환하기
				// out에다가 던지기
			}
		}
	}()

}

func (rt *RealtimeStock) Output() chan any {
	return rt.out
}

func (rt *RealtimeStock) Close() error {
	rt.stop.NotifyStop()
	rt.wg.Wait()
	return nil
}
