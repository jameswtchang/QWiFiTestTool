# Android ARM64 靜態二進制檔案

本目錄應包含以下靜態編譯的二進制檔案：

## 必需檔案

1. **wpa_supplicant** (v2.10+)
   - 用途: Station 模式的核心 daemon
   - 編譯選項: 靜態連結，支援 nl80211 驅動

2. **hostapd** (v2.10+) 
   - 用途: AP 模式的核心 daemon
   - 編譯選項: 靜態連結，支援 nl80211 驅動

3. **iw** (可選)
   - 用途: 調試工具
   - 編譯選項: 靜態連結

## 取得方式

### 選項 1: 從原始碼編譯

```bash
# 下載 wpa_supplicant 原始碼
wget https://w1.fi/releases/wpa_supplicant-2.10.tar.gz
tar xzf wpa_supplicant-2.10.tar.gz
cd wpa_supplicant-2.10/wpa_supplicant

# 配置靜態編譯
cp defconfig .config
echo "CONFIG_DRIVER_NL80211=y" >> .config
echo "CFLAGS += -static" >> .config

# 使用 Android NDK 編譯
export CC=aarch64-linux-android21-clang
make

# 複製到本目錄
cp wpa_supplicant ../../../assets/android_arm64/
```

### 選項 2: 從預編譯版本

如果您有可信的預編譯版本，可直接放置於此。

### 選項 3: 使用 Buildroot

使用 Buildroot 工具鏈編譯靜態版本。

## 驗證

放置檔案後，驗證：

```bash
file wpa_supplicant
# 應輸出: ELF 64-bit LSB executable, ARM aarch64, statically linked

./wpa_supplicant -v
# 應輸出版本資訊
```

## 注意事項

⚠️ **重要**: 這些二進制檔案不包含在版本控制中，因為：
1. 檔案體積較大
2. 授權考量
3. 需要針對目標設備編譯

您必須自行準備這些檔案才能在 Android 上使用本工具。
