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
            "AccountProviderLogin.swift",
            "CommunityOutbox.swift",
            "CommunityWriteClient.swift",
            "DeviceIdentityFlow.swift",
            "DomainModels.swift",
            "EnterpriseCanonicalJson.swift",
            "EnterpriseCompanyAccess.swift",
            "EnterpriseDeviceSync.swift",
            "EnterpriseOperations.swift",
            "EnterpriseQueue.swift",
            "EnterpriseValues.swift",
            "FeedbackDisplay.swift",
            "FeedbackFlow.swift",
            "IosLocalStore.swift",
            "IosPlatformServices.swift",
            "LedgerEntryForm.swift",
            "LedgerExactMath.swift",
            "LedgerIndicators.swift",
            "LedgerModels.swift",
            "LedgerSyncClient.swift",
            "LedgerSyncEngine.swift",
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
            "SignedRequestTransport.swift",
            "TankFillUseCase.swift",
            "VehicleAdmission.swift",
        ]
        for fname in expected_core:
            self.assertTrue((IOS_ROOT / "Sources/AnpFuelCore" / fname).is_file(), fname)

        expected_shell = [
            "AnpFuelApp.swift",
            "AppStateViewModel.swift",
            "CommunityPublishingViews.swift",
            "CommunityTabViews.swift",
            "DesignSystem.swift",
            "EnterpriseCoordinator.swift",
            "EnterpriseHubViews.swift",
            "ExploreTabViews.swift",
            "FeedbackView.swift",
            "IdentityCoordinator.swift",
            "LedgerCoordinator.swift",
            "LedgerViews.swift",
            "MapTabViews.swift",
            "OnboardingAndTermsViews.swift",
            "ProfileTabViews.swift",
        ]
        for fname in expected_shell:
            self.assertTrue((IOS_ROOT / "Sources/AnpFuelShell" / fname).is_file(), fname)

        expected_tests = [
            "CommunityOutboxTests.swift",
            "CommunityWriteWireTests.swift",
            "DomainAndFeaturesTests.swift",
            "EnterpriseAccessWireTests.swift",
            "EnterpriseCanonicalJsonTests.swift",
            "EnterpriseOperationTests.swift",
            "EnterpriseQueueTests.swift",
            "EnterpriseTransportTests.swift",
            "FeedbackDisplayTests.swift",
            "FeedbackFlowTests.swift",
            "FeedbackLifecycleTests.swift",
            "LedgerFixtures.swift",
            "LedgerMathTests.swift",
            "LedgerModelsTests.swift",
            "LedgerSyncTests.swift",
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
            "SignedRequestTransportTests.swift",
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

    def test_no_force_unwrap_in_core_and_background_location_is_session_scoped(self):
        force_unwrap = re.compile(r"[A-Za-z0-9_\)\]]!(?![=])")
        for swift in sorted((IOS_ROOT / "Sources/AnpFuelCore").glob("*.swift")):
            text = swift.read_text(encoding="utf-8")
            for idx, line in enumerate(text.splitlines(), 1):
                self.assertIsNone(
                    force_unwrap.search(line),
                    f"Forbidden force-unwrap pattern in {swift.name}:{idx}: {line}",
                )

        continuous = (
            "startUpdatingLocation", "allowsBackgroundLocationUpdates", "showsBackgroundLocationIndicator",
            "startMonitoringSignificantLocationChanges", "CLBackgroundActivitySession",
            "CLLocationUpdate", "liveUpdates", "CLMonitor",
        )
        allowed = {
            "Sources/AnpFuelShell/ActiveNavigationLocation.swift",
        }
        for swift in sorted((IOS_ROOT / "Sources").rglob("*.swift")):
            text = swift.read_text(encoding="utf-8")
            self.assertNotIn("requestAlwaysAuthorization", text, f"Always authorization in {swift}")
            if swift.relative_to(IOS_ROOT).as_posix() not in allowed:
                for token in continuous:
                    self.assertNotIn(token, text, f"Forbidden location token {token} in {swift}")

        adapter = (IOS_ROOT / "Sources/AnpFuelShell/ActiveNavigationLocation.swift").read_text(encoding="utf-8")
        for token in (
            "requestWhenInUseAuthorization()", "applicationState == .active", "allowsBackgroundLocationUpdates = true",
            "showsBackgroundLocationIndicator = true", "stopUpdatingLocation()", "allowsBackgroundLocationUpdates = false",
        ):
            self.assertIn(token, adapter)

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

    def test_enterprise_commands_are_device_signed_and_grant_no_local_authority(self):
        core = IOS_ROOT / "Sources/AnpFuelCore"
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        sync = (core / "EnterpriseDeviceSync.swift").read_text(encoding="utf-8")
        # Every device-plane command and read goes through SignedRequestTransport.
        self.assertEqual(sync.count("transport.signedRequest("), 1)
        self.assertIn('method: "POST", path: path, body: body, expectedOwnerScope: ownerScope, expectedSessionFamilyId: sessionFamilyId', sync)
        self.assertIn('body: body, ownerScope: ownerScope, sessionFamilyId: session.familyId', sync)
        for route in ("/sync/operations", "/sync/reference", "/sync/attachments", "devices/", '"check"', '"enroll"', '"contact"'):
            self.assertIn(route, sync)
        # The only unsigned request is the capability-bound content ingress of a saved receipt.
        self.assertEqual(sync.count("uploads.send("), 1)
        self.assertIn("/content/upload", sync)
        self.assertIn("EnterpriseAttachmentWire.capabilityHeader", sync)
        transport = (core / "SignedRequestTransport.swift").read_text(encoding="utf-8")
        self.assertIn("SignedRequestProfile.enterpriseErrorCode(from: data)", transport)

        # The cookie plane reads contexts, invitations and checklists; it never carries device proof.
        company = (core / "EnterpriseCompanyAccess.swift").read_text(encoding="utf-8")
        self.assertNotIn('forHTTPHeaderField: "Signature', company)
        self.assertIn('"/v1/org-access/session/login"', company)
        self.assertIn('"/v1/org-access/contexts/list"', company)
        self.assertIn('"/v1/org-access/invitations/"', company)
        self.assertIn("request.httpShouldHandleCookies = false", company)

        coordinator = (shell / "EnterpriseCoordinator.swift").read_text(encoding="utf-8")
        views = (shell / "EnterpriseHubViews.swift").read_text(encoding="utf-8")
        for text in (coordinator, views):
            self.assertNotIn("URLRequest", text)
            self.assertNotIn("URLSession", text)
        self.assertIn("sync.deliver(partition: current.partition", coordinator)
        self.assertIn("EnterpriseOperationBuilder.startTrip(", coordinator)
        self.assertIn("EnterpriseOperationBuilder.submitExpense(", coordinator)

        # No locally minted authorization code, voucher or sample company.
        enterprise_sources = sorted(core.glob("Enterprise*.swift")) + sorted(shell.glob("Enterprise*.swift"))
        self.assertGreaterEqual(len(enterprise_sources), 8)
        for path in enterprise_sources:
            text = path.read_text(encoding="utf-8")
            for token in (
                "fuelingAuthCode", "fueling-authorizations/issue", "redemptions/confirm",
                "voucherCode", "/vouchers/issue", "joinSampleEnterprise", "sampleCompany",
            ):
                self.assertNotIn(token, text, f"{token} in {path.name}")
            self.assertNotRegex(
                text, r"(?i)(?:mint|issue|create|generate)[_\s]*(?:fueling[_\s]*)?(?:authorization|voucher)",
                f"Field-side authorization issuance in {path.name}",
            )
            # Tracking stays off on iOS: no location API in company mode.
            for token in ("CLLocationManager", "requestAlwaysAuthorization", "startMonitoring", "CoreLocation"):
                self.assertNotIn(token, text, f"{token} in {path.name}")
        self.assertRegex(views, r'Text\(\s*String\(localized:\s*"enterprise\.tracking_off"')

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        self.assertNotIn("joinSampleEnterprise", state)
        self.assertNotIn("fuelingAuthCode", state)
        logout = state.split("public func logout()", 1)[1].split("public func deleteAccountAndPurge", 1)[0]
        self.assertIn("enterprise.purgeOwnedData()", logout)
        deletion = state.split("private func finishAccountDeletion", 1)[1].split("public func exportMyDataJson", 1)[0]
        self.assertIn("enterprise.purgeOwnedData()", deletion)
        environment = state.split("public func setApiEnvironment", 1)[1].split("public func togglePriceDropAlert", 1)[0]
        self.assertIn("enterprise.purgeOwnedData()", environment)

        # The hub is the real flow, wired to the coordinator; the placeholder is gone.
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        self.assertNotIn("struct EnterpriseHubSheetView", profile)
        self.assertRegex(
            profile,
            r'profileNavRow\(\s*icon:\s*"building\.2\.fill",\s*'
            r'title:\s*String\(localized:\s*"enterprise\.title"[^\n]*\),\s*'
            r'subtitle:\s*String\(localized:\s*"enterprise\.profile_subtitle"',
        )
        self.assertIn("public struct EnterpriseHubSheetView: View", views)
        self.assertIn("enterprise: viewModel.enterprise", views)
        self.assertNotIn("ContentUnavailableView", views)
        app = (shell / "AnpFuelApp.swift").read_text(encoding="utf-8")
        self.assertIn("EnterpriseHubSheetView(viewModel: viewModel)", app)

        # Protected queue: per-partition files, backup-excluded and file-protected, purged by account.
        queue = (core / "EnterpriseQueue.swift").read_text(encoding="utf-8")
        self.assertIn("values.isExcludedFromBackup = true", queue)
        self.assertIn(".completeFileProtectionUntilFirstUserAuthentication", queue)
        self.assertIn("func purgeAccounts(except accountId: String)", queue)
        self.assertIn("public static let maxAttempts = 25", queue)

    def test_enterprise_xctest_vectors_match_the_shared_contracts(self):
        tests = IOS_ROOT / "Tests/AnpFuelCoreTests"
        fixtures = ROOT / "contracts/testdata/enterprise"

        def rows(relative):
            lines = (fixtures / relative).read_text(encoding="utf-8").splitlines()
            return {line.split("|")[0]: line.split("|") for line in lines[1:]}

        canonical = rows("sync/canonical-v1.psv")
        source = (tests / "EnterpriseCanonicalJsonTests.swift").read_text(encoding="utf-8")
        copied = re.findall(
            r'CanonicalVector\(id: "([^"]+)", rawHex: "([0-9a-f]*)", canonicalHex: (nil|"[0-9a-f]*"), failure: (nil|\.[a-z]+)\)',
            source,
        )
        self.assertGreaterEqual(len(copied), 20)
        for vector_id, raw, expected, failure in copied:
            row = canonical[vector_id]
            self.assertIn("raw_hex=" + raw, row[2].split(";"), vector_id)
            if expected == "nil":
                self.assertEqual(row[3], "error=" + failure[1:].upper(), vector_id)
            else:
                self.assertTrue(row[3].startswith("canonical_hex=" + expected.strip('"') + ";"), vector_id)

        operations = {}
        for relative in ("trip/trip-operations-v3.psv", "expense/expense-submit-v1.psv", "checklist/run-v1.psv"):
            operations.update(rows(relative))
        source = (tests / "EnterpriseOperationTests.swift").read_text(encoding="utf-8")
        copied = re.findall(r'static let ([a-z0-9_]+) = #"(.*)"#', source)
        self.assertGreaterEqual(len(copied), 9)
        for name, text in copied:
            row = operations[name.replace("_", "-")]
            self.assertEqual(row[3], "OK", name)
            self.assertEqual(bytes.fromhex(row[4]).decode("utf-8"), text, name)

        acks = {k: v for k, v in rows("sync/operations-v1.psv").items() if v[1] == "ACK"}
        copied = re.findall(r'AckVector\(id: "([^"]+)", expected: "([A-Z_]+)", raw: #"(.*)"#\)', source)
        self.assertEqual(len(copied), len(acks))
        for vector_id, expected, raw in copied:
            row = acks[vector_id]
            self.assertEqual(bytes.fromhex(row[2].split("raw_hex=", 1)[1].split(";", 1)[0]).decode("utf-8"), raw, vector_id)
            outcome = row[3].split(";")[1].split("=")[1] if row[3].startswith("final") else row[3].split("=")[1]
            self.assertEqual(outcome, expected, vector_id)

        reference = rows("sync/reference-feed-v1.psv")
        attachment = rows("sync/attachment-session-v1.psv")
        source = (tests / "EnterpriseAccessWireTests.swift").read_text(encoding="utf-8")
        copied = dict(re.findall(r'static let ([a-z0-9_]+) = #"(.*)"#', source))
        self.assertEqual(bytes.fromhex(reference["request-no-cursor"][5]).decode(), copied.pop("request_no_cursor"))
        self.assertEqual(bytes.fromhex(reference["request-with-cursor"][5]).decode(), copied.pop("request_with_cursor"))
        self.assertEqual(bytes.fromhex(attachment["request-meter-reading"][5]).decode(), copied.pop("request_meter_reading"))
        self.assertGreaterEqual(len(copied), 10)
        for name, text in copied.items():
            row = reference.get(name.replace("_", "-")) or attachment[name.replace("_", "-")]
            self.assertEqual(bytes.fromhex(row[2]).decode("utf-8"), text, name)

    def test_ledger_uses_a_selected_vehicle_exact_rules_and_real_sync_routes(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        core = IOS_ROOT / "Sources/AnpFuelCore"
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        views = (shell / "LedgerViews.swift").read_text(encoding="utf-8")
        self.assertNotIn("public struct VehiclesAndLedgerSheetView", profile)
        self.assertEqual(views.count("public struct VehiclesAndLedgerSheetView: View {"), 1)
        editor = views.split("struct LedgerEntryEditorView: View {", 1)[1].split("struct LedgerIndicatorsSection", 1)[0]
        self.assertIn("selection: $ledgerVehicleId", editor)
        self.assertRegex(
            editor,
            r'Text\(\s*String\(localized:\s*"ledger\.select_vehicle"[^\n]*\)\)\s*\.tag\(nil as String\?\)',
        )
        self.assertIn("self._ledgerVehicleId = State(initialValue: existing?.vehicleId)", editor)
        self.assertIn(".disabled(ledgerVehicleId == nil)", editor)
        self.assertNotIn('?? "default"', views)
        # Indicators come only from the exact Core port of Android MonthlyIndicators; the former
        # "unavailable" placeholder is gone and every unknown result keeps its reason.
        self.assertNotIn("PremiumEnergyCalculator.IntervalInput", views)
        self.assertNotIn("Indisponíveis enquanto o diário", views)
        self.assertIn("ledger.indicators(vehicle: vehicle, year: year, month: month)", views)
        self.assertIn("LedgerCoordinator.reasonLabel(reason)", views)
        coordinator = (shell / "LedgerCoordinator.swift").read_text(encoding="utf-8")
        self.assertIn("try LedgerIndicators.monthly(", coordinator)
        form = (core / "LedgerEntryForm.swift").read_text(encoding="utf-8")
        self.assertIn("guard let vehicleId, let vehicle = vehicles.first(where: { $0.id == vehicleId }) else {", form)
        self.assertNotIn("Double(", form)
        indicators = (core / "LedgerIndicators.swift").read_text(encoding="utf-8")
        for reason in ("odometer-unit-unverified", "no-records", "phev-shared-distance", "odometer-missing",
                       "declared-fill-gap", "odometer-segment-boundary"):
            self.assertIn(f'"{reason}"', indicators)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        vehicle = state.split("func addVehicle(", 1)[1].split("func removeVehicle", 1)[0]
        editor = (core / "VehicleEditing.swift").read_text(encoding="utf-8")
        self.assertIn("draft.build(existing:", vehicle)
        self.assertIn("PortableMoney.parseBatteryCapacityWh", editor)
        self.assertNotIn("Double(batteryKwh", vehicle + editor)
        self.assertIn("ledger.admissionForNewVehicle()", vehicle)
        self.assertNotIn("VehicleRecord.maxFreeVehicles", vehicle)
        self.assertNotIn("func addLedgerEntry(", state)
        self.assertEqual(state.count("ledger.purgeOwnedData(accountId: ownerId)"), 2)

        client = (core / "LedgerSyncClient.swift").read_text(encoding="utf-8")
        backend = (ROOT / "backend/internal/modules/ledger/adapters/http/http.go").read_text(encoding="utf-8")
        for route in ("/v1/ledger/commands", "/v1/ledger/records", "/v1/ledger/consent", "/v1/ledger/erase"):
            self.assertIn(f'"{route}"', client)
            self.assertIn(f'r.Post("{route}"', backend)
        for path in ("commandsPath", "consentPath", "erasePath"):
            self.assertIn(f'transport.signedRequest(method: "POST", path: LedgerSyncWire.{path}', client)
        self.assertIn("transport.plainPost(path: LedgerSyncWire.recordsPath", client)
        self.assertIn("guard session.accountId == accountId else { throw LedgerSyncError.ownerMismatch }", client)

        tests = (IOS_ROOT / "Tests/AnpFuelCoreTests/LedgerSyncTests.swift").read_text(encoding="utf-8")
        api = ROOT / "contracts/testdata/api"
        commands = json.loads((api / "ledger-commands-valid.json").read_text(encoding="utf-8"))
        self.assertTrue(commands["valid"])
        value = commands["value"]
        self.assertIn(f'familyId: "{value["family_id"]}"', tests)
        self.assertIn(f'accessToken: "{value["access_token"]}"', tests)
        self.assertIn(f'operationId: "{value["commands"][0]["operation_id"]}"', tests)
        self.assertIn(f'"{value["commands"][0]["id"]}"', tests)
        page = json.loads((api / "ledger-page-valid.json").read_text(encoding="utf-8"))["value"]
        self.assertIn(f'dataset: .{page["dataset"]}, vehicleId: nil, limit: {page["limit"]}, cursor: nil', tests)
        consent = json.loads((api / "ledger-consent-valid.json").read_text(encoding="utf-8"))["value"]
        self.assertIn(f'datasets: [.{consent["datasets"][0]}], enabled: true', tests)
        erase = json.loads((api / "ledger-erase-valid.json").read_text(encoding="utf-8"))["value"]
        self.assertEqual(set(erase), {"family_id", "access_token"})
        self.assertIn('Set(erase.keys), ["family_id", "access_token"]', tests)
        invalid = json.loads((api / "ledger-consent-invalid.json").read_text(encoding="utf-8"))["value"]
        self.assertIn('\\"datasets\\":[\\"' + invalid["datasets"][0] + '\\"]', tests)

    def test_ledger_tests_replay_the_shared_premium_corpora_verbatim(self):
        fixtures = (IOS_ROOT / "Tests/AnpFuelCoreTests/LedgerFixtures.swift").read_text(encoding="utf-8")
        premium = ROOT / "contracts/testdata/premium"
        expected_lines = {"financial-execution-v1.csv": 37, "energy-execution-v1.csv": 12, "financial-wire-v1.csv": 23}
        for name, count in expected_lines.items():
            lines = [line for line in (premium / name).read_text(encoding="utf-8").splitlines() if line]
            self.assertEqual(len(lines), count, name)
            for line in lines:
                self.assertIn("        " + line + "\n", fixtures, f"{name}: {line}")
        registration = json.loads((premium / "vehicle-registration-v2.json").read_text(encoding="utf-8"))
        self.assertEqual(registration["version"], 2)
        self.assertEqual(fixtures.count("RegistrationCase(id: "), len(registration["cases"]))
        for case in registration["cases"]:
            self.assertIn(
                f'RegistrationCase(id: "{case["id"]}", registeredBefore: {case["registered_before"]}, '
                f'newDistinctIds: {case["new_distinct_ids"]}, scope: "{case["scope"]}", '
                f'authority: "{case["authority"]}", outcome: "{case["outcome"]}", '
                f'registeredAfter: {case["registered_after"]})',
                fixtures,
            )
        tests = IOS_ROOT / "Tests/AnpFuelCoreTests"
        math = (tests / "LedgerMathTests.swift").read_text(encoding="utf-8")
        self.assertIn("LedgerFixtures.financialExecution", math)
        self.assertIn("XCTAssertEqual(lines.count, 37)", math)
        self.assertIn("LedgerFixtures.energyExecution", math)
        self.assertIn("XCTAssertEqual(combinedRows, 2)", math)
        self.assertIn("LedgerFixtures.vehicleRegistration", math)
        models = (tests / "LedgerModelsTests.swift").read_text(encoding="utf-8")
        self.assertIn("LedgerFixtures.financialWire", models)

    def test_vehicle_photos_stay_local_and_purge_with_vehicle_and_owner(self):
        views = (IOS_ROOT / "Sources/AnpFuelShell/VehicleEditorViews.swift").read_text(encoding="utf-8")
        self.assertIn("PhotosPicker(", views)
        self.assertIn("VehicleLocalPhoto(sanitizing: bytes)", views)
        self.assertNotIn("URLSession", views)

        vehicle = (IOS_ROOT / "Sources/AnpFuelCore/VehicleEditing.swift").read_text(encoding="utf-8")
        self.assertIn("scope.storageKey", vehicle)
        self.assertIn("options.isExcludedFromBackup = true", vehicle)
        self.assertIn("data.write(to: file, options: [.atomic, .completeFileProtection])", vehicle)

        state = (IOS_ROOT / "Sources/AnpFuelShell/AppStateViewModel.swift").read_text(encoding="utf-8")
        for function in ("setApiEnvironment", "logout", "finishAccountDeletion"):
            start = state.index(f"func {function}(")
            end = state.index("\n    }", start)
            self.assertIn("purgeVehiclePreferences()", state[start:end], function)
        start = state.index("func removeVehicles(")
        end = state.index("\n    }", start)
        self.assertIn("vehicleDetails.remove(id: id)", state[start:end])

    def test_manual_shift_uses_selected_vehicle_exact_money_and_clock(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        driver = profile.split("public struct DriverHubSheetView: View {", 1)[1].split(
            "public struct ArcadeGamesSheetView", 1
        )[0]
        self.assertIn("selection: $driverVehicleId", driver)
        self.assertIn("PortableMoney.parseBrlCentavos", driver)
        self.assertNotIn('"driver-vehicle"', driver)
        self.assertNotIn("Double(cleaned)", driver)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        self.assertIn("startedAtLabel: Date().ISO8601Format()", state)
        self.assertIn("endedAtLabel: Date().ISO8601Format()", state)

    def test_driver_gps_uses_shared_location_and_only_persists_aggregate(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        self.assertIn("NavigationCoordinator(location: continuousLocation)", state)
        self.assertIn("DriverGpsCoordinator(location: continuousLocation)", state)
        for function in ("setApiEnvironment", "logout", "finishAccountDeletion"):
            start = state.index(f"func {function}(")
            end = state.index("\n    }", start)
            self.assertIn("purgeDriverGps()", state[start:end], function)

        gps = (shell / "DriverGpsCoordinator.swift").read_text(encoding="utf-8")
        self.assertLess(gps.index("store.begin("), gps.index("location.start(consumer: .driver)"))
        self.assertIn("location.stop(consumer: .driver)", gps)
        self.assertNotIn("CLLocationManager", gps)
        self.assertNotIn("URLSession", gps)

        journal = (IOS_ROOT / "Sources/AnpFuelCore/DriverGpsDistance.swift").read_text(encoding="utf-8")
        self.assertIn("scope.storageKey", journal)
        self.assertIn("exclusion.isExcludedFromBackup = true", journal)
        self.assertIn(".completeFileProtection", journal)
        for coordinate in ("latitude", "longitude", "CLLocation"):
            self.assertNotIn(coordinate, journal)

    def test_info_plist_privacy_manifest_and_entitlements_are_valid_and_safe(self):
        with (IOS_ROOT / "App/Info.plist").open("rb") as fh:
            info = plistlib.load(fh)
        self.assertEqual(info["CFBundleDisplayName"], "abastevo")
        self.assertEqual(info["CFBundleDevelopmentRegion"], "en")
        self.assertEqual(info["CFBundleShortVersionString"], "3.1.18")
        self.assertEqual(info["CFBundleVersion"], "30118")
        self.assertIn("NSLocationWhenInUseUsageDescription", info)
        self.assertNotIn("NSLocationAlwaysUsageDescription", info)
        self.assertNotIn("NSLocationAlwaysAndWhenInUseUsageDescription", info)
        self.assertCountEqual(info["UIBackgroundModes"], ["location", "audio"])
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
        self.assertIn("developmentRegion = en;", pbx)
        known_regions = re.search(r"knownRegions = \(([^)]+)\);", pbx)
        self.assertIsNotNone(known_regions)
        self.assertEqual(
            {name.strip().strip('"') for name in known_regions.group(1).split(",")},
            {"Base", "en", "pt-BR", "es", "de", "fr", "ja", "ru", "zh-Hans"},
        )
        self.assertEqual(re.findall(r"SWIFT_VERSION = ([^;]+);", pbx), ["5.0", "5.0"])
        self.assertIn('SWIFT_VERSION: "5.0"', (IOS_ROOT / "project.yml").read_text())
        self.assertIn("ENABLE_TESTABILITY = YES;", pbx)

        ET.parse(IOS_ROOT / "AnpFuel.xcodeproj/xcshareddata/xcschemes/AnpFuel.xcscheme")

        icon_catalog = json.loads((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset/Contents.json").read_text(encoding="utf-8"))
        for entry in icon_catalog["images"]:
            self.assertTrue((IOS_ROOT / "Assets.xcassets/AppIcon.appiconset" / entry["filename"]).is_file(), entry["filename"])

    def test_xcode_project_registers_every_swift_source_and_catalog(self):
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
        canonical_text = models_text.split('public static let fullTextPtBr = """\n', 1)[1].split('\n"""', 1)[0]
        self.assertEqual(canonical_text.encode("utf-8") + b"\n", backend_terms)
        self.assertIn('Data((fullTextPtBr + "\\n").utf8)', models_text)

        views = (IOS_ROOT / "Sources/AnpFuelShell/TrainingConsentViews.swift").read_text(encoding="utf-8")
        self.assertIn("TrainingTermsContract.verifiedDigestMatches()", views)
        self.assertIn("android.privacy_training_unavailable", views)
        self.assertIn("android.privacy_rights_body", views)
        consent_wire = (IOS_ROOT / "Sources/AnpFuelCore/TrainingConsent.swift").read_text(encoding="utf-8")
        self.assertIn('statusPath = "/v1/privacy/training-consent"', consent_wire)
        self.assertIn("signedOwnerGet(", consent_wire)
        self.assertIn("requiresDeviceProof: true", consent_wire)
        for source in IOS_ROOT.glob("Sources/**/*Photo*.swift"):
            self.assertNotIn("TrainingConsent", source.read_text(encoding="utf-8"), source.name)

        energy_fixture = json.loads((ROOT / "contracts/testdata/premium/energy-v1.json").read_text(encoding="utf-8"))
        self.assertEqual(energy_fixture["cases"][0]["expected_wh_per_km"], "800/3")
        test_source = (IOS_ROOT / "Tests/AnpFuelCoreTests/DomainAndFeaturesTests.swift").read_text(encoding="utf-8")
        self.assertIn("for vector in intervals", test_source)
        pbx = (IOS_ROOT / "AnpFuel.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
        self.assertIn("../contracts/testdata/premium/energy-v1.json", pbx)
        self.assertIn("energy-v1.json in Resources", pbx)

    def test_signed_transport_replays_the_backend_identity_vectors(self):
        tests = (IOS_ROOT / "Tests/AnpFuelCoreTests/SignedRequestTransportTests.swift").read_text(encoding="utf-8")
        vectors = {}
        for path in sorted((ROOT / "contracts/testdata/identity").glob("*.json")):
            vector = json.loads(path.read_text(encoding="utf-8"))
            vectors[vector["id"]] = vector
        for vector_id in ("auth-valid-register", "auth-valid-body"):
            vector = vectors[vector_id]
            self.assertTrue(vector["valid"])
            self.assertIn(vector["signature"], tests, vector_id)
            for line in vector["base_lines"]:
                self.assertIn(json.dumps(line), tests, vector_id)
        register = vectors["auth-valid-register"]
        for key in ("jwk_x", "jwk_y", "private_d"):
            self.assertIn(f'"{register[key]}"', tests)
        tampered = [vid for vid, v in vectors.items() if not v["valid"] and vid not in ("auth-expired", "auth-wrong-key", "auth-der-encoding")]
        self.assertGreaterEqual(len(tampered), 7)
        for vector_id in tampered:
            vector = vectors[vector_id]
            self.assertIn(f'id: "{vector_id}"', tests)
            self.assertIn(vector["signature"], tests, vector_id)
            for line in vector["base_lines"]:
                self.assertIn(json.dumps(line), tests, vector_id)
        self.assertIn(vectors["auth-der-encoding"]["signature"], tests)

        transport = (IOS_ROOT / "Sources/AnpFuelCore/SignedRequestTransport.swift").read_text(encoding="utf-8")
        backend = (ROOT / "backend/internal/modules/identity/adapters/auth/auth.go").read_text(encoding="utf-8")
        for header in ("Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce"):
            self.assertIn(f'"{header}"', backend)
            self.assertIn(f'"{header}"', transport)
        proof = (IOS_ROOT / "Sources/AnpFuelCore/PortableAnonymousProof.swift").read_text(encoding="utf-8")
        self.assertIn("SHA512.hash(data: baseData)", proof)
        self.assertIn("SHA256.hash(data: Data(statement.utf8))", proof)
        self.assertIn("kSecAttrSynchronizable as String: false", (IOS_ROOT / "Sources/AnpFuelCore/DeviceIdentityFlow.swift").read_text(encoding="utf-8"))

    def test_identity_login_and_community_writes_use_real_routes(self):
        core = IOS_ROOT / "Sources/AnpFuelCore"
        identity = (core / "DeviceIdentityFlow.swift").read_text(encoding="utf-8")
        for route in ("/v1/identity/challenges", "/v1/contributors", "/v1/accounts/bindings/bind"):
            self.assertIn(f'"{route}"', identity)
        writes = (core / "CommunityWriteClient.swift").read_text(encoding="utf-8")
        for route in ("/v1/observations", "/v1/contributors/me/observations", "/v1/photo-captures", "/v1/uploads",
                      "/v1/feedback/comments", "/v1/forum/topics", "/v1/forum/posts/"):
            self.assertIn(f'"{route}', writes)
        self.assertIn('"region_uf"', writes)
        self.assertNotIn('"region":', writes)
        provider = (core / "AccountProviderLogin.swift").read_text(encoding="utf-8")
        self.assertIn('"/v1/accounts/providers/login"', provider)
        self.assertIn('"authorization_code"', provider)

        shell = IOS_ROOT / "Sources/AnpFuelShell"
        apple = (shell / "CommunityPublishingViews.swift").read_text(encoding="utf-8")
        self.assertIn("SignInWithAppleButton", apple)
        self.assertIn("request.requestedScopes = []", apple)
        self.assertIn("request.nonce = viewModel.prepareAppleSignInNonce()", apple)
        profile = (shell / "ProfileTabViews.swift").read_text(encoding="utf-8")
        self.assertIn("AppleSignInSection(viewModel: viewModel)", profile)
        self.assertNotIn("Indisponível nesta versão.", profile)

        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        auth = state.split("// MARK: - Auth & Account Management", 1)[1].split("// MARK: - Contributions and Photo Pipeline", 1)[0]
        for call in ("requestEmailCode", "consumeEmailCode", "createKeyAccount", "loginWithKey", "loginWithProvider", "logout()", "deleteAccount()"):
            self.assertNotIn(f"switch authFlow.{call}", auth)
        self.assertIn("AuthBlockingWork.run", auth)
        self.assertNotIn("savePriceContributionDraft", state)
        self.assertNotIn("saveStationCommentDraft", state)
        self.assertNotIn("saveForumTopicDraft", state)

    def test_map_routes_and_download_state_have_no_sample_data(self):
        shell = IOS_ROOT / "Sources/AnpFuelShell"
        explore = (shell / "ExploreTabViews.swift").read_text(encoding="utf-8")
        maps = (shell / "MapTabViews.swift").read_text(encoding="utf-8")
        self.assertIn("NativeRouteSheetView(viewModel: viewModel, station: station)", explore)
        self.assertIn("station.pinTier.allowsMapPin", maps)
        view = (shell / "ActiveNavigationViews.swift").read_text(encoding="utf-8")
        guidance = (shell / "NavigationCoordinator.swift").read_text(encoding="utf-8")
        state = (shell / "AppStateViewModel.swift").read_text(encoding="utf-8")
        audio = (IOS_ROOT / "Sources/AnpFuelCore/IosPlatformServices.swift").read_text(encoding="utf-8")
        self.assertIn("await viewModel.requestRouteOrigin()", view)
        self.assertIn("MKDirections(request: request)", guidance)
        self.assertIn("try await directions.calculate()", guidance)
        self.assertIn("MKLaunchOptionsDirectionsModeDriving", view)
        self.assertIn("location.start {", guidance)
        self.assertIn("location.stop()", guidance)
        self.assertIn("NavigationGuidancePolicy.nativeLocaleFor", guidance)
        self.assertIn("AVSpeechSynthesizer()", audio)
        self.assertIn(".duckOthers", audio)
        self.assertGreaterEqual(state.count("navigation.stop()"), 3)
        offline = (shell / "OfflineNavigationViews.swift").read_text(encoding="utf-8")
        # B-BR-IOS-01: offline navigation hands off to Apple Maps offline maps;
        # the app never claims its own regional package is available.
        self.assertRegex(offline, r'Text\(\s*String\(localized:\s*"map\.offline\.unavailable"')
        self.assertRegex(offline, r'Label\(\s*String\(localized:\s*"map\.offline\.apple_maps"')
        self.assertIn("https://maps.apple.com/", offline)
        self.assertNotIn("FileManager", offline)
        store = (IOS_ROOT / "Sources/AnpFuelCore/IosLocalStore.swift").read_text(encoding="utf-8")
        self.assertNotIn("downloadedMapRegions", store)


if __name__ == "__main__":
    unittest.main(verbosity=2)
