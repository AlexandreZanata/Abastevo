package com.anpfuel.domain.contract

import com.anpfuel.domain.portable.PortableMoney
import com.anpfuel.domain.rule.FuelProductNormalizationRule
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T01: Kotlin contract harness and compatibility adapters.
 *
 * Freezes the agreed fixtures shared by Go and Kotlin so legacy
 * fuel/CNPJ/precision differences stay explicit:
 * - `contracts/testdata/compat/legacy-deltas.json` (A01-A13 deltas),
 * - `contracts/testdata/compat/money-portable-v1.json` (exact milli-BRL),
 * - `contracts/testdata/anp/manifest.json` (identical vs legacy-divergent).
 *
 * Legacy behavior is preserved: [FuelProduct] keeps GASOLINE_PREMIUM,
 * [Cnpj] stays numeric-only, [PriceAmount] stays 2-decimal. The wire
 * bridge (GASOLINE_ADDITIVED), alphanumeric CNPJ and exact milli-BRL
 * live in bounded data adapters owned by this harness.
 */
class P10ContractHarnessTest {

    @Test
    fun legacyDeltasFreezeWireVocabulary() {
        val body = repoFile("contracts/testdata/compat/legacy-deltas.json").readText()
        assertTrue(body.contains("GASOLINE_ADDITIVED"), "wire enum missing")
        assertTrue(body.contains("GASOLINE_PREMIUM"), "legacy map missing")
        assertTrue(
            body.contains("GASOLINE_PREMIUM never appears on the wire"),
            "A05 rule missing",
        )
    }

    @Test
    fun legacyFuelLabelStillMapsToPremium() {
        // Legacy enum name is preserved; the wire bridge is asserted in
        // :data WireFuelMapperTest, never by renaming this enum.
        assertEquals(
            FuelProduct.GASOLINE_PREMIUM,
            FuelProductNormalizationRule.normalize("GASOLINA ADITIVADA").getOrThrow(),
        )
    }

    @Test
    fun unknownFuelEnumRefusedWithoutCoercion() {
        assertTrue(FuelProductNormalizationRule.normalize("COMBUSTIVEL DESCONHECIDO").isFailure)
        assertTrue(FuelProductNormalizationRule.normalize("").isFailure)
    }

    @Test
    fun legacyCnpjStaysNumericOnly() {
        val numeric = Cnpj.parse("04.218.406/0001-04")
        assertEquals("04218406000104", numeric.digits)
        // Alphanumeric backend identifiers are NOT accepted by the legacy
        // value: the legacy digit filter leaves fewer than 14 digits and
        // the constructor refuses. The backend adapter (BackendCnpjMapper
        // in :data, asserted separately) owns them without coercion.
        var refused = false
        try {
            Cnpj.parse("12ABC34501DE35")
        } catch (e: Exception) {
            refused = true
        }
        assertTrue(refused, "legacy Cnpj must refuse alphanumeric input")
    }

    @Test
    fun exactMoneyVectorsAgreeWithPortable() {
        val body = repoFile("contracts/testdata/compat/money-portable-v1.json").readText()
        assertTrue(body.contains("\"text\": \"5,999\""))
        assertTrue(body.contains("\"milli\": 5999"))
        assertEquals(5999L, PortableMoney.parse("5,999"))
        assertEquals("over-precision", PortableMoney.codeOf("5,9999"))
        assertEquals("zero-price", PortableMoney.codeOf("0,00"))
        assertEquals("over-range", PortableMoney.codeOf("1000,01"))
    }

    @Test
    fun legacyPriceAmountDivergenceDocumented() {
        // Legacy rounds to 2dp; portable keeps exact milli and refuses.
        assertEquals(5499L, PortableMoney.parse("5,499"))
        assertEquals("5.50", PriceAmount.of("5.499").value.toPlainString())
        assertEquals("over-precision", PortableMoney.codeOf("5,9999"))
        assertEquals("1.23", PriceAmount.of("1.2345").value.toPlainString())
    }

    @Test
    fun anpManifestKeepsCompatibilityClasses() {
        val body = repoFile("contracts/testdata/anp/manifest.json").readText()
        assertTrue(body.contains("legacy-divergent"))
        assertTrue(body.contains("backend-quarantine"))
        assertTrue(body.contains("04218406000104"))
        assertTrue(body.contains("12ABC34501DE35"))
    }

    private fun repoRoot(): File {
        var dir = File(System.getProperty("user.dir"))
        while (true) {
            if (File(dir, "contracts/testdata/compat/legacy-deltas.json").exists()) return dir
            dir = dir.parentFile ?: throw AssertionError("repo root not found")
        }
    }

    private fun repoFile(relative: String): File {
        val file = File(repoRoot(), relative)
        assertTrue(file.exists(), "missing fixture: $relative")
        return file
    }
}
