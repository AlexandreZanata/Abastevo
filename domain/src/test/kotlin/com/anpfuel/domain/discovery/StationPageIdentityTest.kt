package com.anpfuel.domain.discovery

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class StationPageIdentityTest {
    @Test fun canonicalIdAndFullCnpjStayDistinct() {
        assertTrue(StationPageIdentity.parse("d6c74c23-63db-4c24-a2e5-408cb23bad26") is StationPageIdentity.Canonical)
        val legacy = StationPageIdentity.parse("04218406000104") as StationPageIdentity.Legacy
        assertEquals("04218406000104", legacy.cnpj)
        assertTrue(StationPageIdentity.parse("12ABC34501DE35") is StationPageIdentity.Legacy)
    }
    @Test fun malformedAndPathInjectionNeverBecomeAStation() {
        for (value in listOf("", "123", "posto-central", "../auth", "１２３４５６７８０００１９５")) {
            assertNull(StationPageIdentity.parse(value))
        }
    }
}
