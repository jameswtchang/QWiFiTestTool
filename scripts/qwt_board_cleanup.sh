#!/bin/sh
# qwt_board_cleanup.sh - E2E test cleanup scoped to one wireless interface.
#
# Usage: qwt_board_cleanup.sh [--dry-run] <iface> [process-name ...]
#
# Mirrors QWifiTestTool's internal/sys/proc_iface.go rules so test cleanup never
# touches daemons serving other interfaces (e.g. the eth0 DHCP client a board
# relies on for network adb/SSH):
#   - QWifiTestTool                      : always killed (test tool instances)
#   - udhcpc / dhclient                   : killed only if bound to <iface>
#     (argument or -i/--interface)
#   - dnsmasq                             : killed if its declared interfaces include
#     <iface>, or it declares none (wildcard / listen-address only / config gone)
#   - wpa_supplicant[_s1g] / hostapd[_s1g]: killed if bound to <iface> or if the
#     binding is unknown (D-Bus/global mode, missing config); kept if bound only
#     to other interfaces
#
# Default process names: all of the above. --dry-run prints "PID NAME" instead of killing.
# POSIX sh only (works with busybox ash and bash).

DRY_RUN=0
if [ "$1" = "--dry-run" ]; then
	DRY_RUN=1
	shift
fi

IFACE="$1"
if [ -z "$IFACE" ]; then
	echo "usage: $0 [--dry-run] <iface> [process-name ...]" >&2
	exit 2
fi
shift

NAMES="$*"
[ -n "$NAMES" ] || NAMES="QWifiTestTool udhcpc dhclient dnsmasq wpa_supplicant wpa_supplicant_s1g hostapd hostapd_s1g"

NL='
'

# name_selected NAME: succeeds if NAME is in the requested process list
name_selected() {
	for n in $NAMES; do
		[ "$n" = "$1" ] && return 0
	done
	return 1
}

# conf_ifaces FILE: print interfaces named by interface= lines (comma lists split)
conf_ifaces() {
	[ -r "$1" ] || return 0
	sed -n 's/^[[:space:]]*interface[[:space:]]*=//p' "$1" | tr ',' '\n' |
		sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | grep -v '^$'
}

# list_has LIST WORD: succeeds if newline-separated LIST contains WORD
list_has() {
	printf '%s\n' "$1" | grep -qxF -- "$2"
}

# explicit_bound ARGS...: argument, -i<if>, -i <if>, --interface[=| ]<if> names IFACE
explicit_bound() {
	while [ $# -gt 0 ]; do
		case "$1" in
		"$IFACE" | "-i$IFACE" | "--interface=$IFACE") return 0 ;;
		-i | --interface) [ "$2" = "$IFACE" ] && return 0 ;;
		esac
		shift
	done
	return 1
}

# dnsmasq_bound ARGS...: declared interfaces (-i/--interface, interface= in its config
# files, default /etc/dnsmasq.conf) include IFACE, or none are declared at all
# (wildcard / listen-address only / config gone: it may hold DHCP/DNS ports on IFACE)
dnsmasq_bound() {
	confs=""
	ifs_found=""
	while [ $# -gt 0 ]; do
		case "$1" in
		-i | --interface) [ $# -gt 1 ] && { ifs_found="$ifs_found$NL$(printf '%s\n' "$2" | tr ',' '\n')"; shift; } ;;
		--interface=*) ifs_found="$ifs_found$NL$(printf '%s\n' "${1#--interface=}" | tr ',' '\n')" ;;
		-i?*) ifs_found="$ifs_found$NL$(printf '%s\n' "${1#-i}" | tr ',' '\n')" ;;
		-C | --conf-file) [ $# -gt 1 ] && { confs="$confs$NL$2"; shift; } ;;
		--conf-file=*) confs="$confs$NL${1#--conf-file=}" ;;
		-C?*) confs="$confs$NL${1#-C}" ;;
		esac
		shift
	done
	[ -n "$confs" ] || confs="/etc/dnsmasq.conf"
	old_ifs=$IFS
	IFS=$NL
	for c in $confs; do
		[ -n "$c" ] && ifs_found="$ifs_found$NL$(conf_ifaces "$c")"
	done
	IFS=$old_ifs
	ifs_found=$(printf '%s\n' "$ifs_found" | grep -v '^$')
	[ -z "$ifs_found" ] && return 0
	list_has "$ifs_found" "$IFACE"
}

# daemon_ifaces NAME ARGS...: print interfaces a wpa_supplicant/hostapd process is bound to
daemon_ifaces() {
	case "$1" in hostapd*) is_hostapd=1 ;; *) is_hostapd=0 ;; esac
	shift
	while [ $# -gt 0 ]; do
		case "$1" in
		-i) [ $# -gt 1 ] && { printf '%s\n' "$2" | tr ',' '\n'; shift; } ;;
		-i?*) printf '%s\n' "${1#-i}" | tr ',' '\n' ;;
		-b)
			if [ "$is_hostapd" = 1 ] && [ $# -gt 1 ]; then
				case "$2" in *:*) conf_ifaces "${2#*:}" ;; esac
				shift
			fi
			;;
		-P | -e | -f | -g | -G | -T) [ "$is_hostapd" = 1 ] && [ $# -gt 1 ] && shift ;;
		-*) ;;
		*) [ "$is_hostapd" = 1 ] && conf_ifaces "$1" ;;
		esac
		shift
	done
}

# should_kill NAME ARGS...: apply the per-process rule
should_kill() {
	name="$1"
	shift
	case "$name" in
	QWifiTestTool) return 0 ;;
	udhcpc | dhclient) explicit_bound "$@" ;;
	dnsmasq) dnsmasq_bound "$@" ;;
	wpa_supplicant | wpa_supplicant_s1g | hostapd | hostapd_s1g)
		bound=$(daemon_ifaces "$name" "$@" | grep -v '^$')
		[ -z "$bound" ] && return 0
		list_has "$bound" "$IFACE"
		;;
	*) return 1 ;;
	esac
}

SELF=$$
for dir in /proc/[0-9]*; do
	pid=${dir#/proc/}
	[ "$pid" = "$SELF" ] && continue
	[ -r "$dir/cmdline" ] || continue

	# argv is NUL-separated; rebuild it as positional parameters
	cmdline=$(tr '\0' '\n' <"$dir/cmdline" 2>/dev/null)
	[ -n "$cmdline" ] || continue
	old_ifs=$IFS
	IFS=$NL
	# shellcheck disable=SC2086
	set -- $cmdline
	IFS=$old_ifs

	name=${1##*/}
	name_selected "$name" || continue
	shift

	if should_kill "$name" "$@"; then
		if [ "$DRY_RUN" = 1 ]; then
			echo "$pid $name"
		elif kill -9 "$pid" 2>/dev/null; then
			echo "killed $pid ($name)"
		fi
	fi
done
exit 0
