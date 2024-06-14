package pipeline

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/Goboolean/core-system.worker/internal/job/adapter"
	"github.com/Goboolean/core-system.worker/internal/job/analyzer"
	"github.com/Goboolean/core-system.worker/internal/job/executer"
	"github.com/Goboolean/core-system.worker/internal/job/fetcher"
	"github.com/Goboolean/core-system.worker/internal/job/joiner"
	"github.com/Goboolean/core-system.worker/internal/job/transmitter"
	"github.com/Goboolean/core-system.worker/internal/model"
	"github.com/Goboolean/core-system.worker/internal/util"
)

var ErrTypeNotMatch = errors.New("pipeline: cannot build a pipeline because the types are not compatible between the jobs")

// 아키텍처 설계 상 이 구조는 변경되면 안 된다.
type Normal struct {
	//jobs
	fetcher       fetcher.Fetcher
	joiner        joiner.Joiner
	modelExecuter executer.ModelExecutor
	adapter       adapter.Adapter
	resAnalyzer   analyzer.Analyzer
	transmitter   transmitter.Transmitter

	//utils
	mux util.ChannelMux[model.Packet]
}

func newNormalWithAdapter(
	fetcher fetcher.Fetcher,
	joiner joiner.Joiner,
	modelExecuter executer.ModelExecutor,
	adapter adapter.Adapter,
	resAnalyzer analyzer.Analyzer,
	transmitter transmitter.Transmitter) (*Normal, error) {

	instance := Normal{
		fetcher:       fetcher,
		joiner:        joiner,
		modelExecuter: modelExecuter,
		adapter:       adapter,
		resAnalyzer:   resAnalyzer,
		transmitter:   transmitter,

		mux: util.ChannelMux[model.Packet]{},
	}

	instance.mux.SetInput(instance.fetcher.Output())
	instance.modelExecuter.SetInput(instance.mux.Output())
	instance.adapter.SetInput(instance.modelExecuter.Output())
	instance.joiner.SetModelInput(instance.adapter.Output())
	instance.joiner.SetRefInput(instance.mux.Output())
	instance.resAnalyzer.SetInput(instance.joiner.Output())
	instance.transmitter.SetInput(instance.resAnalyzer.Output())

	return &instance, nil
}

func newNormalWithoutAdapter(
	fetch fetcher.Fetcher,
	join joiner.Joiner,
	modelExec executer.ModelExecutor,
	resAnalyze analyzer.Analyzer,
	transmit transmitter.Transmitter) (*Normal, error) {

	instance := Normal{
		fetcher:       fetch,
		modelExecuter: modelExec,
		joiner:        join,
		resAnalyzer:   resAnalyze,
		transmitter:   transmit,
	}

	instance.mux.SetInput(instance.fetcher.Output())
	instance.modelExecuter.SetInput(instance.mux.Output())
	instance.joiner.SetModelInput(instance.modelExecuter.Output())
	instance.joiner.SetRefInput(instance.mux.Output())
	instance.resAnalyzer.SetInput(instance.joiner.Output())
	instance.transmitter.SetInput(instance.resAnalyzer.Output())

	return &instance, nil

}

func (n *Normal) Run() {

	n.fetcher.Execute()
	n.modelExecuter.Execute()
	if !reflect.ValueOf(n.adapter).IsNil() {
		n.adapter.Execute()
	}
	n.joiner.Execute()
	n.resAnalyzer.Execute()
	n.transmitter.Execute()
}

func (n *Normal) Stop() error {

	if err := n.fetcher.Close(); err != nil {
		return fmt.Errorf("pipeline: failed to shutdown fetch job %w", err)
	}
	if err := n.modelExecuter.Close(); err != nil {
		return fmt.Errorf("pipeline: failed to shutdown model execute job %w", err)
	}
	if err := n.adapter.Close(); err != nil {
		return fmt.Errorf("pipeline: failed to shutdown adapt job %w", err)
	}
	if err := n.resAnalyzer.Close(); err != nil {
		return fmt.Errorf("pipeline: failed to shutdown analyze job %w", err)
	}
	if err := n.transmitter.Close(); err != nil {
		return fmt.Errorf("pipeline: failed to shutdown transmit job %w", err)
	}

	return nil
}
