package com.anpfuel.domain.valueobject

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Test

class GeographicScopeTest {

    @Test
    fun enumContainsGlossaryScopes() {
        assertEquals(5, GeographicScope.entries.size)
        assertEquals(
            setOf(
                GeographicScope.NATIONAL,
                GeographicScope.REGION,
                GeographicScope.STATE,
                GeographicScope.MUNICIPALITY,
                GeographicScope.STATION,
            ),
            GeographicScope.entries.toSet(),
        )
    }
}
