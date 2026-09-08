# Implementation Plan - Add Iw Info Display

Goal: Display `channel`, `width`, and `txpower` from `iw dev <iface> info` in both Station and AP modes.

## Proposed Changes

### [wifi]

#### [NEW] internal/wifi/iw_helper.go

- Create a new file to hold the `ShowInterfaceDetail(iface string)` function.
- This function will:
  - Execute `iw dev <iface> info`.
  - Parse the output to extract:
    - `channel`: e.g., "4 (2427 MHz)"
    - `width`: e.g., "20 MHz"
    - `txpower`: e.g., "18.00 dBm"
  - Print these details nicely.

#### [MODIFY] internal/wifi/connector_helpers.go

- In `showConnectionInfo`, call `ShowInterfaceDetail(iface)`.

#### [MODIFY] internal/wifi/hotspot.go

- In `showAPInfo`, call `ShowInterfaceDetail(iface)`.

## Verification Plan

### Manual Verification

1. **Build and Deploy**:
    - Run `go build -o wifi-tool cmd/main.go`
    - `adb push wifi-tool /data/local/tmp/`
    - `adb shell chmod +x /data/local/tmp/wifi-tool`
2. **Test Station Mode**:
    - Run `wifi-tool`.
    - Select Station Mode -> Connect to a WiFi.
    - Verify that after connection, the "Interface Details" section appears with Channel, Width, and TxPower.
3. **Test AP Mode**:
    - Run `wifi-tool`.
    - Select AP Mode -> Start Hotspot.
    - Verify that after AP starts, the "Interface Details" section appears.
