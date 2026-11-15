package com.anpfuel.app.ui.enterprise

import java.nio.file.Files
import java.nio.file.Path
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EMP-FLEET-013 AC5 / B-BR-EMP-348: no source of "Meus ativos" or its cache reaches the personal
 * vehicle model, its tables or the personal database; the enterprise asset never converts to one.
 */
class MyAssetsPersonalIsolationTest {
    private val root: Path = generateSequence(Path.of("").toAbsolutePath()) { it.parent }.first { Files.exists(it.resolve("settings.gradle.kts")) }
    private val sources = listOf(
        "domain/src/main/kotlin/com/anpfuel/domain/enterprise/fleet/ReferenceFeed.kt",
        "application/src/main/kotlin/com/anpfuel/application/enterprise/fleet/MyAssets.kt",
        "data/src/main/kotlin/com/anpfuel/data/enterprise/fleet/ProtectedReferenceCacheStore.kt",
        "data/src/main/kotlin/com/anpfuel/data/enterprise/fleet/EnterpriseReferenceHttpClient.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/fleet/MyAssetsViewModel.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/fleet/MyAssetsScreen.kt",
    )
    private val personal = Regex("""\bVehicle(Dao|Entity|Repository|Admission\w*|Capability\w*|Registered|Updated|Removed)?\b|AnpFuelDatabase|\bvehicle\b|com\.anpfuel\.(domain|application)\.vehicle|data\.local\.(dao|entity)""")

    @Test fun `my assets sources never reference the personal vehicle model or database`() {
        val files = sources.map(root::resolve)
        assertTrue(files.all(Files::isRegularFile), "Every FLEET-013 source must exist")
        val leaks = files.flatMap { file ->
            Files.readAllLines(file).mapIndexedNotNull { index, line -> if (personal.containsMatchIn(line)) "${root.relativize(file)}:${index + 1}" else null }
        }
        assertEquals(emptyList<String>(), leaks)
    }
}
