package parser

import (
	"fmt"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/process"
	"sync"
	"time"
)

type Parser interface {
	Start() error
	Stop(wait bool) error
}

type ParseLineFunc func(string) (events.Event, error, bool)

type BaseParser struct {
	name      string
	process   process.Process
	lines     <-chan string
	events    chan<- events.Event
	quit      chan struct{}
	wg        sync.WaitGroup
	parseLine ParseLineFunc
}

func NewParser(
	name string,
	lines <-chan string,
	events chan<- events.Event,
	process process.Process,
	parseLine ParseLineFunc,
) *BaseParser {
	return &BaseParser{
		name:      name,
		process:   process,
		lines:     lines,
		events:    events,
		quit:      make(chan struct{}),
		parseLine: parseLine,
	}
}

func (b *BaseParser) Start() error {
	go b.parse()
	return nil
}

func (b *BaseParser) Stop(wait bool) error {
	b.quit <- struct{}{}
	err := b.process.Stop()
	if err != nil {
		return err
	}
	if wait {
		b.wg.Wait()
	}
	return nil
}

func (b *BaseParser) parse() {
	b.wg.Add(1)
	defer b.wg.Done()
	for {
		select {
		case <-b.quit:
			return
		default:
			time.Sleep(time.Nanosecond)
		case line := <-b.lines:
			event, err, triedToParse := b.parseLine(line)
			if !triedToParse {
				continue
			}
			if err != nil {
				fmt.Println(fmt.Errorf("failed to parse ptp4l line: %s", err))
			}
			b.events <- event
		}
	}
}
