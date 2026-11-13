package com.anpfuel.data.enterprise

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption.ATOMIC_MOVE
import java.nio.file.StandardOpenOption.*
import java.nio.channels.FileChannel
import javax.crypto.spec.SecretKeySpec

/** Explicitly exportable synthetic provider shared by child JVMs; never an Android key substitute. */
internal class FileTestVaultKeys(private val root: Path, private val committed: (String) -> Unit = {}) : TestVaultKeys() {
    init { Files.createDirectories(root) }
    private fun path(alias: String): Path { require(Regex("[a-z0-9_-]+").matches(alias)); return root.resolve(alias + ".key") }
    @Synchronized override fun existing(alias: String): EnterpriseVaultKey? {
        val file = path(alias)
        if (!Files.exists(file)) { values.remove(alias); return null }
        val bytes = Files.readAllBytes(file); check(bytes.size == 32)
        return EnterpriseVaultKey(SecretKeySpec(bytes, "AES"), null).also { values[alias] = it }
    }
    @Synchronized override fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey {
        check(!Files.exists(path(alias))); require(managedUnlockSeconds == null)
        val key = super.create(alias, null)
        val temporary = root.resolve(alias + ".new")
        FileChannel.open(temporary, CREATE, TRUNCATE_EXISTING, WRITE).use { channel ->
            val bytes = java.nio.ByteBuffer.wrap(key.secret.encoded)
            while (bytes.hasRemaining()) check(channel.write(bytes) > 0)
            channel.force(true)
        }
        Files.move(temporary, path(alias), ATOMIC_MOVE)
        FileChannel.open(root, READ).use { it.force(true) }
        committed(alias)
        return key
    }
    @Synchronized override fun delete(alias: String) {
        super.delete(alias); Files.deleteIfExists(path(alias)); FileChannel.open(root, READ).use { it.force(true) }
    }
    @Synchronized override fun aliases(prefix: String): List<String> {
        val all = Files.list(root).use { stream -> stream.map { it.fileName.toString() }.filter { it.endsWith(".key") }.map { it.removeSuffix(".key") }.toList() }
        values.keys.retainAll(all.toSet()); all.forEach(::existing)
        return all.filter { it.startsWith(prefix) }
    }
}
