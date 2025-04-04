package main

import (
	"flag"
	"ptplogparser/pkg/process"
	"ptplogparser/pkg/ublox"
	"time"
)

func main() {
	flag.Set("v", "2")
	flag.Parse()

	ch := make(chan process.Event)
	u := ublox.New(ch)
	u.Start()
	time.Sleep(5 * time.Minute)
	u.Stop()
}
