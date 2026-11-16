package com.anpfuel.app.ui.enterprise

import java.nio.file.Files
import java.nio.file.Path
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EMP-ROUTE-004 (B-BR-EMP-534/535, EDR-346): the mission and trip screens and the navigation hand-off build no external
 * intent or maps URI; the navigation resolution and its action reach no tracking, location, queue or network port (the trip
 * screen hosts the EMP-GPS-002 tracking card, which the driver starts and stops on its own); navigation is the app's own
 * map tab only.
 */
class TripNavigationIsolationTest {
    private val root: Path = generateSequence(Path.of("").toAbsolutePath()) { it.parent }.first { Files.exists(it.resolve("settings.gradle.kts")) }
    private val sources = listOf(
        "domain/src/main/kotlin/com/anpfuel/domain/enterprise/trip/TripNavigation.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/trip/TripNavigationAction.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/trip/MissionInbox.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/trip/TripScreen.kt",
        "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/trip/TripViewModel.kt",
    )
    private val external = Regex("""\bIntent\b|ACTION_VIEW|startActivity|geo:|google\.navigation|maps\.google|waze|Uri\.parse|ACTION_SEND""")
    private val reach = Regex("""\btracking\b|Tracking\w*|gps\.|LocationManager|FusedLocation|OkHttp|Retrofit|HttpUrl|EnterpriseQueueService|EnterpriseOperationQueue""")

    /** The navigation resolution and its action: neither an external hand-off nor any tracking, location, queue or network port. */
    private val navigation = sources.take(2)

    @Test fun `the trip and mission navigation sources build no external hand-off and reach no tracking or network`() {
        val files = sources.map(root::resolve)
        assertTrue(files.all(Files::isRegularFile), "Every ROUTE-004 source must exist")
        val leaks = files.flatMap { file ->
            val strict = root.relativize(file).toString() in navigation
            Files.readAllLines(file).mapIndexedNotNull { index, line ->
                val code = line.substringBefore("//").let { if (it.trimStart().startsWith("*")) "" else it }
                if (external.containsMatchIn(code) || strict && reach.containsMatchIn(code)) "${root.relativize(file)}:${index + 1}" else null
            }
        }
        assertEquals(emptyList<String>(), leaks)
    }

    @Test fun `the enterprise screen hands navigation to the app's own map graph only`() {
        val screen = String(Files.readAllBytes(root.resolve("app/src/main/kotlin/com/anpfuel/app/ui/enterprise/EnterpriseContextScreen.kt")))
        assertTrue(!external.containsMatchIn(screen), "The company screen opens no other app")
        val graph = String(Files.readAllBytes(root.resolve("app/src/main/kotlin/com/anpfuel/app/navigation/AnpNavGraph.kt")))
        // The hand-off is the existing in-app address target, never a parsed point or an external intent.
        assertTrue(Regex("""onNavigate\s*=\s*\{\s*query\s*->\s*pendingRouteTarget\s*=\s*com\.anpfuel\.app\.ui\.map\.MapRouteTarget\.Address\(query\)""")
            .containsMatchIn(graph), "Navegar must hand the query to the in-app MapRouteTarget.Address")
    }
}
