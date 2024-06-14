package fetcher_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Goboolean/core-system.worker/internal/job"
	"github.com/Goboolean/core-system.worker/internal/job/fetcher"
	"github.com/Goboolean/core-system.worker/internal/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func makeStockAggregateExample() *model.StockAggregate {
	return &model.StockAggregate{
		OpenTime:   0,
		ClosedTime: 0,
		Open:       12,
		Close:      150,
		High:       150,
		Low:        23,
		Volume:     12,
	}
}

// TestPastStock is a unit test function that tests the functionality of fetching past stock data.
// It verifies that the fetched stock data matches the expected results.
func TestPastStock(t *testing.T) {
	t.Run("Past stock fetch 테스트", func(t *testing.T) {
		num := 100
		productID := "stock.aapl.usa"
		timeFrame := "1m"
		productType := "stock"
		startTime := time.Now().AddDate(-1, 0, 0).Truncate(time.Second)
		endTime := time.Now().Truncate(time.Second)

		ctl := gomock.NewController(t)

		mockSession := fetcher.NewMockFetchingSession(ctl)

		mockSession.EXPECT().Next().Return(false).Times(1).
			After(mockSession.EXPECT().Next().Return(true).Times(num))
		mockSession.EXPECT().Value(gomock.Any()).
			Return(makeStockAggregateExample(), nil).Times(num)

		mockRepo := fetcher.NewMockTradeRepository(ctl)
		mockRepo.EXPECT().SelectProduct(productID, timeFrame, productType)
		mockRepo.EXPECT().SetRangeByTime(startTime, endTime)
		mockRepo.EXPECT().Session().Return(mockSession, nil)
		mockRepo.EXPECT().Close()

		fetchJob, err := fetcher.NewPastStock(mockRepo, &job.UserParams{
			job.ProductID: productID,
			job.StartDate: fmt.Sprint(startTime.Unix()),
			job.EndDate:   fmt.Sprint(endTime.Unix()),
		})

		outCh := fetchJob.Output()

		fetchJob.Execute()
		outData := make([]model.Packet, 0, num)
		for v := range outCh {
			outData = append(outData, v)
		}

		assert.NoError(t, err)
		assert.Equal(t, num, len(outData))
		for _, e := range outData {
			assert.Equal(t, makeStockAggregateExample(), e.Data)
		}
	})
}
