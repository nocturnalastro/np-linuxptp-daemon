package main

import (
	"flag"
	"fmt"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/ublox"
	"sync"
	"time"
)

func main() {
	flag.Set("v", "2")
	flag.Parse()

	wg := sync.WaitGroup{}
	events := make(chan events.Event, 10)
	quit := make(chan bool, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-events:
				fmt.Println(e)
			case <-quit:
				return
			}
			time.Sleep(time.Nanosecond)
		}
	}()

	lines := make(chan string, 100)
	process := ublox.NewProcess(lines)
	parser := ublox.NewParser(lines, events, &process)

	parser.Start()
	time.Sleep(5 * time.Minute)
	quit <- true
	parser.Stop(true)
	wg.Wait()
}
