package sys

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procRoot is the procfs mount point (overridable in tests)
var procRoot = "/proc"

// dnsmasqDefaultConf is the file dnsmasq reads when started without -C / --conf-file
const dnsmasqDefaultConf = "/etc/dnsmasq.conf"

// FindProcsForIface returns PIDs of processes whose executable name is in names and
// that are bound to iface, so cleanup never touches daemons serving other interfaces
// (e.g. the eth0 DHCP client a board relies on for network adb/SSH).
//
// A process is considered bound to iface when its command line contains:
//   - iface as a standalone argument          (dhclient -1 -v wlan0)
//   - -i<iface> / -i <iface> / --interface=<iface>   (udhcpc -i wlan0)
//   - for dnsmasq: see dnsmasqServesIface (declared interfaces include iface,
//     or none are declared at all)
func FindProcsForIface(names []string, iface string) []int {
	if iface == "" {
		return nil
	}
	return scanProcs(names, func(argv []string) bool {
		return cmdlineBindsIface(argv, iface)
	})
}

// KillProcsForIface sends SIGKILL to every process found by FindProcsForIface
// and returns how many were killed.
func KillProcsForIface(names []string, iface string) int {
	return killPids(FindProcsForIface(names, iface), names, iface)
}

// scanProcs returns PIDs (excluding this process) whose executable base name is in
// names and whose argv satisfies match.
func scanProcs(names []string, match func(argv []string) bool) []int {
	if len(names) == 0 {
		return nil
	}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil // No procfs (e.g. Windows host)
	}

	self := os.Getpid()
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue // Exited, or kernel thread
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		if nameSet[filepath.Base(argv[0])] && match(argv) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// killPids sends SIGKILL to pids and returns how many were killed
func killPids(pids []int, names []string, iface string) int {
	killed := 0
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Kill(); err == nil {
			fmt.Printf("[CMD] kill -9 %d (%s, %s)\n", pid, strings.Join(names, "/"), iface)
			killed++
		}
	}
	return killed
}

// cmdlineBindsIface reports whether argv targets iface (see FindProcsForIface)
func cmdlineBindsIface(argv []string, iface string) bool {
	if filepath.Base(argv[0]) == "dnsmasq" {
		return dnsmasqServesIface(argv, iface)
	}
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == iface,
			arg == "-i"+iface,
			arg == "--interface="+iface:
			return true
		case (arg == "-i" || arg == "--interface") && i+1 < len(argv) && argv[i+1] == iface:
			return true
		}
	}
	return false
}

// dnsmasqServesIface reports whether a dnsmasq process may serve iface. Interfaces come
// from -i/--interface options and interface= lines in its config files (-C/--conf-file,
// or /etc/dnsmasq.conf by default). A dnsmasq that declares no interface at all listens
// on every interface (or only via listen-address=, as with dnsmasq_bind_interfaces=0),
// or its config is gone; it may hold the DHCP/DNS ports on iface, so it counts as serving it.
func dnsmasqServesIface(argv []string, iface string) bool {
	var ifaces, confFiles []string
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case (arg == "-i" || arg == "--interface") && i+1 < len(argv):
			ifaces = append(ifaces, strings.Split(argv[i+1], ",")...)
			i++
		case strings.HasPrefix(arg, "--interface="):
			ifaces = append(ifaces, strings.Split(strings.TrimPrefix(arg, "--interface="), ",")...)
		case strings.HasPrefix(arg, "-i") && len(arg) > 2:
			ifaces = append(ifaces, strings.Split(arg[2:], ",")...)
		case (arg == "-C" || arg == "--conf-file") && i+1 < len(argv):
			confFiles = append(confFiles, argv[i+1])
			i++
		case strings.HasPrefix(arg, "--conf-file="):
			confFiles = append(confFiles, strings.TrimPrefix(arg, "--conf-file="))
		case strings.HasPrefix(arg, "-C") && len(arg) > 2:
			confFiles = append(confFiles, arg[2:])
		}
	}
	if len(confFiles) == 0 {
		confFiles = []string{dnsmasqDefaultConf}
	}
	for _, conf := range confFiles {
		ifaces = append(ifaces, confInterfaces(conf)...)
	}

	if len(ifaces) == 0 {
		return true // wildcard / listen-address only / unknown
	}
	for _, v := range ifaces {
		if strings.TrimSpace(v) == iface {
			return true
		}
	}
	return false
}

// confDeclaresIface reports whether a dnsmasq/hostapd config file contains interface=<iface>
func confDeclaresIface(path, iface string) bool {
	for _, v := range confInterfaces(path) {
		if v == iface {
			return true
		}
	}
	return false
}

// confInterfaces returns every interface named by interface= lines in a config file.
// dnsmasq accepts a comma-separated list (interface=wlan0,wlan1); hostapd a single name.
func confInterfaces(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var ifaces []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok || strings.TrimSpace(key) != "interface" {
			continue
		}
		for _, v := range strings.Split(val, ",") {
			if v = strings.TrimSpace(v); v != "" {
				ifaces = append(ifaces, v)
			}
		}
	}
	return ifaces
}

// hostapdArgOpts are hostapd options that consume the following argument
var hostapdArgOpts = map[string]bool{"-P": true, "-e": true, "-f": true, "-g": true, "-G": true, "-b": true, "-i": true, "-T": true}

// daemonIfaces returns the interfaces a wpa_supplicant/hostapd process is bound to,
// taken from -i options (hostapd: comma-separated list) and, for hostapd, the
// interface= lines of its config files. An empty result means the binding is unknown.
func daemonIfaces(argv []string) []string {
	isHostapd := strings.HasPrefix(filepath.Base(argv[0]), "hostapd")
	var ifaces []string
	addList := func(list string) {
		for _, v := range strings.Split(list, ",") {
			if v = strings.TrimSpace(v); v != "" {
				ifaces = append(ifaces, v)
			}
		}
	}

	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "-i":
			if i+1 < len(argv) {
				addList(argv[i+1])
				i++
			}
		case strings.HasPrefix(arg, "-i") && len(arg) > 2:
			addList(arg[2:])
		case isHostapd && arg == "-b" && i+1 < len(argv):
			// -b <phyname>:<bss config file>
			if _, conf, ok := strings.Cut(argv[i+1], ":"); ok {
				ifaces = append(ifaces, confInterfaces(conf)...)
			}
			i++
		case isHostapd && hostapdArgOpts[arg]:
			i++ // skip option value
		case isHostapd && !strings.HasPrefix(arg, "-"):
			ifaces = append(ifaces, confInterfaces(arg)...) // positional config file
		}
	}
	return ifaces
}

// FindDaemonsForIface returns PIDs of wpa_supplicant/hostapd-style daemons (by executable
// name) that may control iface: those bound to it, plus those whose binding is unknown
// (e.g. D-Bus mode "wpa_supplicant -u", Android's -g@android:wpa_wlan0, or a hostapd whose
// config file was deleted). Daemons explicitly bound only to other interfaces are kept.
func FindDaemonsForIface(names []string, iface string) []int {
	if iface == "" {
		return nil
	}
	return scanProcs(names, func(argv []string) bool {
		bound := daemonIfaces(argv)
		if len(bound) == 0 {
			return true
		}
		for _, b := range bound {
			if b == iface {
				return true
			}
		}
		return false
	})
}

// KillDaemonsForIface sends SIGKILL to every process found by FindDaemonsForIface
// and returns how many were killed.
func KillDaemonsForIface(names []string, iface string) int {
	return killPids(FindDaemonsForIface(names, iface), names, iface)
}
