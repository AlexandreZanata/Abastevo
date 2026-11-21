package com.anpfuel.domain.enterprise.stock

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class StockValuationVectorsTest {
    private fun loadVectors(): List<String> {
        val candidates = listOf(
            File("../contracts/testdata/enterprise/stock/stock-valuation-v1.psv"),
            File("contracts/testdata/enterprise/stock/stock-valuation-v1.psv"),
        )
        val file = candidates.firstOrNull(File::isFile)
            ?: error("Missing stock-valuation-v1.psv fixture")
        return file.readLines().drop(1).filter { it.isNotBlank() }
    }

    private fun parseMap(raw: String): Map<String, String> =
        raw.split(';').filter { it.contains('=') }.associate { part ->
            val (k, v) = part.split('=', limit = 2)
            k to v
        }

    @Test
    fun `matches canonical stock valuation golden vectors`() {
        val rows = loadVectors()
        assertEquals(8, rows.size)

        for (line in rows) {
            val parts = line.split('|')
            val id = parts[0]
            val op = parts[1]
            val args = parseMap(parts[2])
            val exp = parseMap(parts[3])

            when (op) {
                "RECEIVE" -> {
                    val pool = StockPool(args.getValue("tank_product"), args.getValue("pool_ml").toLong(), args.getValue("pool_cents").toLong())
                    val out = pool.receive(args.getValue("product"), args.getValue("rx_ml").toLong(), args.getValue("rx_cents").toLong())
                    assertEquals(exp.getValue("status"), out.status, id)
                    assertEquals(exp["error"], out.error?.name, id)
                    assertEquals(exp.getValue("after_ml").toLong(), out.afterQuantityMl, id)
                    assertEquals(exp.getValue("after_cents").toLong(), out.afterValueCentavos, id)
                    exp["recognized_expense_cents"]?.toLong()?.let { assertEquals(it, out.recognizedExpenseCentavos, id) }
                }
                "DISPENSE" -> {
                    val pool = StockPool(args.getValue("tank_product"), args.getValue("pool_ml").toLong(), args.getValue("pool_cents").toLong())
                    val out = pool.dispense(args.getValue("product"), args.getValue("tx_ml").toLong())
                    assertEquals(exp.getValue("status"), out.status, id)
                    assertEquals(exp["error"], out.error?.name, id)
                    assertEquals(exp.getValue("after_ml").toLong(), out.afterQuantityMl, id)
                    assertEquals(exp.getValue("after_cents").toLong(), out.afterValueCentavos, id)
                    exp["dispense_cents"]?.toLong()?.let { assertEquals(it, out.movedCentavos, id) }
                    exp["recognized_expense_cents"]?.toLong()?.let { assertEquals(it, out.recognizedExpenseCentavos, id) }
                }
                "TRANSFER" -> {
                    val src = StockPool(args.getValue("src_product"), args.getValue("src_ml").toLong(), args.getValue("src_cents").toLong())
                    val dst = StockPool(args.getValue("dst_product"), args.getValue("dst_ml").toLong(), args.getValue("dst_cents").toLong())
                    val out = src.transferTo(dst, args.getValue("product"), args.getValue("tx_ml").toLong())
                    assertEquals(exp.getValue("status"), out.status, id)
                    assertEquals(exp.getValue("transfer_cents").toLong(), out.movedCentavos, id)
                    assertEquals(exp.getValue("src_after_ml").toLong(), out.srcAfterQuantityMl, id)
                    assertEquals(exp.getValue("src_after_cents").toLong(), out.srcAfterValueCentavos, id)
                    assertEquals(exp.getValue("dst_after_ml").toLong(), out.dstAfterQuantityMl, id)
                    assertEquals(exp.getValue("dst_after_cents").toLong(), out.dstAfterValueCentavos, id)
                    assertEquals(exp.getValue("recognized_expense_cents").toLong(), out.recognizedExpenseCentavos, id)
                }
                "ADJUST" -> {
                    val pool = StockPool("DIESEL_S10", args.getValue("pool_ml").toLong(), args.getValue("pool_cents").toLong())
                    val out = pool.adjust(args.getValue("delta_ml").toLong(), args.getValue("reason"))
                    assertEquals(exp.getValue("status"), out.status, id)
                    assertEquals(exp["error"], out.error?.name, id)
                    assertEquals(exp.getValue("after_ml").toLong(), out.afterQuantityMl, id)
                    assertEquals(exp.getValue("after_cents").toLong(), out.afterValueCentavos, id)
                }
                else -> error("Unknown operation $op")
            }
        }
    }

    @Test
    fun `covers adjustments transfers product mismatches and validation edges`() {
        assertEquals("MOVING_AVG_UNIT_V1", FuelStockValuation.METHOD_VERSION)
        assertEquals(0L, FuelStockValuation.proportionHalfUp(0L, 10L, 100L))
        val pool = StockPool("DIESEL_S10", 100_000L, 50_000L)
        assertEquals(StockError.VALIDATION_FAILED, pool.receive("DIESEL_S10", 0L, 100L).error)
        assertEquals(StockError.VALIDATION_FAILED, pool.receive("DIESEL_S10", 10L, -1L).error)
        assertEquals(StockError.STOCK_PRODUCT_MISMATCH, pool.dispense("ETHANOL", 10L).error)
        assertEquals(StockError.VALIDATION_FAILED, pool.dispense("DIESEL_S10", 0L).error)
        val allowNeg = StockPool("DIESEL_S10", 0L, 0L, allowNegative = true).dispense("DIESEL_S10", 100L)
        assertEquals("ACCEPTED", allowNeg.status)
        assertEquals(-100L, allowNeg.afterQuantityMl)
        assertEquals(0L, allowNeg.movedCentavos)

        val dstEthanol = StockPool("ETHANOL", 0L, 0L)
        assertEquals(StockError.STOCK_PRODUCT_MISMATCH, pool.transferTo(dstEthanol, "DIESEL_S10", 100L).error)
        assertEquals(StockError.STOCK_INSUFFICIENT, pool.transferTo(pool.copy(), "DIESEL_S10", 200_000L).error)

        assertEquals(StockError.STOCK_INSUFFICIENT, pool.adjust(-200_000L, "leak").error)
        val negAdj = pool.adjust(-40_000L, "evaporation")
        assertEquals("ACCEPTED", negAdj.status)
        assertEquals(60_000L, negAdj.afterQuantityMl)
        assertEquals(30_000L, negAdj.afterValueCentavos)
        val posAdj = pool.adjust(20_000L, "recount gain")
        assertEquals("ACCEPTED", posAdj.status)
        assertEquals(120_000L, posAdj.afterQuantityMl)
        assertEquals(60_000L, posAdj.afterValueCentavos)
        val fullLoss = pool.adjust(-100_000L, "drain")
        assertEquals(0L, fullLoss.afterValueCentavos)
        assertEquals(0L, StockPool("DIESEL_S10", 0L, 0L, allowNegative = true).adjust(-10L, "neg").movedCentavos)
    }
}
