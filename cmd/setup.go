package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/config"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/controller"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/daemon"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/features"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/leap"
	ptpv1 "github.com/k8snetworkplumbingwg/ptp-operator/api/v1"
	ptpclient "github.com/k8snetworkplumbingwg/ptp-operator/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Git commit of current build set at build time
var GitCommit = "Undefined"

type cliParams struct {
	updateInterval  int
	profileDir      string
	pmcPollInterval int
	useController   bool
}

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ptpv1.AddToScheme(scheme))
}

// Parse Command line flags
func flagInit(cp *cliParams) {
	flag.IntVar(&cp.updateInterval, "update-interval", config.DefaultUpdateInterval,
		"Interval to update PTP status")
	flag.StringVar(&cp.profileDir, "linuxptp-profile-path", config.DefaultProfilePath,
		"profile to start linuxptp processes")
	flag.IntVar(&cp.pmcPollInterval, "pmc-poll-interval", config.DefaultPmcPollInterval,
		"Interval for periodical PMC poll")
	flag.BoolVar(&cp.useController, "use-controller", false,
		"Use Kubernetes controller to watch PtpConfig resources (default: false)")
}

type setupVars struct {
	kubeConfig     *rest.Config
	kubeClient     *kubernetes.Clientset
	ptpClient      *ptpclient.Clientset
	confUpdate     *daemon.LinuxPTPConfUpdate
	nodeName       string
	podName        string
	namespace      string
	plugins        []string
	stdoutToSocket bool
	stopCh         chan struct{}
	leapManager    *leap.LeapManager
	cp             *cliParams
	ctlrMgrCancel  context.CancelFunc
	sigCh          chan os.Signal
}

func (sv *setupVars) Setup() (chan struct{}, error) {
	sv.parseFlags()

	sv.stopCh = make(chan struct{})
	sv.sigCh = make(chan os.Signal, 1)
	signal.Notify(sv.sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	err := sv.setConfUpdate()
	if err != nil {
		return sv.stopCh, err
	}
	err = sv.setEnvVars()
	if err != nil {
		return sv.stopCh, err
	}
	err = sv.setClients()
	if err != nil {
		return sv.stopCh, err
	}
	sv.initFeatures()
	return sv.stopCh, nil
}

func (sv *setupVars) setConfUpdate() error {
	ptpConfUpdate, err := daemon.NewLinuxPTPConfUpdate()
	if err != nil {
		glog.Errorf("failed to create a ptp config update: %v", err)
		return fmt.Errorf("failed to create a ptp config update: %v", err)
	}
	sv.confUpdate = ptpConfUpdate
	return nil
}

func (sv *setupVars) parseFlags() {
	cp := &cliParams{}
	flagInit(cp)
	flag.Parse()
	sv.cp = cp

	glog.Infof("resync period set to: %d [s]", cp.updateInterval)
	glog.Infof("linuxptp profile path set to: %s", cp.profileDir)
	glog.Infof("pmc poll interval set to: %d [s]", cp.pmcPollInterval)
}

func (sv *setupVars) setClients() error {
	cfg, err := config.GetKubeConfig()
	if err != nil {
		glog.Errorf("get kubeconfig failed: %v", err)
		return fmt.Errorf("get kubeconfig failed: %v", err)
	}
	sv.kubeConfig = cfg

	glog.Infof("successfully get kubeconfig")

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Errorf("cannot create new config for kubeClient: %v", err)
		return fmt.Errorf("cannot create new config for kubeClient: %v", err)
	}
	sv.kubeClient = kubeClient

	ptpClient, err := ptpclient.NewForConfig(cfg)
	if err != nil {
		glog.Errorf("cannot create new config for ptpClient: %v", err)
		return fmt.Errorf("cannot create new config for ptpClient: %v", err)
	}
	sv.ptpClient = ptpClient
	return nil
}

func (sv *setupVars) setEnvVars() error {
	sv.nodeName = os.Getenv("NODE_NAME")
	sv.podName = os.Getenv("POD_NAME")
	sv.plugins = make([]string, 0)
	sv.stdoutToSocket = false

	if sv.nodeName == "" {
		glog.Error("cannot find NODE_NAME environment variable")
		return errors.New("cannot find NODE_NAME environment variable")
	}

	if val, ok := os.LookupEnv("LOGS_TO_SOCKET"); ok && val != "" {
		if ret, err := strconv.ParseBool(val); err == nil {
			sv.stdoutToSocket = ret
		}
	}

	if val, ok := os.LookupEnv("PLUGINS"); ok && val != "" {
		sv.plugins = strings.Split(val, ",")
	}
	return nil
}

func (sv *setupVars) setupLeapManager() (chan bool, error) {
	lm, err := leap.New(sv.kubeClient, daemon.PtpNamespace)
	if err != nil {
		glog.Error("failed to initialize Leap manager, ", err)
		return lm.Close, fmt.Errorf("failed to initialize Leap manager, %w", err)
	}
	sv.leapManager = lm
	go lm.Run()
	return lm.Close, nil
}

type updateFuncType func(bool, []ptpv1.HwConfig)

func (sv *setupVars) refreshNodeDeviceAndProfile(refreshNodePtpDevice bool, hwconfigs []ptpv1.HwConfig) {
	sv.refreshNodeDevice(refreshNodePtpDevice, hwconfigs)

	nodeProfile := filepath.Join(sv.cp.profileDir, sv.nodeName)
	if _, err := os.Stat(nodeProfile); err != nil {
		glog.Errorf("error stating node profile %v: %v", sv.nodeName, err)
		return
	}
	nodeProfilesJSON, err := os.ReadFile(nodeProfile)
	if err != nil {
		glog.Errorf("error reading node profile: %v", nodeProfile)
		return
	}

	err = sv.confUpdate.UpdateConfig(nodeProfilesJSON)
	if err != nil {
		glog.Errorf("error updating the node configuration using the profiles loaded: %v", err)
	}
}

func (sv *setupVars) refreshNodeDevice(refreshNodePtpDevice bool, hwconfigs []ptpv1.HwConfig) {
	if sv.cp.useController {
		glog.Infof("ticker pull (controller mode)")
	} else {
		glog.Infof("ticker pull")
	}
	// Run a loop to update the device status
	if refreshNodePtpDevice {
		go daemon.RunDeviceStatusUpdate(sv.ptpClient, sv.nodeName, &hwconfigs)
		refreshNodePtpDevice = false
	}
}

func (sv *setupVars) initFeatures() {
	version := features.GetLinuxPTPPackageVersion()
	ocpVersion := features.GetOCPVersion()
	// TODO: version needs to be sent to cloud event proxy when we intergrate feature flags there
	features.SetFlags(version, ocpVersion)
	features.Flags.Print()

}

func (sv *setupVars) setupController() error {
	glog.Info("Setting up controller manager for PtpConfig resources")
	// Create manager
	mgr, err := ctrl.NewManager(sv.kubeConfig, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: ":8082",
		LeaderElection:         false, // Disable leader election for daemon
		// PtpConfig is cluster-scoped, so don't restrict cache by namespace
	})
	if err != nil {
		glog.Errorf("unable to start controller manager: %v", err)
		return err
	}

	// Add health checks
	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		glog.Errorf("unable to set up health check: %v", err)
		return err
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		glog.Errorf("unable to set up ready check: %v", err)
		return err
	}

	// Setup PtpConfig controller
	ptpConfigReconciler := &controller.PtpConfigReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		NodeName:     sv.nodeName,
		ConfigUpdate: sv.confUpdate,
	}

	if err = ptpConfigReconciler.SetupWithManager(mgr); err != nil {
		glog.Errorf("unable to create controller for PtpConfig: %v", err)
		return err
	}

	// Start the manager in a goroutine
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	go func() {
		glog.Info("Starting controller manager")
		if err = mgr.Start(mgrCtx); err != nil {
			glog.Errorf("problem running controller manager: %v", err)
		}
	}()
	sv.ctlrMgrCancel = mgrCancel

	// When using controller mode, we only need to handle device status updates and shutdown signals
	// The controller will handle configuration updates automatically via PtpConfig watches
	glog.Info("Running in controller mode - PtpConfig resources will be watched automatically")

	// Trigger initial reconciliation by listing all PtpConfigs
	time.Sleep(2 * time.Second)
	go func() {
		glog.Info("Triggering initial PtpConfig reconciliation")

		ptpConfigReconciler := &controller.PtpConfigReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			NodeName:     sv.nodeName,
			ConfigUpdate: sv.confUpdate,
		}

		_, err := ptpConfigReconciler.Reconcile(context.Background(), ctrl.Request{})
		if err != nil {
			glog.Errorf("Initial reconciliation failed: %v", err)
		}
	}()
	return nil
}

func (sv *setupVars) getUpdateFunc(useController bool) (updateFuncType, error) {
	if useController {
		err := sv.setupController()
		if err != nil {
			return nil, err
		}
		return sv.refreshNodeDevice, nil
	} else {
		return sv.refreshNodeDeviceAndProfile, nil
	}

}

func (sv *setupVars) getUpdateInterval() time.Duration {
	return time.Second * time.Duration(sv.cp.updateInterval)
}

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

func (sv *setupVars) labelPod() error {
	// label the current linux-ptp-daemon pod with a nodeName label
	pod, err := sv.kubeClient.CoreV1().Pods(daemon.PtpNamespace).Get(context.TODO(), sv.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting linuxptp-daemon pod, err=%s", err)
	}
	if pod == nil {
		return fmt.Errorf("could not find linux-ptp-daemon pod to label")
	}
	if sv.nodeName != "" && strings.Contains(sv.nodeName, ".") {
		sv.nodeName = strings.Split(sv.nodeName, ".")[0]
	}

	payload := []patchStringValue{{
		Op:    "replace",
		Path:  "/metadata/labels/nodeName",
		Value: sv.nodeName,
	}}
	payloadBytes, _ := json.Marshal(payload)

	_, err = sv.kubeClient.CoreV1().Pods(pod.GetNamespace()).Patch(context.TODO(), pod.GetName(), types.JSONPatchType, payloadBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("could not label ns=%s pod %s, err=%s", pod.GetName(), pod.GetNamespace(), err)
	}
	glog.Infof("Pod %s labelled successfully.", pod.GetName())
	return nil
}
