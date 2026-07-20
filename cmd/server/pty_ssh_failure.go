// pty_ssh_failure.go SSH 连接失败检测，Unix/Windows 共用（无 build tag）。
package main

import "strings"

// sshConnectFailurePatterns 检测 SSH 连接失败的输出特征。
var sshConnectFailurePatterns = []string{
	"No route to host",
	"no route to host",
	"Connection refused",
	"connection refused",
	"Connection timed out",
	"connection timed out",
	"Connection reset by peer",
	"connection reset by peer",
	"Network is unreachable",
	"network is unreachable",
	"Name or service not known",
	"Could not resolve hostname",
	"could not resolve hostname",
	"No such host",
	"ssh: Could not resolve",
	"ssh_exchange_identification",
	"Connection closed by remote host",
	"too many authentication failures",
	"Received disconnect",
	"Disconnected from remote",
}

// detectSSHConnectFailure 检查一行是否包含 SSH 连接失败特征。
func detectSSHConnectFailure(line string) bool {
	for _, p := range sshConnectFailurePatterns {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}
