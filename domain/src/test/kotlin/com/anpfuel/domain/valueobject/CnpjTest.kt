package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class CnpjTest {

    @Test
    fun parseNormalizesFormattedCnpj() {
        val cnpj = Cnpj.parse("12.345.678/0001-95")

        assertEquals("12345678000195", cnpj.digits)
        assertEquals("12.345.678/0001-95", cnpj.formatted())
    }

    @Test
    fun parseAcceptsDigitsOnly() {
        val cnpj = Cnpj.parse("12345678000195")

        assertEquals("12345678000195", cnpj.digits)
    }

    @Test
    fun invalidLengthThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            Cnpj.parse("123456789")
        }
    }

    @Test
    fun rejectsNonDigitCharactersAfterNormalization() {
        assertThrows(DomainException::class.java) {
            Cnpj.parse("1234567800019X")
        }
    }

    @Test
    fun equalityHashCodeAndToStringUseDigits() {
        val first = Cnpj.parse("12345678000195")
        val second = Cnpj.parse("12.345.678/0001-95")
        val different = Cnpj.parse("61602199002409")

        assertEquals(first, second)
        assertEquals(first.hashCode(), second.hashCode())
        assertNotEquals(first, different)
        assertTrue(first.toString().contains("12345678000195"))
    }
}
