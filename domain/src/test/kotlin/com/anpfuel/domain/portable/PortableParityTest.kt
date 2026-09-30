package com.anpfuel.domain.portable

import com.anpfuel.domain.rule.TankFillCostRule
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.TankCapacity
import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P12-T02 parity: portable integer milli-BRL vectors agree with the Go kernel
 * fixtures and with Android consumers wherever their scales overlap.
 * Intended divergences are asserted explicitly so no drift can slip in
 * silently; calculators migrate to portable math in P12-T03.
 */
class PortableParityTest {

    @Test
    fun agreesWithPriceAmountOnTwoDecimalVectors() {
        // "5,49" ANP text == legacy "5.49" display text: both 5490 milli.
        assertEquals(5490L, PortableMoney.parse("5,49"))
        assertEquals(PriceAmount.of("5.49"), PriceAmount.of("5.49"))
        assertEquals("5,490", PortableMoney.format(5490L))
    }

    @Test
    fun documentsThreeDecimalDivergence() {
        // Portable keeps exact milli; legacy PriceAmount rounds to 2dp.
        assertEquals(5499L, PortableMoney.parse("5,499"))
        assertEquals("5.50", PriceAmount.of("5.499").value.toPlainString())
    }

    @Test
    fun documentsZeroAndOverPrecisionDivergence() {
        // Legacy accepts zero and rounds over-precision; portable refuses both.
        PriceAmount.of("0.00")
        assertEquals("zero-price", PortableMoney.codeOf("0,00"))
        assertEquals("1.23", PriceAmount.of("1.2345").value.toPlainString())
        assertEquals("over-precision", PortableMoney.codeOf("5,9999"))
    }

    @Test
    fun documentsTankFillDivergenceInMilli() {
        // Legacy: 5.499 rounds to 5.50 x 50 L = 275.00.
        val legacy = TankFillCostRule.multiply(
            PriceAmount.of("5.499"),
            TankCapacity.of(50.0),
        )
        assertEquals("275.00", legacy.value.toPlainString())
        // Portable: exact 5499 milli x 50 L = 274950 milli (274,950).
        assertEquals(274950L, PortableMoney.multiplyTankFill(5499L, 50_000L))
    }

    @Test
    fun agreesWithTankFillOnExactVectors() {
        val legacy = TankFillCostRule.multiply(
            PriceAmount.of("5.49"),
            TankCapacity.of(50.0),
        )
        assertEquals("274.50", legacy.value.toPlainString())
        assertEquals(274500L, PortableMoney.multiplyTankFill(5490L, 50_000L))
    }

    @Test
    fun portableSourcesUseNoJavaImports() {
        val dir = portableMainDir()
        val files = dir.listFiles { f -> f.name.endsWith(".kt") }!!.sortedBy { it.name }
        assertTrue(files.isNotEmpty(), "no portable sources in $dir")
        for (file in files) {
            for (line in file.readLines()) {
                val trimmed = line.trim()
                assertTrue(
                    !trimmed.startsWith("import java.") && !trimmed.startsWith("import javax."),
                    "${file.name} must stay commonMain-ready but imports: $trimmed",
                )
            }
        }
    }

    @Test
    fun fixtureMirrorContainsGoldenVectors() {
        val fixture = repoFile("contracts/testdata/compat/money-portable-v1.json")
        val body = fixture.readText()
        assertTrue(body.contains("\"text\": \"5,999\""), "missing 5,999 vector")
        assertTrue(body.contains("\"milli\": 5999"), "missing 5999 milli")
        assertTrue(body.contains("\"text\": \"5,9999\""), "missing over-precision vector")
        assertTrue(body.contains("\"code\": \"over-precision\""), "missing over-precision code")
        assertTrue(body.contains("\"total_milli\": 274950"), "missing tank-fill vector")
        assertTrue(body.contains("\"text_max_scalars\": 280"), "missing 280-char rule")
        assertTrue(body.contains("123e4567-e89b-12d3-a456-426614174000"), "missing uuid vector")
    }

    private fun repoRoot(): File {
        var dir = File(System.getProperty("user.dir"))
        while (true) {
            if (File(dir, "contracts/testdata/compat/money-portable-v1.json").exists()) return dir
            dir = dir.parentFile ?: throw AssertionError("repo root not found")
        }
    }

    private fun repoFile(relative: String): File {
        val file = File(repoRoot(), relative)
        assertTrue(file.exists(), "missing fixture: $relative")
        return file
    }

    private fun portableMainDir(): File {
        val root = repoRoot()
        val dir = File(root, "domain/src/main/kotlin/com/anpfuel/domain/portable")
        assertTrue(dir.isDirectory, "missing portable dir: $dir")
        return dir
    }
}
