package com.anpfuel.domain.routing

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class NavigationFixTest {
    private val now=100_000_000_000L
    private fun fix(age:Long=0, accuracy:Float=5f,lat:Double=-25.43,lon:Double=-49.27,time:Long=1)=NavigationFix(lat,lon,accuracy,time,now-age)
    @Test fun `only fresh finite accurate samples may advance route and speech`() {
        listOf(fix(age=15_000_000_001),fix(age=-1),fix(accuracy=50.001f),fix(accuracy=Float.NaN),fix(accuracy=0f),
            fix(lat=Double.NaN),fix(lon=181.0),fix(lat=91.0),fix(lon=Double.POSITIVE_INFINITY),fix(time=0)).forEach {
            assertFalse(NavigationFixPolicy.accepts(it,now))
        }
        assertTrue(NavigationFixPolicy.accepts(fix(age=15_000_000_000,accuracy=50f),now))
        assertTrue(NavigationFixPolicy.accepts(fix(),now))
    }
    @Test fun `diagnostic representation does not serialize private position`() {
        assertFalse(fix().toString().contains("25.43"))
    }
}
