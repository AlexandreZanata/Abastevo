package com.anpfuel.app.ui.enterprise

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EMP-GPS-015 hidden status (B-BR-EMP-548, EDR-357), a structural pin like the manifest checks: the indicator is drawn once,
 * in the top bar of the enterprise scaffold that every enterprise panel renders under, fed by the running collector only;
 * the personal map and driver screens never draw it. The on-screen proof on each panel is the device test (OWED).
 */
class EnterpriseTrackingIndicatorPlacementTest {
    private val enterprise = File("src/main/kotlin/com/anpfuel/app/ui/enterprise")

    @Test fun `hidden status - the indicator is drawn once in the header every enterprise screen shares`() {
        val screen = File(enterprise, "EnterpriseContextScreen.kt").readText()
        val topBar = screen.substring(screen.indexOf("topBar = {"), screen.indexOf(") { padding ->"))
        assertTrue(topBar.contains("EnterpriseTrackingIndicator(trackingActivity,"), "the indicator is in the shared top bar")
        val calls = enterprise.walkTopDown().filter { it.isFile && it.extension == "kt" }
            .sumOf { Regex("EnterpriseTrackingIndicator\\(").findAll(it.readText()).count() }
        // One declaration and one call: never a per-panel copy that could be forgotten.
        assertEquals(2, calls)
        // Every enterprise panel renders inside that scaffold's content.
        for (panel in listOf("MyTrackingPanel(", "ChecklistHomePanel(", "ExpensePanel(", "FuelingPanel(", "TripPanel(", "MyAssetsPanel(",
            "EnterpriseSyncPanel(", "ContextChooser(")) assertTrue(screen.indexOf(panel) > screen.indexOf(") { padding ->"), panel)
        // The indicator's source is the runtime of this process, never the stored book.
        assertTrue(screen.contains("val trackingActivity by trackingViewModel.activity.collectAsState()"))
    }

    @Test fun `personal session displayed as corporate - personal screens never draw the organization indicator`() {
        val personal = listOf("src/main/kotlin/com/anpfuel/app/ui/map", "src/main/kotlin/com/anpfuel/app/ui/driver").map(::File)
        for (root in personal) {
            assertTrue(root.isDirectory, root.path)
            root.walkTopDown().filter { it.isFile && it.extension == "kt" }.forEach {
                val text = it.readText()
                assertTrue(!text.contains("EnterpriseTrackingIndicator") && !text.contains("TrackingRuntime") && !text.contains("MyTrackingViewModel"), it.path)
            }
        }
    }
}
