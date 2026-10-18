package com.anpfuel.data.local.quality

import java.io.File
import java.nio.file.Files
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class QualityPublicCacheTest {
    private fun directory()=Files.createTempDirectory("quality-public-test").toFile()
    @Test fun `restart retains exact origin key while expiry and clock rollback refuse reuse`() {
        val dir=directory()
        try {
            var now=1000L
            val url="https://a.invalid/v1/quality/summary?station_id=synthetic"
            QualityPublicCache(dir){now}.save(url,"{}","\"v1\"")
            val restarted=QualityPublicCache(dir){now}
            assertEquals("{}",restarted.load(url)?.body)
            assertNull(restarted.load(url.replace("a.invalid","b.invalid")))
            now+=QualityPublicCache.RETENTION_MILLIS
            assertNull(restarted.load(url))
            restarted.save(url,"{}",null)
            now--
            assertNull(restarted.load(url))
            now++
            assertNull(restarted.load(url),"clock recovery cannot resurrect purged bodies")
        }finally {dir.deleteRecursively()}
    }
    @Test fun `corrupt missing clock and tampered envelopes fail closed without foreign deletion`() {
        val dir=directory()
        try {
            val cache=QualityPublicCache(dir){1000L}
            val a="https://a.invalid/v1/quality/summary"
            val b="https://b.invalid/v1/quality/summary"
            cache.save(a,"{}",null);cache.save(b,"{}",null)
            cache.clearOrigin("https://a.invalid/")
            assertNull(cache.load(a));assertNotNull(cache.load(b))
            File(dir,"clock-v1").delete()
            assertNull(cache.load(b))
            cache.save(a,"{}",null)
            dir.listFiles()!!.single {it.name.endsWith(".json")}.writeText("untrusted")
            assertNull(cache.load(a))
            cache.save(a,"{}",null);File(dir,"clock-v1").writeText("corrupt")
            assertNull(cache.load(a))
        }finally {dir.deleteRecursively()}
    }
    @Test fun `body count total and abandoned temporary files are bounded`() {
        val dir=directory()
        try {
            val cache=QualityPublicCache(dir){1000L}
            repeat(70){cache.save("https://a.invalid/v1/quality/assays?cursor=$it","{}",null)}
            assertTrue(dir.listFiles()!!.count {it.name.endsWith(".json")}<=QualityPublicCache.MAX_ENTRIES)
            repeat(20){cache.save("https://a.invalid/v1/quality/assays?large=$it","x".repeat(QualityPublicCache.MAX_BODY),null)}
            assertTrue(dir.listFiles()!!.filter {it.name.endsWith(".json")}.sumOf {it.length()}<=QualityPublicCache.MAX_TOTAL)
            cache.save("https://a.invalid/oversized","x".repeat(QualityPublicCache.MAX_BODY+1),null)
            assertNull(cache.load("https://a.invalid/oversized"))
            val temp=File(dir,"a".repeat(64)+".json.tmp");temp.writeText("interrupted")
            QualityPublicCache(dir){1000L}.load("https://a.invalid/missing")
            assertFalse(temp.exists())
        }finally {dir.deleteRecursively()}
    }
}
