import re

with open('d:\\QWiFiTestTool\\QWifiTestTool\\assets\\default_modules.xml', 'r', encoding='utf-8') as f:
    text = f.read()

fc21_pattern = r"(    <!-- Quectel FC21 \(Realtek RTL8821CU Chipset\) -->.*?    </Module>\n)"
fcm360w_pattern = r"(    <!-- Quectel FCM360W \(MediaTek MT7663 Chipset\) -->.*?    </Module>\n)"

fc21_match = re.search(fc21_pattern, text, re.DOTALL)
fcm360w_match = re.search(fcm360w_pattern, text, re.DOTALL)

if fc21_match and fcm360w_match:
    fc21_str = fc21_match.group(1)
    fcm360w_str = fcm360w_match.group(1)
    text = text.replace(fc21_str, "PLACEHOLDER_FC21\n")
    text = text.replace(fcm360w_str, "PLACEHOLDER_FCM360W\n")
    text = text.replace("PLACEHOLDER_FC21\n", fcm360w_str)
    text = text.replace("PLACEHOLDER_FCM360W\n", fc21_str)

fgh200m_pattern = r"(    <!-- Quectel FGH200M \(Morse Micro Chipset\) -->.*?    </Module>\n)"
fc905a_pattern = r"(    <!-- Quectel FC905A -->.*?    </Module>\n)"

fgh200m_match = re.search(fgh200m_pattern, text, re.DOTALL)
fc905a_match = re.search(fc905a_pattern, text, re.DOTALL)

if fgh200m_match and fc905a_match:
    fgh200m_str = fgh200m_match.group(1)
    fc905a_str = fc905a_match.group(1)
    text = text.replace(fgh200m_str, "PLACEHOLDER_FGH200M\n")
    text = text.replace(fc905a_str, "PLACEHOLDER_FC905A\n")
    text = text.replace("PLACEHOLDER_FGH200M\n", fc905a_str)
    text = text.replace("PLACEHOLDER_FC905A\n", fgh200m_str)

with open('d:\\QWiFiTestTool\\QWifiTestTool\\assets\\default_modules.xml', 'w', encoding='utf-8') as f:
    f.write(text)

with open('d:\\QWiFiTestTool\\QWifiTestTool\\internal\\models\\default_modules.xml', 'w', encoding='utf-8') as f:
    f.write(text)

