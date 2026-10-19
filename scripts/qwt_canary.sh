#!/bin/sh
# qwt_canary.sh - fake daemons bound to another interface, used by the E2E test to prove that
# interface-scoped cleanup (QWifiTestTool's own and qwt_board_cleanup.sh) leaves them alone.
#
# Usage: qwt_canary.sh start|check|stop [canary-iface]
#   start : launch udhcpc / dnsmasq / wpa_supplicant / hostapd look-alikes bound to
#           <canary-iface> (default eth9; it does not need to exist), then print check output
#   check : print "alive <name> <pid>" or "dead <name> <pid>" for each canary
#   stop  : kill all canaries and remove the work directory
#
# Each canary is the system shell symlinked under the daemon's name, so /proc/<pid>/cmdline
# starts with that name and carries the binding arguments the cleanup rules parse:
#   udhcpc / wpa_supplicant : -i <canary-iface>
#   dnsmasq                 : -C <conf with interface=<canary-iface>> -i <canary-iface>
#   hostapd                 : positional config file with interface=<canary-iface>
# POSIX sh only.

DIR=/data/local/tmp/qwt_canary
CIF=${2:-eth9}
NAMES="udhcpc dnsmasq wpa_supplicant hostapd"

# real_shell: resolved path of the system shell (a busybox multi-call binary would run the
# applet named by the symlink instead of a shell, so it is rejected)
real_shell() {
	for s in /bin/sh /system/bin/sh; do
		if [ -x "$s" ]; then
			readlink -f "$s"
			return
		fi
	done
}

do_check() {
	for n in $NAMES; do
		pid=$(cat "$DIR/$n.pid" 2>/dev/null)
		argv0=$(tr '\0' '\n' <"/proc/$pid/cmdline" 2>/dev/null | head -n 1)
		if [ -n "$pid" ] && [ "${argv0##*/}" = "$n" ]; then
			echo "alive $n $pid"
		else
			echo "dead $n ${pid:-none}"
		fi
	done
}

do_stop() {
	for n in $NAMES; do
		pid=$(cat "$DIR/$n.pid" 2>/dev/null)
		[ -n "$pid" ] && kill -9 "$pid" 2>/dev/null
	done
	rm -rf "$DIR"
}

case "$1" in
start)
	do_stop
	shell=$(real_shell)
	case "$shell" in
	"" | *busybox*)
		echo "unsupported shell: ${shell:-none}"
		exit 3
		;;
	esac
	mkdir -p "$DIR" || exit 1
	printf 'interface=%s\n' "$CIF" >"$DIR/canary.conf"
	for n in $NAMES; do
		ln -sf "$shell" "$DIR/$n"
		case "$n" in
		dnsmasq) set -- -C "$DIR/canary.conf" -i "$CIF" ;;
		hostapd) set -- "$DIR/canary.conf" ;;
		*) set -- -i "$CIF" ;;
		esac
		# Short sleeps so the child left behind after a kill -9 of the canary exits within 1s
		nohup "$DIR/$n" -c 'while :; do sleep 1; done' canary "$@" >/dev/null 2>&1 &
		echo $! >"$DIR/$n.pid"
	done
	sleep 1
	do_check
	;;
check) do_check ;;
stop) do_stop ;;
*)
	echo "usage: $0 start|check|stop [canary-iface]" >&2
	exit 2
	;;
esac
exit 0
