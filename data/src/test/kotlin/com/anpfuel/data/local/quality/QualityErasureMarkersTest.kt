package com.anpfuel.data.local.quality

import java.nio.file.Files
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityErasureMarkersTest {
    @Test fun `independent restart marker preserves owner scope without claim auth or foreign erasure`() {
        val dir=Files.createTempDirectory("quality-erasure-marker").toFile()
        try {
            val marker=FileQualityErasureMarkers(dir)
            marker.block("account:synthetic-a:device-a");marker.block("account:synthetic-b:device-b")
            val restarted=FileQualityErasureMarkers(dir)
            assertTrue(restarted.blocked("account:synthetic-a:device-a"))
            assertEquals(2,restarted.pending().size)
            restarted.clear("account:synthetic-a:device-a")
            assertEquals(listOf("account:synthetic-b:device-b"),restarted.pending())
            assertTrue(dir.listFiles()!!.all {it.name.matches(Regex("[0-9a-f]{64}\\.erase"))})
        }finally {dir.deleteRecursively()}
    }
    @Test fun `corruption and capacity refuse rather than falsely clearing cleanup`() {
        val dir=Files.createTempDirectory("quality-erasure-refusal").toFile()
        try {
            val marker=FileQualityErasureMarkers(dir)
            repeat(64){marker.block("account:synthetic-$it:device")}
            assertThrows(java.io.IOException::class.java){marker.block("account:synthetic-overflow:device")}
            dir.listFiles()!!.first().writeText("foreign metadata")
            assertThrows(java.io.IOException::class.java){FileQualityErasureMarkers(dir).pending()}
        }finally {dir.deleteRecursively()}
    }
    @Test fun `interrupted atomic marker resumes before any queued command can dispatch`() {
        val dir=Files.createTempDirectory("quality-erasure-interrupted").toFile()
        try {
            val marker=FileQualityErasureMarkers(dir)
            val owner="account:synthetic-a:device-a";marker.block(owner)
            val file=dir.listFiles()!!.single()
            assertTrue(file.renameTo(java.io.File(dir,file.name+".tmp")))
            val restarted=FileQualityErasureMarkers(dir)
            assertTrue(restarted.blocked(owner));assertEquals(listOf(owner),restarted.pending())
            assertTrue(file.exists());assertEquals(1,dir.listFiles()!!.size)
        }finally {dir.deleteRecursively()}
    }

}
