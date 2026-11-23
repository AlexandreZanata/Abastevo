#!/usr/bin/env python3
"""Static, structural, and contract-parity verifier for iosApp/ on Linux and macOS."""

import hashlib
import json
from pathlib import Path
import plistlib
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
IOS_ROOT = ROOT / "iosApp"


class IosAppIntegrityTests(unittest.TestCase):
    def test_city_picker_and_onboarding_share_the_android_ibge_asset(self):
        asset = ROOT / "data/src/main/assets/ibge_municipalities.json"
        rows = json.loads(asset.read_text(encoding="utf-8"))
        self.assertGreaterEqual(len(rows), 5570)
        self.assertEqual(len({row["ibgeCode"] for row in rows}), len(rows))
        self.assertEqual(len({row["state"] for row in rows}), 27)
        self.assertFalse(any(IOS_ROOT.rglob(asset.name)))

        project = (IOS_ROOT / "AnpFuel.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
        manifest = (IOS_ROOT / "project.yml").read_text(encoding="utf-8")
        source_path = "../" + asset.relative_to(ROOT).as_posix()
        self.assertIn(f'path = "{source_path}"; sourceTree = SOURCE_ROOT;', project)
        self.assertEqual(manifest.count(f"path: {source_path}"), 2)
        self.assertEqual(len(re.findall(r"^\s*[A-F0-9]{24} /\* ibge_municipalities\.json in Resources \*/,", project, re.M)), 2)
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        for name in ("ExploreTabViews.swift", "OnboardingAndTermsViews.swift"):
            source = (shell / name).read_text(encoding="utf-8")
            self.assertIn("viewModel.municipalityCatalog.search(", source)
            self.assertNotIn("SeedCatalog.municipalities", source)

    def test_public_explore_uses_real_sources_and_never_sample_prices(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        explore = (shell / "ExploreTabViews.swift").read_text(encoding="utf-8")
        client = (IOS_ROOT / "Sources/AnpFuelCore/AbastevoBackendClient.swift").read_text(encoding="utf-8")
        self.assertIn('"/v1/stations"', client)
        self.assertIn("/prices", client)
        self.assertNotIn("SeedCatalog.stationsFor", state)
        self.assertNotIn("SeedCatalog.officialAverage", state)
        self.assertNotIn("SeedCatalog.priceHistory", explore)
        self.assertNotIn("downloadedWeekIds.append", state)

    def test_required_core_shell_test_and_xcode_files_exist(self):
        expected_core = [
            "AbastevoBackendClient.swift",
            "DomainModels.swift",
            "FeedbackDisplay.swift",
            "FeedbackFlow.swift",
            "IosLocalStore.swift",
            "IosPlatformServices.swift",
            "LocationSignals.swift",
            "PortableAnonymousProof.swift",
            "PortableAuth.swift",
            "PortableAuthFlow.swift",
            "PortableFeedback.swift",
            "PortableIdTime.swift",
            "PortableLocation.swift",
            "PortableLocationRecovery.swift",
            "PortableMoney.swift",
            "PortableOcrAndFraming.swift",
            "PortableOutboxAndSync.swift",
            "PortablePhoto.swift",
            "PortablePhotoFlow.swift",
            "PortableText.swift",
            "TankFillUseCase.swift",
        ]
        for fname in expected_core:
            self.assertTrue((IOS_ROOT / "Sources/AnpFuelCore" / fname).is_file(), fname)

        expected_shell = [
            "AnpFuelApp.swift",
            "AppStateViewModel.swift",
            "CommunityTabViews.swift",
            "DesignSystem.swift",
            "ExploreTabViews.swift",
            "FeedbackView.swift",
            "MapTabViews.swift",
            "OnboardingAndTermsViews.swift",
            "ProfileTabViews.swift",
        ]
        for fname in expected_shell:
            self.assertTrue((IOS_ROOT / "Sources/AnpFuelShell" / fname).is_file(), fname)

        expected_tests = [
            "DomainAndFeaturesTests.swift",
            "FeedbackDisplayTests.swift",
            "FeedbackFlowTests.swift",
            "FeedbackLifecycleTests.swift",
            "LocationSignalsTests.swift",
            "PortableAnonymousProofTests.swift",
            "PortableAuthFlowTests.swift",
            "PortableAuthTests.swift",
            "PortableFeedbackTests.swift",
            "PortableIdTimeTests.swift",
            "PortableLocationRecoveryTests.swift",
            "PortableLocationTests.swift",
            "PortableMoneyTests.swift",
            "PortableOcrAndFramingTests.swift",
            "PortableOutboxAndSyncTests.swift",
            "PortablePhotoFlowTests.swift",
            "PortablePhotoTests.swift",
            "PortableTextTests.swift",
            "TankFillUseCaseTests.swift",
        ]
        for fname in expected_tests:
            self.assertTrue((IOS_ROOT / "Tests/AnpFuelCoreTests" / fname).is_file(), fname)

        for rel in [
            "Package.swift",
            "project.yml",
            "App/Info.plist",
            "App/PrivacyInfo.xcprivacy",
            "App/AnpFuel.entitlements",
            "AnpFuel.xcodeproj/project.pbxproj",
            "AnpFuel.xcodeproj/xcshareddata/xcschemes/AnpFuel.xcscheme",
            "Assets.xcassets/AppIcon.appiconset/Contents.json",
        ]:
            self.assertTrue((IOS_ROOT / rel).is_file(), rel)

    def test_no_force_unwrap_in_core_and_no_background_location_in_sources(self):
        force_unwrap = re.compile(r"[A-Za-z0-9_\)\]]!(?![=])")
        for swift in sorted((IOS_ROOT / "Sources/AnpFuelCore").glob("*.swift")):
            text = swift.read_text(encoding="utf-8")
            for idx, line in enumerate(text.splitlines(), 1):
                self.assertIsNone(
                    force_unwrap.search(line),
                    f"Forbidden force-unwrap pattern in {swift.name}:{idx}: {line}",
                )

        banned = ("startUpdatingLocation", "allowsBackgroundLocationUpdates")
        for swift in sorted((IOS_ROOT / "Sources").rglob("*.swift")):
            text = swift.read_text(encoding="utf-8")
            for token in banned:
                self.assertNotIn(token, text, f"Forbidden location token {token} in {swift}")

    def test_swift_files_have_balanced_braces_and_parens(self):
        for swift in sorted((IOS_ROOT / "Sources").rglob("*.swift")) + sorted((IOS_ROOT / "Tests").rglob("*.swift")):
            text = swift.read_text(encoding="utf-8")
            braces = 0
            parens = 0
            in_str = False
            in_line_comment = False
            in_block_comment = False
            escaped = False
            i = 0
            while i < len(text):
                ch = text[i]
                nxt = text[i + 1] if i + 1 < len(text) else ""
                if in_line_comment:
                    if ch == "\n":
                        in_line_comment = False
                    i += 1
                    continue
                if in_block_comment:
                    if ch == "*" and nxt == "/":
                        in_block_comment = False
                        i += 2
                        continue
                    i += 1
                    continue
                if in_str:
                    if escaped:
                        escaped = False
                    elif ch == "\\":
                        escaped = True
                    elif ch == '"':
                        in_str = False
                    i += 1
                    continue
                if ch == "/" and nxt == "/":
                    in_line_comment = True
                    i += 2
                    continue
                if ch == "/" and nxt == "*":
                    in_block_comment = True
                    i += 2
                    continue
                if ch == '"':
                    in_str = True
                    i += 1
                    continue
                if ch == "{":
                    braces += 1
                elif ch == "}":
                    braces -= 1
                elif ch == "(":
                    parens += 1
                elif ch == ")":
                    parens -= 1
                self.assertGreaterEqual(braces, 0, f"Unbalanced closing brace in {swift.name}")
                self.assertGreaterEqual(parens, 0, f"Unbalanced closing paren in {swift.name}")
                i += 1
            self.assertEqual(braces, 0, f"Unclosed brace in {swift.name}")
            self.assertEqual(parens, 0, f"Unclosed paren in {swift.name}")

    def test_unintegrated_enterprise_screen_cannot_issue_local_authorizations(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        enterprise = profile.split("public struct EnterpriseHubSheetView: View {", 1)[1].split(
            "public struct ArcadeGamesSheetView", 1
        )[0]
        self.assertIn("ContentUnavailableView", enterprise)
        self.assertNotIn("TextField(", enterprise)
        self.assertNotIn("viewModel", enterprise)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        self.assertNotIn("joinSampleEnterprise", state)
        self.assertNotIn("fuelingAuthCode", state)

    def test_ledger_cannot_use_sample_metrics_or_an_unselected_vehicle(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        ledger = profile.split("public struct VehiclesAndLedgerSheetView: View {", 1)[1].split(
            "public struct DriverHubSheetView", 1
        )[0]
        self.assertIn("selection: $ledgerVehicleId", ledger)
        self.assertNotIn('?? "default"', ledger)
        self.assertNotIn("PremiumEnergyCalculator.IntervalInput", ledger)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        vehicle = state.split("func addVehicle(", 1)[1].split("func removeVehicle", 1)[0]
        self.assertIn("PortableMoney.parseBatteryCapacityWh", vehicle)
        self.assertNotIn("Double(batteryKwhText", vehicle)

    def test_manual_shift_uses_selected_vehicle_exact_money_and_clock(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        driver = profile.split("public struct DriverHubSheetView: View {", 1)[1].split(
            "public struct EnterpriseHubSheetView", 1
        )[0]
        self.assertIn("selection: $driverVehicleId", driver)
        self.assertIn("PortableMoney.parseBrlCentavos", driver)
        self.assertNotIn('"driver-vehicle"', driver)
        self.assertNotIn("Double(cleaned)", driver)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        self.assertIn("startedAtLabel: Date().ISO8601Format()", state)
        self.assertIn("endedAtLabel: Date().ISO8601Format()", state)

    def test_info_plist_privacy_manifest_and_entitlements_are_valid_and_safe(self):
        with (IOS_ROOT / "App/Info.plist").open("rb") as fh:
            info = plistlib.load(fh)
        self.assertEqual(info["CFBundleDisplayName"], "abastevo")
        self.assertEqual(info["CFBundleShortVersionString"], "3.1.18")
        self.assertEqual(info["CFBundleVersion"], "30118")
        self.assertIn("NSLocationWhenInUseUsageDescription", info)
        self.assertNotIn("NSLocationAlwaysAndWhenInUseUsageDescription", info)
        self.assertIn("NSCameraUsageDescription", info)
        self.assertIn("NSPhotoLibraryUsageDescription", info)
        self.assertFalse(info["NSAppTransportSecurity"]["NSAllowsArbitraryLoads"])
        self.assertEqual(info["CFBundleURLTypes"][0]["CFBundleURLSchemes"], ["anpfuel"])
        self.assertEqual(len(info["CFBundleLocalizations"]), 8)

        with (IOS_ROOT / "App/PrivacyInfo.xcprivacy").open("rb") as fh:
            privacy = plistlib.load(fh)
        self.assertFalse(privacy["NSPrivacyTracking"])
        self.assertEqual(privacy["NSPrivacyTrackingDomains"], [])
        reasons = {entry["NSPrivacyAccessedAPIType"]: entry["NSPrivacyAccessedAPITypeReasons"] for entry in privacy["NSPrivacyAccessedAPITypes"]}
        self.assertEqual(reasons["NSPrivacyAccessedAPICategoryUserDefaults"], ["CA92.1"])
        self.assertEqual(reasons["NSPrivacyAccessedAPICategoryFileTimestamp"], ["C617.1"])
        self.assertEqual(reasons["NSPrivacyAccessedAPICategoryDiskSpace"], ["E174.1"])

        with (IOS_ROOT / "App/AnpFuel.entitlements").open("rb") as fh:
            ent = plistlib.load(fh)
        self.assertEqual(ent["com.apple.developer.applesignin"], ["Default"])
        self.assertIn("applinks:abastevo.com.br", ent["com.apple.developer.associated-domains"])

    def test_xcode_project_references_every_swift_and_icon_asset(self):
        pbx = (IOS_ROOT / "AnpFuel.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
        for folder in ["Sources/AnpFuelCore", "Sources/AnpFuelShell", "Tests/AnpFuelCoreTests"]:
            for swift in (IOS_ROOT / folder).glob("*.swift"):
                self.assertIn(swift.name, pbx, f"{swift.name} missing from project.pbxproj")
        self.assertIn("ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;", pbx)
        self.assertIn("PRODUCT_BUNDLE_IDENTIFIER = com.anpfuel.app;", pbx)
        self.assertIn("PrivacyInfo.xcprivacy", pbx)
        self.assertEqual(re.findall(r"SWIFT_VERSION = ([^;]+);", pbx), ["5.0", "5.0"])
        self.assertIn('SWIFT_VERSION: "5.0"', (IOS_ROOT / "project.yml").read_text())
        self.assertIn("ENABLE_TESTABILITY = YES;", pbx)

        ET.parse(IOS_ROOT / "AnpFuel.xcodeproj/xcshareddata/xcschemes/AnpFuel.xcscheme")

        icon_catalog = json.loads((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset/Contents.json").read_text(encoding="utf-8"))
        for entry in icon_catalog["images"]:
            self.assertTrue((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset" / entry["filename"]).is_file(), entry["filename"])

    def test_xcode_project_registers_every_swift_source(self):
        import subprocess
        import sys

        result = subprocess.run(
            [sys.executable, str(ROOT / "scripts/ios_sync_xcodeproj.py"), "--check"],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_training_terms_sha256_and_backend_origins_match_main(self):
        backend_terms = (ROOT / "backend/internal/platform/legalterms/text/training-consent-pt-BR.txt").read_bytes()
        expected_sha = hashlib.sha256(backend_terms).hexdigest()
        models_text = (IOS_ROOT / "Sources/AnpFuelCore/DomainModels.swift").read_text(encoding="utf-8")
        self.assertIn(expected_sha, models_text)
        self.assertIn("training-terms-v1", models_text)

        http_text = (IOS_ROOT / "Sources/AnpFuelCore/AbastevoBackendClient.swift").read_text(encoding="utf-8")
        self.assertIn("https://abastevo.com.br", http_text)
        self.assertIn("/v1/community/feed", http_text)
        profile = (IOS_ROOT / "Sources/AnpFuelShell/ProfileTabViews.swift").read_text(encoding="utf-8")
        consent = profile.split("public struct TrainingConsentSheetView", 1)[1].split("public struct SettingsSheetView", 1)[0]
        self.assertIn("TrainingTermsContract.verifiedDigestMatches()", consent)
        self.assertNotIn("grantTrainingConsent", consent)
        self.assertNotIn("revokeTrainingConsentImmediately", consent)

        energy_fixture = json.loads((ROOT / "contracts/testdata/premium/energy-v1.json").read_text(encoding="utf-8"))
        self.assertEqual(energy_fixture["cases"][0]["expected_wh_per_km"], "800/3")
        test_source = (IOS_ROOT / "Tests/AnpFuelCoreTests/DomainAndFeaturesTests.swift").read_text(encoding="utf-8")
        self.assertIn("for vector in intervals", test_source)
        pbx = (IOS_ROOT / "AnpFuel.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
        self.assertIn("../contracts/testdata/premium/energy-v1.json", pbx)
        self.assertIn("energy-v1.json in Resources", pbx)

    def test_map_routes_and_download_state_have_no_sample_data(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        explore = (shell / "ExploreTabViews.swift").read_text(encoding="utf-8")
        maps = (shell / "MapTabViews.swift").read_text(encoding="utf-8")
        self.assertIn("NativeRouteSheetView(viewModel: viewModel, station: station)", explore)
        self.assertIn("station.pinTier.allowsMapPin", maps)
        self.assertIn("await viewModel.requestRouteOrigin()", maps)
        self.assertIn("await MKDirections(request: request).calculate()", maps)
        self.assertIn("MKLaunchOptionsDirectionsModeDriving", maps)
        offline = (shell / "OfflineNavigationViews.swift").read_text(encoding="utf-8")
        # B-BR-IOS-01: offline navigation hands off to Apple Maps offline maps;
        # the app never claims its own regional package is available.
        self.assertIn("Mapas regionais indisponíveis", offline)
        self.assertIn("Mapas Offline", offline)
        self.assertIn("https://maps.apple.com/", offline)
        self.assertNotIn("FileManager", offline)
        store = (IOS_ROOT / "Sources/AnpFuelCore/IosLocalStore.swift").read_text(encoding="utf-8")
        self.assertNotIn("downloadedMapRegions", store)


if __name__ == "__main__":
    unittest.main(verbosity=2)
