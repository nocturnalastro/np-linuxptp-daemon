package cep

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"maps"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/auth"
	"k8s.io/apimachinery/pkg/util/wait"
)

const tlsReloadInterval = 5 * time.Second

type tlsSnapshot struct {
	digests    map[string][sha256.Size]byte
	server     *tls.Config
	client     *http.Client
	serverLeaf *x509.Certificate
	clientLeaf *x509.Certificate
}

type tlsMaterial struct {
	config  *auth.Config
	timeout time.Duration
	current atomic.Pointer[tlsSnapshot]
}

// reload is called by a single writer, first during startup and then by the
// listener's polling goroutine. Readers use immutable configuration snapshots.
func (m *tlsMaterial) reload() error {
	serverPaths := m.config.ServerTLSPaths()
	files, err := auth.ReadTLSFiles(nil, serverPaths[:]...)
	if err != nil {
		return err
	}
	var callbackErr error
	if m.config.EnableMTLS {
		files, callbackErr = auth.ReadTLSFiles(files, m.config.ClientCertPath, m.config.ClientKeyPath)
	}
	digests := make(map[string][sha256.Size]byte, len(files))
	for path, content := range files {
		if content != nil {
			digests[path] = sha256.Sum256(content)
		}
	}
	old := m.current.Load()
	unchanged := old != nil && maps.Equal(old.digests, digests)
	callbackKeypairPresent := m.config.EnableMTLS && callbackErr == nil &&
		files[m.config.ClientCertPath] != nil && files[m.config.ClientKeyPath] != nil
	// Retry a present keypair if the outbound client could not be initialized.
	if unchanged && (old.client != nil || !callbackKeypairPresent) {
		return nil
	}
	server, err := m.config.CreateServerTLSConfigFromFiles(files)
	if err != nil {
		return err
	}
	if m.config.EnableMTLS && server.ClientCAs == nil {
		return fmt.Errorf("mTLS requires an inbound client CA pool")
	}
	serverLeaf := server.Certificates[0].Leaf
	if leafErr := checkLeaf(serverLeaf, x509.ExtKeyUsageServerAuth); leafErr != nil {
		return fmt.Errorf("serving certificate: %w", leafErr)
	}
	// A new handshake must use the current trust pool, including after a CA is removed.
	server.SessionTicketsDisabled = true
	var client *http.Client
	var clientLeaf *x509.Certificate
	if !m.config.EnableMTLS || callbackKeypairPresent {
		var clientTLS *tls.Config
		clientTLS, err = m.config.CreateTLSConfigFromFiles(files)
		if err == nil {
			client, err = auth.ConfigureAuthenticatedClient(&http.Client{Timeout: m.timeout}, m.config, clientTLS)
		}
		if err == nil && m.config.EnableMTLS {
			clientLeaf = clientTLS.Certificates[0].Leaf
			err = checkLeaf(clientLeaf, x509.ExtKeyUsageClientAuth)
		}
		if err != nil {
			if !m.config.EnableMTLS {
				return err
			}
			callbackErr = err
			client, clientLeaf = nil, nil
		}
	}
	m.current.Store(&tlsSnapshot{
		digests: digests, server: server, client: client,
		serverLeaf: serverLeaf, clientLeaf: clientLeaf,
	})
	if callbackErr != nil && !unchanged {
		glog.Warningf("Event callback client unavailable: %v", callbackErr)
	}
	if old != nil {
		if old.client != nil {
			old.client.CloseIdleConnections()
		}
		if !unchanged || old.client != client {
			glog.Info("Event API TLS material reloaded")
		}
	}
	return nil
}

func (m *tlsMaterial) serverConfig(*tls.ClientHelloInfo) (*tls.Config, error) {
	snapshot := m.current.Load()
	if err := checkLeafTime(snapshot.serverLeaf); err != nil {
		return nil, err
	}
	return snapshot.server, nil
}

func checkLeaf(leaf *x509.Certificate, usage x509.ExtKeyUsage) error {
	if leaf == nil {
		return fmt.Errorf("missing parsed leaf certificate")
	}
	if err := checkLeafTime(leaf); err != nil {
		return err
	}
	if len(leaf.ExtKeyUsage) == 0 && len(leaf.UnknownExtKeyUsage) == 0 {
		return nil
	}
	for _, allowed := range leaf.ExtKeyUsage {
		if allowed == usage || allowed == x509.ExtKeyUsageAny {
			return nil
		}
	}
	return fmt.Errorf("certificate extended key usage does not allow its TLS role")
}

func checkLeafTime(leaf *x509.Certificate) error {
	if leaf != nil {
		now := time.Now()
		if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
			return fmt.Errorf("TLS certificate is outside its validity period")
		}
	}
	return nil
}

func (m *tlsMaterial) watch(ctx context.Context) {
	var lastError string
	_ = wait.PollUntilContextCancel(ctx, tlsReloadInterval, false, func(context.Context) (bool, error) {
		if err := m.reload(); err != nil {
			if err.Error() != lastError {
				glog.Warningf("Event API TLS reload failed; retaining previous configuration: %v", err)
			}
			lastError = err.Error()
		} else {
			lastError = ""
		}
		return false, nil
	})
}
