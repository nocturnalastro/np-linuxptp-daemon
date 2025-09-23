package main

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/daemon"
	ptpv1 "github.com/k8snetworkplumbingwg/ptp-operator/api/v1"
)

func main() {
	fmt.Printf("Git commit: %s\n", GitCommit)
	setup := setupVars{}

	stopCh, err := setup.Setup()
	if err != nil {
		glog.Error(err)
		return
	}
	defer close(stopCh)

	leapManagerCh, err := setup.setupLeapManager()
	if err != nil {
		glog.Error(err)
		return
	}
	defer close(leapManagerCh)

	setup.labelPod()

	hwconfigs := []ptpv1.HwConfig{}
	refreshNodePtpDevice := true
	closeProcessManager := make(chan bool)

	tracker := &daemon.ReadyTracker{}

	go daemon.New(
		setup.nodeName,
		daemon.PtpNamespace,
		setup.stdoutToSocket,
		setup.kubeClient,
		setup.confUpdate,
		setup.stopCh,
		setup.plugins,
		&hwconfigs,
		&refreshNodePtpDevice,
		closeProcessManager,
		setup.cp.pmcPollInterval,
		tracker,
	).Run()

	glog.Info("H")
	// by default metrics is hosted here,if LOGS_TO_SOCKET variable is set then metrics are disabled
	if !setup.stdoutToSocket { // if not sending metrics (log) out to a socket then host metrics here
		daemon.StartMetricsServer("0.0.0.0:9091")
	}
	daemon.StartReadyServer("0.0.0.0:8081", tracker)

	// Wait for one ticker interval before loading the profile
	// This allows linuxptp-daemon connection to the cloud-event-proxy container to
	// be up and running before PTP state logs are printed.
	time.Sleep(setup.getUpdateInterval() / 2)

	// Setup Controller if requested and get update func
	updateFunc, err := setup.getUpdateFunc(setup.cp.useController)
	if err != nil {
		return
	}

	tickerPull := time.NewTicker(setup.getUpdateInterval())
	defer tickerPull.Stop()
	for {
		select {
		case <-tickerPull.C:
			updateFunc(refreshNodePtpDevice, hwconfigs)
		case sig := <-setup.sigCh:
			glog.Info("signal received, shutting down", sig)
			closeProcessManager <- true
			if setup.cp.useController {
				setup.ctlrMgrCancel()
			}
			return
		}
	}
}
