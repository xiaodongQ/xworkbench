//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const windowsServiceName = "xworkbench"
const windowsServiceDisplayName = "xworkbench HTTP Server"

var serviceFlag = flag.String("service", "", `"run" starts service, "install" registers, "uninstall" removes`)

// serviceStopCh is closed by the service when it receives a stop/shutdown request.
var serviceStopCh chan struct{}

// xworkbenchService implements svc.Service.
type xworkbenchService struct {
	runServer func(stopCh chan struct{})
}

func (s *xworkbenchService) Execute(_ []string, svcCh <-chan svc.ChangeRequest, changesCh chan<- svc.Status) (ssec bool, errno uint32) {
	svcStopCh := make(chan struct{})
	serviceStopCh = svcStopCh // package-level so main's closure sees the same channel

	changesCh <- svc.Status{State: svc.StartPending}
	go s.runServer(svcStopCh)
	changesCh <- svc.Status{State: svc.Running}

	for {
		select {
		case c := <-svcCh:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changesCh <- svc.Status{State: svc.StopPending}
				close(svcStopCh)
				time.Sleep(30 * time.Second)
				return false, 0
			case svc.Interrogate:
				changesCh <- c.CurrentStatus
			}
		case <-svcStopCh:
			return false, 0
		}
	}
}

// runServiceFlag handles --service install/uninstall/run.
// startFn is the HTTP server starter (passed as a closure from main so it captures local vars).
// Returns (handled bool, stopCh chan struct{}) - if handled==true, caller should return.
// stopCh is non-nil when handled==false and service mode was detected (Windows non-service).
func runServiceFlag(startFn func(chan struct{})) (handled bool, stopCh chan struct{}) {
	if serviceFlag == nil || *serviceFlag == "" {
		return false, nil
	}

	switch *serviceFlag {
	case "run":
		err := svc.Run(windowsServiceName, &xworkbenchService{runServer: startFn})
		if err != nil {
			logger.Fatalf("Windows service error: %v", err)
		}
		return true, nil

	case "install":
		if err := InstallService(); err != nil {
			logger.Fatalf("install service failed: %v", err)
		}
		logger.Infof("Service %q installed. Run 'net start %s' to start.", windowsServiceName, windowsServiceName)
		return true, nil

	case "uninstall":
		if err := RemoveService(); err != nil {
			logger.Fatalf("uninstall service failed: %v", err)
		}
		logger.Infof("Service %q uninstalled.", windowsServiceName)
		return true, nil

	default:
		logger.Fatalf("unknown --service value %q, expected: run | install | uninstall", *serviceFlag)
		return true, nil
	}
}

// InstallService registers xworkbench as a Windows Service.
func InstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}

	svcCfg := mgr.Config{
		DisplayName: windowsServiceDisplayName,
		Description: "xworkbench AI CLI task management server",
		StartType:   mgr.StartAutomatic,
	}

	existing, err := m.OpenService(windowsServiceName)
	if err == nil {
		existing.Close()
		if err := m.UpdateServiceConfig(windowsServiceName, svcCfg); err != nil {
			return fmt.Errorf("UpdateServiceConfig: %w", err)
		}
	} else {
		s, err := m.CreateService(windowsServiceName, exePath, svcCfg, "--service", "run")
		if err != nil {
			return fmt.Errorf("CreateService: %w", err)
		}
		s.Close()
	}
	return nil
}

// RemoveService uninstalls xworkbench from Windows Services.
func RemoveService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(windowsServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", windowsServiceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

// isWindowsService returns true on Windows.
func isWindowsService() bool { return true }

// shutdownHTTP is called on Windows when the service receives a stop.
// It gracefully shuts down the HTTP server within 30s.
func shutdownHTTP(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorw("http shutdown failed", "err", err)
	}
}

// handleWindowsSignals starts a goroutine that closes stopCh on SIGINT/SIGTERM.
// Returns the stopCh. Only used when running on Windows interactively (not as a service).
func handleWindowsSignals() chan struct{} {
	stopCh := make(chan struct{})
	go func() {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		logger.Infow("shutdown signal received...")
		close(stopCh)
	}()
	return stopCh
}