param(
    [switch]$MatrixOnly,
    [switch]$RfOnly,
    [switch]$RfAll,
    [switch]$ApOnly,
    [switch]$StaOnly,
    # Only Phase 4 (diagnosis menu, driver unload/load/reload, AP after reload). Opt-in: it reloads
    # the driver, and the board may need a reboot afterwards if the reload leaves the radio degraded.
    [switch]$DiagOnly,
    # Module is picked by name from the tool's own menu, so reordering the XML cannot misdirect the tests
    [string]$ModuleName = "FME170Q",
    [string]$Iface = "wlan0",
    # Fake daemons are bound to this interface; they must survive every cleanup aimed at $Iface
    [string]$CanaryIface = "eth9",
    # Phase 5 (Station connection to a real reference AP): add it to the run, or run only it
    [switch]$StaConnect,
    [switch]$StaConnectOnly,
    # Reference AP file, UTF-8: line 1 = SSID, line 2 = password (empty for an open network).
    # Kept outside the repo; the password is masked in every console line, artifact and result.
    [string]$RefApFile = (Join-Path $env:USERPROFILE ".qwt\ref_ap.txt"),
    # results.json and per-case failure artifacts (TUI transcript, configs, dmesg) are written here
    [string]$OutDir = (Join-Path $env:TEMP ("qwt_e2e_" + (Get-Date -Format "yyyyMMdd_HHmmss")))
)

$ErrorActionPreference = "Continue"

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
# Child stdin writers are created with Console.InputEncoding at Process.Start, and a UTF-8 encoding
# with BOM writes EF BB BF right away: the tool would read "﻿y" and ignore the language answer.
try { [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false) } catch {}

$TARGET_BIN = "/data/local/tmp/QWifiTestTool"
$TEST_IFACE = $Iface
# Board-side cleanup helper: kills only daemons bound to $TEST_IFACE (keeps e.g. eth0 DHCP alive)
$CLEANUP_SH = "/data/local/tmp/qwt_board_cleanup.sh"
# Board-side canary helper: fake udhcpc/dnsmasq/wpa_supplicant/hostapd bound to $CanaryIface
$CANARY_SH = "/data/local/tmp/qwt_canary.sh"
$CANARY_NAMES = @("udhcpc", "dnsmasq", "wpa_supplicant", "hostapd")
# The tests overwrite and delete these board files; originals are backed up here and restored at the end
$BACKUP_DIR = "/data/local/tmp/qwt_e2e_backup"
$BOARD_CONFIGS = "/etc/wpa_supplicant.conf /etc/hostapd.conf /etc/dnsmasq.conf"
# Without /etc/wpa_supplicant.conf Station mode writes wpa_supplicant_<module>.conf into its working
# directory (Linux: the adb shell cwd; Android: /data/local/tmp/wifi_tools). Phase 5 stores the
# reference AP password there, so these files are backed up and restored as well. Set after ADB connects.
$GEN_CONF_DIR = "/"
# The tool is considered to be waiting on a prompt once its output has been quiet this long
$QUIET_MS = 300
# Printed by the tool when it rejects an input, which means the scripted inputs went out of step
$INVALID_INPUT_RE = "無效|Invalid (option|selection|country code|channel)"
$GOODBYE_RE = "再見!|Goodbye!"
# ui.ShowError prefix and the hostapd init failure message (zh and en)
$AP_FAIL_RE = "❌ 錯誤:|❌ Error:|hostapd 啟動失敗|hostapd failed to start|Could not initialize interface"
# CJK ideographs / punctuation / full-width forms: must not appear once English UI is selected
$CJK_RE = '[　-〿㐀-䶿一-鿿＀-￯]'

$GlobalResults = [System.Collections.Generic.List[PSObject]]::new()
$MODULE_IDX = ""
$MODULE_LABEL = ""
# Main kernel module name, read from the diagnosis output ("[Main] <name>: loaded")
$DRIVER_MOD = ""
# Canary bookkeeping: PIDs at start, the phase after which each one was found dead, phases checked
$CanaryPids = @{}
$CanaryDeaths = [ordered]@{}
$CanaryChecked = [System.Collections.Generic.List[string]]::new()
$CanaryError = ""
$CanaryInconclusive = $false
# Strings that must never reach the console, artifacts or results (the reference AP password)
$SECRETS = [System.Collections.Generic.List[string]]::new()
$REF_SSID = ""
$REF_PSK = ""
$WPA_CTRL = "/var/run/wpa_supplicant"
$StaInfo = ""
# Per-check outcomes of Phase 5 ("" = passed), filled in by Check steps inside tool sessions
$StaCheck = @{}

if (-not ("QwtStreamPump" -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.IO;
using System.Text;
using System.Threading;

// Drains a child process stream on a background thread. Unlike ReadLine it also captures
// prompts printed without a trailing newline, and records when output last arrived.
public class QwtStreamPump {
    private readonly Stream _stream;
    private readonly StringBuilder _buf = new StringBuilder();
    private readonly object _lock = new object();
    private long _lastTicks = DateTime.UtcNow.Ticks;
    private volatile bool _ended;

    public QwtStreamPump(Stream stream) {
        _stream = stream;
        Thread t = new Thread(Run);
        t.IsBackground = true;
        t.Start();
    }

    private void Run() {
        Decoder dec = new UTF8Encoding(false).GetDecoder();
        byte[] bytes = new byte[4096];
        char[] chars = new char[8192];
        try {
            int n;
            while ((n = _stream.Read(bytes, 0, bytes.Length)) > 0) {
                int c = dec.GetChars(bytes, 0, n, chars, 0);
                lock (_lock) {
                    _buf.Append(chars, 0, c);
                    _lastTicks = DateTime.UtcNow.Ticks;
                }
            }
        } catch (Exception) {
        }
        _ended = true;
    }

    public bool Ended { get { return _ended; } }
    public int Length { get { lock (_lock) { return _buf.Length; } } }
    public double IdleMs { get { lock (_lock) { return (DateTime.UtcNow.Ticks - _lastTicks) / 10000.0; } } }
    public string Text { get { lock (_lock) { return _buf.ToString(); } } }
}
'@
}

function Exec-Adb {
    param([string]$Cmd)
    $out = adb shell $Cmd
    if ($null -eq $out) { return "" }
    return ($out -join "`n").Trim()
}

function Remove-Ansi {
    param([string]$Text)
    return (($Text -replace "\x1b\[[0-9;?]*[A-Za-z]", "") -replace "`r", "")
}

function Hide-Secrets {
    # Masks every registered secret (the reference AP password) before text leaves the script
    param([string]$Text)
    foreach ($s in $SECRETS) {
        if ($s) { $Text = $Text.Replace($s, "***") }
    }
    return $Text
}

function Save-Artifact {
    # Writes a failure artifact (transcript, config, dmesg) to $OutDir as <TcId>.<Name>
    param([string]$TcId, [string]$Name, [string]$Content)
    $path = Join-Path $OutDir "$TcId.$Name"
    [System.IO.File]::WriteAllText($path, (Hide-Secrets $Content), [System.Text.UTF8Encoding]::new($false))
}

function Add-Result {
    param([string]$Category, [string]$TcId, [string]$Name, [string]$Status, [string]$Details)
    $GlobalResults.Add([PSCustomObject]@{
        Category = $Category
        Id = $TcId
        Name = $Name
        Status = $Status
        Details = (Hide-Secrets $Details)
    })
}

function Push-BoardScript {
    # Pushes scripts/<Name> to $Dest, normalized to LF (a CRLF checkout would break sh on the board)
    param([string]$Name, [string]$Dest)
    $src = Join-Path $PSScriptRoot $Name
    if (-not (Test-Path $src)) { return $false }
    $tmp = Join-Path $env:TEMP $Name
    [System.IO.File]::WriteAllText($tmp, ((Get-Content -Raw $src) -replace "`r`n", "`n"))
    adb push $tmp $Dest 2>&1 | Out-Null
    $ok = ($LASTEXITCODE -eq 0)
    Remove-Item $tmp -ErrorAction SilentlyContinue
    return $ok
}

function Backup-BoardConfigs {
    # A leftover backup means a previous run was interrupted before restoring: it still holds the
    # originals, so it is kept instead of being overwritten with test-generated files.
    $cmd = 'if [ -f @BK@/.done ]; then echo REUSE; else rm -rf @BK@; mkdir -p @BK@/gen && for f in @FILES@; do if [ -f $f ]; then cp -p $f @BK@/; else echo $f >> @BK@/.absent; fi; done && for f in @GEN@/wpa_supplicant_*.conf; do if [ -f $f ]; then cp -p $f @BK@/gen/; fi; done; touch @BK@/.done && echo NEW; fi'
    $out = Exec-Adb ($cmd.Replace("@BK@", $BACKUP_DIR).Replace("@FILES@", $BOARD_CONFIGS).Replace("@GEN@", $GEN_CONF_DIR))
    if ($out -match "REUSE") {
        Write-Host "[WARN] Found backup from an interrupted run in $BACKUP_DIR; keeping it (it holds the originals)" -ForegroundColor Yellow
        return $true
    }
    return ($out -match "NEW")
}

function Restore-BoardConfigs {
    # Puts back the original files and deletes the ones that did not exist before the run; generated
    # wpa_supplicant_*.conf files are replaced wholesale by the backed-up set (possibly none)
    $cmd = 'if [ -f @BK@/.done ]; then for f in @FILES@; do b=$(basename $f); if [ -f @BK@/$b ]; then cp -p @BK@/$b $f; elif grep -qxF $f @BK@/.absent 2>/dev/null; then rm -f $f; fi; done; rm -f @GEN@/wpa_supplicant_*.conf; for f in @BK@/gen/*.conf; do if [ -f $f ]; then cp -p $f @GEN@/; fi; done; rm -rf @BK@; echo RESTORED; fi'
    $out = Exec-Adb ($cmd.Replace("@BK@", $BACKUP_DIR).Replace("@FILES@", $BOARD_CONFIGS).Replace("@GEN@", $GEN_CONF_DIR))
    if ($out -match "RESTORED") {
        Write-Host "Board configs restored ($BOARD_CONFIGS $GEN_CONF_DIR/wpa_supplicant_*.conf)" -ForegroundColor Green
    } else {
        Write-Host "[WARN] Board config restore did not run; check $BACKUP_DIR on the board" -ForegroundColor Yellow
    }
}

function Get-IfaceHostapdPids {
    # PIDs of hostapd processes that may control $TEST_IFACE (same rules as the cleanup helper)
    $out = Exec-Adb "sh $CLEANUP_SH --dry-run $TEST_IFACE hostapd hostapd_s1g 2>/dev/null"
    $pids = @($out -split "`n" | ForEach-Object { ($_.Trim() -split '\s+')[0] } | Where-Object { $_ -match '^\d+$' })
    return ($pids -join " ")
}

function Cleanup-Environment {
    Exec-Adb "sh $CLEANUP_SH $TEST_IFACE 2>/dev/null" | Out-Null
    Exec-Adb "rm -f /etc/hostapd.conf /etc/dnsmasq.conf /var/run/hostapd/* /var/run/wpa_supplicant/* 2>/dev/null" | Out-Null
    Exec-Adb "ip addr flush dev $TEST_IFACE 2>/dev/null" | Out-Null
    Exec-Adb "ip link set $TEST_IFACE down 2>/dev/null; sleep 0.3; ip link set $TEST_IFACE up 2>/dev/null" | Out-Null
}

# ==================== Interface isolation canaries ====================

function ConvertFrom-CanaryOutput {
    # Parses "alive|dead <name> <pid>" lines into @{ name = @{ Alive; Pid } }
    param([string]$Text)
    $state = @{}
    foreach ($line in ($Text -split "`n")) {
        if ($line.Trim() -match '^(alive|dead)\s+(\S+)\s+(\S+)$') {
            $state[$matches[2]] = @{ Alive = ($matches[1] -eq "alive"); Pid = $matches[3] }
        }
    }
    return $state
}

function Start-Canaries {
    # Starts the canaries and proves the check is not vacuous: the cleanup rules must classify
    # every canary as a daemon of $CanaryIface (positive control) and none as one of $TEST_IFACE.
    $out = Exec-Adb "sh $CANARY_SH start $CanaryIface"
    if ($out -match "unsupported shell") {
        $script:CanaryInconclusive = $true
        $script:CanaryError = "canaries need a non-busybox shell ($($out.Trim()))"
        return
    }
    $state = ConvertFrom-CanaryOutput $out
    foreach ($n in $CANARY_NAMES) {
        if (-not ($state.ContainsKey($n) -and $state[$n].Alive)) {
            $script:CanaryError = "canary '$n' failed to start ($($out -replace "`n", '; '))"
            return
        }
        $script:CanaryPids[$n] = $state[$n].Pid
    }

    $names = $CANARY_NAMES -join " "
    $own = @((Exec-Adb "sh $CLEANUP_SH --dry-run $CanaryIface $names") -split "`n" | ForEach-Object { ($_.Trim() -split '\s+')[0] })
    $test = @((Exec-Adb "sh $CLEANUP_SH --dry-run $TEST_IFACE $names") -split "`n" | ForEach-Object { ($_.Trim() -split '\s+')[0] })
    foreach ($n in $CANARY_NAMES) {
        $p = $CanaryPids[$n]
        if ($own -notcontains $p) { $script:CanaryError = "positive control failed: cleanup rules do not see canary '$n' ($p) as a $CanaryIface daemon"; return }
        if ($test -contains $p) { $script:CanaryError = "cleanup rules would kill canary '$n' ($p) when cleaning $TEST_IFACE"; return }
    }
    Write-Host "Canaries on ${CanaryIface}: $(($CANARY_NAMES | ForEach-Object { "$_($($CanaryPids[$_]))" }) -join ', ')" -ForegroundColor Green
}

function Update-CanaryStatus {
    # Records, for each canary, the first phase after which it was found dead
    param([string]$After)
    if ($CanaryError -or $CanaryPids.Count -eq 0) { return }
    $state = ConvertFrom-CanaryOutput (Exec-Adb "sh $CANARY_SH check")
    foreach ($n in $CANARY_NAMES) {
        $alive = $state.ContainsKey($n) -and $state[$n].Alive -and $state[$n].Pid -eq $CanaryPids[$n]
        if (-not $alive -and -not $CanaryDeaths.Contains($n)) { $CanaryDeaths[$n] = $After }
    }
    $CanaryChecked.Add($After)
}

function Add-CanaryResult {
    $id = "TC-SYS-01"
    $name = "Daemons on other interfaces survive"
    if ($CanaryInconclusive) {
        Add-Result "Interface Isolation" $id $name "INCONCLUSIVE" $CanaryError
    } elseif ($CanaryError) {
        Add-Result "Interface Isolation" $id $name "FAIL" $CanaryError
    } elseif ($CanaryDeaths.Count -gt 0) {
        $killed = ($CanaryDeaths.Keys | ForEach-Object { "$_ (found dead after $($CanaryDeaths[$_]))" }) -join ", "
        Add-Result "Interface Isolation" $id $name "FAIL" "Killed although bound to ${CanaryIface}: $killed"
    } else {
        Add-Result "Interface Isolation" $id $name "PASS" "$($CANARY_NAMES -join '/') on $CanaryIface alive after: $($CanaryChecked -join ', ')"
    }
}

# ==================== Tool session driver ====================

function Start-ToolSession {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = "adb"
    $psi.Arguments = "shell -t $TARGET_BIN"
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false

    $proc = [System.Diagnostics.Process]::Start($psi)
    return [PSCustomObject]@{
        Proc = $proc
        In   = $proc.StandardInput
        Out  = [QwtStreamPump]::new($proc.StandardOutput.BaseStream)
        Err  = [QwtStreamPump]::new($proc.StandardError.BaseStream)
    }
}

function Wait-ToolIdle {
    # Returns once the tool has printed something past $Mark and then gone quiet for $QUIET_MS,
    # i.e. it is most likely blocked on its next prompt. Returns $false on timeout or exit.
    param($Session, [int]$Mark, [int]$TimeoutMs = 15000)
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.ElapsedMilliseconds -lt $TimeoutMs) {
        if ($Session.Out.Length -gt $Mark -and $Session.Out.IdleMs -ge $QUIET_MS) { return $true }
        if ($Session.Out.Ended) { return $false }
        Start-Sleep -Milliseconds 50
    }
    return $false
}

function Send-ToolInputs {
    # Feeds inputs one at a time, each after the tool has settled on its prompt.
    # Returns $false if the tool exited before every input was sent.
    param($Session, [string[]]$Inputs)
    [void](Wait-ToolIdle $Session 0 20000)   # banner + language prompt
    foreach ($inp in $Inputs) {
        if ($Session.Proc.HasExited) { return $false }
        $mark = $Session.Out.Length
        try { $Session.In.WriteLine($inp) } catch { return $false }
        [void](Wait-ToolIdle $Session $mark)
    }
    return $true
}

function Get-SessionText {
    param($Session)
    return (Hide-Secrets (Remove-Ansi ($Session.Out.Text + $Session.Err.Text)))
}

function Stop-ToolSession {
    param($Session)
    try { if (-not $Session.Proc.HasExited) { $Session.Proc.Kill() } } catch {}
    Exec-Adb "killall -9 QWifiTestTool 2>/dev/null" | Out-Null
    Start-Sleep -Milliseconds 300
}

function Invoke-ToolScript {
    # Runs one scripted session that must end with [0] at the main menu (tool prints goodbye and exits).
    # Ok=$false means the inputs went out of step with the menus or the tool hung/crashed.
    param([string[]]$Inputs)
    $s = Start-ToolSession
    $allSent = Send-ToolInputs $s $Inputs
    $exited = $s.Proc.WaitForExit(8000)
    $text = Get-SessionText $s
    Stop-ToolSession $s

    $reason = ""
    if (-not $allSent) {
        $reason = "tool exited before all inputs were sent"
    } elseif ($text -match $INVALID_INPUT_RE) {
        $reason = "input out of step with menus (tool printed '$($matches[0])')"
    } elseif (-not $exited) {
        $reason = "tool did not exit after the final [0] (inputs out of step or hung)"
    } elseif ($text -notmatch $GOODBYE_RE) {
        $reason = "tool exited without the goodbye message (crash?)"
    }
    return [PSCustomObject]@{ Ok = ($reason -eq ""); Reason = $reason; Transcript = $text }
}

function Resolve-ModuleIndex {
    # Reads the tool's module menu and returns the index whose name contains $ModuleName
    $s = Start-ToolSession
    [void](Wait-ToolIdle $s 0 20000)
    $mark = $s.Out.Length
    try { $s.In.WriteLine("N") } catch {}
    [void](Wait-ToolIdle $s $mark)
    $text = Get-SessionText $s
    Stop-ToolSession $s

    $hits = @([regex]::Matches($text, '(?m)^\s*\[(\d+)\]\s+(.+)$') |
        Where-Object { $_.Groups[2].Value -match [regex]::Escape($ModuleName) })
    if ($hits.Count -ne 1) {
        Write-Host "Error: expected exactly 1 module matching '$ModuleName' in the tool menu, found $($hits.Count)" -ForegroundColor Red
        foreach ($h in $hits) { Write-Host "  $($h.Value.Trim())" }
        return $false
    }
    $script:MODULE_IDX = $hits[0].Groups[1].Value
    $script:MODULE_LABEL = ($hits[0].Groups[2].Value -replace '\s+', ' ').Trim()
    return $true
}

function Get-LangInput {
    # Answer to the "Switch to English? [y/N]" prompt
    param([string]$Lang)
    if ($Lang -eq "en") { return "y" }
    return "N"
}

function Find-CjkLines {
    # Lines with CJK text after the language prompt (the banner before it is always Chinese)
    param([string]$Transcript)
    $i = $Transcript.IndexOf("Switch to English?")
    $text = if ($i -ge 0) { $Transcript.Substring($i) } else { $Transcript }
    return @($text -split "`n" | Where-Object { $_ -match $CJK_RE } | ForEach-Object { $_.Trim() } | Select-Object -Unique)
}

function Format-CjkFinding {
    param([string[]]$Lines)
    $shown = ($Lines | Select-Object -First 3) -join " | "
    return "Chinese text in English UI ($($Lines.Count) lines): $shown"
}

function Wait-ToolText {
    # Waits until the output after raw offset $From matches $Pattern.
    # Returns "ok", "fail:<text>" ($FailPattern matched first), "exit" or "timeout".
    param($Session, [int]$From, [string]$Pattern, [string]$FailPattern = "", [int]$TimeoutMs = 15000)
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.ElapsedMilliseconds -lt $TimeoutMs) {
        $raw = $Session.Out.Text
        $t = if ($From -lt $raw.Length) { Remove-Ansi $raw.Substring($From) } else { "" }
        if ($FailPattern -and $t -match $FailPattern) { return "fail:$($matches[0])" }
        if ($t -match $Pattern) { return "ok" }
        if ($Session.Out.Ended) { return "exit" }
        Start-Sleep -Milliseconds 200
    }
    return "timeout"
}

function Invoke-ToolSteps {
    # Expect-style session. Each step is a hashtable:
    #   @{ Send = "x"; Expect = "regex"; Fail = "regex"; Timeout = ms }  send a line, then wait for Expect
    #   @{ Expect = "regex" }                                            wait from the start of output
    #   @{ Check = { ... } }                                             board check; returns "" or an error
    # The last step must make the tool exit. Returns Ok / Reason / Transcript.
    param([object[]]$Steps)
    $s = Start-ToolSession
    $reason = ""
    $n = 0
    foreach ($st in $Steps) {
        $n++
        if ($st.Check) {
            $err = & $st.Check
            if ($err) { $reason = "step ${n}: $err"; break }
            continue
        }
        $mark = 0
        $label = "at start"
        if ($null -ne $st.Send) {
            # A scriptblock Send is computed from the output so far (e.g. a scan list index)
            $value = $st.Send
            if ($value -is [scriptblock]) {
                $value = & $value (Get-SessionText $s)
                if ($null -eq $value) { $reason = "step ${n}: could not determine '$($st.Label)' from the output"; break }
            }
            $mark = $s.Out.Length
            $label = if ($st.Secret) { "after '<password>'" } elseif ($st.Label) { "after '$($st.Label)'" } else { "after '$value'" }
            try { $s.In.WriteLine($value) } catch { $reason = "step ${n}: tool exited before the input ($label) was sent"; break }
        }
        $timeout = if ($st.Timeout) { $st.Timeout } else { 15000 }
        $r = Wait-ToolText $s $mark $st.Expect $st.Fail $timeout
        if ($r -ne "ok") {
            $why = switch -Regex ($r) {
                '^fail:' { "tool reported '$($r.Substring(5))'" }
                '^exit$' { "tool exited while waiting for /$($st.Expect)/" }
                default { "/$($st.Expect)/ not seen within $([int]($timeout / 1000))s" }
            }
            $reason = "step $n ($label): $why"
            break
        }
    }
    $exited = if ($reason) { $true } else { $s.Proc.WaitForExit(8000) }
    $text = Get-SessionText $s
    Stop-ToolSession $s
    if (-not $exited) { $reason = "tool did not exit after the last step" }
    return [PSCustomObject]@{ Ok = ($reason -eq ""); Reason = $reason; Transcript = $text }
}

# ==================== Phase 1: AP Preset Matrix (29 Presets) ====================

# Keys whose presence depends on the preset; any of them not listed in a case's expectation must be absent
$AP_MANAGED_KEYS = @(
    "ieee80211n", "ieee80211ac", "ieee80211ax", "ieee80211be", "ht_capab", "op_class",
    "vht_oper_chwidth", "he_oper_chwidth", "eht_oper_chwidth",
    "vht_oper_centr_freq_seg0_idx", "he_oper_centr_freq_seg0_idx", "eht_oper_centr_freq_seg0_idx"
)

function ConvertFrom-ConfText {
    # Parses key=value lines (comments and blanks ignored) into a hashtable
    param([string]$Text)
    $map = @{}
    foreach ($line in ($Text -split "`n")) {
        $l = $line.Trim()
        if ($l -eq "" -or $l.StartsWith("#")) { continue }
        $i = $l.IndexOf("=")
        if ($i -gt 0) { $map[$l.Substring(0, $i)] = $l.Substring($i + 1) }
    }
    return $map
}

function Test-ApPresetConfig {
    param(
        [string]$TcId,
        [string]$Name,
        [int]$BandKey,
        [int]$PresetKey,
        # Exact key=value lines; all other $AP_MANAGED_KEYS must be absent
        [string[]]$Expected,
        # "en" also asserts that no Chinese text is shown after the language prompt
        [string]$Lang = "zh"
    )

    Write-Host -NoNewline "  [$TcId] $Name ... "

    # Remove previous config so we know it is freshly generated
    Exec-Adb "rm -f /etc/hostapd.conf" | Out-Null

    # Inputs: Language -> Module -> Menu(4: AP Edit) -> Submenu(4: Preset Wizard) -> Band -> Preset -> Save(y) -> Exit Main(0)
    # (saving returns straight to the main menu, so a single [0] exits the tool)
    $session = Invoke-ToolScript @((Get-LangInput $Lang), $MODULE_IDX, "4", "4", "$BandKey", "$PresetKey", "y", "0")

    $hostapdConf = Exec-Adb "cat /etc/hostapd.conf 2>/dev/null"

    $mismatches = [System.Collections.Generic.List[string]]::new()
    if (-not $session.Ok) { $mismatches.Add($session.Reason) }
    if ($Lang -eq "en") {
        $cjk = Find-CjkLines $session.Transcript
        if ($cjk.Count -gt 0) { $mismatches.Add((Format-CjkFinding $cjk)) }
    }

    if ([string]::IsNullOrWhiteSpace($hostapdConf)) {
        $mismatches.Add("hostapd.conf not generated")
    } else {
        $map = ConvertFrom-ConfText $hostapdConf
        $expectedKeys = @{}
        foreach ($kv in $Expected) {
            $k, $v = $kv.Split("=", 2)
            $expectedKeys[$k] = $true
            if (-not $map.ContainsKey($k)) {
                $mismatches.Add("Missing '$kv'")
            } elseif ($map[$k] -cne $v) {
                $mismatches.Add("'$k=$($map[$k])' (expected '$v')")
            }
        }
        foreach ($k in $AP_MANAGED_KEYS) {
            if ($map.ContainsKey($k) -and -not $expectedKeys.ContainsKey($k)) {
                $mismatches.Add("Unexpected '$k=$($map[$k])'")
            }
        }
    }

    if ($mismatches.Count -eq 0) {
        Write-Host "[PASS]" -ForegroundColor Green
        Add-Result "AP Preset Matrix" $TcId $Name "PASS" "hostapd.conf verified ($($Expected.Count) exact lines, no stray width/mode keys)"
    } else {
        Write-Host "[FAIL] ($($mismatches -join ', '))" -ForegroundColor Red
        Save-Artifact $TcId "tui.log" $session.Transcript
        Save-Artifact $TcId "hostapd.conf" $hostapdConf
        Add-Result "AP Preset Matrix" $TcId $Name "FAIL" ($mismatches -join "; ")
    }
}

# ==================== Phase 2: STA Policy Matrix (11 Policies) ====================

function Test-StaPolicyConfig {
    param(
        [string]$TcId,
        [string]$Name,
        [string[]]$EditInputs,
        [string[]]$ExpectedTokens,
        [string[]]$ForbiddenTokens = @(),
        [string]$ExpectedBand = "", # "2.4G", "5G_NO_DFS", "5G_DFS", "6G", "ALL"
        [string]$Lang = "zh"
    )

    Write-Host -NoNewline "  [$TcId] $Name ... "

    # Ensure wpa_supplicant is not running so restartNetworkIfNeeded won't ask interactive question
    Exec-Adb "sh $CLEANUP_SH $TEST_IFACE wpa_supplicant 2>/dev/null" | Out-Null
    Exec-Adb "rm -f /etc/wpa_supplicant.conf 2>/dev/null" | Out-Null

    # Inputs: Language -> Module -> Menu(2: STA Edit) -> Edit(1) -> [Specific Inputs] -> Finish(0) -> Save(y) -> Exit Main(0)
    $session = Invoke-ToolScript (@((Get-LangInput $Lang), $MODULE_IDX, "2", "1") + $EditInputs + @("0", "y", "0"))

    # Verify /etc/wpa_supplicant.conf
    $wpaConf = Exec-Adb "cat /etc/wpa_supplicant.conf 2>/dev/null"

    $mismatches = [System.Collections.Generic.List[string]]::new()
    if (-not $session.Ok) { $mismatches.Add($session.Reason) }
    if ($Lang -eq "en") {
        $cjk = Find-CjkLines $session.Transcript
        if ($cjk.Count -gt 0) { $mismatches.Add((Format-CjkFinding $cjk)) }
    }
    foreach ($token in $ExpectedTokens) {
        if (-not $wpaConf.Contains($token)) {
            $mismatches.Add("Missing '$token'")
        }
    }
    foreach ($ftoken in $ForbiddenTokens) {
        if ($wpaConf.Contains($ftoken)) {
            $mismatches.Add("Found forbidden '$ftoken'")
        }
    }

    # Approach A: Physical RF Scanning Closed-Loop Verification
    $rfScanSummary = ""
    $inconclusive = $false
    if ($mismatches.Count -eq 0 -and $ExpectedBand -and (-not $MatrixOnly)) {
        # 1. Ensure clean managed mode on the test interface
        Exec-Adb "sh $CLEANUP_SH $TEST_IFACE wpa_supplicant hostapd 2>/dev/null" | Out-Null
        Exec-Adb "ip link set $TEST_IFACE down 2>/dev/null; iw dev $TEST_IFACE set type managed 2>/dev/null; ip link set $TEST_IFACE up 2>/dev/null" | Out-Null
        Start-Sleep -Milliseconds 400

        # 2. Trigger native nl80211 physical RF probe scan with flush (hardware TX/RX probe)
        $scanOutput = ""
        if ($wpaConf -match 'freq_list=([0-9 ]+)') {
            $freqList = $matches[1].Trim()
            $scanOutput = Exec-Adb "iw dev $TEST_IFACE scan flush freq $freqList"
        } else {
            $scanOutput = Exec-Adb "iw dev $TEST_IFACE scan flush"
        }

        # 2.5. Check for iw scan errors (permission denied, device busy, etc.)
        $scanFailed = $false
        if ($scanOutput -match '(Operation not permitted|command failed|No such device|Device or resource busy)') {
            $scanFailed = $true
            $mismatches.Add("iw scan failed: $($matches[1])")
        }

        # 3. Read captured physical RF frequencies
        if (-not $scanFailed) {
            $scannedFreqs = [System.Collections.Generic.List[int]]::new()
            foreach ($line in ($scanOutput -split "`n")) {
                if ($line -match '^\s*freq:\s*(\d+)') {
                    [void]$scannedFreqs.Add([int]$matches[1])
                }
            }
            $uniqueFreqs = @($scannedFreqs | Select-Object -Unique | Sort-Object)

            # 3.5. Guard: 0 BSS = inconclusive (shielded room / no AP on this band), never reported as PASS
            if ($uniqueFreqs.Count -eq 0) {
                $inconclusive = $true
                $rfScanSummary = " + RF Scan: 0 BSS (inconclusive, cannot verify band boundary)"
            } else {
                # 4. Spectrum Enforcement Assertions
                $rfLeaks = [System.Collections.Generic.List[string]]::new()
                foreach ($f in $uniqueFreqs) {
                    if ($ExpectedBand -eq "2.4G") {
                        if ($f -lt 2400 -or $f -gt 2500) {
                            $rfLeaks.Add("Leak outside 2.4G ($f MHz)")
                        }
                    } elseif ($ExpectedBand -eq "5G_NO_DFS") {
                        if ($f -lt 5000 -or $f -gt 5900) {
                            $rfLeaks.Add("Leak outside 5G ($f MHz)")
                        }
                        if ($f -ge 5260 -and $f -le 5720) {
                            $rfLeaks.Add("DFS Leakage ($f MHz)")
                        }
                    } elseif ($ExpectedBand -eq "5G_DFS") {
                        if ($f -lt 5000 -or $f -gt 5900) {
                            $rfLeaks.Add("Leak outside 5G ($f MHz)")
                        }
                    } elseif ($ExpectedBand -eq "6G") {
                        if ($f -lt 5945 -or $f -gt 7125) {
                            $rfLeaks.Add("Leak outside 6G ($f MHz)")
                        }
                    }
                }

                if ($rfLeaks.Count -gt 0) {
                    $mismatches.Add("Physical RF Scan Leak: " + (($rfLeaks | Select-Object -Unique) -join ", "))
                } else {
                    $freqCount = $uniqueFreqs.Count
                    $rfScanSummary = " + Physical RF Scan Verified ($freqCount BSS) ($($uniqueFreqs -join ',') MHz)"
                }
            }
        }
    }

    if ($mismatches.Count -gt 0) {
        Write-Host "[FAIL] ($($mismatches -join ', '))" -ForegroundColor Red
        Save-Artifact $TcId "tui.log" $session.Transcript
        Save-Artifact $TcId "wpa_supplicant.conf" $wpaConf
        Add-Result "STA Policy Matrix" $TcId $Name "FAIL" ($mismatches -join "; ")
    } elseif ($inconclusive) {
        Write-Host "[INCONCLUSIVE] (config OK, RF scan found 0 BSS)" -ForegroundColor Yellow
        Add-Result "STA Policy Matrix" $TcId $Name "INCONCLUSIVE" ("Policy verified" + $rfScanSummary)
    } else {
        Write-Host "[PASS]" -ForegroundColor Green
        Add-Result "STA Policy Matrix" $TcId $Name "PASS" ("Policy verified" + $rfScanSummary)
    }
}

# ==================== Phase 3: Hardware RF Closed-Loop Verification ====================

function Run-ApRfTestCase {
    param(
        [string]$TcId,
        [string]$Name,
        [int]$BandKey,
        [int]$PresetKey,
        [string]$ExpectedTuiMode,
        [string]$ExpectedTuiCh,
        [string]$ExpectedRfWidth,
        [string]$Lang = "zh",
        [string]$Category = "AP Hardware RF"
    )

    Write-Host ""
    Write-Host "=======================================================" -ForegroundColor Cyan
    Write-Host ">> Running ${TcId}: ${Name} (Hardware RF Broadcast)" -ForegroundColor Cyan
    Write-Host "=======================================================" -ForegroundColor Cyan

    Cleanup-Environment
    Start-Sleep -Seconds 1

    # Inputs: Language -> Module -> Menu(4: AP Edit) -> Preset(4) -> Band -> Preset -> Save(y) -> Start AP(5)
    $session = Start-ToolSession
    [void](Send-ToolInputs $session @((Get-LangInput $Lang), $MODULE_IDX, "4", "4", "$BandKey", "$PresetKey", "y", "5"))

    # DFS CAC requires 60 seconds. Go side waits up to 75s after hostapd start, plus driver load / IP setup.
    $timeoutSec = 35
    if ($BandKey -eq 2 -and ($PresetKey -eq 1 -or $PresetKey -eq 5 -or $PresetKey -eq 9)) {
        $timeoutSec = 120
    }

    $apReady = $false
    $failReason = ""
    $printed = 0
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        $text = Get-SessionText $session

        # Echo newly completed TUI lines of interest
        $lastNl = $text.LastIndexOf("`n")
        if ($lastNl -ge $printed) {
            foreach ($line in ($text.Substring($printed, $lastNl - $printed + 1) -split "`n")) {
                $trimmed = $line.Trim()
                if ($trimmed -match "(\[INFO\]|\[CMD\]|SSID:|channel|width:|Ctrl\+C|stop|fail)") {
                    Write-Host "  [TUI] $trimmed" -ForegroundColor Gray
                }
            }
            $printed = $lastNl + 1
        }

        # Dynamically extend timeout if DFS CAC is detected in output
        if ($text -match "(CAC.*60|ieee80211h=1|DFS)") { $timeoutSec = 120 }

        if ($text -match "Ctrl\+C") { $apReady = $true; break }
        if ($text -match $AP_FAIL_RE) { $failReason = "tool reported '$($matches[0])'"; break }
        if ($session.Out.Ended) { $failReason = "tool exited unexpectedly"; break }
        Start-Sleep -Milliseconds 500
    }

    if (-not $apReady) {
        if (-not $failReason) { $failReason = "AP start timeout (${timeoutSec}s)" }
        Save-Artifact $TcId "tui.log" (Get-SessionText $session)
        Save-Artifact $TcId "hostapd.conf" (Exec-Adb "cat /etc/hostapd.conf 2>/dev/null")
        Save-Artifact $TcId "dmesg.log" (Exec-Adb "dmesg | tail -n 200")
        Stop-ToolSession $session
        Cleanup-Environment
        Write-Host "  ==> [FAIL] $failReason" -ForegroundColor Red
        Add-Result $Category $TcId $Name "FAIL" $failReason
        return
    }

    $fullLog = Get-SessionText $session

    # Assertion 1: TUI (plus, for English UI, no Chinese text after the language prompt)
    $tuiModeOk = $fullLog.Contains($ExpectedTuiMode)
    $tuiChOk = $fullLog.Contains($ExpectedTuiCh)
    $cjk = @()
    if ($Lang -eq "en") { $cjk = Find-CjkLines $fullLog }
    $tuiPass = $tuiModeOk -and $tuiChOk -and ($cjk.Count -eq 0)

    # Assertion 2: Linux RF state (iw)
    $iwInfo = Exec-Adb "iw dev $TEST_IFACE info"
    $isAp = $iwInfo.Contains("type AP")

    if ($ExpectedRfWidth -eq "320 MHz") {
        $rfWidthOk = $iwInfo.Contains("320 MHz") -or $iwInfo.Contains("center1: 6105")
    } elseif ($ExpectedRfWidth -eq "40 MHz" -and $BandKey -eq 1) {
        # 2.4GHz 40MHz may fallback to 20MHz if OBSS coexistence is triggered by nearby APs
        $rfWidthOk = $iwInfo.Contains("40 MHz") -or $iwInfo.Contains("20 MHz")
    } elseif ($ExpectedRfWidth) {
        $rfWidthOk = $iwInfo.Contains($ExpectedRfWidth)
    } else {
        $rfWidthOk = $true
    }

    $rfPass = $isAp -and $rfWidthOk

    # Assertion 3: Daemon running (hostapd for $TEST_IFACE; hostapd on other interfaces is ignored)
    $hostapdPid = Get-IfaceHostapdPids
    $daemonPass = ($hostapdPid -ne "")

    Write-Host "  [RF INFO] Hardware RF: AP=$isAp, WidthMatch=$rfWidthOk, hostapd PID=$hostapdPid" -ForegroundColor Green

    # Send SIGINT (graceful shutdown)
    Exec-Adb "killall -2 QWifiTestTool" | Out-Null
    Start-Sleep -Seconds 2

    # Assertion 4: Clean teardown (no hostapd left for $TEST_IFACE, including ones with unknown binding)
    $hostapdAfter = Get-IfaceHostapdPids
    $teardownPass = ($hostapdAfter -eq "")

    $passed = $tuiPass -and $rfPass -and $daemonPass -and $teardownPass
    if (-not $passed) {
        Save-Artifact $TcId "tui.log" (Get-SessionText $session)
        Save-Artifact $TcId "iw_info.log" $iwInfo
        Save-Artifact $TcId "hostapd.conf" (Exec-Adb "cat /etc/hostapd.conf 2>/dev/null")
    }

    Stop-ToolSession $session
    Cleanup-Environment
    Start-Sleep -Milliseconds 500

    $statusStr = if ($passed) { "PASS" } else { "FAIL" }

    $details = [System.Collections.Generic.List[string]]::new()
    if (-not ($tuiModeOk -and $tuiChOk)) { $details.Add("TUI mismatch (Mode:$tuiModeOk, Ch:$tuiChOk)") }
    if ($cjk.Count -gt 0) { $details.Add((Format-CjkFinding $cjk)) }
    if (-not $rfPass) { $details.Add("RF state mismatch (AP:$isAp, Width:$rfWidthOk)") }
    if (-not $daemonPass) { $details.Add("hostapd daemon missing") }
    if (-not $teardownPass) { $details.Add("hostapd remaining after stop") }

    $detailStr = if ($details.Count -gt 0) { $details -join "; " } else { "Physical RF ${ExpectedRfWidth} perfectly matched, teardown clean" }

    $color = if ($passed) { "Green" } else { "Red" }
    Write-Host "  ==> [$($statusStr)] $detailStr" -ForegroundColor $color

    Add-Result $Category $TcId $Name $statusStr $detailStr
}

# ==================== Phase 4: Diagnosis & Driver Lifecycle ====================

# zh UI markers used by the diagnosis tests
$DIAG_PASS_MENU = "所有檢查通過[\s\S]*請輸入選項"
$DIAG_FAIL = "⚠️  錯誤:|❌ 錯誤:"
$MAIN_MENU = "選擇操作"

function Get-DriverState {
    # Returns @{ Loaded; Iface } for $DRIVER_MOD (/proc/modules) and $TEST_IFACE (/sys/class/net)
    $out = Exec-Adb "grep -c '^$DRIVER_MOD ' /proc/modules; [ -d /sys/class/net/$TEST_IFACE ] && echo iface=1 || echo iface=0"
    $lines = @($out -split "`n" | ForEach-Object { $_.Trim() })
    return @{ Loaded = ($DRIVER_MOD -and $lines[0] -match '^[1-9]'); Iface = ($out -match "iface=1") }
}

function Test-DriverLoaded {
    # Board check for Invoke-ToolSteps: "" when the driver and interface are both present
    $st = Get-DriverState
    if (-not $st.Loaded) { return "driver '$DRIVER_MOD' not in /proc/modules" }
    if (-not $st.Iface) { return "$TEST_IFACE missing although the driver is loaded" }
    return ""
}

function Test-DriverUnloaded {
    $st = Get-DriverState
    if ($st.Loaded) { return "driver '$DRIVER_MOD' still in /proc/modules after unload" }
    if ($st.Iface) { return "$TEST_IFACE still present after unload" }
    return ""
}

function Test-DiagCase {
    param([string]$TcId, [string]$Name, [object[]]$Steps)
    Write-Host -NoNewline "  [$TcId] $Name ... "
    Cleanup-Environment
    $r = Invoke-ToolSteps $Steps
    if ($r.Ok) {
        Write-Host "[PASS]" -ForegroundColor Green
        Add-Result "Diagnosis & Driver" $TcId $Name "PASS" "All steps reached the expected screens"
    } else {
        Write-Host "[FAIL] ($($r.Reason))" -ForegroundColor Red
        Save-Artifact $TcId "tui.log" $r.Transcript
        Save-Artifact $TcId "dmesg.log" (Exec-Adb "dmesg | tail -n 200")
        Add-Result "Diagnosis & Driver" $TcId $Name "FAIL" $r.Reason
    }
    return $r
}

function Invoke-DiagnosisPhase {
    # Steps shared by every case: language, module, then the main menu
    $intro = @(
        @{ Expect = "Switch to English" },
        @{ Send = "N"; Expect = "選擇模組編號" },
        @{ Send = $MODULE_IDX; Expect = $MAIN_MENU }
    )

    # TC-DIAG-01: sanity check passes and offers the post-diagnosis menu
    $r = Test-DiagCase "TC-DIAG-01" "Diagnosis: all checks pass" ($intro + @(
        @{ Send = "1"; Expect = $DIAG_PASS_MENU; Fail = $DIAG_FAIL; Timeout = 30000 },
        @{ Send = "0"; Expect = $MAIN_MENU },
        @{ Send = "0"; Expect = $GOODBYE_RE }
    ))
    if ($r.Transcript -match '\[Main\] (\S+): loaded') { $script:DRIVER_MOD = $matches[1] }
    if (-not $r.Ok -or -not $DRIVER_MOD) {
        $why = if ($r.Ok) { "main driver name not found in diagnosis output" } else { "TC-DIAG-01 failed" }
        foreach ($id in "TC-DIAG-02", "TC-DIAG-03", "TC-DIAG-04") {
            Add-Result "Diagnosis & Driver" $id "(skipped)" "FAIL" "Not run: $why"
        }
        return
    }

    # TC-DIAG-02: unload from the post-diagnosis menu, then diagnosis detects it and loads it back
    [void](Test-DiagCase "TC-DIAG-02" "Unload driver, reload via diagnosis prompt" ($intro + @(
        @{ Send = "1"; Expect = $DIAG_PASS_MENU; Fail = $DIAG_FAIL; Timeout = 30000 },
        @{ Send = "1"; Expect = "驅動已成功卸載[\s\S]*$MAIN_MENU"; Fail = "卸載驅動失敗|❌ 錯誤:"; Timeout = 30000 },
        @{ Check = { Test-DriverUnloaded } },
        @{ Send = "1"; Expect = "是否嘗試載入驅動"; Timeout = 30000 },
        @{ Send = "y"; Expect = $DIAG_PASS_MENU; Fail = "驅動載入失敗|❌ 錯誤:"; Timeout = 60000 },
        @{ Send = "0"; Expect = $MAIN_MENU },
        @{ Check = { Test-DriverLoaded } },
        @{ Send = "0"; Expect = $GOODBYE_RE }
    )))

    # TC-DIAG-03: unload + reload in one action
    [void](Test-DiagCase "TC-DIAG-03" "Reload driver from diagnosis menu" ($intro + @(
        @{ Send = "1"; Expect = $DIAG_PASS_MENU; Fail = $DIAG_FAIL; Timeout = 30000 },
        @{ Send = "2"; Expect = "驅動已成功重新載入[\s\S]*$MAIN_MENU"; Fail = "卸載驅動失敗|載入驅動失敗|❌ 錯誤:"; Timeout = 60000 },
        @{ Check = { Test-DriverLoaded } },
        @{ Send = "0"; Expect = $GOODBYE_RE }
    )))

    # TC-DIAG-04: the reloaded driver must still bring up an AP
    Run-ApRfTestCase -TcId "TC-DIAG-04" -Name "2.4GHz AP RF TX after driver reload" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz" -Category "Diagnosis & Driver"
}

# ==================== Phase 5: Station Connection (reference AP) ====================

function Read-RefAp {
    # Loads SSID + password from $RefApFile and registers the password as a secret
    if (-not (Test-Path $RefApFile)) { return $false }
    $lines = @(Get-Content -Encoding UTF8 $RefApFile)
    if ($lines.Count -lt 1 -or -not $lines[0].Trim()) { return $false }
    $script:REF_SSID = $lines[0].Trim()
    # The tool trims the password it reads, so do the same here
    $script:REF_PSK = if ($lines.Count -ge 2) { $lines[1].Trim() } else { "" }
    if ($REF_PSK) { $SECRETS.Add($REF_PSK) }
    return $true
}

function ConvertFrom-WpaEscaped {
    # Decodes wpa_cli's printf_encode output (\xNN bytes, \\, \", \n, \r, \t, \e) back to text
    param([string]$Text)
    $bytes = [System.Collections.Generic.List[byte]]::new()
    $i = 0
    while ($i -lt $Text.Length) {
        if ($Text[$i] -eq '\' -and $i + 1 -lt $Text.Length) {
            $n = $Text[$i + 1]
            if ($n -eq 'x' -and $i + 3 -lt $Text.Length -and $Text.Substring($i + 2, 2) -match '^[0-9a-fA-F]{2}$') {
                $bytes.Add([Convert]::ToByte($Text.Substring($i + 2, 2), 16)); $i += 4; continue
            }
            $map = @{ '\' = 0x5c; '"' = 0x22; 'n' = 0x0a; 'r' = 0x0d; 't' = 0x09; 'e' = 0x1b }
            if ($map.ContainsKey([string]$n)) { $bytes.Add([byte]$map[[string]$n]); $i += 2; continue }
        }
        foreach ($b in [System.Text.Encoding]::UTF8.GetBytes([string]$Text[$i])) { $bytes.Add($b) }
        $i++
    }
    return [System.Text.Encoding]::UTF8.GetString($bytes.ToArray())
}

function Find-ScanIndex {
    # Returns the scan-list number whose SSID column equals $REF_SSID; records C01's outcome
    param([string]$Transcript)
    $rows = [regex]::Matches($Transcript, '(?m)^\[(\d+)\] SSID: (.*?)\s*\t')
    foreach ($m in $rows) {
        if ($m.Groups[2].Value.TrimEnd() -ceq $REF_SSID) {
            $script:StaCheck["C01"] = ""
            return $m.Groups[1].Value
        }
    }
    # Neighbouring SSIDs can carry people's names: report only the count, the transcript artifact has the list
    $script:StaCheck["C01"] = "'$REF_SSID' not in scan list ($($rows.Count) other networks listed)"
    return $null
}

function Wait-RefApVisible {
    # Polls a raw iw scan until $REF_SSID is on air. Phone hotspots stop beaconing while idle, so a
    # missing AP is an environment problem (INCONCLUSIVE), not a tool failure.
    param([int]$TimeoutSec = 90)
    Exec-Adb "ip link set $TEST_IFACE up 2>/dev/null" | Out-Null
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $hinted = $false
    while ((Get-Date) -lt $deadline) {
        $raw = Exec-Adb "iw dev $TEST_IFACE scan 2>/dev/null | sed -n 's/^[[:space:]]*SSID: //p'"
        # iw escapes bytes >= 0x80 as \xNN, the same form ConvertFrom-WpaEscaped decodes
        $ssids = @($raw -split "`n" | ForEach-Object { ConvertFrom-WpaEscaped $_.TrimEnd("`r") })
        if ($ssids -ccontains $REF_SSID) { return $true }
        if (-not $hinted) {
            Write-Host "  Waiting up to ${TimeoutSec}s for '$REF_SSID' to beacon (phone hotspot: keep its settings screen open)..." -ForegroundColor Yellow
            $hinted = $true
        }
        Start-Sleep -Seconds 5
    }
    return $false
}

function Get-StaState {
    # wpa_supplicant state, decoded SSID, IPv4 address and default gateway of $TEST_IFACE
    $out = Exec-Adb "wpa_cli -p $WPA_CTRL -i $TEST_IFACE status 2>/dev/null; echo @@; ip -4 addr show dev $TEST_IFACE 2>/dev/null; echo @@; ip route show dev $TEST_IFACE 2>/dev/null"
    $parts = @($out -split '@@') + @("", "", "")
    $st = @{ State = ""; Ssid = ""; Ip = ""; Gw = "" }
    if ($parts[0] -match '(?m)^wpa_state=(\S+)') { $st.State = $matches[1] }
    if ($parts[0] -match '(?m)^ssid=(.*)$') { $st.Ssid = ConvertFrom-WpaEscaped $matches[1].Trim() }
    if ($parts[1] -match 'inet (\S+)') { $st.Ip = $matches[1] }
    if ($parts[2] -match 'default via (\S+)') { $st.Gw = $matches[1] }
    return $st
}

function Test-StaConnected {
    # "" when $TEST_IFACE is associated to the reference AP with an IP, the config holds the SSID
    # in the right encoding and the gateway answers ping; otherwise the list of problems
    $st = Get-StaState
    $problems = [System.Collections.Generic.List[string]]::new()
    if ($st.State -ne "COMPLETED") { $problems.Add("wpa_state=$($st.State)") }
    if ($st.Ssid -cne $REF_SSID) { $problems.Add("wpa_cli ssid='$($st.Ssid)'") }
    if (-not $st.Ip) { $problems.Add("no IPv4 on $TEST_IFACE") }

    # Non-ASCII SSIDs must be written as unquoted hex, plain ones as a quoted string
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($REF_SSID)
    $expected = if ($bytes | Where-Object { $_ -ge 0x80 }) {
        "ssid=" + (($bytes | ForEach-Object { '{0:x2}' -f $_ }) -join "")
    } else { "ssid=`"$REF_SSID`"" }
    # Same priority as the tool's ConfigManager: custom -> system -> generated in its working directory
    $conf = (Exec-Adb "for f in $GEN_CONF_DIR/wpa_supplicant_*_custom.conf /etc/wpa_supplicant.conf /etc/wpa_supplicant/wpa_supplicant.conf $GEN_CONF_DIR/wpa_supplicant_*.conf; do if [ -f `$f ]; then echo `$f; break; fi; done").Trim()
    $confSsids = @((Exec-Adb "grep -E '^[[:space:]]*ssid=' $conf 2>/dev/null") -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if (-not $conf) {
        $problems.Add("no wpa_supplicant config found")
    } elseif (-not ($confSsids | Where-Object { $_ -eq $expected })) {
        $problems.Add("$conf lacks '$expected' (has: $($confSsids -join ', '))")
    }

    if ($st.Gw) {
        $ping = Exec-Adb "ping -c 3 -W 2 -I $TEST_IFACE $($st.Gw) 2>&1 | tail -n 2"
        if (-not ($ping -match '(\d+) (packets )?received' -and [int]$matches[1] -gt 0)) { $problems.Add("gateway $($st.Gw) not reachable") }
    } elseif ($st.Ip) {
        $problems.Add("no default route via $TEST_IFACE")
    }
    if ($problems.Count -eq 0) { $script:StaInfo = "COMPLETED, $($st.Ip), gateway $($st.Gw) reachable, $conf has $expected" }
    return ($problems -join "; ")
}

function Test-StaDisconnected {
    # "" when nothing of the station session is left on $TEST_IFACE
    $problems = [System.Collections.Generic.List[string]]::new()
    $left = Exec-Adb "sh $CLEANUP_SH --dry-run $TEST_IFACE wpa_supplicant udhcpc dhclient 2>/dev/null"
    if ($left.Trim()) { $problems.Add("still running: $($left -replace "`n", ', ')") }
    $st = Get-StaState
    if ($st.Ip) { $problems.Add("IPv4 $($st.Ip) still on $TEST_IFACE") }
    return ($problems -join "; ")
}

function Add-StaResult {
    # One result per check; a check that never ran inherits the session's failure reason, or is
    # INCONCLUSIVE when it was blocked by a failed prerequisite ($DependsOn) or a missing AP ($Blocked)
    param([string]$Key, [string]$Name, $Session, [string]$PassDetails, [switch]$Last, [string]$DependsOn, [string]$Blocked)
    $id = "TC-STA-$Key"
    if (-not $Blocked -and $DependsOn -and -not $StaCheck.ContainsKey($Key) -and
        (-not $StaCheck.ContainsKey($DependsOn) -or $StaCheck[$DependsOn])) {
        $Blocked = "blocked: prerequisite TC-STA-$DependsOn did not pass"
    }
    if ($Blocked) {
        Write-Host "  [$id] $Name ... [INCONCLUSIVE] ($Blocked)" -ForegroundColor Yellow
        if ($Session) { Save-Artifact $id "tui.log" $Session.Transcript }
        Add-Result "STA Connection" $id $Name "INCONCLUSIVE" $Blocked
        return
    }
    if (-not $StaCheck.ContainsKey($Key)) {
        $err = "not reached: $($Session.Reason)"
    } else {
        $err = $StaCheck[$Key]
        if (-not $err -and $Last -and -not $Session.Ok) { $err = $Session.Reason }
    }
    if ($err) {
        Write-Host "  [$id] $Name ... [FAIL] ($(Hide-Secrets $err))" -ForegroundColor Red
        Save-Artifact $id "tui.log" $Session.Transcript
        Add-Result "STA Connection" $id $Name "FAIL" $err
    } else {
        Write-Host "  [$id] $Name ... [PASS]" -ForegroundColor Green
        Add-Result "STA Connection" $id $Name "PASS" $PassDetails
    }
}

function Invoke-StaConnectionPhase {
    $ssidRe = [regex]::Escape($REF_SSID)
    $intro = @(
        @{ Expect = "Switch to English" },
        @{ Send = "N"; Expect = "選擇模組編號" },
        @{ Send = $MODULE_IDX; Expect = $MAIN_MENU }
    )
    # Scan -> pick the reference AP -> password. Starts with no system or saved config, so the tool
    # generates wpa_supplicant_<module>.conf in its working directory (and offers to save it as custom).
    # C04 then checks that this generated file is reused with its network (N5 regression) and C05 that
    # the connection survives the tool exiting (N6 regression).
    $freshSystemConf = "rm -f /etc/wpa_supplicant.conf $GEN_CONF_DIR/wpa_supplicant_*.conf"
    $scanAndPick = @(
        @{ Send = "3"; Expect = "是否保存為客製化配置"; Timeout = 20000 },
        @{ Send = "N"; Expect = "選擇網路編號"; Timeout = 60000 },
        @{ Send = { param($t) Find-ScanIndex $t }; Label = "scan index of reference AP"; Expect = "輸入密碼"; Timeout = 10000 }
    )
    $script:StaCheck = @{}
    $names = [ordered]@{
        C01 = "Scan list shows reference SSID '$REF_SSID'"
        C02 = "Connect via scan + password + DHCP"
        C03 = "Disconnect leaves $TEST_IFACE clean"
        C04 = "Auto-reconnect from saved config"
        C05 = "Detect and keep existing connection"
        C06 = "Wrong password is reported as failure"
    }
    $offAir = "reference AP '$REF_SSID' not on air within 90s (environment, not the tool)"

    # Session A: TC-STA-C01 scan, C02 connect, C03 disconnect
    Cleanup-Environment
    if (-not (Wait-RefApVisible)) {
        foreach ($k in $names.Keys) { Add-StaResult $k $names[$k] $null "" -Blocked $offAir }
        return
    }
    Exec-Adb $freshSystemConf | Out-Null
    $a = Invoke-ToolSteps ($intro + $scanAndPick + @(
        @{ Send = $REF_PSK; Secret = $true; Expect = "(連接成功!|連接失敗，診斷資訊|已連接但未獲取 IP)[\s\S]*是否斷開連線"; Timeout = 120000 },
        @{ Check = { $script:StaCheck["C02"] = Test-StaConnected; "" } },
        @{ Send = "y"; Expect = "繼續其他操作"; Timeout = 30000 },
        @{ Check = { $script:StaCheck["C03"] = Test-StaDisconnected; "" } },
        @{ Send = "N"; Expect = $GOODBYE_RE }
    ))
    if ($StaCheck.ContainsKey("C02") -and -not $StaCheck["C02"] -and $a.Transcript -notmatch "連接成功!") {
        $StaCheck["C02"] = "board is connected but the tool did not report success"
    }
    Add-StaResult "C01" $names.C01 $a "Listed and selectable by its decoded name"
    Add-StaResult "C02" $names.C02 $a $StaInfo -DependsOn "C01"
    Add-StaResult "C03" $names.C03 $a "No wpa_supplicant/DHCP client or IPv4 left on $TEST_IFACE" -Last -DependsOn "C02"

    # Session B: TC-STA-C04 auto-connect from the saved config (hex SSID round trip), connection kept.
    # Needs the network C02 saved; skipped (INCONCLUSIVE) otherwise.
    $b = $null
    if ($StaCheck.ContainsKey("C02") -and -not $StaCheck["C02"]) {
        Cleanup-Environment
        $b = Invoke-ToolSteps ($intro + @(
            # The generated config is rebuilt from XML on every entry, keeping the network saved by C02
            @{ Send = "3"; Expect = "已保留先前儲存的網路設定[\s\S]*是否保存為客製化配置"; Timeout = 20000 },
            @{ Send = "N"; Expect = "已自動連接到: $ssidRe[\s\S]*繼續其他操作"; Fail = "自動連接失敗"; Timeout = 60000 },
            @{ Check = { $script:StaCheck["C04"] = Test-StaConnected; "" } },
            @{ Send = "N"; Expect = $GOODBYE_RE }
        ))
    }
    Add-StaResult "C04" $names.C04 $b $StaInfo -Last -DependsOn "C02"

    # Session C: TC-STA-C05 an existing connection is detected and kept (needs B's live connection)
    $c = $null
    if ($StaCheck.ContainsKey("C04") -and -not $StaCheck["C04"]) {
        $c = Invoke-ToolSteps ($intro + @(
            @{ Send = "3"; Expect = "當前連接的網路: $ssidRe[\s\S]*是否斷開並重新連接"; Timeout = 20000 },
            @{ Send = "N"; Expect = "保持當前連線[\s\S]*繼續其他操作"; Timeout = 15000 },
            @{ Check = { $script:StaCheck["C05"] = Test-StaConnected; "" } },
            @{ Send = "N"; Expect = $GOODBYE_RE }
        ))
    }
    Add-StaResult "C05" $names.C05 $c "Detected '$REF_SSID', kept it, still connected after the tool exited" -Last -DependsOn "C04"

    # Session D: TC-STA-C06 wrong password is reported as a failure, not as success.
    # Its own scan re-records C01, so a vanished AP turns C06 INCONCLUSIVE instead of FAIL.
    Cleanup-Environment
    if (-not (Wait-RefApVisible)) {
        Add-StaResult "C06" $names.C06 $null "" -Blocked $offAir
        Cleanup-Environment
        return
    }
    Exec-Adb $freshSystemConf | Out-Null
    $d = Invoke-ToolSteps ($intro + $scanAndPick + @(
        @{ Send = "qwt-wrong-pass-0000"; Expect = "連接失敗，診斷資訊[\s\S]*是否斷開連線"; Fail = "連接成功!"; Timeout = 120000 },
        @{ Check = { $st = Get-StaState; $script:StaCheck["C06"] = if ($st.State -eq "COMPLETED") { "board reports COMPLETED with a wrong password" } else { "" }; "" } },
        @{ Send = "y"; Expect = "繼續其他操作"; Timeout = 30000 },
        @{ Send = "N"; Expect = $GOODBYE_RE }
    ))
    $reasons = ""
    if ($d.Transcript -match '可能原因：\s*\n((?:.*\n){1,4})') { $reasons = ($matches[1] -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ }) -join " / " }
    Add-StaResult "C06" $names.C06 $d "Failure reported; reasons: $reasons" -Last -DependsOn "C01"

    Cleanup-Environment
}

function Show-SummaryReport {
    param([string]$KernelVer)
    Write-Host ""
    Write-Host ("=" * 95) -ForegroundColor Cyan
    Write-Host "### QWiFiTestTool Comprehensive E2E Verification Report" -ForegroundColor Yellow
    Write-Host "- Time: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
    Write-Host "- Target Module: [$MODULE_IDX] $MODULE_LABEL"
    Write-Host "- Target Interface: $TEST_IFACE"
    Write-Host "- Target Kernel: $KernelVer"
    Write-Host ""
    Write-Host "| Category | TC ID | Test Case | Status | Verification Details |"
    Write-Host "|:---|:---|:---|:---:|:---|"
    foreach ($r in $GlobalResults) {
        Write-Host "| $($r.Category) | $($r.Id) | $($r.Name) | [$($r.Status)] | $($r.Details) |"
    }
    Write-Host ("=" * 95) -ForegroundColor Cyan

    $total = $GlobalResults.Count
    $passCount = @($GlobalResults | Where-Object { $_.Status -eq "PASS" }).Count
    $failCount = @($GlobalResults | Where-Object { $_.Status -eq "FAIL" }).Count
    $inconCount = @($GlobalResults | Where-Object { $_.Status -eq "INCONCLUSIVE" }).Count
    $rate = if ($total -gt 0) { [math]::Round(($passCount / $total) * 100, 1) } else { 0 }

    $color = if ($failCount -gt 0) { "Red" } elseif ($inconCount -gt 0) { "Yellow" } else { "Green" }
    Write-Host ""
    Write-Host ">> Summary: Total $total | Passed: $passCount | Failed: $failCount | Inconclusive: $inconCount | Pass Rate: $rate%" -ForegroundColor $color
    Write-Host ">> Artifacts: $OutDir"
    Write-Host ""

    return [PSCustomObject]@{ Total = $total; Passed = $passCount; Failed = $failCount; Inconclusive = $inconCount }
}

# ======================== Main Execution Flow ========================

$devCheck = Exec-Adb "uname -a"
if ([string]::IsNullOrWhiteSpace($devCheck)) {
    Write-Host "Error: Cannot connect to board via ADB!" -ForegroundColor Red
    exit 1
}

Write-Host "ADB Connected: $devCheck" -ForegroundColor Green
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

if (-not (Push-BoardScript "qwt_board_cleanup.sh" $CLEANUP_SH)) {
    Write-Host "Error: Failed to push qwt_board_cleanup.sh to board ($CLEANUP_SH)!" -ForegroundColor Red
    exit 1
}
if (-not (Push-BoardScript "qwt_canary.sh" $CANARY_SH)) {
    Write-Host "Error: Failed to push qwt_canary.sh to board ($CANARY_SH)!" -ForegroundColor Red
    exit 1
}

Exec-Adb "killall -9 QWifiTestTool 2>/dev/null" | Out-Null
if (-not (Resolve-ModuleIndex)) { exit 1 }
Write-Host "Test module: [$MODULE_IDX] $MODULE_LABEL (interface $TEST_IFACE)" -ForegroundColor Green

if (($StaConnect -or $StaConnectOnly) -and -not (Read-RefAp)) {
    Write-Host "Error: -StaConnect needs the reference AP file $RefApFile (UTF-8; line 1 = SSID, line 2 = password)" -ForegroundColor Red
    exit 1
}
if ($StaConnect -or $StaConnectOnly) {
    $pskNote = if ($REF_PSK) { "password from $RefApFile, masked in all output" } else { "open network" }
    Write-Host "Reference AP: $REF_SSID ($pskNote)" -ForegroundColor Green
}

$GEN_CONF_DIR = if ((Exec-Adb "getprop ro.build.version.sdk 2>/dev/null").Trim()) { "/data/local/tmp/wifi_tools" } else { (Exec-Adb "pwd").Trim().TrimEnd("/") }
if (-not (Backup-BoardConfigs)) {
    Write-Host "Error: Failed to back up board configs to $BACKUP_DIR!" -ForegroundColor Red
    exit 1
}

$runMatrix = (-not $RfOnly) -and (-not $RfAll) -and (-not $DiagOnly) -and (-not $StaConnectOnly)
$runRf = (-not $MatrixOnly) -and (-not $DiagOnly) -and (-not $StaConnectOnly)
# Phase 5 needs a reference AP, so it is opt-in as well
$runStaConnect = $StaConnect -or $StaConnectOnly
# Phase 4 unloads/reloads the driver and can leave the board in a degraded state, so it is opt-in only
$runDiag = [bool]$DiagOnly

# hostapd.conf expectations (exact lines), derived from the generator for this module's presets
$G6 = @("hw_mode=a", "channel=37"); $G5 = @("hw_mode=a", "channel=36"); $G24 = @("hw_mode=g", "channel=6")
$F6_BE = @("ieee80211be=1", "ieee80211ax=1"); $F6_AX = @("ieee80211ax=1")
$F5_BE = @("ieee80211be=1", "ieee80211ax=1", "ieee80211ac=1", "ieee80211n=1")
$F5_AX = @("ieee80211ax=1", "ieee80211ac=1", "ieee80211n=1")
$F24_BE = @("ieee80211be=1", "ieee80211ax=1", "ieee80211n=1"); $F24_AX = @("ieee80211ax=1", "ieee80211n=1")
$F_AC = @("ieee80211ac=1", "ieee80211n=1"); $F_N = @("ieee80211n=1")
$HT40 = @("ht_capab=[HT40+][SHORT-GI-20][SHORT-GI-40]"); $HT20 = @("ht_capab=[HT20][SHORT-GI-20]")

try {
    Cleanup-Environment
    Start-Canaries

    # ----------------- PHASE 1: AP Preset Matrix (All 29 Presets) -----------------
    if ($runMatrix -and (-not $StaOnly)) {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 1: AP Quick Preset Matrix Verification (29 Presets Full Coverage)" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta

        # --- 6 GHz (Band 3) ---
        Write-Host "`n[6 GHz Band Presets]" -ForegroundColor Cyan
        Test-ApPresetConfig "TC-AP-M01" "6GHz WiFi 7 320M" 3 1 ($G6 + $F6_BE + @("eht_oper_chwidth=3", "he_oper_chwidth=2", "he_oper_centr_freq_seg0_idx=47", "eht_oper_centr_freq_seg0_idx=31", "op_class=137"))
        Test-ApPresetConfig "TC-AP-M02" "6GHz WiFi 7 160M" 3 2 ($G6 + $F6_BE + @("eht_oper_chwidth=2", "he_oper_chwidth=2", "he_oper_centr_freq_seg0_idx=47", "eht_oper_centr_freq_seg0_idx=47", "op_class=134"))
        Test-ApPresetConfig "TC-AP-M03" "6GHz WiFi 7 80M"  3 3 ($G6 + $F6_BE + @("eht_oper_chwidth=1", "he_oper_chwidth=1", "he_oper_centr_freq_seg0_idx=39", "eht_oper_centr_freq_seg0_idx=39", "op_class=133"))
        Test-ApPresetConfig "TC-AP-M04" "6GHz WiFi 7 40M"  3 4 ($G6 + $F6_BE + @("eht_oper_chwidth=0", "he_oper_chwidth=0", "he_oper_centr_freq_seg0_idx=35", "eht_oper_centr_freq_seg0_idx=35", "op_class=132"))
        Test-ApPresetConfig "TC-AP-M05" "6GHz WiFi 7 20M"  3 5 ($G6 + $F6_BE + @("op_class=131"))
        Test-ApPresetConfig "TC-AP-M06" "6GHz WiFi 6 160M" 3 6 ($G6 + $F6_AX + @("he_oper_chwidth=2", "he_oper_centr_freq_seg0_idx=47", "op_class=134"))
        Test-ApPresetConfig "TC-AP-M07" "6GHz WiFi 6 80M"  3 7 ($G6 + $F6_AX + @("he_oper_chwidth=1", "he_oper_centr_freq_seg0_idx=39", "op_class=133"))
        Test-ApPresetConfig "TC-AP-M08" "6GHz WiFi 6 40M"  3 8 ($G6 + $F6_AX + @("he_oper_chwidth=0", "he_oper_centr_freq_seg0_idx=35", "op_class=132"))
        Test-ApPresetConfig "TC-AP-M09" "6GHz WiFi 6 20M"  3 9 ($G6 + $F6_AX + @("op_class=131"))

        # --- 5 GHz (Band 2) ---
        Write-Host "`n[5 GHz Band Presets]" -ForegroundColor Cyan
        Test-ApPresetConfig "TC-AP-M10" "5GHz WiFi 7 160M" 2 1  ($G5 + $F5_BE + $HT40 + @("vht_oper_chwidth=2", "he_oper_chwidth=2", "eht_oper_chwidth=2", "vht_oper_centr_freq_seg0_idx=50", "he_oper_centr_freq_seg0_idx=50", "eht_oper_centr_freq_seg0_idx=50"))
        Test-ApPresetConfig "TC-AP-M11" "5GHz WiFi 7 80M"  2 2  ($G5 + $F5_BE + $HT40 + @("vht_oper_chwidth=1", "he_oper_chwidth=1", "eht_oper_chwidth=1", "vht_oper_centr_freq_seg0_idx=42", "he_oper_centr_freq_seg0_idx=42", "eht_oper_centr_freq_seg0_idx=42"))
        Test-ApPresetConfig "TC-AP-M12" "5GHz WiFi 7 40M"  2 3  ($G5 + $F5_BE + $HT40)
        Test-ApPresetConfig "TC-AP-M13" "5GHz WiFi 7 20M"  2 4  ($G5 + $F5_BE + $HT20)
        Test-ApPresetConfig "TC-AP-M14" "5GHz WiFi 6 160M" 2 5  ($G5 + $F5_AX + $HT40 + @("vht_oper_chwidth=2", "he_oper_chwidth=2", "vht_oper_centr_freq_seg0_idx=50", "he_oper_centr_freq_seg0_idx=50"))
        Test-ApPresetConfig "TC-AP-M15" "5GHz WiFi 6 80M"  2 6  ($G5 + $F5_AX + $HT40 + @("vht_oper_chwidth=1", "he_oper_chwidth=1", "vht_oper_centr_freq_seg0_idx=42", "he_oper_centr_freq_seg0_idx=42"))
        Test-ApPresetConfig "TC-AP-M16" "5GHz WiFi 6 40M"  2 7  ($G5 + $F5_AX + $HT40)
        Test-ApPresetConfig "TC-AP-M17" "5GHz WiFi 6 20M"  2 8  ($G5 + $F5_AX + $HT20)
        Test-ApPresetConfig "TC-AP-M18" "5GHz WiFi 5 160M" 2 9  ($G5 + $F_AC + $HT40 + @("vht_oper_chwidth=2", "vht_oper_centr_freq_seg0_idx=50"))
        Test-ApPresetConfig "TC-AP-M19" "5GHz WiFi 5 80M"  2 10 ($G5 + $F_AC + $HT40 + @("vht_oper_chwidth=1", "vht_oper_centr_freq_seg0_idx=42"))
        Test-ApPresetConfig "TC-AP-M20" "5GHz WiFi 5 40M"  2 11 ($G5 + $F_AC + $HT40)
        Test-ApPresetConfig "TC-AP-M21" "5GHz WiFi 5 20M"  2 12 ($G5 + $F_AC + $HT20)
        Test-ApPresetConfig "TC-AP-M22" "5GHz WiFi 4 40M"  2 13 ($G5 + $F_N + $HT40)
        Test-ApPresetConfig "TC-AP-M23" "5GHz WiFi 4 20M"  2 14 ($G5 + $F_N + $HT20)

        # --- 2.4 GHz (Band 1) ---
        Write-Host "`n[2.4 GHz Band Presets]" -ForegroundColor Cyan
        Test-ApPresetConfig "TC-AP-M24" "2.4GHz WiFi 7 40M" 1 1 ($G24 + $F24_BE + $HT40)
        Test-ApPresetConfig "TC-AP-M25" "2.4GHz WiFi 7 20M" 1 2 ($G24 + $F24_BE + $HT20)
        Test-ApPresetConfig "TC-AP-M26" "2.4GHz WiFi 6 40M" 1 3 ($G24 + $F24_AX + $HT40)
        Test-ApPresetConfig "TC-AP-M27" "2.4GHz WiFi 6 20M" 1 4 ($G24 + $F24_AX + $HT20)
        Test-ApPresetConfig "TC-AP-M28" "2.4GHz WiFi 4 40M" 1 5 ($G24 + $F_N + $HT40)
        Test-ApPresetConfig "TC-AP-M29" "2.4GHz WiFi 4 20M" 1 6 ($G24 + $F_N + $HT20)

        # --- English UI ---
        Write-Host "`n[English UI]" -ForegroundColor Cyan
        Test-ApPresetConfig "TC-AP-EN01" "5GHz WiFi 6 80M (English UI)" 2 6 ($G5 + $F5_AX + $HT40 + @("vht_oper_chwidth=1", "he_oper_chwidth=1", "vht_oper_centr_freq_seg0_idx=42", "he_oper_centr_freq_seg0_idx=42")) -Lang "en"
        Update-CanaryStatus "Phase 1"
    }

    # ----------------- PHASE 2: STA Policy Matrix (11 Policies) -----------------
    if ($runMatrix -and (-not $ApOnly)) {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 2: STA Band Lock, Protocol Cap & Bandwidth Cap (11 Policies)" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta

        # --- Band Lock (Option 11) ---
        Write-Host "`n[STA Band Lock Policies]" -ForegroundColor Cyan
        # 6GHz Lock: [11] -> [2]
        Test-StaPolicyConfig "TC-STA-P01" "STA 6GHz Band Lock" @("11", "2") @("freq_list=5955") -ExpectedBand "6G"
        # 5GHz Lock (No DFS): [11] -> [3] -> Has 5180, No 5260 (DFS)
        Test-StaPolicyConfig "TC-STA-P02" "STA 5GHz Lock (Exclude DFS)" @("11", "3") @("5180") @("5260", "5280") -ExpectedBand "5G_NO_DFS"
        # 5GHz Lock (With DFS): [11] -> [4] -> Has 5260 (DFS)
        Test-StaPolicyConfig "TC-STA-P03" "STA 5GHz Lock (Include DFS)" @("11", "4") @("5180", "5260") -ExpectedBand "5G_DFS"
        # 2.4GHz Lock: [11] -> [5] -> Has 2412, 2437
        Test-StaPolicyConfig "TC-STA-P04" "STA 2.4GHz Band Lock" @("11", "5") @("2412", "2437") -ExpectedBand "2.4G"
        # Clear Band Lock: Set [11] -> [2] then [11] -> [1]
        Test-StaPolicyConfig "TC-STA-P05" "STA Clear Band Lock" @("11", "2", "11", "1") @() @("freq_list=") -ExpectedBand "ALL"

        # --- Protocol Cap (Option 12) ---
        Write-Host "`n[STA Protocol Cap Policies]" -ForegroundColor Cyan
        # Cap WiFi 6: [12] -> [2] -> disable_eht=1
        Test-StaPolicyConfig "TC-STA-P06" "STA Protocol Cap: Max WiFi 6" @("12", "2") @("disable_eht=1")
        # Cap WiFi 5: [12] -> [3] -> disable_eht=1, disable_he=1
        Test-StaPolicyConfig "TC-STA-P07" "STA Protocol Cap: Max WiFi 5" @("12", "3") @("disable_eht=1", "disable_he=1")
        # Cap WiFi 4: [12] -> [4] -> disable_eht=1, disable_he=1, disable_vht=1
        Test-StaPolicyConfig "TC-STA-P08" "STA Protocol Cap: Max WiFi 4" @("12", "4") @("disable_eht=1", "disable_he=1", "disable_vht=1")
        # Clear Protocol Cap: Set [12] -> [4] then [12] -> [1]
        Test-StaPolicyConfig "TC-STA-P09" "STA Clear Protocol Cap" @("12", "4", "12", "1") @() @("disable_eht=1", "disable_he=1", "disable_vht=1")

        # --- Bandwidth Cap (Option 13) ---
        Write-Host "`n[STA Bandwidth Cap Policies]" -ForegroundColor Cyan
        $supplicantSupportsHt40 = Exec-Adb 'W=$(which wpa_supplicant 2>/dev/null || which wpa_supplicant_s1g 2>/dev/null || echo /usr/sbin/wpa_supplicant); grep -a -q disable_ht40 $W 2>/dev/null && echo 1 || echo 0'
        $opt2Input = if ($supplicantSupportsHt40.Trim() -eq "1") { @("13", "2") } else { @("13", "2", "y") }
        # Cap 20MHz: [13] -> [2] (-> y if unsupported prompt) -> disable_ht40=1
        Test-StaPolicyConfig "TC-STA-P10" "STA Bandwidth Cap: Force 20MHz" $opt2Input @("disable_ht40=1")
        # Clear Bandwidth Cap: Set [13] -> [2] (-> y) then [13] -> [1]
        Test-StaPolicyConfig "TC-STA-P11" "STA Clear Bandwidth Cap" ($opt2Input + @("13", "1")) @() @("disable_ht40=1")

        # --- English UI (config only, no RF scan) ---
        Write-Host "`n[English UI]" -ForegroundColor Cyan
        Test-StaPolicyConfig "TC-STA-EN01" "STA 2.4GHz Band Lock (English UI)" @("11", "5") @("2412", "2437") -Lang "en"
        Update-CanaryStatus "Phase 2"
    }

    # ----------------- PHASE 3: Hardware RF Closed-Loop Verification -----------------
    if ($runRf -and (-not $StaOnly)) {
        if ($RfAll) {
            Write-Host ""
            Write-Host "==========================================================================" -ForegroundColor Magenta
            Write-Host ">> Phase 3 (Full 29 RF): All 29 AP Presets Live RF Verification (-RfAll)" -ForegroundColor Magenta
            Write-Host "==========================================================================" -ForegroundColor Magenta

            # --- 6 GHz (Band 3: 9 presets) ---
            Write-Host "`n[6 GHz Live RF Broadcasts]" -ForegroundColor Cyan
            Run-ApRfTestCase -TcId "TC-RF-01" -Name "6GHz WiFi 7 320M RF TX" -BandKey 3 -PresetKey 1 -ExpectedTuiMode "320MHz" -ExpectedTuiCh "Center: 31" -ExpectedRfWidth "320 MHz"
            Run-ApRfTestCase -TcId "TC-RF-02" -Name "6GHz WiFi 7 160M RF TX" -BandKey 3 -PresetKey 2 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"
            Run-ApRfTestCase -TcId "TC-RF-03" -Name "6GHz WiFi 7 80M RF TX"  -BandKey 3 -PresetKey 3 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 39" -ExpectedRfWidth "80 MHz"
            Run-ApRfTestCase -TcId "TC-RF-04" -Name "6GHz WiFi 7 40M RF TX"  -BandKey 3 -PresetKey 4 -ExpectedTuiMode "40MHz"  -ExpectedTuiCh "Center: 35" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-05" -Name "6GHz WiFi 7 20M RF TX"  -BandKey 3 -PresetKey 5 -ExpectedTuiMode "WiFi 7"  -ExpectedTuiCh "channel 37" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-06" -Name "6GHz WiFi 6 160M RF TX" -BandKey 3 -PresetKey 6 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"
            Run-ApRfTestCase -TcId "TC-RF-07" -Name "6GHz WiFi 6 80M RF TX"  -BandKey 3 -PresetKey 7 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 39" -ExpectedRfWidth "80 MHz"
            Run-ApRfTestCase -TcId "TC-RF-08" -Name "6GHz WiFi 6 40M RF TX"  -BandKey 3 -PresetKey 8 -ExpectedTuiMode "40MHz"  -ExpectedTuiCh "Center: 35" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-09" -Name "6GHz WiFi 6 20M RF TX"  -BandKey 3 -PresetKey 9 -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 37" -ExpectedRfWidth "20 MHz"

            # --- 5 GHz (Band 2: 14 presets) ---
            Write-Host "`n[5 GHz Live RF Broadcasts]" -ForegroundColor Cyan
            Run-ApRfTestCase -TcId "TC-RF-10" -Name "5GHz WiFi 7 160M RF TX" -BandKey 2 -PresetKey 1  -ExpectedTuiMode "160MHz"  -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"
            Run-ApRfTestCase -TcId "TC-RF-11" -Name "5GHz WiFi 7 80M RF TX"  -BandKey 2 -PresetKey 2  -ExpectedTuiMode "80MHz"   -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
            Run-ApRfTestCase -TcId "TC-RF-12" -Name "5GHz WiFi 7 40M RF TX"  -BandKey 2 -PresetKey 3  -ExpectedTuiMode "WiFi 7"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-13" -Name "5GHz WiFi 7 20M RF TX"  -BandKey 2 -PresetKey 4  -ExpectedTuiMode "WiFi 7"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-14" -Name "5GHz WiFi 6 160M RF TX" -BandKey 2 -PresetKey 5  -ExpectedTuiMode "160MHz"  -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"
            Run-ApRfTestCase -TcId "TC-RF-15" -Name "5GHz WiFi 6 80M RF TX"  -BandKey 2 -PresetKey 6  -ExpectedTuiMode "80MHz"   -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
            Run-ApRfTestCase -TcId "TC-RF-16" -Name "5GHz WiFi 6 40M RF TX"  -BandKey 2 -PresetKey 7  -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-17" -Name "5GHz WiFi 6 20M RF TX"  -BandKey 2 -PresetKey 8  -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-18" -Name "5GHz WiFi 5 160M RF TX" -BandKey 2 -PresetKey 9  -ExpectedTuiMode "160MHz"  -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"
            Run-ApRfTestCase -TcId "TC-RF-19" -Name "5GHz WiFi 5 80M RF TX"  -BandKey 2 -PresetKey 10 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
            Run-ApRfTestCase -TcId "TC-RF-20" -Name "5GHz WiFi 5 40M RF TX"  -BandKey 2 -PresetKey 11 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-21" -Name "5GHz WiFi 5 20M RF TX"  -BandKey 2 -PresetKey 12 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-22" -Name "5GHz WiFi 4 40M RF TX"  -BandKey 2 -PresetKey 13 -ExpectedTuiMode "WiFi 4"   -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-23" -Name "5GHz WiFi 4 20M RF TX"  -BandKey 2 -PresetKey 14 -ExpectedTuiMode "WiFi 4"   -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"

            # --- 2.4 GHz (Band 1: 6 presets) ---
            Write-Host "`n[2.4 GHz Live RF Broadcasts]" -ForegroundColor Cyan
            Run-ApRfTestCase -TcId "TC-RF-24" -Name "2.4GHz WiFi 7 40M RF TX" -BandKey 1 -PresetKey 1 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-25" -Name "2.4GHz WiFi 7 20M RF TX" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-26" -Name "2.4GHz WiFi 6 40M RF TX" -BandKey 1 -PresetKey 3 -ExpectedTuiMode "WiFi 6" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-27" -Name "2.4GHz WiFi 6 20M RF TX" -BandKey 1 -PresetKey 4 -ExpectedTuiMode "WiFi 6" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
            Run-ApRfTestCase -TcId "TC-RF-28" -Name "2.4GHz WiFi 4 40M RF TX" -BandKey 1 -PresetKey 5 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
            Run-ApRfTestCase -TcId "TC-RF-29" -Name "2.4GHz WiFi 4 20M RF TX" -BandKey 1 -PresetKey 6 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
        } else {
            Write-Host ""
            Write-Host "==========================================================================" -ForegroundColor Magenta
            Write-Host ">> Phase 3: Hardware RF Closed-Loop Physical Verification (Core 6 Broadcasts)" -ForegroundColor Magenta
            Write-Host "==========================================================================" -ForegroundColor Magenta

            # TC-RF-01: 6GHz WiFi 7 320M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-01" -Name "6GHz WiFi 7 320M RF TX" -BandKey 3 -PresetKey 1 -ExpectedTuiMode "320MHz" -ExpectedTuiCh "Center: 31" -ExpectedRfWidth "320 MHz"

            # TC-RF-06: 6GHz WiFi 6 160M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-06" -Name "6GHz WiFi 6 160M RF TX" -BandKey 3 -PresetKey 6 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"

            # TC-RF-10: 5GHz WiFi 7 160M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-10" -Name "5GHz WiFi 7 160M RF TX" -BandKey 2 -PresetKey 1 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"

            # TC-RF-19: 5GHz WiFi 5 80M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-19" -Name "5GHz WiFi 5 80M RF TX" -BandKey 2 -PresetKey 10 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"

            # TC-RF-25: 2.4GHz WiFi 7 20M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-25" -Name "2.4GHz WiFi 7 20M RF TX" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"

            # TC-RF-29: 2.4GHz WiFi 4 20M TX (Core)
            Run-ApRfTestCase -TcId "TC-RF-29" -Name "2.4GHz WiFi 4 20M RF TX" -BandKey 1 -PresetKey 6 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
        }

        # English UI through the whole AP start path (TUI wording differs, so only RF state + no Chinese)
        Run-ApRfTestCase -TcId "TC-RF-EN01" -Name "2.4GHz WiFi 7 20M RF TX (English UI)" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "" -ExpectedTuiCh "" -ExpectedRfWidth "20 MHz" -Lang "en"
        Update-CanaryStatus "Phase 3"
    }

    # ----------------- PHASE 5: Station Connection (reference AP) -----------------
    if ($runStaConnect) {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 5: Station Connection to Reference AP '$REF_SSID'" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Invoke-StaConnectionPhase
        Update-CanaryStatus "Phase 5"
    }

    # ----------------- PHASE 4: Diagnosis & Driver Lifecycle -----------------
    if ($runDiag) {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 4: Diagnosis Menu & Driver Unload / Load / Reload" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Invoke-DiagnosisPhase
        Update-CanaryStatus "Phase 4"
    }

    Add-CanaryResult
} finally {
    # Runs on normal completion and on Ctrl+C, so the board always gets its original configs back
    Exec-Adb "killall -9 QWifiTestTool 2>/dev/null" | Out-Null
    Exec-Adb "sh $CANARY_SH stop" | Out-Null
    Cleanup-Environment
    Restore-BoardConfigs
    if ($runDiag -and $DRIVER_MOD -and -not (Get-DriverState).Loaded) {
        Write-Host "[WARN] Driver '$DRIVER_MOD' is not loaded after Phase 4. Load it from the tool: [1] Diagnosis -> answer y." -ForegroundColor Yellow
    }
}

$summary = Show-SummaryReport (Exec-Adb "uname -r")

$report = [PSCustomObject]@{
    Time = (Get-Date -Format "yyyy-MM-dd HH:mm:ss")
    Module = "[$MODULE_IDX] $MODULE_LABEL"
    Interface = $TEST_IFACE
    Summary = $summary
    Results = $GlobalResults
}
$report | ConvertTo-Json -Depth 4 | Out-File -Encoding utf8 (Join-Path $OutDir "results.json")

# Exit code: 0 = no FAIL (INCONCLUSIVE allowed), 1 = at least one FAIL or a setup error
if ($summary.Failed -gt 0) { exit 1 }
exit 0
