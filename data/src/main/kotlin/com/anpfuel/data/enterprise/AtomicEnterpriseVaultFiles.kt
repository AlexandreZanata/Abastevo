package com.anpfuel.data.enterprise

import java.io.File
import java.nio.ByteBuffer
import java.nio.channels.FileChannel
import java.nio.file.Files
import java.nio.file.LinkOption.NOFOLLOW_LINKS
import java.nio.file.StandardCopyOption.ATOMIC_MOVE
import java.nio.file.StandardCopyOption.REPLACE_EXISTING
import java.nio.file.StandardOpenOption.*
import java.nio.file.attribute.PosixFilePermissions
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/** A stored vault file outside the sealed-size bounds: corrupted or truncated, never transient. */
class EnterpriseVaultCorrupted : IllegalStateException("Enterprise vault ciphertext malformed")

interface EnterpriseVaultFiles {
    fun <T> locked(accountTag: String, work: () -> T): T
    /** Throws [EnterpriseVaultCorrupted] when the stored file can never be a sealed snapshot. */
    fun read(token: String): ByteArray?
    fun replace(token: String, ciphertext: ByteArray)
    fun denied(token: String): Boolean
    fun deny(token: String)
    fun allowFresh(token: String)
    fun erase(token: String)
    fun tokens(accountTag: String): List<String>
    /** Whether anything (including a lock file) was ever created for the account; creates nothing. */
    fun present(accountTag: String): Boolean
}

/** Ciphertext-only files; production root must be the dedicated noBackupFilesDir child. */
class AtomicEnterpriseVaultFiles(directory: File) : EnterpriseVaultFiles {
    private val root = directory.toPath().toAbsolutePath().normalize()
    private fun ensureRoot() {
        require(!Files.isSymbolicLink(root))
        Files.createDirectories(root)
        require(Files.isDirectory(root, NOFOLLOW_LINKS))
        Files.setPosixFilePermissions(root, PosixFilePermissions.fromString("rwx------"))
    }

    private fun path(token: String, suffix: String) = root.resolve(token + suffix).also {
        ensureRoot()
        require(tokenPattern.matches(token) && !Files.isSymbolicLink(it))
    }

    override fun <T> locked(accountTag: String, work: () -> T): T = processLock.withLock {
        require(accountTag.length == 64)
        FileChannel.open(path(accountTag, ".lck"), CREATE, WRITE, NOFOLLOW_LINKS).use { channel ->
            channel.lock().use { work() }
        }
    }

    override fun read(token: String): ByteArray? {
        val file = path(token, ".vault")
        if (!Files.exists(file, NOFOLLOW_LINKS)) return null
        FileChannel.open(file, READ, NOFOLLOW_LINKS).use { channel ->
            val size = channel.size()
            if (size !in 32..EnterpriseVaultCodec.MAX_SEALED.toLong()) throw EnterpriseVaultCorrupted()
            val bytes = ByteArray(size.toInt())
            val buffer = ByteBuffer.wrap(bytes)
            while (buffer.hasRemaining()) check(channel.read(buffer) > 0)
            check(channel.read(ByteBuffer.allocate(1)) == -1)
            return bytes
        }
    }

    override fun replace(token: String, ciphertext: ByteArray) {
        require(ciphertext.size in 32..EnterpriseVaultCodec.MAX_SEALED)
        atomic(token, ".vault", ciphertext)
    }

    private fun atomic(token: String, suffix: String, bytes: ByteArray) {
        val temporary = path(token, ".new")
        val target = path(token, suffix)
        try {
            FileChannel.open(temporary, CREATE, TRUNCATE_EXISTING, WRITE, NOFOLLOW_LINKS).use { channel ->
                val buffer = ByteBuffer.wrap(bytes)
                while (buffer.hasRemaining()) check(channel.write(buffer) > 0)
                channel.force(true)
            }
            Files.move(temporary, target, ATOMIC_MOVE, REPLACE_EXISTING)
            syncDirectory()
        } finally { Files.deleteIfExists(temporary) }
    }

    override fun denied(token: String): Boolean = Files.exists(path(token, ".blocked"), NOFOLLOW_LINKS)
    override fun deny(token: String) = atomic(token, ".blocked", byteArrayOf(1))
    override fun allowFresh(token: String) {
        Files.deleteIfExists(path(token, ".blocked"))
        syncDirectory()
    }
    override fun erase(token: String) {
        Files.deleteIfExists(path(token, ".vault"))
        Files.deleteIfExists(path(token, ".new"))
        syncDirectory()
    }

    override fun tokens(accountTag: String): List<String> {
        require(accountTag.length == 64 && tokenPattern.matches(accountTag))
        return Files.newDirectoryStream(root).use { entries ->
            entries.map { it.fileName.toString().substringBefore('.') }
                .filter { it.startsWith(accountTag + "-") && tokenPattern.matches(it) }.distinct().sorted()
        }
    }

    override fun present(accountTag: String): Boolean {
        require(accountTag.length == 64 && tokenPattern.matches(accountTag))
        if (!Files.exists(root, NOFOLLOW_LINKS)) return false
        require(!Files.isSymbolicLink(root) && Files.isDirectory(root, NOFOLLOW_LINKS))
        return Files.newDirectoryStream(root).use { entries ->
            entries.any { it.fileName.toString().startsWith(accountTag) }
        }
    }

    private fun syncDirectory() = FileChannel.open(root, READ).use { it.force(true) }

    companion object {
        private val tokenPattern = Regex("[0-9a-f]{64}(-[0-9a-f]{64})?")
        // JVM file locks throw on overlap instead of waiting; serialize before the OS lock.
        private val processLock = ReentrantLock()
    }
}
