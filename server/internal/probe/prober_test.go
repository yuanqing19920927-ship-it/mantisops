package probe

import (
	"net"
	"testing"
	"time"
)

func TestProbeOnce_ClosedPort(t *testing.T) {
	// 用一个肯定不开的端口测试 down
	up, latency := ProbeOnce("127.0.0.1", 59999, 2*time.Second)
	if up {
		t.Fatal("port 59999 should be down")
	}
	if latency <= 0 {
		t.Fatal("latency should be positive")
	}
}

func TestProbeOnce_OpenPort(t *testing.T) {
	// 启动一个临时 TCP listener，验证探测能检测到 up
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	up, latency := ProbeOnce("127.0.0.1", port, 2*time.Second)
	if !up {
		t.Fatalf("port %d should be up", port)
	}
	if latency <= 0 {
		t.Fatal("latency should be positive")
	}
}
