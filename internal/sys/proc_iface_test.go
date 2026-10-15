package sys

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// fakeProc builds a fake procfs tree: pid -> argv
func fakeProc(t *testing.T, procs map[int][]string) {
	t.Helper()
	root := t.TempDir()
	for pid, argv := range procs {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		cmdline := strings.Join(argv, "\x00") + "\x00"
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Non-PID entries must be ignored
	_ = os.MkdirAll(filepath.Join(root, "sys"), 0755)

	orig := procRoot
	procRoot = root
	t.Cleanup(func() { procRoot = orig })
}

func TestFindProcsForIface(t *testing.T) {
	confDir := t.TempDir()
	apConf := filepath.Join(confDir, "dnsmasq_ap.conf")
	os.WriteFile(apConf, []byte("# AP\ninterface=wlan0\ndhcp-range=192.168.50.100,192.168.50.200\n"), 0644)
	lanConf := filepath.Join(confDir, "dnsmasq_lan.conf")
	os.WriteFile(lanConf, []byte("interface=eth0,br0\n"), 0644)
	listConf := filepath.Join(confDir, "dnsmasq_list.conf")
	os.WriteFile(listConf, []byte("interface = eth1, wlan0\n"), 0644)
	// FGH200M style (dnsmasq_bind_interfaces=0): no interface= line, listen-address only
	listenOnly := filepath.Join(confDir, "dnsmasq_listen.conf")
	os.WriteFile(listenOnly, []byte("listen-address=192.168.50.1\ndhcp-range=192.168.50.100,192.168.50.200\n"), 0644)

	fakeProc(t, map[int][]string{
		101: {"/sbin/udhcpc", "-i", "wlan0", "-n", "-q"},                       // wlan0 -> kill
		102: {"udhcpc", "-b", "-i", "eth0", "-p", "/run/udhcpc.pid"},           // eth0 -> keep
		103: {"/sbin/dhclient", "-1", "-v", "wlan0"},                           // wlan0 -> kill
		104: {"/sbin/dhclient", "-pf", "/run/dhclient.eth0.pid", "eth0"},       // eth0 -> keep
		105: {"udhcpc", "-iwlan0"},                                             // compact form -> kill
		106: {"udhcpc", "--interface=wlan0"},                                   // long form -> kill
		107: {"/usr/sbin/dnsmasq", "-k", "-C", apConf, "--log-facility=-"},     // AP dnsmasq -> kill
		108: {"/usr/sbin/dnsmasq", "--conf-file=" + lanConf},                   // LAN dnsmasq -> keep
		109: {"/usr/sbin/dnsmasq", "-C" + listConf},                            // list incl. wlan0 -> kill
		110: {"/usr/sbin/hostapd", "wlan0"},                                    // not in names -> keep
		111: {"udhcpc", "-i", "wlan01"},                                        // different iface -> keep
		112: {"/usr/sbin/dnsmasq", "-k", "-C", listenOnly, "--log-facility=-"}, // no interface= (wildcard) -> kill
		113: {"/usr/sbin/dnsmasq", "-C", filepath.Join(confDir, "gone.conf")},  // config deleted (unknown) -> kill
		114: {"/usr/sbin/dnsmasq", "-i", "eth9", "-C", lanConf},                // explicit other ifaces only -> keep (explicit -C: host /etc/dnsmasq.conf must not leak in)
		115: {"/usr/sbin/dnsmasq", "--interface=eth9,wlan0", "-C", lanConf},    // explicit list incl. wlan0 -> kill
		116: {"udhcpc"},                                                        // no -i (busybox default eth0) -> keep
	})

	got := FindProcsForIface([]string{"udhcpc", "dhclient", "dnsmasq"}, "wlan0")
	sort.Ints(got)
	want := []int{101, 103, 105, 106, 107, 109, 112, 113, 115}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindProcsForIface = %v, want %v", got, want)
	}
}

func TestFindProcsForIface_EmptyInputs(t *testing.T) {
	fakeProc(t, map[int][]string{1: {"udhcpc", "-i", "wlan0"}})
	if got := FindProcsForIface(nil, "wlan0"); got != nil {
		t.Errorf("expected nil for empty names, got %v", got)
	}
	if got := FindProcsForIface([]string{"udhcpc"}, ""); got != nil {
		t.Errorf("expected nil for empty iface, got %v", got)
	}
}

func TestConfDeclaresIface(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.conf")
	os.WriteFile(path, []byte("#interface=wlan0\nlisten-address=1.2.3.4\n"), 0644)
	if confDeclaresIface(path, "wlan0") {
		t.Error("commented-out interface line must not match")
	}
	if confDeclaresIface(filepath.Join(t.TempDir(), "missing.conf"), "wlan0") {
		t.Error("missing file must not match")
	}
}

func TestDaemonIfaces(t *testing.T) {
	confDir := t.TempDir()
	apConf := filepath.Join(confDir, "hostapd.conf")
	os.WriteFile(apConf, []byte("interface=wlan0\ndriver=nl80211\nssid=Test\n"), 0644)
	ap1Conf := filepath.Join(confDir, "hostapd_wlan1.conf")
	os.WriteFile(ap1Conf, []byte("interface=wlan1\n"), 0644)

	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"tool STA", []string{"/usr/sbin/wpa_supplicant", "-i", "wlan0", "-D", "nl80211", "-c", "/etc/wpa_supplicant.conf"}, []string{"wlan0"}},
		{"compact -i", []string{"wpa_supplicant", "-B", "-iwlan1", "-c/etc/w.conf"}, []string{"wlan1"}},
		{"multi iface -N", []string{"wpa_supplicant", "-i", "wlan1", "-c", "a", "-N", "-i", "wlan0", "-c", "b"}, []string{"wlan1", "wlan0"}},
		{"dbus mode", []string{"/usr/sbin/wpa_supplicant", "-u", "-s", "-O", "/run/wpa_supplicant"}, nil},
		{"android hal", []string{"/vendor/bin/hw/wpa_supplicant", "-O/data/vendor/wifi/wpa/sockets", "-dd", "-g@android:wpa_wlan0"}, nil},
		{"tool AP", []string{"/usr/sbin/hostapd", "-dd", apConf}, []string{"wlan0"}},
		{"hostapd -B -P pid", []string{"hostapd", "-B", "-P", "/run/hostapd.pid", ap1Conf}, []string{"wlan1"}},
		{"hostapd -i list", []string{"hostapd", "-i", "wlan2,wlan3", apConf}, []string{"wlan2", "wlan3", "wlan0"}},
		{"hostapd -b phy:conf", []string{"hostapd", "-b", "phy0:" + ap1Conf}, []string{"wlan1"}},
		{"hostapd missing conf", []string{"hostapd", "-dd", filepath.Join(confDir, "deleted.conf")}, nil},
		{"hostapd_s1g", []string{"/usr/sbin/hostapd_s1g", apConf}, []string{"wlan0"}},
	}
	for _, tt := range tests {
		if got := daemonIfaces(tt.argv); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: daemonIfaces = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFindDaemonsForIface(t *testing.T) {
	confDir := t.TempDir()
	apConf := filepath.Join(confDir, "hostapd.conf")
	os.WriteFile(apConf, []byte("interface=wlan0\n"), 0644)
	ap1Conf := filepath.Join(confDir, "hostapd_wlan1.conf")
	os.WriteFile(ap1Conf, []byte("interface=wlan1\n"), 0644)

	fakeProc(t, map[int][]string{
		201: {"/usr/sbin/wpa_supplicant", "-i", "wlan0", "-c", "/etc/wpa_supplicant.conf"},      // wlan0 -> kill
		202: {"/usr/sbin/wpa_supplicant", "-i", "wlan1", "-c", "/etc/wpa_wlan1.conf"},           // other iface -> keep
		203: {"/usr/sbin/wpa_supplicant", "-u", "-s", "-O", "/run/wpa_supplicant"},              // D-Bus/global -> kill
		204: {"/usr/sbin/wpa_supplicant_s1g", "-i", "wlan0", "-c", "/etc/s1g.conf"},             // s1g wlan0 -> kill
		205: {"/usr/sbin/hostapd", "-dd", apConf},                                               // AP on wlan0 -> kill
		206: {"/usr/sbin/hostapd", "-B", ap1Conf},                                               // AP on wlan1 -> keep
		207: {"/usr/sbin/hostapd", "-dd", filepath.Join(confDir, "gone.conf")},                  // unknown -> kill
		208: {"/usr/sbin/wpa_supplicant", "-i", "eth0", "-D", "wired", "-c", "/etc/8021x.conf"}, // wired 802.1X -> keep
		209: {"/usr/sbin/dnsmasq", "-i", "wlan0"},                                               // not in names -> keep
	})

	got := FindDaemonsForIface([]string{"wpa_supplicant", "wpa_supplicant_s1g", "hostapd", "hostapd_s1g"}, "wlan0")
	sort.Ints(got)
	if want := []int{201, 203, 204, 205, 207}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindDaemonsForIface = %v, want %v", got, want)
	}
	if got := FindDaemonsForIface([]string{"wpa_supplicant"}, ""); got != nil {
		t.Errorf("expected nil for empty iface, got %v", got)
	}
}
