package com.anpfuel.domain.driver

import java.math.BigInteger

/** Checked conversion supported on the Android API 26 minimum runtime. */
internal fun BigInteger.driverLongExact(): Long {
    if (this < BigInteger.valueOf(Long.MIN_VALUE) || this > BigInteger.valueOf(Long.MAX_VALUE)) {
        throw ArithmeticException("driver-integer-overflow")
    }
    return toLong()
}
