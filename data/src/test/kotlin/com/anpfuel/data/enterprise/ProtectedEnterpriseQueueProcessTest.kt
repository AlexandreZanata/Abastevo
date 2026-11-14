package com.anpfuel.data.enterprise

import com.anpfuel.data.enterprise.ProtectedQueueFixture.ack
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import java.io.File
import java.net.URLClassLoader
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedEnterpriseQueueProcessTest {
    @TempDir lateinit var root: Path
    private fun child(directory: Path, keys: Path, stage: String, operationId: String): Process {
        val classpath = linkedSetOf<String>()
        var loader: ClassLoader? = javaClass.classLoader
        while (loader != null) {
            if (loader is URLClassLoader) classpath += loader.getURLs().map { Path.of(it.toURI()).toString() }
            loader = loader.parent
        }
        classpath += requireNotNull(System.getProperty("java.class.path")).split(File.pathSeparator)
        return ProcessBuilder(requireNotNull(System.getProperty("java.home")) + "/bin/java", "-cp",
            classpath.joinToString(File.pathSeparator), ProtectedQueueProcessProbe::class.java.name,
            directory.toString(), keys.toString(), stage, operationId)
            .redirectErrorStream(true).redirectOutput(root.resolve("$stage.log").toFile()).start()
    }

    @Test fun actualProcessDeathAroundSaveAndReceiptHeadsNeverSplitsTheirOwningProjection(): Unit = runBlocking {
        for (stage in listOf("save-before", "save-after", "ack-before", "ack-after")) {
            val directory = root.resolve("$stage-vault"); val keyDirectory = root.resolve("$stage-provider")
            val f = VaultFixture(directory, FileTestVaultKeys(keyDirectory)); f.confirm(partition())
            f.store.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(7)))
            val queue = ProtectedEnterpriseQueue(f.store); val operation = operation(operationId = id(500))
            if (stage.startsWith("ack-")) {
                queue.save(attempt(f), operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) }
                queue.save(attempt(f), operation(operationId = id(501))) { it.put(EnterpriseLocalRecord(partition(), B, byteArrayOf(6))) }
            }
            val process = child(directory, keyDirectory, stage, operation.operationId)
            try {
                assertTrue(process.waitFor(30, TimeUnit.SECONDS), "Synthetic queue child timed out")
                assertEquals(if (stage.endsWith("-before")) ProtectedQueueProcessProbe.BEFORE_DURABLE_REPLACE
                    else ProtectedQueueProcessProbe.AFTER_TRUSTED_HEAD, process.exitValue(), stage)
            } finally { if (process.isAlive) process.destroyForcibly() }
            val reopened = VaultFixture(directory, FileTestVaultKeys(keyDirectory))
            val restored = ProtectedEnterpriseQueue(reopened.store); val captured = attempt(reopened)
            val after = stage.endsWith("-after")
            val expected = if (stage.startsWith("save-")) (if (after) 8 else 7) else (if (after) 9 else 8)
            assertArrayEquals(byteArrayOf(expected.toByte()), reopened.store.record(partition(), A)!!.envelope, stage)
            var projected = 0
            if (stage.startsWith("save-")) {
                assertEquals(!after, restored.save(captured, operation) { projected++; it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) }, stage)
                assertEquals(1, restored.pending(captured).size)
            } else {
                assertEquals(!after, restored.acknowledge(captured, operation, ack(operation)) { projected++; it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(9))) }, stage)
                assertEquals(listOf(id(501)), restored.pending(captured).map { it.operation.operationId })
                assertArrayEquals(byteArrayOf(6), reopened.store.record(partition(), B)!!.envelope)
                assertArrayEquals(ack(operation).canonicalBytes, restored.entry(captured, operation.operationId)!!.acknowledgement!!.canonicalBytes)
            }
            assertEquals(if (after) 0 else 1, projected, "Ambiguous success replays without another projection")
        }
    }
}
