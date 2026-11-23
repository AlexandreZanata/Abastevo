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

        ET.parse(IOS_ROOT / "AnpFuel.xcodeproj/xcshareddata/xcschemes/AnpFuel.xcscheme")

        icon_catalog = json.loads((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset/Contents.json").read_text(encoding="utf-8"))
        for entry in icon_catalog["images"]:
            self.assertTrue((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset" / entry["filename"]).is_file(), entry["filename"])

    def test_training_terms_sha256_and_backend_origins_match_main(self):
        backend_terms = (ROOT / "backend/internal/platform/legalterms/text/training-consent-pt-BR.txt").read_bytes()
        expected_sha = hashlib.sha256(backend_terms).hexdigest()
        models_text = (IOS_ROOT / "Sources/AnpFuelCore/DomainModels.swift").read_text(encoding="utf-8")
        self.assertIn(expected_sha, models_text)
        self.assertIn("training-terms-v1", models_text)

        http_text = (IOS_ROOT / "Sources/AnpFuelCore/AbastevoBackendClient.swift").read_text(encoding="utf-8")
        self.assertIn("https://abastevo.com.br", http_text)
        self.assertIn("/v1/community/feed", http_text)
        self.assertIn("/v1/privacy/training-consent/grant", http_text)
        self.assertIn("/v1/privacy/training-consent/revoke", http_text)

        energy_fixture = json.loads((ROOT / "contracts/testdata/premium/energy-v1.json").read_text(encoding="utf-8"))
        self.assertEqual(energy_fixture["cases"][0]["expected_wh_per_km"], "800/3")
        tests_text = (IOS_ROOT / "Tests/AnpFuelCoreTests/DomainAndFeaturesTests.swift").read_text(encoding="utf-8")
        self.assertIn('"800/3"', tests_text)
        self.assertIn('"phev-shared-distance"', tests_text)


if __name__ == "__main__":
    unittest.main(verbosity=2)
