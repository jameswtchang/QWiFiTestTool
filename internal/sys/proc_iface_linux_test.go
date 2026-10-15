//go:build linux

package sys

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeProcEnv makes the test binary act as an idle fake daemon (see TestMain), so
// real processes with arbitrary argv can be spawned without touching real daemons.
const fakeProcEnv = "QWT_FAKE_PROC"

func TestMain(m *testing.M) {
	if os.Getenv(fakeProcEnv) == "1" {
		// QWT_FAKE_SECONDS lets the binary double as a long-lived canary on the board
		secs, err := strconv.Atoi(os.Getenv("QWT_FAKE_SECONDS"))
		if err != nil || secs <= 0 {
			secs = 120
		}
		time.Sleep(time.Duration(secs) * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// spawnFake starts the test binary with the given argv (argv[0] sets the process name)
func spawnFake(t *testing.T, argv ...string) int {
	t.Helper()
	cmd := &exec.Cmd{
		Path: os.Args[0],
		Args: argv,
		Env:  append(os.Environ(), fakeProcEnv+"=1"),
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn %v: %v", argv, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd.Process.Pid
}

// onlyPids filters found to the given candidate set (ignores real daemons on the host)
func onlyPids(found []int, candidates map[int]bool) []int {
	var out []int
	for _, p := range found {
		if candidates[p] {
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

// Runs against the real /proc with real processes; on the board also validates
// scripts/qwt_board_cleanup.sh when QWT_CLEANUP_SCRIPT points to it.
func TestRealProcessScoping(t *testing.T) {
	const iface = "qwtfake0" // never a real interface
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	apConf := write("hostapd.conf", "interface="+iface+"\nssid=T\n")
	otherAP := write("hostapd_eth9.conf", "interface=eth9\n")
	dnsConf := write("dnsmasq.conf", "interface="+iface+"\nbind-interfaces\n")
	otherDns := write("dnsmasq_eth9.conf", "interface=eth9\n")
	listenOnly := write("dnsmasq_listen.conf", "listen-address=192.168.50.1\n") // FGH200M style

	// Expected matches for DHCP/dnsmasq (bound only) and daemons (bound or unknown)
	wantProcs := map[int]bool{}
	wantDaemons := map[int]bool{}
	all := map[int]bool{}
	add := func(set map[int]bool, argv ...string) {
		pid := spawnFake(t, argv...)
		all[pid] = true
		if set != nil {
			set[pid] = true
		}
	}

	add(wantProcs, "/sbin/udhcpc", "-i", iface, "-n", "-q")
	add(nil, "/sbin/udhcpc", "-b", "-i", "eth9")
	add(wantProcs, "/sbin/dhclient", "-1", "-v", iface)
	add(nil, "/sbin/dhclient", "-pf", "/run/dhclient.eth9.pid", "eth9")
	add(wantProcs, "/usr/sbin/dnsmasq", "-k", "-C", dnsConf, "--log-facility=-")
	add(nil, "/usr/sbin/dnsmasq", "--conf-file="+otherDns)
	add(wantProcs, "/usr/sbin/dnsmasq", "-k", "-C", listenOnly, "--log-facility=-")
	add(wantProcs, "/usr/sbin/dnsmasq", "-C", filepath.Join(dir, "gone.conf"))
	add(nil, "/usr/sbin/dnsmasq", "-i", "eth9", "-C", otherDns)
	add(wantDaemons, "/usr/sbin/wpa_supplicant", "-i", iface, "-D", "nl80211", "-c", "/etc/wpa_supplicant.conf")
	add(nil, "/usr/sbin/wpa_supplicant", "-i", "eth9", "-D", "wired", "-c", "/etc/8021x.conf")
	add(wantDaemons, "/usr/sbin/wpa_supplicant", "-u", "-s", "-O", "/run/wpa_supplicant")
	add(wantDaemons, "/usr/sbin/wpa_supplicant_s1g", "-i"+iface, "-c", "/etc/s1g.conf")
	add(wantDaemons, "/usr/sbin/hostapd", "-dd", apConf)
	add(nil, "/usr/sbin/hostapd", "-B", "-P", "/run/h.pid", otherAP)

	gotProcs := onlyPids(FindProcsForIface([]string{"udhcpc", "dhclient", "dnsmasq"}, iface), all)
	if want := sortedKeys(wantProcs); !reflect.DeepEqual(gotProcs, want) {
		t.Errorf("FindProcsForIface = %v, want %v", gotProcs, want)
	}
	gotDaemons := onlyPids(FindDaemonsForIface([]string{"wpa_supplicant", "wpa_supplicant_s1g", "hostapd", "hostapd_s1g"}, iface), all)
	if want := sortedKeys(wantDaemons); !reflect.DeepEqual(gotDaemons, want) {
		t.Errorf("FindDaemonsForIface = %v, want %v", gotDaemons, want)
	}

	script := os.Getenv("QWT_CLEANUP_SCRIPT")
	if script == "" {
		t.Log("QWT_CLEANUP_SCRIPT not set; skipping board cleanup script check")
		return
	}
	out, err := exec.Command("sh", script, "--dry-run", iface).CombinedOutput()
	if err != nil {
		t.Fatalf("cleanup script failed: %v\n%s", err, out)
	}
	var scriptPids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			if pid, err := strconv.Atoi(f[0]); err == nil {
				scriptPids = append(scriptPids, pid)
			}
		}
	}
	want := sortedKeys(wantProcs)
	want = append(want, sortedKeys(wantDaemons)...)
	sort.Ints(want)
	if got := onlyPids(scriptPids, all); !reflect.DeepEqual(got, want) {
		t.Errorf("qwt_board_cleanup.sh --dry-run = %v, want %v\noutput:\n%s", got, want, out)
	}

	// Name filter: only wpa_supplicant requested
	out, _ = exec.Command("sh", script, "--dry-run", iface, "wpa_supplicant").CombinedOutput()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] != "wpa_supplicant" {
			t.Errorf("name filter leaked %q", line)
		}
	}
}

func sortedKeys(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
