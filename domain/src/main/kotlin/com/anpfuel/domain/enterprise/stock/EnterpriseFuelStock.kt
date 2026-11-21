package com.anpfuel.domain.enterprise.stock

import java.math.BigInteger

/**
 * EMP-STOCK-001..005 / VAL-32: Exact moving-average valuation for internal fuel
 * tanks. Fuel purchases adjust the inventory pool with zero immediate operating
 * expense; internal dispenses recognize proportional cost once via HALF_UP division
 * while transfers and adjustments preserve conservation.
 */
object FuelStockValuation {
    const val METHOD_VERSION = "MOVING_AVG_UNIT_V1"

    fun proportionHalfUp(totalCentavos: Long, partMl: Long, poolMl: Long): Long {
        if (totalCentavos <= 0L || partMl <= 0L || poolMl <= 0L) return 0L
        val num = BigInteger.valueOf(totalCentavos).multiply(BigInteger.valueOf(partMl))
        val den = BigInteger.valueOf(poolMl)
        val half = den.shiftRight(1)
        return num.add(half).divide(den).longValueExact()
    }
}

enum class StockError {
    STOCK_INSUFFICIENT,
    STOCK_PRODUCT_MISMATCH,
    VALIDATION_FAILED,
}

data class StockOutcome(
    val status: String,
    val error: StockError? = null,
    val movedCentavos: Long = 0L,
    val recognizedExpenseCentavos: Long = 0L,
    val afterQuantityMl: Long = 0L,
    val afterValueCentavos: Long = 0L,
)

data class TransferOutcome(
    val status: String,
    val error: StockError? = null,
    val movedCentavos: Long = 0L,
    val recognizedExpenseCentavos: Long = 0L,
    val srcAfterQuantityMl: Long = 0L,
    val srcAfterValueCentavos: Long = 0L,
    val dstAfterQuantityMl: Long = 0L,
    val dstAfterValueCentavos: Long = 0L,
)

data class StockPool(
    val product: String,
    val quantityMl: Long,
    val valueCentavos: Long,
    val allowNegative: Boolean = false,
) {
    private fun rejected(err: StockError) = StockOutcome(
        status = "REJECTED",
        error = err,
        afterQuantityMl = quantityMl,
        afterValueCentavos = valueCentavos,
    )

    fun receive(movementProduct: String, rxMl: Long, rxCentavos: Long): StockOutcome {
        if (movementProduct.isBlank() || movementProduct != product) {
            return rejected(StockError.STOCK_PRODUCT_MISMATCH)
        }
        if (rxMl <= 0L || rxCentavos < 0L) {
            return rejected(StockError.VALIDATION_FAILED)
        }
        return StockOutcome(
            status = "ACCEPTED",
            movedCentavos = rxCentavos,
            recognizedExpenseCentavos = 0L,
            afterQuantityMl = quantityMl + rxMl,
            afterValueCentavos = valueCentavos + rxCentavos,
        )
    }

    fun dispense(movementProduct: String, txMl: Long): StockOutcome {
        if (movementProduct.isBlank() || movementProduct != product) {
            return rejected(StockError.STOCK_PRODUCT_MISMATCH)
        }
        if (txMl <= 0L) {
            return rejected(StockError.VALIDATION_FAILED)
        }
        if (!allowNegative && quantityMl < txMl) {
            return rejected(StockError.STOCK_INSUFFICIENT)
        }
        val movedCents = when {
            quantityMl <= 0L || valueCentavos <= 0L -> 0L
            txMl == quantityMl -> valueCentavos
            else -> FuelStockValuation.proportionHalfUp(valueCentavos, txMl, quantityMl)
                .coerceAtMost(valueCentavos)
        }
        val afterMl = quantityMl - txMl
        val afterCents = if (afterMl <= 0L) 0L else (valueCentavos - movedCents).coerceAtLeast(0L)
        return StockOutcome(
            status = "ACCEPTED",
            movedCentavos = movedCents,
            recognizedExpenseCentavos = movedCents,
            afterQuantityMl = afterMl,
            afterValueCentavos = afterCents,
        )
    }

    fun transferTo(destination: StockPool, movementProduct: String, txMl: Long): TransferOutcome {
        if (movementProduct.isBlank() || movementProduct != product || destination.product != product) {
            return TransferOutcome(
                status = "REJECTED",
                error = StockError.STOCK_PRODUCT_MISMATCH,
                srcAfterQuantityMl = quantityMl,
                srcAfterValueCentavos = valueCentavos,
                dstAfterQuantityMl = destination.quantityMl,
                dstAfterValueCentavos = destination.valueCentavos,
            )
        }
        val disp = dispense(movementProduct, txMl)
        if (disp.status != "ACCEPTED") {
            return TransferOutcome(
                status = "REJECTED",
                error = disp.error,
                srcAfterQuantityMl = quantityMl,
                srcAfterValueCentavos = valueCentavos,
                dstAfterQuantityMl = destination.quantityMl,
                dstAfterValueCentavos = destination.valueCentavos,
            )
        }
        return TransferOutcome(
            status = "ACCEPTED",
            movedCentavos = disp.movedCentavos,
            recognizedExpenseCentavos = 0L,
            srcAfterQuantityMl = disp.afterQuantityMl,
            srcAfterValueCentavos = disp.afterValueCentavos,
            dstAfterQuantityMl = destination.quantityMl + txMl,
            dstAfterValueCentavos = destination.valueCentavos + disp.movedCentavos,
        )
    }

    fun adjust(deltaMl: Long, reason: String): StockOutcome {
        if (reason.isBlank() || deltaMl == 0L) {
            return rejected(StockError.VALIDATION_FAILED)
        }
        if (!allowNegative && quantityMl + deltaMl < 0L) {
            return rejected(StockError.STOCK_INSUFFICIENT)
        }
        val movedCents: Long
        val afterCents: Long
        if (deltaMl < 0L) {
            val loss = -deltaMl
            movedCents = if (quantityMl <= 0L || valueCentavos <= 0L) 0L
            else if (loss >= quantityMl) valueCentavos
            else FuelStockValuation.proportionHalfUp(valueCentavos, loss, quantityMl).coerceAtMost(valueCentavos)
            afterCents = if (quantityMl + deltaMl <= 0L) 0L else (valueCentavos - movedCents).coerceAtLeast(0L)
        } else {
            movedCents = FuelStockValuation.proportionHalfUp(valueCentavos, deltaMl, quantityMl)
            afterCents = valueCentavos + movedCents
        }
        return StockOutcome(
            status = "ACCEPTED",
            movedCentavos = movedCents,
            recognizedExpenseCentavos = 0L,
            afterQuantityMl = quantityMl + deltaMl,
            afterValueCentavos = afterCents,
        )
    }
}
