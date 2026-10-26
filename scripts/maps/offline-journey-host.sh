#!/bin/bash
# MAP-24 host half (set E="adb -s <serial>"): needs native-proof-server on 127.0.0.1:18765 and an
# `adb reverse tcp:18765 tcp:18765`. Run the online phase first, then this script.
# Host half of the MAP-24 offline journey: freeze count, airplane, clean restart, run, compare.
E="adb -s ${MAP_PROOF_SERIAL:?set MAP_PROOF_SERIAL to the exclusive emulator/device serial}"
count() { curl -s http://127.0.0.1:18765/proof/requests | python3 -c "import sys,json;d=json.load(sys.stdin);print(len([r for r in d['requests'] if r.startswith('/v1')]))"; }
BEFORE=$(count)
$E shell cmd connectivity airplane-mode enable
$E shell svc wifi disable
$E shell svc data disable
sleep 6
echo "airplane=$($E shell settings get global airplane_mode_on) wlan0=$($E shell ip -br addr show wlan0 2>&1 | head -1)"
$E shell am force-stop com.anpfuel.app.debug
$E shell am instrument -w -r -e class com.anpfuel.app.ui.map.MapOfflineJourneyInstrumentedTest#offlineJourneyAfterCleanRestartUsesOnlyLastGoodLocalData -e mapOfflinePhase offline com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner > /tmp/map24-journey-offline.log 2>&1
AFTER=$(count)
$E shell cmd connectivity airplane-mode disable
$E shell svc wifi enable
$E shell svc data enable
echo "v1-requests before=$BEFORE after=$AFTER"
grep -E "^OK|Tests run|FAILURES|stack=" /tmp/map24-journey-offline.log | head -5 | cut -c1-500
