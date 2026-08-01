package executor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/logger"
	"golang.org/x/crypto/ssh"
)

// readPrivateKey 从文件读取 ssh.Signer（支持 RSA / Ed25519 / ECDSA）。
// passphrase 为空时使用 ssh.ParsePrivateKey；非空时使用 ssh.ParsePrivateKeyWithPassphrase 解密。
func readPrivateKey(path string, passphrase string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(data)
}

// quoteArgs 把 cmd 数组拼成可被远端 sh -c 接受的字符串。
// 含空格的元素用单引号包裹；单引号本身用 '\'' 经典 escape。
func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		// 空字符串 → '' ，避免 join 后变成无引号空
		if a == "" {
			out[i] = "''"
			continue
		}
		// 含 shell metacharacter / 空格 / 引号 / 反斜杠 → 包裹
		if strings.ContainsAny(a, " \t\n\"'`$\\;|&<>(){}*?!#~") {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			out[i] = a
		}
	}
	return out
}

// streamLines 从 r 读行，回调 onChunk（每行含 "\n"），并把内容累计到 builder。
// isErr=true 时给 chunk 加 "[err] " 前缀（与本地 executor.Run 行为对齐）。
func streamLines(r io.Reader, isErr bool, onChunk func(string), builder *strings.Builder) {
	scanner := bufio.NewScanner(r)
	// claude -p 输出单行可能超过 64K（json dump），放大到 1MB
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		if isErr {
			line = "[err] " + line
		}
		builder.WriteString(line)
		if onChunk != nil {
			onChunk(line)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		logger.Logger.Warnw("ssh: streamLines read error", "is_err", isErr, "error", err.Error())
	}
}

// ensureRemoteBinary 检查远端是否安装了 CLI 工具，返回绝对路径。
// 非交互 SSH 的 PATH 很窄，需要依次尝试多种方式加载完整环境。
func ensureRemoteBinary(client *ssh.Client, bin string) (string, error) {
	if bin == "" {
		return "", fmt.Errorf("binary name is empty")
	}
	for _, cmd := range []string{
		// 1. 当前窄 PATH
		fmt.Sprintf("command -v %s 2>/dev/null", bin),
		// 2. 常见标准路径
		fmt.Sprintf("test -x /usr/local/bin/%s && echo /usr/local/bin/%s", bin, bin),
		fmt.Sprintf("test -x $HOME/.local/bin/%s && echo $HOME/.local/bin/%s", bin, bin),
		fmt.Sprintf("test -x /opt/homebrew/bin/%s && echo /opt/homebrew/bin/%s", bin, bin),
		// 3. source .zshrc / .bashrc（PATH 通常配在这些 rc 文件里）
		fmt.Sprintf("zsh -c 'source ~/.zshenv 2>/dev/null; source ~/.zshrc 2>/dev/null; command -v %s' 2>/dev/null", bin),
		fmt.Sprintf("bash -c 'set +m; source ~/.bashrc 2>/dev/null; command -v %s' 2>/dev/null", bin),
		// 4. login shell
		fmt.Sprintf("bash -l -c 'command -v %s' 2>/dev/null", bin),
		// 5. find 兜底扫描
		fmt.Sprintf("find /usr/local /opt /home -maxdepth 6 -name %s -type f -executable 2>/dev/null | head -1", bin),
	} {
		sess, err := client.NewSession()
		if err != nil {
			return "", fmt.Errorf("ensure: new session: %w", err)
		}
		out, sessErr := sess.Output(cmd)
		sess.Close()
		if sessErr != nil {
			continue
		}
		p := strings.TrimSpace(string(out))
		if p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("binary %q not found on remote", bin)
}

// EnsureRemoteBinary 连接远端并检查指定二进制是否存在。返回其绝对路径。
func EnsureRemoteBinary(cfg SSHConfig, bin string) (string, error) {
	client, err := dialSSH(cfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()
	return ensureRemoteBinary(client, bin)
}
