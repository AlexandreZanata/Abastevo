package com.anpfuel.domain.routing

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class NativeRouteTest {
    private val start=RoutePoint(-25.43,-49.27,"Map point")
    private val end=RoutePoint(-25.42,-49.26,"Station reference")
    @Test fun `route requires distinct finite geographic checkpoints within pinned core limit`() {
        listOf(emptyList(),listOf(start),listOf(start,start),listOf(start,end,start),
            listOf(start,end.copy(latitude=Double.NaN)),listOf(start,end.copy(longitude=181.0)),listOf(start,end.copy(latitude=91.0)),
            listOf(start,end.copy(longitude=Double.POSITIVE_INFINITY)),listOf(start,end.copy(label="x".repeat(513))),
            (0..102).map { RoutePoint(it/1000.0,0.0,"point") }).forEach { points ->
            assertThrows(IllegalArgumentException::class.java) { NativeRouteRequest(points).validate() }
        }
    }
    @Test fun `manual route with stop and genuine avoidances works without location`() {
        NativeRouteRequest(listOf(start,RoutePoint(-25.425,-49.265,"stop"),end), RoadAvoidance.entries.toSet(),"pt-BR").validate()
        assertTrue(end.approximateEntrance)
        NativeRouteRequest(listOf(RoutePoint(-90.0,-180.0,"south"),RoutePoint(90.0,180.0,"north"))).validate()
        assertThrows(IllegalArgumentException::class.java) { NativeRouteRequest(listOf(start,end),language="").validate() }
    }
}
