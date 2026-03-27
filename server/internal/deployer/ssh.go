package deployer

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	host     string
	port     int
	user     string
	password string // stored for sudo -S
	auth     ssh.AuthMethod
	kbdAuth  ssh.AuthMethod // keyboard-interactive fallback for password auth
	hostKey  ssh.PublicKey   // nil = accept any (TOFU first connect)
	conn     *ssh.Client
}

func NewSSHClientPassword(host string, port int, user, password string, hostKeyStr string) *SSHClient {
	c := &SSHClient{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		auth:     ssh.Password(password),
		kbdAuth: ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = password
			}
			return answers, nil
		}),
	}
	if hostKeyStr != "" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKeyStr))
		if err == nil {
			c.hostKey = key
		}
	}
	return c
}

func NewSSHClientKey(host string, port int, user, privateKey, passphrase string, hostKeyStr string) (*SSHClient, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(privateKey))
	}
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	c := &SSHClient{
		host: host,
		port: port,
		user: user,
		auth: ssh.PublicKeys(signer),
	}
	if hostKeyStr != "" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKeyStr))
		if err == nil {
			c.hostKey = key
		}
	}
	return c, nil
}

func (c *SSHClient) authMethods() []ssh.AuthMethod {
	methods := []ssh.AuthMethod{c.auth}
	if c.kbdAuth != nil {
		methods = append(methods, c.kbdAuth)
	}
	return methods
}

func (c *SSHClient) TestConnection() (latencyMs int, hostKey string, arch string, osName string, err error) {
	start := time.Now()

	var receivedKey ssh.PublicKey
	config := &ssh.ClientConfig{
		User:    c.user,
		Auth:    c.authMethods(),
		Timeout: 10 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			receivedKey = key
			if c.hostKey != nil {
				if string(key.Marshal()) != string(c.hostKey.Marshal()) {
					return fmt.Errorf("host key mismatch")
				}
			}
			return nil
		},
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return 0, "", "", "", fmt.Errorf("SSH dial %s: %w", addr, err)
	}
	defer conn.Close()

	latencyMs = int(time.Since(start).Milliseconds())

	if receivedKey != nil {
		hostKey = string(ssh.MarshalAuthorizedKey(receivedKey))
		hostKey = strings.TrimSpace(hostKey)
	}

	session, err := conn.NewSession()
	if err == nil {
		out, err2 := session.Output("uname -m")
		if err2 == nil {
			rawArch := strings.TrimSpace(string(out))
			switch rawArch {
			case "x86_64":
				arch = "amd64"
			case "aarch64":
				arch = "arm64"
			default:
				arch = rawArch
			}
		}
		session.Close()
	}

	session2, err := conn.NewSession()
	if err == nil {
		out, err2 := session2.Output("cat /etc/os-release 2>/dev/null | grep ^PRETTY_NAME= | cut -d'\"' -f2")
		if err2 == nil {
			osName = strings.TrimSpace(string(out))
		}
		session2.Close()
	}

	return latencyMs, hostKey, arch, osName, nil
}

func (c *SSHClient) Connect() error {
	config := &ssh.ClientConfig{
		User:    c.user,
		Auth:    c.authMethods(),
		Timeout: 10 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if c.hostKey != nil {
				if string(key.Marshal()) != string(c.hostKey.Marshal()) {
					return fmt.Errorf("host key mismatch for %s", hostname)
				}
			}
			return nil
		},
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connect %s: %w", addr, err)
	}
	c.conn = conn
	return nil
}

func (c *SSHClient) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *SSHClient) Execute(cmd string) (stdout, stderr string, err error) {
	if c.conn == nil {
		return "", "", fmt.Errorf("not connected")
	}
	session, err := c.conn.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var outBuf, errBuf strings.Builder
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	// If command uses sudo and we have a password, pipe it via stdin
	if strings.Contains(cmd, "sudo ") && c.password != "" {
		session.Stdin = strings.NewReader(c.password + "\n")
		cmd = strings.ReplaceAll(cmd, "sudo ", "sudo -S ")
	}

	err = session.Run(cmd)
	return outBuf.String(), errBuf.String(), err
}

func (c *SSHClient) Upload(localPath, remotePath string) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		return fmt.Errorf("SFTP client: %w", err)
	}
	defer sftpClient.Close()

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local %s: %w", localPath, err)
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return fmt.Errorf("copy to %s: %w", remotePath, err)
	}

	return nil
}

func (c *SSHClient) WriteFile(remotePath string, content []byte, perm os.FileMode) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		return fmt.Errorf("SFTP client: %w", err)
	}
	defer sftpClient.Close()

	f, err := sftpClient.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open remote %s: %w", remotePath, err)
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", remotePath, err)
	}

	// Non-fatal: some systems may not support chmod via SFTP
	_ = sftpClient.Chmod(remotePath, perm)

	return nil
}

func (c *SSHClient) DetectArch() (string, error) {
	stdout, _, err := c.Execute("uname -m")
	if err != nil {
		return "", fmt.Errorf("detect arch: %w", err)
	}
	raw := strings.TrimSpace(stdout)
	switch raw {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	default:
		return raw, nil
	}
}
