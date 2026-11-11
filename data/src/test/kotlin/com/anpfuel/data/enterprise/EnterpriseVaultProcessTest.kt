package com.anpfuel.data.enterprise

import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import java.io.File
import java.net.URLClassLoader
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class EnterpriseVaultProcessTest {
    @TempDir lateinit var root: Path
    private fun child(directory: Path, key: Path, action: String, id: String): Process {
        val paths = linkedSetOf<String>()
        var loader: ClassLoader? = javaClass.classLoader
        while (loader != null) {
            if (loader is URLClassLoader) paths += loader.getURLs().map { Path.of(it.toURI()).toString() }
            loader = loader.parent
        }
        paths += requireNotNull(System.getProperty("java.class.path")).split(File.pathSeparator)
        return ProcessBuilder(
            requireNotNull(System.getProperty("java.home")) + "/bin/java", "-cp", paths.joinToString(File.pathSeparator),
            VaultProcessProbe::class.java.name, directory.toString(), key.toString(), action, id,
        ).redirectErrorStream(true).redirectOutput(root.resolve(action + "-" + id + ".log").toFile()).start()
    }
    private fun complete(process: Process) {
        try {
            assertTrue(process.waitFor(30, TimeUnit.SECONDS), "Child timed out")
            assertEquals(0, process.exitValue(), "Child failed; inspect temporary probe output")
        } finally { if (process.isAlive) process.destroyForcibly() }
    }
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')

    @Test
    fun independentProcessesAndAbruptDeathKeepCompleteSnapshotsAndRetrySemantics(): Unit = runBlocking {
        val p = partition()
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        val key = root.resolve("synthetic-test-key")
        Files.write(key, f.keys.values.values.single().secret.encoded)
        complete(child(directory, key, "read", id(40)))
        val writers = (41..44).map { child(directory, key, "write", id(it)) }
        writers.forEach(::complete)
        assertEquals(4, f.store.pending(p, 50).size)
        complete(child(directory, key, "crash-before", id(45)))
        assertTrue(f.store.enqueue(EnterprisePendingOperation(p, id(45), A, byteArrayOf(9))))
        complete(child(directory, key, "crash-after", id(46)))
        assertFalse(f.store.enqueue(EnterprisePendingOperation(p, id(46), A, byteArrayOf(9))))
        assertEquals(6, f.store.pending(p, 50).size)
        assertArrayEquals(byteArrayOf(7), f.store.record(p, A)!!.envelope)
    }
}
