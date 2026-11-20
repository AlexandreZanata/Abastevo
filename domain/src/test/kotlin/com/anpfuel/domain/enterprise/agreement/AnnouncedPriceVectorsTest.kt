package com.anpfuel.domain.enterprise.agreement

import java.io.File
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

/**
 * EMP-STN-002..004 golden vectors: the Kotlin station commerce rules must
 * match the shared announced-price PSV fixture the Go backend consumes.
 */
class AnnouncedPriceVectorsTest {
    private fun loadVectors(): List<String> {
        val candidates = listOf(
            File("../contracts/testdata/enterprise/commerce/announced-price-v1.psv"),
            File("contracts/testdata/enterprise/commerce/announced-price-v1.psv"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing announced-price-v1.psv fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').filter { it.contains('=') }.associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    private fun epoch(raw: String): Long = Instant.parse(raw).epochSecond

    @Test
    fun `matches canonical announced price and promotion golden vectors`() {
        val rows = loadVectors()
        assertEquals(19, rows.size)
        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])
            when (op) {
                "DISPLAY_STATE" -> {
                    val display = AnnouncedPrice.displayState(
                        args["state"]!!, epoch(args["now"]!!), epoch(args["valid_from"]!!), epoch(args["valid_until"]!!),
                    )
                    assertEquals(exp["display_state"], display, id)
                }
                "ANNOUNCE" -> {
                    val outcome = runCatching {
                        AnnouncedPrice(
                            product = args["product"].orEmpty(),
                            unit = args["unit"].orEmpty(),
                            priceMilliBrl = args["price_milli_brl"]?.toLongOrNull() ?: 0L,
                            conditions = args["conditions"].orEmpty(),
                            fromEpochSeconds = epoch(args["valid_from"]!!),
                            untilEpochSeconds = epoch(args["valid_until"]!!),
                        )
                    }
                    if (exp["status"] == "REJECTED") {
                        assertEquals(exp["error"], outcome.exceptionOrNull()?.message, id)
                    } else {
                        assertNull(outcome.exceptionOrNull(), id)
                    }
                }
                "PROMOTION_STATE" -> {
                    val display = Promotion.displayState(
                        args["state"]!!, epoch(args["now"]!!), epoch(args["valid_from"]!!), epoch(args["valid_until"]!!),
                    )
                    assertEquals(exp["display_state"], display, id)
                }
                "PROMOTE" -> {
                    val outcome = runCatching {
                        Promotion(
                            label = args["label"].orEmpty(),
                            description = args["description"].orEmpty(),
                            product = args["fuel_product"].orEmpty().ifEmpty { null },
                            fromEpochSeconds = epoch(args["valid_from"]!!),
                            untilEpochSeconds = epoch(args["valid_until"]!!),
                        )
                    }
                    if (exp["status"] == "REJECTED") {
                        assertEquals(exp["error"], outcome.exceptionOrNull()?.message, id)
                    } else {
                        assertNull(outcome.exceptionOrNull(), id)
                    }
                }
            }
        }
    }
}
