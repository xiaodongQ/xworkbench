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
	runServer func()
}

func (s *xworkbenchService) Execute(_ []string, svcCh <-chan svc.ChangeRequest, changesCh chan<- svc.Status) (ssec bool, errno uint32) {
	stopCh := make(chan struct{})
	serviceStopCh = stopCh // package-level so main's closure sees the same channel

	changesCh <- svc.Status{State: svc.StartPending}
	go s.runServer()
	changesCh <- svc.Status{State: svc.Running}

	for {
		select {
		case c := <-svcCh:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changesCh <- svc.Status{State: svc.StopPending}
				close(stopCh)
				return false, 0
			case svc.Interrogate:
				changesCh <- c.CurrentStatus
			}
		case <-stopCh:
			return false, 0
		}
	}
}

// runServiceFlag handles --service install/uninstall/run.
// Matches the non-Windows signature: func(startFn, stopFn func()) bool.
// Returns true if service mode was handled (caller should return).
func runServiceFlag(startFn, _ func()) bool {
	if serviceFlag == nil || *serviceFlag == "" {
		return false
	}

	switch *serviceFlag {
	case "run":
		err := svc.Run(windowsServiceName, &xworkbenchService{runServer: startFn})
		if err != nil {
			logger.Fatalf("Windows service error: %v", err)
		}
		return true

	case "install":
		if err := InstallService(); err != nil {
			logger.Fatalf("install service failed: %v", err)
		}
		logger.Infof("Service %q installed. Run 'net start %s' to start.", windowsServiceName, windowsServiceName)
		return true

	case "uninstall":
		if err := RemoveService(); err != nil {
			logger.Fatalf("uninstall service failed: %v", err)
		}
		logger.Infof("Service %q uninstalled.", windowsServiceName)
		return true

	default:
		logger.Fatalf("unknown --service value %q, expected: run | install | uninstall", *serviceFlag)
		return true
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
		// Service exists - update skipped on Windows
		if err := existing.Close(); err != nil {
			return fmt.Errorf("Close: %w", err)
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
		return fmt.Errorf("OpenService: %w", err)
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

// isWindowsService returns true on Windows.
func isWindowsService() bool { return true }

// shutdownHTTP initiates graceful shutdown of the HTTP server.
func shutdownHTTP(srv *http.Server) {
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
}

// handleWindowsSignals sets up signal handlers for Windows (console mode).
// Returns the stop channel.
func handleWindowsSignals() chan struct{} {
	stopCh := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		logger.Infow("shutdown signal received...")
		close(stopCh)
	}()
	return stopCh
}
