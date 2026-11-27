// Command spike verifies the job transport design from ADR 0007:
// an API process serves HTTP on a Unix socket placed on a shared tmpfs
// volume, and a worker container with network_mode "none" reaches it
// while having no other network access.
//
// Modes:
//
//	spike server   listen on the socket and answer /job
//	spike worker   call /job over the socket, then run isolation probes
//	spike noop     exit 0 (used by the noexec probe)
//
// The worker exits 0 only if the socket call succeeds and every
// isolation probe passes. Stdlib only; no external modules.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const defaultSocket = "/run/pixsheaf/api.sock"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike server|worker")
		os.Exit(2)
	}
	sock := os.Getenv("SPIKE_SOCKET")
	if sock == "" {
		sock = defaultSocket
	}
	switch os.Args[1] {
	case "server":
		os.Exit(runServer(sock))
	case "worker":
		os.Exit(runWorker(sock))
	case "noop":
		os.Exit(0)
	default:
		fmt.Fprintln(os.Stderr, "unknown mode:", os.Args[1])
		os.Exit(2)
	}
}

func runServer(sock string) int {
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		printDiagnostics("server", filepath.Dir(sock))
		return 1
	}
	printDiagnostics("server", filepath.Dir(sock))
	if err := os.Chmod(sock, 0o660); err != nil {
		fmt.Fprintln(os.Stderr, "chmod:", err)
		return 1
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /job", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "job-ok")
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	fmt.Println("server: listening on", sock)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	return 0
}

type result struct {
	name string
	ok   bool
	info string
}

func runWorker(sock string) int {
	var results []result
	add := func(name string, ok bool, info string) {
		results = append(results, result{name, ok, info})
	}

	// 1. The socket call must succeed (retry while the server starts).
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}
	var body string
	var lastErr error
	for i := 0; i < 30; i++ {
		resp, err := client.Get("http://pixsheaf/job")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			body, lastErr = string(b), nil
			break
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	add("unix socket call", lastErr == nil && body == "job-ok", fmt.Sprintf("body=%q err=%v", body, lastErr))
	printDiagnostics("worker", filepath.Dir(sock))

	// The worker mounts the socket volume read-only: it may connect but not
	// create files there.
	probe := filepath.Join(filepath.Dir(sock), "worker-write-test")
	wf, werr := os.Create(probe)
	if wf != nil {
		_ = wf.Close()
		_ = os.Remove(probe)
	}
	add("socket dir not writable", werr != nil, fmt.Sprint(werr))

	// 2. No network interfaces other than loopback.
	ifaces, err := net.Interfaces()
	var names []string
	onlyLo := err == nil
	for _, ifc := range ifaces {
		names = append(names, ifc.Name)
		if ifc.Flags&net.FlagLoopback == 0 {
			onlyLo = false
		}
	}
	add("only loopback interface", onlyLo, strings.Join(names, ","))

	// 3. DNS and outbound connections must fail.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, dnsErr := net.DefaultResolver.LookupHost(ctx, "example.com")
	cancel()
	add("DNS lookup blocked", dnsErr != nil, fmt.Sprint(dnsErr))
	for _, target := range []string{"1.1.1.1:443", "8.8.8.8:53", "[2606:4700:4700::1111]:443"} {
		conn, err := net.DialTimeout("tcp", target, 3*time.Second)
		if conn != nil {
			_ = conn.Close()
		}
		add("TCP "+target+" blocked", err != nil, fmt.Sprint(err))
	}

	// 4. Process hardening as seen from inside the container.
	status := readStatus()
	add("UID >= 10000", os.Getuid() >= 10000, fmt.Sprint("uid=", os.Getuid()))
	add("no effective capabilities", status["CapEff"] == "0000000000000000", "CapEff="+status["CapEff"])
	add("no_new_privs set", status["NoNewPrivs"] == "1", "NoNewPrivs="+status["NoNewPrivs"])
	add("seccomp filter active", status["Seccomp"] == "2", "Seccomp="+status["Seccomp"])

	// 5. Root filesystem must be read-only.
	f, err := os.Create("/spike-write-test")
	if f != nil {
		_ = f.Close()
		_ = os.Remove("/spike-write-test")
	}
	add("root filesystem read-only", err != nil, fmt.Sprint(err))

	// 6. tmpfs must be writable but not executable (noexec).
	tmpOK, tmpInfo := checkTmpNoexec()
	add("tmpfs writable and noexec", tmpOK, tmpInfo)

	failed := 0
	for _, r := range results {
		mark := "PASS"
		if !r.ok {
			mark = "FAIL"
			failed++
		}
		fmt.Printf("%s  %-32s %s\n", mark, r.name, r.info)
	}
	if failed > 0 {
		fmt.Printf("worker: %d of %d checks failed\n", failed, len(results))
		return 1
	}
	fmt.Printf("worker: all %d checks passed\n", len(results))
	return 0
}

func readStatus() map[string]string {
	m := map[string]string{}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return m
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		k, v, ok := strings.Cut(s.Text(), ":")
		if ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

func checkTmpNoexec() (bool, string) {
	// Copy our own static binary to /tmp and try to run it. The images
	// have no shell, so a script would fail for the wrong reason.
	path := "/tmp/spike-exec-test"
	self, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		return false, "read self failed: " + err.Error()
	}
	if err := os.WriteFile(path, self, 0o700); err != nil {
		return false, "write failed: " + err.Error()
	}
	defer os.Remove(path)
	// Run as a child process so a successful exec cannot replace the worker.
	err = exec.Command(path, "noop").Run()
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true, "exec denied: " + err.Error()
	}
	return false, fmt.Sprint("exec not denied: err=", err)
}

// printDiagnostics shows ownership and mount options of the socket
// directory, so a permission failure can be explained from the logs.
func printDiagnostics(who, dir string) {
	fmt.Printf("%s: uid=%d gid=%d groups=%v\n", who, os.Getuid(), os.Getgid(), groups())
	var st syscall.Stat_t
	if err := syscall.Stat(dir, &st); err != nil {
		fmt.Printf("%s: stat %s: %v\n", who, dir, err)
	} else {
		fmt.Printf("%s: %s owner=%d:%d mode=%#o\n", who, dir, st.Uid, st.Gid, st.Mode&0o7777)
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) > 4 && fields[4] == dir {
			fmt.Printf("%s: mount %s\n", who, sc.Text())
		}
	}
}

func groups() []int {
	g, err := os.Getgroups()
	if err != nil {
		return nil
	}
	return g
}
