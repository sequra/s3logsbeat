package input

import (
	"fmt"
	"sync"
	"time"

	"github.com/sequra/s3logsbeat/pipeline"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/mitchellh/hashstructure"
)

// Input is the interface common to all input
type Input interface {
	Run()
	Stop()
	Wait()
}

// Runner encapsulate the lifecycle of the input
type Runner struct {
	config    GlobalConfig
	input     Input
	done      chan struct{}
	wg        *sync.WaitGroup
	ID        uint64
	Once      bool
	beatDone  chan struct{}
	outSQS    chan *pipeline.SQS
	outS3List chan *pipeline.S3List
}

// New instantiates a new Runner
func New(
	conf *common.Config,
	beatDone chan struct{},
	outSQS chan *pipeline.SQS,
	outS3List chan *pipeline.S3List,
) (*Runner, error) {
	input := &Runner{
		config:    defaultConfig,
		wg:        &sync.WaitGroup{},
		done:      make(chan struct{}),
		Once:      false,
		beatDone:  beatDone,
		outSQS:    outSQS,
		outS3List: outS3List,
	}

	var err error
	if err = conf.Unpack(&input.config); err != nil {
		return nil, err
	}

	var h map[string]interface{}
	conf.Unpack(&h)
	input.ID, err = hashstructure.Hash(h, nil)
	if err != nil {
		return nil, err
	}

	var f Factory
	f, err = GetFactory(input.config.Type)
	if err != nil {
		return input, err
	}

	context := Context{
		Done:      input.done,
		BeatDone:  input.beatDone,
		OutSQS:    input.outSQS,
		OutS3List: input.outS3List,
	}
	var ipt Input
	ipt, err = f(conf, context)
	if err != nil {
		return input, err
	}
	input.input = ipt

	return input, nil
}

// Start starts the input
func (p *Runner) Start() {
	p.wg.Add(1)
	logp.Info("Starting input of type: %v; ID: %d ", p.config.Type, p.ID)

	onceWg := sync.WaitGroup{}
	if p.Once {
		// Make sure start is only completed when Run did a complete first scan
		defer onceWg.Wait()
	}

	onceWg.Add(1)
	// Add waitgroup to make sure input is finished
	go func() {
		defer func() {
			onceWg.Done()
			p.stop()
			p.wg.Done()
		}()

		p.Run()
	}()
}

// Run starts scanning through all the file paths and fetch the related files. Start a harvester for each file
func (p *Runner) Run() {
	// Initial input run
	p.input.Run()

	// Shuts down after the first complete run of all input
	if p.Once {
		return
	}

	logp.Debug("s3logsbeat", "Running input with ID=%d each %s", p.ID, p.config.PollFrequency.String())

	for {
		select {
		case <-p.done:
			logp.Info("input ticker stopped")
			return
		case <-time.After(p.config.PollFrequency):
			logp.Debug("s3logsbeat", "Tick on input with ID=%d", p.ID)
			p.input.Run()
		}
	}
}

// Stop stops the input and with it all harvesters
func (p *Runner) Stop() {
	// Stop scanning and wait for completion
	close(p.done)
	p.wg.Wait()
}

func (p *Runner) stop() {
	logp.Info("Stopping Input: %d", p.ID)

	// In case of once, it will be waited until harvesters close itself
	if p.Once {
		p.input.Wait()
	} else {
		p.input.Stop()
	}
}

func (p *Runner) String() string {
	return fmt.Sprintf("input [type=%s, ID=%d]", p.config.Type, p.ID)
}

// Type gets the input type
func (p *Runner) Type() string {
	return p.config.Type
}
