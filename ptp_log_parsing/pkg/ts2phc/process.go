package ts2phc

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
	ProcessName = "phc2sys"
)

type Phc2SysProcess struct {
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
	return &Phc2SysProcess{
		lines: lines,
		opts:  opts,
	}
}

func (p *Phc2SysProcess) Name() string {
	return ProcessName
}

func (p *Phc2SysProcess) Status() process.Status {
	p.statusMutex.RLock()
	defer p.statusMutex.RUnlock()
	return p.status
}

func (p *Phc2SysProcess) setStatus(val process.Status) {
	p.statusMutex.Lock()
	p.status = val
	p.statusMutex.Unlock()
}

func (p *Phc2SysProcess) Start() error {
	if p.Status() != process.New && p.Status() != process.Dead {
		return nil
	}
	p.start()
	go p.processReads()
	return nil
}

func (p *Phc2SysProcess) Reset() {
	pid := p.cmd.Process.Pid
	glog.Infof("Stopping phc2sys polling with PID=%d", pid)
	_ = p.cmd.Process.Kill()
	if p.Status() != process.Stopped {
		p.setStatus(process.Dead)
	}
	p.cmd.Wait()
	p.start()
}

func (u *Phc2SysProcess) Stop() error {
	pid := u.cmd.Process.Pid
	glog.Infof("Stopping phc2sys with PID=%d", pid)
	u.setStatus(process.Stopped)
	_ = u.cmd.Process.Kill()
	u.cmd.Wait()
	u.wg.Done()
	return nil
}

const (
	processPath = "/usr/sbin/ts2phc"
)

func (p *Phc2SysProcess) start() error {
	joinedOpts := strings.Join(p.opts, " ")
	opts := strings.Fields(joinedOpts) // Lets not assue that opts were seperated properly beforehand
	fmt.Printf("%s %s\n", processPath, joinedOpts)
	p.cmd = exec.Command(processPath, opts...)
	stdoutreader, _ := p.cmd.StdoutPipe()
	// u.reader = bufio.NewReader(io.TeeReader(stdoutreader, os.Stdout))
	p.reader = bufio.NewReader(stdoutreader)
	err := p.cmd.Start()
	if err != nil {
		glog.Errorf("Failed to start ts2phc: %s", err.Error())
		p.setStatus(process.Stopped)
		return fmt.Errorf("failed to start ts2phc: %w", err)
	} else {
		p.setStatus(process.Running)
		pid := p.cmd.Process.Pid
		glog.Infof("Starting ts2phc process PID=%d", pid)
	}
	return nil
}

func (p *Phc2SysProcess) processReads() {
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
				glog.Errorf("phc2sys log read error %s", err)
			} else {
				continue
			}
			time.Sleep(time.Nanosecond)
		}
	}
}
