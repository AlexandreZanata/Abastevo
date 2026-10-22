package com.anpfuel.domain.premium

import java.io.File
import java.math.BigInteger
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** EE-02 exact contract oracle; not the EE-11 production billing engine. */
class EVContractVectorsTest {
    @Test
    fun reviewedExactVectors() {
        val lines = File("../contracts/testdata/ev/exact-v1.csv").readLines().drop(1).filter { it.isNotBlank() }
        assertEquals(24, lines.size)
        val seen = mutableSetOf<String>()
        val one = BigInteger.ONE
        fun n(value: Long) = BigInteger.valueOf(value)
        for (line in lines) {
            val row = line.split(',')
            assertEquals(8, row.size)
            assertTrue(seen.add(row[0]))
            val a = row[2].toBigInteger()
            val b = row[3].toBigInteger()
            val c = row[4].toBigInteger()
            val d = row[5].toBigInteger()
            val (num, den) = when (row[1]) {
                "WH_KM_KWH100" -> a to n(10)
                "WH_KM_MJ" -> a * n(36) to n(10000)
                "ENERGY_COST" -> a * b + d * n(1000) * c to n(1000) * c
                "COST_KM" -> a * n(1000) to b
                "ENERGY100" -> a * n(100) to b
                "TIME_COST" -> a * b to n(60)
                "IDLE" -> ((a - b).max(BigInteger.ZERO) + c - one) / c * c * d to n(60)
                "SUM" -> a + b + c + d to one
                "PHEV_COST" -> (a + b) * n(1000) to c
                "DIFFERENCE" -> a - b to one
                "SOC_MODEL" -> a * b to n(10000)
                "ROUND_HALF_UP" -> (a * n(2) + b) / (b * n(2)) to one
                else -> error("Unknown reviewed operation")
            }
            assertTrue(den.signum() > 0)
            val divisor = num.gcd(den)
            assertEquals(row[6].toBigInteger() to row[7].toBigInteger(), num / divisor to den / divisor, row[0])
        }
    }
}
