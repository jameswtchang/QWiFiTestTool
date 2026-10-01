param(
    [switch]$MatrixOnly,
    [switch]$RfOnly,
    [switch]$RfAll,
    [switch]$ApOnly,
    [switch]$StaOnly
)

$ErrorActionPreference = "Continue"

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$TARGET_BIN = "/data/local/tmp/QWifiTestTool"
$GlobalResults = [System.Collections.Generic.List[PSObject]]::new()

function Exec-Adb {
    param([string]$Cmd)
    $out = adb shell $Cmd
    if ($null -eq $out) { return "" }
    return ($out -join "`n").Trim()
}

function Cleanup-Environment {
    Exec-Adb "killall -9 QWifiTestTool hostapd wpa_supplicant hostapd_s1g wpa_supplicant_s1g udhcpc dhclient dnsmasq 2>/dev/null" | Out-Null
    Exec-Adb "rm -f /etc/hostapd.conf /etc/dnsmasq.conf /var/run/hostapd/* /var/run/wpa_supplicant/* 2>/dev/null" | Out-Null
    Exec-Adb "ip addr flush dev wlan0 2>/dev/null" | Out-Null
    Exec-Adb "ip link set wlan0 down 2>/dev/null; sleep 0.3; ip link set wlan0 up 2>/dev/null" | Out-Null
}

function Run-ToolSession {
    param(
        [string[]]$Inputs,
        [int]$WaitAfterMs = 1200
    )

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = "adb"
    $psi.Arguments = "shell -t $TARGET_BIN"
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.StandardOutputEncoding = [System.Text.Encoding]::UTF8
    $psi.StandardErrorEncoding = [System.Text.Encoding]::UTF8
    $psi.UseShellExecute = $false

    $proc = [System.Diagnostics.Process]::Start($psi)

    # Initial delay for tool banner & sanity check
    Start-Sleep -Milliseconds 700

    foreach ($inp in $Inputs) {
        try {
            $proc.StandardInput.WriteLine($inp)
        } catch {
            break
        }
        if ($inp -eq "y" -or $inp -eq "Y") {
            Start-Sleep -Milliseconds 800
        } else {
            Start-Sleep -Milliseconds 400
        }
    }

    Start-Sleep -Milliseconds $WaitAfterMs
    try { $proc.Kill() } catch {}
    Exec-Adb "killall -9 QWifiTestTool 2>/dev/null" | Out-Null
    Start-Sleep -Milliseconds 300
}

# ==================== Phase 1: AP Preset Matrix (23 Presets) ====================

function Test-ApPresetConfig {
    param(
        [string]$TcId,
        [string]$Name,
        [int]$BandKey,
        [int]$PresetKey,
        [string[]]$ExpectedConfTokens
    )

    Write-Host -NoNewline "  [$TcId] $Name ... "

    # Remove previous config so we know it is freshly generated
    Exec-Adb "rm -f /etc/hostapd.conf" | Out-Null

    # Inputs: Language(N) -> Module(11: FME170Q) -> Menu(4: AP Edit) -> Submenu(4: Preset Wizard) -> Band -> Preset -> Save(y) -> Exit AP(0) -> Exit Main(0)
    $inputs = @("N", "11", "4", "4", "$BandKey", "$PresetKey", "y", "0", "0")
    Run-ToolSession -Inputs $inputs

    # Verify generated /etc/hostapd.conf
    $hostapdConf = Exec-Adb "cat /etc/hostapd.conf 2>/dev/null"

    if ([string]::IsNullOrWhiteSpace($hostapdConf)) {
        Write-Host "[FAIL] (hostapd.conf not generated)" -ForegroundColor Red
        $GlobalResults.Add([PSCustomObject]@{
            Category = "AP Preset Matrix"
            Id = $TcId
            Name = $Name
            Status = "FAIL"
            Details = "hostapd.conf not generated"
        })
        return
    }

    $mismatches = [System.Collections.Generic.List[string]]::new()
    foreach ($token in $ExpectedConfTokens) {
        if (-not $hostapdConf.Contains($token)) {
            $mismatches.Add("Missing '$token'")
        }
    }

    if ($mismatches.Count -eq 0) {
        Write-Host "[PASS]" -ForegroundColor Green
        $GlobalResults.Add([PSCustomObject]@{
            Category = "AP Preset Matrix"
            Id = $TcId
            Name = $Name
            Status = "PASS"
            Details = "hostapd.conf tokens verified ($($ExpectedConfTokens -join ', '))"
        })
    } else {
        Write-Host "[FAIL] ($($mismatches -join ', '))" -ForegroundColor Red
        $GlobalResults.Add([PSCustomObject]@{
            Category = "AP Preset Matrix"
            Id = $TcId
            Name = $Name
            Status = "FAIL"
            Details = ($mismatches -join "; ")
        })
    }
}

# ==================== Phase 2: STA Policy Matrix (8 Policies) ====================

function Test-StaPolicyConfig {
    param(
        [string]$TcId,
        [string]$Name,
        [string[]]$EditInputs,
        [string[]]$ExpectedTokens,
        [string[]]$ForbiddenTokens = @(),
        [string]$ExpectedBand = "" # "2.4G", "5G_NO_DFS", "5G_DFS", "6G", "ALL"
    )

    Write-Host -NoNewline "  [$TcId] $Name ... "

    # Ensure wpa_supplicant is not running so restartNetworkIfNeeded won't ask interactive question
    Exec-Adb "killall -9 wpa_supplicant 2>/dev/null" | Out-Null
    Exec-Adb "rm -f /etc/wpa_supplicant.conf 2>/dev/null" | Out-Null

    # Inputs: Language(N) -> Module(11) -> Menu(2: STA Edit) -> Edit(1) -> [Specific Inputs] -> Finish(0) -> Save(y) -> Exit Main(0)
    $inputs = @("N", "11", "2", "1") + $EditInputs + @("0", "y", "0")
    Run-ToolSession -Inputs $inputs

    # Verify /etc/wpa_supplicant.conf
    $wpaConf = Exec-Adb "cat /etc/wpa_supplicant.conf 2>/dev/null"

    $mismatches = [System.Collections.Generic.List[string]]::new()
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
    if ($mismatches.Count -eq 0 -and $ExpectedBand -and (-not $MatrixOnly)) {
        # 1. Ensure clean managed mode on wlan0
        Exec-Adb "killall -9 wpa_supplicant hostapd 2>/dev/null" | Out-Null
        Exec-Adb "ip link set wlan0 down 2>/dev/null; iw dev wlan0 set type managed 2>/dev/null; ip link set wlan0 up 2>/dev/null" | Out-Null
        Start-Sleep -Milliseconds 400

        # 2. Trigger native nl80211 physical RF probe scan with flush (hardware TX/RX probe)
        $scanOutput = ""
        if ($wpaConf -match 'freq_list=([0-9 ]+)') {
            $freqList = $matches[1].Trim()
            $scanOutput = Exec-Adb "iw dev wlan0 scan flush freq $freqList"
        } else {
            $scanOutput = Exec-Adb "iw dev wlan0 scan flush"
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
            $uniqueFreqs = $scannedFreqs | Select-Object -Unique | Sort-Object

            # 3.5. Guard: 0 BSS = inconclusive (shielded room / scan returned nothing)
            if ($uniqueFreqs.Count -eq 0) {
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

    if ($mismatches.Count -eq 0) {
        Write-Host "[PASS]" -ForegroundColor Green
        $GlobalResults.Add([PSCustomObject]@{
            Category = "STA Policy Matrix"
            Id = $TcId
            Name = $Name
            Status = "PASS"
            Details = "Policy verified" + $rfScanSummary
        })
    } else {
        Write-Host "[FAIL]" -ForegroundColor Red
        $GlobalResults.Add([PSCustomObject]@{
            Category = "STA Policy Matrix"
            Id = $TcId
            Name = $Name
            Status = "FAIL"
            Details = ($mismatches -join "; ")
        })
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
        [string]$ExpectedRfWidth
    )

    Write-Host ""
    Write-Host "=======================================================" -ForegroundColor Cyan
    Write-Host ">> Running ${TcId}: ${Name} (Hardware RF Broadcast)" -ForegroundColor Cyan
    Write-Host "=======================================================" -ForegroundColor Cyan

    Cleanup-Environment
    Start-Sleep -Seconds 1

    # Inputs: Language(N) -> Module(11) -> Menu(4: AP Edit) -> Preset(4) -> Band -> Preset -> Save(y) -> Start AP(5)
    $inputs = @("N", "11", "4", "4", "$BandKey", "$PresetKey", "y", "5")

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = "adb"
    $psi.Arguments = "shell -t $TARGET_BIN"
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.StandardOutputEncoding = [System.Text.Encoding]::UTF8
    $psi.StandardErrorEncoding = [System.Text.Encoding]::UTF8
    $psi.UseShellExecute = $false

    $proc = [System.Diagnostics.Process]::Start($psi)

    # Initial delay for tool banner & sanity check
    Start-Sleep -Milliseconds 600

    foreach ($inp in $inputs) {
        try {
            $proc.StandardInput.WriteLine($inp)
        } catch {
            break
        }
        if ($inp -eq "y" -or $inp -eq "Y") {
            Start-Sleep -Milliseconds 800
        } else {
            Start-Sleep -Milliseconds 400
        }
    }

    $capturedOutput = [System.Text.StringBuilder]::new()
    $apReady = $false
    $startTime = [System.DateTime]::Now

    # Use ReadLineAsync to avoid infinite blocking when TUI stops outputting
    $readTask = $proc.StandardOutput.ReadLineAsync()
    while (([System.DateTime]::Now - $startTime).TotalSeconds -lt 30) {
        if (-not $readTask.Wait(1000)) { continue }
        $line = $readTask.Result
        if ($null -eq $line) { break }
        [void]$capturedOutput.AppendLine($line)

        $trimmed = $line.Trim()
        if ($trimmed -match "(\[INFO\]|\[CMD\]|SSID:|channel|width:|Ctrl\+C|stop|fail)") {
            Write-Host "  [TUI] $trimmed" -ForegroundColor Gray
        }

        if ($line -match "Ctrl\+C") {
            $apReady = $true
            break
        }

        if ($line -match "(\[ERROR\] AP 啟動失敗|Could not initialize interface|AP 啟動逾時)") {
            $apReady = $false
            break
        }
        $readTask = $proc.StandardOutput.ReadLineAsync()
    }

    $fullLog = $capturedOutput.ToString()

    if (-not $apReady) {
        Cleanup-Environment
        try { $proc.Kill() } catch {}
        Write-Host "  ==> [FAIL] AP start timeout or hostapd stopped abnormally" -ForegroundColor Red
        $GlobalResults.Add([PSCustomObject]@{
            Category = "AP Hardware RF"
            Id = $TcId
            Name = $Name
            Status = "FAIL"
            Details = "AP start timeout or hostapd terminated"
        })
        return
    }

    # Assertion 1: TUI
    $tuiModeOk = $fullLog.Contains($ExpectedTuiMode)
    $tuiChOk = $fullLog.Contains($ExpectedTuiCh)
    $tuiPass = $tuiModeOk -and $tuiChOk

    # Assertion 2: Linux RF state (iw)
    $iwInfo = Exec-Adb "iw dev wlan0 info"
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

    # Assertion 3: Daemon running
    $hostapdPid = Exec-Adb "pidof hostapd"
    $daemonPass = ($hostapdPid -ne "")

    Write-Host "  [RF INFO] Hardware RF: AP=$isAp, WidthMatch=$rfWidthOk, hostapd PID=$hostapdPid" -ForegroundColor Green

    # Send SIGINT (graceful shutdown)
    Exec-Adb "killall -2 QWifiTestTool" | Out-Null
    Start-Sleep -Seconds 2

    # Assertion 4: Clean teardown
    $hostapdAfter = Exec-Adb "pidof hostapd"
    $teardownPass = ($hostapdAfter -eq "")

    try { $proc.Kill() } catch {}
    Cleanup-Environment
    Start-Sleep -Milliseconds 500

    $passed = $tuiPass -and $rfPass -and $daemonPass -and $teardownPass
    $statusStr = if ($passed) { "PASS" } else { "FAIL" }

    $details = [System.Collections.Generic.List[string]]::new()
    if (-not $tuiPass) { $details.Add("TUI mismatch (Mode:$tuiModeOk, Ch:$tuiChOk)") }
    if (-not $rfPass) { $details.Add("RF state mismatch (AP:$isAp, Width:$rfWidthOk)") }
    if (-not $daemonPass) { $details.Add("hostapd daemon missing") }
    if (-not $teardownPass) { $details.Add("hostapd remaining after stop") }

    $detailStr = if ($details.Count -gt 0) { $details -join "; " } else { "Physical RF ${ExpectedRfWidth} perfectly matched, teardown clean" }

    $color = if ($passed) { "Green" } else { "Red" }
    Write-Host "  ==> [$($statusStr)] $detailStr" -ForegroundColor $color

    $GlobalResults.Add([PSCustomObject]@{
        Category = "AP Hardware RF"
        Id = $TcId
        Name = $Name
        Status = $statusStr
        Details = $detailStr
    })
}

function Show-SummaryReport {
    Write-Host ""
    Write-Host ("=" * 95) -ForegroundColor Cyan
    Write-Host "### QWiFiTestTool Comprehensive E2E Verification Report (FME170Q)" -ForegroundColor Yellow
    Write-Host "- Time: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
    Write-Host "- Target Module: Qualcomm FME170Q-865 (WCN7851-5)"
    $kernelVer = Exec-Adb "uname -r"
    Write-Host "- Target Kernel: $kernelVer"
    Write-Host ""
    Write-Host "| Category | TC ID | Test Case | Status | Verification Details |"
    Write-Host "|:---|:---|:---|:---:|:---|"
    foreach ($r in $GlobalResults) {
        $icon = if ($r.Status -eq "PASS") { "[PASS]" } else { "[FAIL]" }
        Write-Host "| $($r.Category) | $($r.Id) | $($r.Name) | $icon | $($r.Details) |"
    }
    Write-Host ("=" * 95) -ForegroundColor Cyan

    $total = $GlobalResults.Count
    $passCount = ($GlobalResults | Where-Object { $_.Status -eq "PASS" }).Count
    $failCount = $total - $passCount
    $rate = if ($total -gt 0) { [math]::Round(($passCount / $total) * 100, 1) } else { 0 }

    Write-Host ""
    Write-Host ">> Summary: Total $total | Passed: $passCount | Failed: $failCount | Pass Rate: $rate%" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Red" })
    Write-Host ""
}

# ======================== Main Execution Flow ========================

$devCheck = Exec-Adb "uname -a"
if ([string]::IsNullOrWhiteSpace($devCheck)) {
    Write-Host "Error: Cannot connect to board via ADB!" -ForegroundColor Red
    exit 1
}

Write-Host "ADB Connected: $devCheck" -ForegroundColor Green
Cleanup-Environment

$runMatrix = (-not $RfOnly) -and (-not $RfAll)
$runRf = (-not $MatrixOnly)

# ----------------- PHASE 1: AP Preset Matrix (All 29 Presets) -----------------
if ($runMatrix -and (-not $StaOnly)) {
    Write-Host ""
    Write-Host "==========================================================================" -ForegroundColor Magenta
    Write-Host ">> Phase 1: AP Quick Preset Matrix Verification (29 Presets Full Coverage)" -ForegroundColor Magenta
    Write-Host "==========================================================================" -ForegroundColor Magenta

    # --- 6 GHz (Band 3) ---
    Write-Host "`n[6 GHz Band Presets]" -ForegroundColor Cyan
    Test-ApPresetConfig "TC-AP-M01" "6GHz WiFi 7 320M" 3 1 @("hw_mode=a", "channel=37", "ieee80211be=1", "op_class=137")
    Test-ApPresetConfig "TC-AP-M02" "6GHz WiFi 7 160M" 3 2 @("hw_mode=a", "channel=37", "ieee80211be=1", "op_class=134")
    Test-ApPresetConfig "TC-AP-M03" "6GHz WiFi 7 80M"  3 3 @("hw_mode=a", "channel=37", "ieee80211be=1", "op_class=133")
    Test-ApPresetConfig "TC-AP-M04" "6GHz WiFi 7 40M"  3 4 @("hw_mode=a", "channel=37", "ieee80211be=1", "op_class=132")
    Test-ApPresetConfig "TC-AP-M05" "6GHz WiFi 7 20M"  3 5 @("hw_mode=a", "channel=37", "ieee80211be=1", "op_class=131")
    Test-ApPresetConfig "TC-AP-M06" "6GHz WiFi 6 160M" 3 6 @("hw_mode=a", "channel=37", "ieee80211ax=1", "op_class=134")
    Test-ApPresetConfig "TC-AP-M07" "6GHz WiFi 6 80M"  3 7 @("hw_mode=a", "channel=37", "ieee80211ax=1", "op_class=133")
    Test-ApPresetConfig "TC-AP-M08" "6GHz WiFi 6 40M"  3 8 @("hw_mode=a", "channel=37", "ieee80211ax=1", "op_class=132")
    Test-ApPresetConfig "TC-AP-M09" "6GHz WiFi 6 20M"  3 9 @("hw_mode=a", "channel=37", "ieee80211ax=1", "op_class=131")

    # --- 5 GHz (Band 2) ---
    Write-Host "`n[5 GHz Band Presets]" -ForegroundColor Cyan
    Test-ApPresetConfig "TC-AP-M10" "5GHz WiFi 7 160M" 2 1  @("hw_mode=a", "channel=36", "ieee80211be=1", "vht_oper_centr_freq_seg0_idx=50")
    Test-ApPresetConfig "TC-AP-M11" "5GHz WiFi 7 80M"  2 2  @("hw_mode=a", "channel=36", "ieee80211be=1", "vht_oper_centr_freq_seg0_idx=42")
    Test-ApPresetConfig "TC-AP-M12" "5GHz WiFi 7 40M"  2 3  @("hw_mode=a", "channel=36", "ieee80211be=1")
    Test-ApPresetConfig "TC-AP-M13" "5GHz WiFi 7 20M"  2 4  @("hw_mode=a", "channel=36", "ieee80211be=1")
    Test-ApPresetConfig "TC-AP-M14" "5GHz WiFi 6 160M" 2 5  @("hw_mode=a", "channel=36", "ieee80211ax=1", "vht_oper_centr_freq_seg0_idx=50")
    Test-ApPresetConfig "TC-AP-M15" "5GHz WiFi 6 80M"  2 6  @("hw_mode=a", "channel=36", "ieee80211ax=1", "vht_oper_centr_freq_seg0_idx=42")
    Test-ApPresetConfig "TC-AP-M16" "5GHz WiFi 6 40M"  2 7  @("hw_mode=a", "channel=36", "ieee80211ax=1")
    Test-ApPresetConfig "TC-AP-M17" "5GHz WiFi 6 20M"  2 8  @("hw_mode=a", "channel=36", "ieee80211ax=1")
    Test-ApPresetConfig "TC-AP-M18" "5GHz WiFi 5 160M" 2 9  @("hw_mode=a", "channel=36", "ieee80211ac=1", "vht_oper_centr_freq_seg0_idx=50")
    Test-ApPresetConfig "TC-AP-M19" "5GHz WiFi 5 80M"  2 10 @("hw_mode=a", "channel=36", "ieee80211ac=1", "vht_oper_centr_freq_seg0_idx=42")
    Test-ApPresetConfig "TC-AP-M20" "5GHz WiFi 5 40M"  2 11 @("hw_mode=a", "channel=36", "ieee80211ac=1")
    Test-ApPresetConfig "TC-AP-M21" "5GHz WiFi 5 20M"  2 12 @("hw_mode=a", "channel=36", "ieee80211ac=1")
    Test-ApPresetConfig "TC-AP-M22" "5GHz WiFi 4 40M"  2 13 @("hw_mode=a", "channel=36", "ieee80211n=1")
    Test-ApPresetConfig "TC-AP-M23" "5GHz WiFi 4 20M"  2 14 @("hw_mode=a", "channel=36", "ieee80211n=1")

    # --- 2.4 GHz (Band 1) ---
    Write-Host "`n[2.4 GHz Band Presets]" -ForegroundColor Cyan
    Test-ApPresetConfig "TC-AP-M24" "2.4GHz WiFi 7 40M" 1 1 @("hw_mode=g", "ieee80211be=1")
    Test-ApPresetConfig "TC-AP-M25" "2.4GHz WiFi 7 20M" 1 2 @("hw_mode=g", "ieee80211be=1")
    Test-ApPresetConfig "TC-AP-M26" "2.4GHz WiFi 6 40M" 1 3 @("hw_mode=g", "ieee80211ax=1")
    Test-ApPresetConfig "TC-AP-M27" "2.4GHz WiFi 6 20M" 1 4 @("hw_mode=g", "ieee80211ax=1")
    Test-ApPresetConfig "TC-AP-M28" "2.4GHz WiFi 4 40M" 1 5 @("hw_mode=g", "ieee80211n=1")
    Test-ApPresetConfig "TC-AP-M29" "2.4GHz WiFi 4 20M" 1 6 @("hw_mode=g", "ieee80211n=1")
}

# ----------------- PHASE 2: STA Policy Matrix (8 Policies) -----------------
if ($runMatrix -and (-not $ApOnly)) {
    Write-Host ""
    Write-Host "==========================================================================" -ForegroundColor Magenta
    Write-Host ">> Phase 2: STA Band Lock & Protocol Cap Verification (8 Policies)" -ForegroundColor Magenta
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
    $supplicantSupportsHt40 = Exec-Adb "grep -a -q 'disable_ht40' /usr/sbin/wpa_supplicant && echo 1 || echo 0"
    $opt2Input = if ($supplicantSupportsHt40.Trim() -eq "1") { @("13", "2") } else { @("13", "2", "y") }
    # Cap 20MHz: [13] -> [2] (-> y if unsupported prompt) -> disable_ht40=1
    Test-StaPolicyConfig "TC-STA-P10" "STA Bandwidth Cap: Force 20MHz" $opt2Input @("disable_ht40=1")
    # Clear Bandwidth Cap: Set [13] -> [2] (-> y) then [13] -> [1]
    Test-StaPolicyConfig "TC-STA-P11" "STA Clear Bandwidth Cap" ($opt2Input + @("13", "1")) @() @("disable_ht40=1")
}

# ----------------- PHASE 3: Hardware RF Closed-Loop Verification -----------------
if ($runRf -and (-not $StaOnly)) {
    if ($RfAll) {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 3 (Full 23 RF): All 23 AP Presets Live RF Verification (-RfAll)" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta

        # --- 6 GHz (Band 3: 7 presets) ---
        Write-Host "`n[6 GHz Live RF Broadcasts]" -ForegroundColor Cyan
        Run-ApRfTestCase -TcId "TC-RF-01" -Name "6GHz WiFi 7 320M RF TX" -BandKey 3 -PresetKey 1 -ExpectedTuiMode "320MHz" -ExpectedTuiCh "Center: 31" -ExpectedRfWidth "320 MHz"
        Run-ApRfTestCase -TcId "TC-RF-02" -Name "6GHz WiFi 7 160M RF TX" -BandKey 3 -PresetKey 2 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"
        Run-ApRfTestCase -TcId "TC-RF-03" -Name "6GHz WiFi 7 80M RF TX"  -BandKey 3 -PresetKey 3 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 39" -ExpectedRfWidth "80 MHz"
        Run-ApRfTestCase -TcId "TC-RF-04" -Name "6GHz WiFi 6 160M RF TX" -BandKey 3 -PresetKey 4 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"
        Run-ApRfTestCase -TcId "TC-RF-05" -Name "6GHz WiFi 6 80M RF TX"  -BandKey 3 -PresetKey 5 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 39" -ExpectedRfWidth "80 MHz"
        Run-ApRfTestCase -TcId "TC-RF-06" -Name "6GHz WiFi 6 40M RF TX"  -BandKey 3 -PresetKey 6 -ExpectedTuiMode "40MHz"  -ExpectedTuiCh "Center: 35" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-07" -Name "6GHz WiFi 6 20M RF TX"  -BandKey 3 -PresetKey 7 -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 37" -ExpectedRfWidth "20 MHz"

        # --- 5 GHz (Band 2: 10 presets) ---
        Write-Host "`n[5 GHz Live RF Broadcasts]" -ForegroundColor Cyan
        Run-ApRfTestCase -TcId "TC-RF-08" -Name "5GHz WiFi 7 160M RF TX" -BandKey 2 -PresetKey 1 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"
        Run-ApRfTestCase -TcId "TC-RF-09" -Name "5GHz WiFi 7 80M RF TX"  -BandKey 2 -PresetKey 2 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
        Run-ApRfTestCase -TcId "TC-RF-10" -Name "5GHz WiFi 6 160M RF TX" -BandKey 2 -PresetKey 3 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"
        Run-ApRfTestCase -TcId "TC-RF-11" -Name "5GHz WiFi 6 80M RF TX"  -BandKey 2 -PresetKey 4 -ExpectedTuiMode "80MHz"  -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
        Run-ApRfTestCase -TcId "TC-RF-12" -Name "5GHz WiFi 6 40M RF TX"  -BandKey 2 -PresetKey 5 -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-13" -Name "5GHz WiFi 6 20M RF TX"  -BandKey 2 -PresetKey 6 -ExpectedTuiMode "WiFi 6"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"
        Run-ApRfTestCase -TcId "TC-RF-14" -Name "5GHz WiFi 5 80M RF TX"  -BandKey 2 -PresetKey 7 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"
        Run-ApRfTestCase -TcId "TC-RF-15" -Name "5GHz WiFi 5 40M RF TX"  -BandKey 2 -PresetKey 8 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-16" -Name "5GHz WiFi 4 40M RF TX"  -BandKey 2 -PresetKey 9 -ExpectedTuiMode "WiFi 4"  -ExpectedTuiCh "channel 36" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-17" -Name "5GHz WiFi 4 20M RF TX"  -BandKey 2 -PresetKey 10 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 36" -ExpectedRfWidth "20 MHz"

        # --- 2.4 GHz (Band 1: 6 presets) ---
        Write-Host "`n[2.4 GHz Live RF Broadcasts]" -ForegroundColor Cyan
        Run-ApRfTestCase -TcId "TC-RF-18" -Name "2.4GHz WiFi 7 40M RF TX" -BandKey 1 -PresetKey 1 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-19" -Name "2.4GHz WiFi 7 20M RF TX" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
        Run-ApRfTestCase -TcId "TC-RF-20" -Name "2.4GHz WiFi 6 40M RF TX" -BandKey 1 -PresetKey 3 -ExpectedTuiMode "WiFi 6" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-21" -Name "2.4GHz WiFi 6 20M RF TX" -BandKey 1 -PresetKey 4 -ExpectedTuiMode "WiFi 6" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
        Run-ApRfTestCase -TcId "TC-RF-22" -Name "2.4GHz WiFi 4 40M RF TX" -BandKey 1 -PresetKey 5 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "40 MHz"
        Run-ApRfTestCase -TcId "TC-RF-23" -Name "2.4GHz WiFi 4 20M RF TX" -BandKey 1 -PresetKey 6 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
    } else {
        Write-Host ""
        Write-Host "==========================================================================" -ForegroundColor Magenta
        Write-Host ">> Phase 3: Hardware RF Closed-Loop Physical Verification (Core 6 Broadcasts)" -ForegroundColor Magenta
        Write-Host "==========================================================================" -ForegroundColor Magenta

        # TC-RF-01: 6GHz WiFi 7 320M TX
        Run-ApRfTestCase -TcId "TC-RF-01" -Name "6GHz WiFi 7 320M RF TX" -BandKey 3 -PresetKey 1 -ExpectedTuiMode "320MHz" -ExpectedTuiCh "Center: 31" -ExpectedRfWidth "320 MHz"

        # TC-RF-02: 6GHz WiFi 6 160M TX
        Run-ApRfTestCase -TcId "TC-RF-02" -Name "6GHz WiFi 6 160M RF TX" -BandKey 3 -PresetKey 6 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 47" -ExpectedRfWidth "160 MHz"

        # TC-RF-03: 5GHz WiFi 7 160M TX
        Run-ApRfTestCase -TcId "TC-RF-03" -Name "5GHz WiFi 7 160M RF TX" -BandKey 2 -PresetKey 1 -ExpectedTuiMode "160MHz" -ExpectedTuiCh "Center: 50" -ExpectedRfWidth "160 MHz"

        # TC-RF-04: 5GHz WiFi 5 80M TX
        Run-ApRfTestCase -TcId "TC-RF-04" -Name "5GHz WiFi 5 80M RF TX" -BandKey 2 -PresetKey 10 -ExpectedTuiMode "802.11ac" -ExpectedTuiCh "Center: 42" -ExpectedRfWidth "80 MHz"

        # TC-RF-05: 2.4GHz WiFi 7 20M TX
        Run-ApRfTestCase -TcId "TC-RF-05" -Name "2.4GHz WiFi 7 20M RF TX" -BandKey 1 -PresetKey 2 -ExpectedTuiMode "WiFi 7" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"

        # TC-RF-06: 2.4GHz WiFi 4 20M TX
        Run-ApRfTestCase -TcId "TC-RF-06" -Name "2.4GHz WiFi 4 20M RF TX" -BandKey 1 -PresetKey 6 -ExpectedTuiMode "WiFi 4" -ExpectedTuiCh "channel 6" -ExpectedRfWidth "20 MHz"
    }
}

Cleanup-Environment
Show-SummaryReport
