package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"mcpeserverproxy/internal/config"
	"mcpeserverproxy/internal/logger"
)

type udpSpeederProcess struct {
	cmd        *exec.Cmd
	waitOnce   sync.Once
	waitErr    error
	waitDoneCh chan struct{}
}

func startUDPSpeeder(serverID string, sc *config.ServerConfig) (*udpSpeederProcess, string, error) {
	if sc == nil || sc.UDPSpeeder == nil || !sc.UDPSpeeder.Enabled {
		return nil, "", nil
	}

	cfg := sc.UDPSpeeder
	if err := cfg.Validate(); err != nil {
		return nil, "", err
	}

	if sc.ProxyOutbound != "" && !strings.EqualFold(sc.ProxyOutbound, "direct") {
		logger.Warn("Server %s: udp_speeder enabled, proxy_outbound will be bypassed (target becomes localhost)", serverID)
	}

	binPath, err := resolveUDPSpeederBinaryPath(cfg.BinaryPath)
	if err != nil {
		return nil, "", err
	}

	localListenAddr, err := resolveUDPSpeederLocalAddr(cfg.LocalListenAddr)
	if err != nil {
		return nil, "", err
	}

	args := []string{
		"-c",
		"-l", localListenAddr,
		"-r", cfg.RemoteAddr,
	}

	fec := strings.TrimSpace(cfg.FEC)
	if fec == "" {
		fec = "20:10"
	}
	args = append(args, "-f", fec)

	if strings.TrimSpace(cfg.Key) != "" {
		args = append(args, "-k", cfg.Key)
	}
	if cfg.Mode > 0 {
		args = append(args, "--mode", strconv.Itoa(cfg.Mode))
	}
	if cfg.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.Itoa(cfg.TimeoutMs))
	}
	if cfg.MTU > 0 {
		args = append(args, "--mtu", strconv.Itoa(cfg.MTU))
	}
	if cfg.DisableObscure {
		args = append(args, "--disable-obscure", "1")
	}
	if cfg.DisableChecksum {
		args = append(args, "--disable-checksum", "1")
	}
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, cfg.ExtraArgs...)
	}

	args = append(args, "--disable-color", "--log-level", "4")

	cmd := exec.Command(binPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("udp_speeder stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", fmt.Errorf("udp_speeder stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("udp_speeder start failed: %w", err)
	}

	p := &udpSpeederProcess{
		cmd:        cmd,
		waitDoneCh: make(chan struct{}),
	}

	go func() {
		p.waitErr = cmd.Wait()
		close(p.waitDoneCh)
	}()

	if logger.IsLevelEnabled(logger.LevelDebug) {
		go scanAndLogLines(serverID, "udp_speeder_stdout", stdout, logger.Debug)
		go scanAndLogLines(serverID, "udp_speeder_stderr", stderr, logger.Debug)
	} else {
		go io.Copy(io.Discard, stdout)
		go io.Copy(io.Discard, stderr)
	}

	logger.Info("Server %s: udp_speeder started: %s -c -l %s -r %s", serverID, filepath.Base(binPath), localListenAddr, cfg.RemoteAddr)
	return p, localListenAddr, nil
}

func (p *udpSpeederProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	var killErr error
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		killErr = err
	}

	p.waitOnce.Do(func() {
		select {
		case <-p.waitDoneCh:
		case <-time.After(2 * time.Second):
		}
	})

	if killErr != nil {
		return fmt.Errorf("udp_speeder kill failed: %w", killErr)
	}
	return nil
}

func resolveUDPSpeederBinaryPath(explicitPath string) (string, error) {
	candidates := make([]string, 0, 4)
	if strings.TrimSpace(explicitPath) != "" {
		candidates = append(candidates, explicitPath)
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(exeDir, "doc", "UDPspeeder", "speederv2.exe"))
			candidates = append(candidates, filepath.Join(exeDir, "speederv2.exe"))
		} else {
			candidates = append(candidates, filepath.Join(exeDir, "doc", "UDPspeeder", "speederv2"))
			candidates = append(candidates, filepath.Join(exeDir, "speederv2"))
		}
	}

	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join("doc", "UDPspeeder", "speederv2.exe"))
		candidates = append(candidates, "speederv2.exe")
	} else {
		candidates = append(candidates, filepath.Join("doc", "UDPspeeder", "speederv2"))
		candidates = append(candidates, "speederv2")
	}

	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("udp_speeder binary not found, tried: %s", strings.Join(candidates, ", "))
}

func resolveUDPSpeederLocalAddr(localListenAddr string) (string, error) {
	if strings.TrimSpace(localListenAddr) != "" {
		return localListenAddr, nil
	}

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	c, err := net.ListenUDP("udp", addr)
	if err != nil {
		return "", err
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return fmt.Sprintf("127.0.0.1:%d", port), nil
}

func scanAndLogLines(serverID, stream string, r io.Reader, logFn func(format string, args ...any)) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		logFn("Server %s: %s: %s", serverID, stream, line)
	}
}

