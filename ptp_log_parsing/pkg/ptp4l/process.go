package ptp4l

import (
	"bufio"
	"fmt"
	"os/exec"
	"ptplogparser/pkg/process"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	ProcessName = "ptp4l"
)

type Ptp4lProcess struct {
	statusMutex sync.RWMutex
	status      process.Status
	wg          sync.WaitGroup
	cmd         *exec.Cmd
	reader      *bufio.Reader
	lines       chan<- string
	opts        []string
	quit        <-chan struct{}
}

func NewProcess(lines chan<- string, opts []string) process.Process {
	return &Ptp4lProcess{
		lines: lines,
		opts:  opts,
	}
}

func (p *Ptp4lProcess) Name() string {
	return ProcessName
}

func (p *Ptp4lProcess) Status() process.Status {
	p.statusMutex.RLock()
	defer p.statusMutex.RUnlock()
	return p.status
}

func (p *Ptp4lProcess) setStatus(val process.Status) {
	p.statusMutex.Lock()
	p.status = val
	p.statusMutex.Unlock()
}

func (p *Ptp4lProcess) Start() error {
	if p.Status() != process.New && p.Status() != process.Dead {
		return nil
	}
	p.start()
	go p.processReads()
	return nil
}

func (p *Ptp4lProcess) Reset() {
	pid := p.cmd.Process.Pid
	glog.Infof("Stopping ptp4l with PID=%d", pid)
	_ = p.cmd.Process.Kill()
	if p.Status() != process.Stopped {
		p.setStatus(process.Dead)
	}
	p.cmd.Wait()
	p.start()
}

func (u *Ptp4lProcess) Stop() error {
	pid := u.cmd.Process.Pid
	glog.Infof("Stopping ptp4l with PID=%d", pid)
	u.setStatus(process.Stopped)
	_ = u.cmd.Process.Kill()
	u.cmd.Wait()
	u.wg.Done()
	return nil
}

const (
	processPath = "/usr/sbin/ptp4l"
)

func (p *Ptp4lProcess) start() error {
	joinedOpts := strings.Join(p.opts, " ")
	opts := strings.Fields(joinedOpts) // Lets not assue that opts were seperated properly beforehand
	fmt.Printf("%s %s\n", processPath, joinedOpts)
	p.cmd = exec.Command(processPath, opts...)
	stdoutreader, _ := p.cmd.StdoutPipe()
	// u.reader = bufio.NewReader(io.TeeReader(stdoutreader, os.Stdout))
	p.reader = bufio.NewReader(stdoutreader)
	err := p.cmd.Start()
	if err != nil {
		glog.Errorf("Failed to start ptp4l: %s", err.Error())
		p.setStatus(process.Stopped)
		return fmt.Errorf("failed to start ptp4l: %w", err)
	} else {
		p.setStatus(process.Running)
		pid := p.cmd.Process.Pid
		glog.Infof("Starting ptp4l process PID=%d", pid)
	}
	return nil
}

func (p *Ptp4lProcess) processReads() {
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
				glog.Errorf("ptp4l log read error %s", err)
			} else {
				continue
			}
			time.Sleep(time.Nanosecond)
		}
	}
}
