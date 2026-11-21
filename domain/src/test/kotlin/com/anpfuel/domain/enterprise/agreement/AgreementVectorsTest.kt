package com.anpfuel.domain.enterprise.agreement

import java.io.File
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

/**
 * EMP-AGREE-001..006 golden vectors: the Kotlin domain rules must match the
 * shared PSV fixtures the Go backend consumes (agreement lifecycle, voucher
 * limits, redemption idempotency and the announced commercial source).
 */
class AgreementVectorsTest {
    private fun loadVectors(name: String): List<String> {
        val base = "enterprise/agreement"
        val candidates = listOf(
            File("../contracts/testdata/$base/$name"),
            File("contracts/testdata/$base/$name"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing $name fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').filter { it.contains('=') }.associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    private fun epoch(raw: String): Long = Instant.parse(raw).epochSecond

    private fun termsOf(raw: String): List<AgreementTerm> =
        raw.split(',').filter { it.isNotBlank() }.map { part ->
            val (key, price) = part.split(':', limit = 2)
            val (product, unit) = key.split('/', limit = 2)
            AgreementTerm(product = product, unit = unit, priceMilliBrl = price.toLong())
        }

    private fun expectRejected(err: Exception?, want: String, id: String = "?") {
        val actual = err ?: throw IllegalArgumentException("missing rejection $want at $id")
        assertEquals(want, actual.message, id)
    }

    @Test
    fun `matches canonical agreement lifecycle golden vectors`() {
        val rows = loadVectors("agreement-lifecycle-v1.psv")
        assertEquals(18, rows.size)
        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])
            when (op) {
                "PROPOSE" -> {
                    val terms = termsOf(args["terms"].orEmpty())
                    val err = runCatching {
                        AgreementHead.propose(epoch(args["valid_from"]!!), epoch(args["valid_until"]!!), terms).state
                    }.exceptionOrNull()
                    if (exp["status"] == "REJECTED") {
                        expectRejected(err as? Exception, expValue(exp), id)
                    } else {
                        val head = runCatching {
                            AgreementHead.propose(epoch(args["valid_from"]!!), epoch(args["valid_until"]!!), terms)
                        }.getOrThrow()
                        assertEquals("PROPOSED", head.state.name)
                        assertEquals(1L, head.revision)
                    }
                }
                "ACCEPT", "DECLINE", "SUSPEND", "END" -> {
                    val head = AgreementHead(AgreementState.valueOf(args["state"]!!))
                    val next = runCatching {
                        when (op) {
                            "ACCEPT" -> head.accept()
                            "DECLINE" -> head.decline()
                            "SUSPEND" -> head.suspend()
                            else -> head.end()
                        }
                    }
                    if (exp["status"] == "REJECTED") {
                        expectRejected(next.exceptionOrNull() as? Exception, expValue(exp), id)
                    } else {
                        assertEquals(exp["state"], next.getOrThrow().state.name, id)
                        assertEquals(exp["revision"]!!.toLong(), next.getOrThrow().revision, id)
                    }
                }
                "USABLE" -> {
                    val head = AgreementHead(
                        state = AgreementState.valueOf(args["state"]!!),
                        fromEpochSeconds = epoch(args["valid_from"]!!),
                        untilEpochSeconds = epoch(args["valid_until"]!!),
                    )
                    assertEquals(exp["usable"] == "true", head.usableAt(epoch(args["now"]!!)), id)
                    val derived = AgreementState.derived(args["state"]!!, epoch(args["now"]!!), epoch(args["valid_from"]!!), epoch(args["valid_until"]!!))
                    if (exp.containsKey("derived_state")) assertEquals(exp["derived_state"], derived, id)
                }
            }
        }
    }

    @Test
    fun `matches canonical voucher limits golden vectors`() {
        val rows = loadVectors("voucher-limits-v1.psv")
        assertEquals(16, rows.size)
        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])
            when (op) {
                "ISSUE" -> {
                    val issued = runCatching {
                        Voucher.issue(
                            args["product"]!!, args["unit"]!!,
                            args["max_quantity_milli"]!!.toLong(),
                            args["max_centavos"]!!.toLong(),
                            epoch(args["valid_until"]!!), epoch(args["now"]!!),
                        )
                    }
                    if (exp["status"] == "REJECTED") {
                        expectRejected(issued.exceptionOrNull() as? Exception, expValue(exp), id)
                    } else {
                        assertEquals("ISSUED", issued.getOrThrow().state.name, id)
                        assertEquals(args["max_quantity_milli"]!!.toLong(), issued.getOrThrow().maxQuantityMilli, id)
                    }
                }
                "CANCEL" -> {
                    val voucher = Voucher(VoucherState.valueOf(args["state"]!!), "DIESEL_S10", "L", 1L, 1L, 0L)
                    val cancelled = runCatching {
                        require(voucher.state == VoucherState.ISSUED) { AgreementError.AUTHORIZATION_TRANSITION_REFUSED.name }
                        voucher.copy(state = VoucherState.CANCELLED)
                    }
                    if (exp["status"] == "REJECTED") {
                        expectRejected(cancelled.exceptionOrNull() as? Exception, expValue(exp), id)
                    } else {
                        assertEquals(exp["state"], cancelled.getOrThrow().state.name, id)
                    }
                }
                "ELIGIBLE" -> {
                    val voucher = Voucher(VoucherState.valueOf(args["state"]!!), "DIESEL_S10", "L", 1L, 1L, epoch(args["valid_until"]!!))
                    val eligible = voucher.eligibleAt(epoch(args["now"]!!))
                    assertEquals(exp["eligible"] == "true", eligible, id)
                    assertEquals(exp["derived_state"], voucher.derivedStateAt(epoch(args["now"]!!)).name, id)
                }
                "ATTEMPT" -> {
                    val voucher = Voucher(
                        VoucherState.valueOf(args["state"]!!), "DIESEL_S10", "L", 1L, 1L, 0L,
                        failedAttempts = args["failed_attempts"]!!.toInt(),
                    )
                    val next = runCatching { voucher.failedAttempt() }
                    if (exp["status"] == "REJECTED") {
                        expectRejected(next.exceptionOrNull() as? Exception, expValue(exp), id)
                    } else {
                        assertEquals(exp["state"], next.getOrThrow().state.name, id)
                        assertEquals(exp["failed_attempts"]!!.toInt(), next.getOrThrow().failedAttempts, id)
                    }
                }
            }
        }
    }

    @Test
    fun `matches canonical redemption idempotency golden vectors`() {
        val rows = loadVectors("redemption-idempotency-v1.psv")
        assertEquals(16, rows.size)
        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])
            when (op) {
                "COMPUTE" -> {
                    val total = AgreementRules.totalCentavos(args["quantity_milli"]!!.toLong(), args["price_milli_brl"]!!.toLong())
                    assertEquals(exp["total_centavos"]!!.toLong(), total, id)
                }
                "REDEEM" -> {
                    val voucher = Voucher(
                        state = VoucherState.valueOf(args["state"]!!),
                        product = args["voucher_product"]!!,
                        unit = args["voucher_unit"]!!,
                        maxQuantityMilli = args["max_quantity_milli"]!!.toLong(),
                        maxCentavos = args["max_centavos"]!!.toLong(),
                        validUntilEpochSeconds = epoch(args["valid_until"]!!),
                    )
                    val outcome = runCatching {
                        voucher.consume(args["product"]!!, args["unit"]!!, args["quantity_milli"]!!.toLong(), args["total_centavos"]!!.toLong(), epoch(args["now"]!!))
                    }
                    if (exp["status"] == "REJECTED") {
                        expectRejected(outcome.exceptionOrNull() as? IllegalStateException, expValue(exp))
                    } else {
                        assertNull(outcome.exceptionOrNull(), id)
                    }
                }
                "REPLAY" -> assertEquals("true", exp["replayed"], id)
            }
        }
    }

    private fun expValue(exp: Map<String, String>): String = exp["error"]!!
}
