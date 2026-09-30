package com.anpfuel.application.portable

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P12-T03 boundary: pure application code keeps zero Android, Room,
 * WorkManager, Hilt and `java.*` dependencies so it moves unchanged to
 * `commonMain`. Native persistence stays an explicit adapter behind ports;
 * no schema change happens in this task.
 */
class PortableAppParityTest {

    @Test
    fun portableApplicationSourcesUseNoPlatformImports() {
        val dir = portableMainDir()
        val files = dir.listFiles { f -> f.name.endsWith(".kt") }!!.sortedBy { it.name }
        assertTrue(files.isNotEmpty(), "no portable application sources in $dir")
        val banned = listOf(
            "import android.", "import androidx.", "import java.",
            "import javax.", "import dagger.", "import dagger ",
            "import androidx", "import com.google.dagger",
            "import kotlinx.coroutines",
        )
        for (file in files) {
            for (line in file.readLines()) {
                val trimmed = line.trim()
                for (prefix in banned) {
                    assertTrue(
                        !trimmed.startsWith(prefix),
                        "${file.name} must stay commonMain-ready but imports: $trimmed",
                    )
                }
            }
        }
    }

    @Test
    fun outboxRecordShapeIsMigrationCompatible() {
        val record = PortableOutbox.toRecord(OutboxCommand("cmd-1", "observation.submit", "{}"))
        assertEquals(
            setOf(
                "command_id", "kind", "payload", "revision",
                "state", "attempts", "next_eligible_tick", "nonce",
            ),
            record.keys,
        )
    }

    private fun repoRoot(): File {
        var dir = File(System.getProperty("user.dir"))
        while (true) {
            if (File(dir, "settings.gradle.kts").exists()) return dir
            dir = dir.parentFile ?: throw AssertionError("repo root not found")
        }
    }

    private fun portableMainDir(): File {
        val dir = File(repoRoot(), "application/src/main/kotlin/com/anpfuel/application/portable")
        assertTrue(dir.isDirectory, "missing portable dir: $dir")
        return dir
    }
}
