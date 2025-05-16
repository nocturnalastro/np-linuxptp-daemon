package process

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
)

type Status int8

const (
	New Status = iota
	Running
	Dead
	Stopped
)

type Process interface {
	Name() string
	Status() Status
	Start() error
	Stop() error
	Reset() error
}

func NewBaseProcess(name, processPath string, opts []string, outputChan chan<- string) Process {
	// Lets not assue that opts were seperated properly beforehand
	joinedOpts := strings.Join(opts, " ")
	opts = strings.Fields(joinedOpts)

	return &BaseProcess{
		name:        name,
		processPath: processPath,
		opts:        opts,
		quit:        make(chan struct{}),
		lines:       outputChan,
	}
}

type BaseProcess struct {
	statusMutex sync.RWMutex
	status      Status
	wg          sync.WaitGroup
	reader      *bufio.Reader
	cmd         *exec.Cmd
	quit        chan struct{}

	// provided
	name        string
	processPath string
	opts        []string
	lines       chan<- string
}

func (p *BaseProcess) Name() string {
	return p.name
}

func (p *BaseProcess) Status() Status {
	p.statusMutex.RLock()
	defer p.statusMutex.RUnlock()
	return p.status
}

func (p *BaseProcess) SetStatus(val Status) {
	p.statusMutex.Lock()
	p.status = val
	p.statusMutex.Unlock()
}

func (p *BaseProcess) Start() error {
	if p.Status() != New && p.Status() != Dead {
		return nil
	}
	p.start()
	go p.processReads()
	return nil
}

func (p *BaseProcess) Reset() error {
	pid := p.cmd.Process.Pid
	glog.Infof("Stopping %s with PID=%d", p.name, pid)
	_ = p.cmd.Process.Kill()
	if p.Status() != Stopped {
		p.SetStatus(Dead)
	}
	p.cmd.Wait()
	return p.start()
}

func (p *BaseProcess) Stop() error {
	pid := p.cmd.Process.Pid
	glog.Infof("Stopping %s with PID=%d", p.name, pid)
	p.SetStatus(Stopped)
	_ = p.cmd.Process.Kill()
	p.cmd.Wait()
	p.wg.Done()
	return nil
}

func (p *BaseProcess) start() error {
	p.cmd = exec.Command(p.processPath, p.opts...)
	stdoutreader, _ := p.cmd.StdoutPipe()

	// p.reader = bufio.NewReader(io.TeeReader(stdoutreader, os.Stdout))
	p.reader = bufio.NewReader(stdoutreader)
	err := p.cmd.Start()
	if err != nil {
		glog.Errorf("Failed to start %s: %s", p.name, err.Error())
		p.SetStatus(Stopped)
		return fmt.Errorf("failed to start %s: %w", p.name, err)
	} else {
		p.SetStatus(Running)
		glog.Infof("Starting %s process PID=%d", p.name, p.cmd.Process.Pid)
	}
	return nil
}

func (p *BaseProcess) processReads() {
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		select {
		case <-p.quit:
			return
		default:
			scanner := bufio.NewScanner(p.reader)
			for scanner.Scan() {
				select {
				case <-p.quit:
					return
				default:
				}

				line := scanner.Text()
				if line == "" {
					continue
				}
				p.lines <- line
			}

			if err := scanner.Err(); err != nil {
				glog.Errorf("%s log read error %s", p.name, err)
			} else {
				continue
			}
			time.Sleep(time.Nanosecond)
		}
	}
}
