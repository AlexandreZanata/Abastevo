#!/usr/bin/env python3
"""EMP-PRIV-003 backup/layout guard; actual native acceptance is recorded separately."""
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / "data/src/main/kotlin/com/anpfuel/data"


class EnterpriseBackupBoundary(unittest.TestCase):
    def test_every_backup_channel_excludes_the_enterprise_location(self):
        old = ET.parse(ROOT / "app/src/main/res/xml/backup_rules.xml").getroot()
        modern = ET.parse(ROOT / "app/src/main/res/xml/data_extraction_rules.xml").getroot()
        for rules in (old, modern.find("cloud-backup"), modern.find("device-transfer")):
            self.assertIsNotNone(rules)
            exclusions = {(e.get("domain"), e.get("path")) for e in rules.findall("exclude")}
            self.assertIn(("root", "no_backup/enterprise-sealed-v1/"), exclusions)
            for domain in ("root", "file", "external"):
                self.assertIn((domain, "enterprise-sealed-v1/"), exclusions)
            self.assertIn(("sharedpref", "."), exclusions)
            self.assertIn(("database", "."), exclusions)

    def test_actual_graph_uses_no_backup_and_accepted_protected_ports(self):
        source = (DATA / "di/EnterpriseModule.kt").read_text()
        self.assertIn('File(context.noBackupFilesDir, "enterprise-sealed-v1")', source)
        self.assertIn("fun persistence(store: ProtectedEnterprisePersistence): EnterprisePersistence = store", source)
        self.assertIn("fun contextProtection(store: ProtectedEnterprisePersistence): EnterpriseContextProtection = store", source)
        self.assertIn("fun contactAuthority(client: EnterpriseDeviceContactClient): EnterpriseContactAuthority = client", source)
        self.assertNotIn("UnavailableEnterprise", source)
        self.assertNotIn("Room.databaseBuilder", source)
        manifest = ET.parse(ROOT / "app/src/main/AndroidManifest.xml").getroot()
        application = manifest.find("application")
        android = "{http://schemas.android.com/apk/res/android}"
        self.assertEqual("@xml/backup_rules", application.get(android + "fullBackupContent"))
        self.assertEqual("@xml/data_extraction_rules", application.get(android + "dataExtractionRules"))

    def test_global_session_and_background_boundaries_are_wired(self):
        auth = (DATA / "di/AuthModule.kt").read_text()
        self.assertIn("EnterpriseFencedSessionStore(KeystoreSessionStore(prefs, keys), fence)", auth)
        self.assertIn("EnterpriseRevokingAccountApi(AccountHttpApi", auth)
        activity = (ROOT / "app/src/main/kotlin/com/anpfuel/app/MainActivity.kt").read_text()
        self.assertIn("override fun onStop()", activity)
        self.assertIn("enterpriseFence.isInitialized && !isChangingConfigurations) enterpriseFence.markBackground()", activity)
        application = (ROOT / "app/src/main/kotlin/com/anpfuel/app/AnpFuelApplication.kt").read_text()
        self.assertIn("enterpriseVaultMaintenance.maintain()", application)
        module = (DATA / "di/EnterpriseModule.kt").read_text()
        self.assertNotIn("fence: EnterpriseSessionFence = EnterpriseSessionFence()", module)

    def test_keystore_use_failures_are_classified(self):
        keys = (DATA / "enterprise/AndroidEnterpriseVaultKeys.kt").read_text()
        self.assertIn("is UserNotAuthenticatedException -> EnterpriseKeyUse.UNLOCK_REQUIRED", keys)
        self.assertIn("is KeyPermanentlyInvalidatedException -> EnterpriseKeyUse.INVALIDATED", keys)


if __name__ == "__main__":
    unittest.main()
