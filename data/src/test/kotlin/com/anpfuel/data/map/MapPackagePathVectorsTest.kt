package com.anpfuel.data.map

import com.anpfuel.domain.map.MapContractException
import com.anpfuel.domain.map.NativeMapPackage
import com.anpfuel.domain.map.isSafeMapPackagePath
import java.io.File
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Cross-language package path vectors (`contracts/testdata/map/
 * package-path-vectors-v1.json`), also asserted by Go
 * `TestPackagePathVectors`. CoMaps country files carry the exact country id,
 * spaces included, so the engine registers them as real countries.
 */
class MapPackagePathVectorsTest {
    private fun vectors(): JSONObject {
        var dir = File(System.getProperty("user.dir"))
        repeat(8) {
            val candidate = File(dir, "contracts/testdata/map/package-path-vectors-v1.json")
            if (candidate.isFile) return JSONObject(candidate.readText())
            dir = dir.parentFile ?: return@repeat
        }
        throw IllegalStateException("path vectors not found")
    }

    private fun pkg(path: String) = NativeMapPackage("brazil-mato-grosso", "REGION", path, 1, "a".repeat(64))

    @Test fun `shared vectors agree with the go contract`() {
        val doc = vectors()
        val valid = doc.getJSONArray("valid")
        val invalid = doc.getJSONArray("invalid")
        assertTrue(valid.length() > 0 && invalid.length() > 0)
        for (i in 0 until valid.length()) {
            val path = valid.getString(i)
            assertTrue(isSafeMapPackagePath(path), "valid path refused: '$path'")
            assertEquals(path, pkg(path).path)
        }
        for (i in 0 until invalid.length()) {
            val path = invalid.getString(i)
            assertFalse(isSafeMapPackagePath(path), "invalid path accepted: '$path'")
            assertThrows(MapContractException::class.java) { pkg(path) }
        }
    }

    @Test fun `spaced country file publishes flat under the native version`() {
        assertEquals("Brazil_Mato Grosso.mwm", publishedName("261004", "261004/Brazil_Mato Grosso.mwm"))
        assertEquals("Brazil_Mato Grosso.mwm", publishedName("261004", "Brazil_Mato Grosso.mwm"))
        assertNull(publishedName("261004", "other/Brazil_Mato Grosso.mwm"))
        assertNull(publishedName("261004", "261004/a b/Brazil_Mato Grosso.mwm"))
    }
}
