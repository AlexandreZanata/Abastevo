package com.anpfuel.data.enterprise

import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import java.nio.file.Files
import java.nio.file.Path
import javax.crypto.spec.SecretKeySpec
import kotlinx.coroutines.runBlocking

/** Separate JVM and abrupt death at the real encrypted-file replacement boundary. Test keys only. */
object VaultProcessProbe {
    /** Distinct from a normal exit and from an uncaught failure, so the parent proves where it died. */
    const val DIED_BEFORE_REPLACE = 71
    const val DIED_AFTER_REPLACE = 72

    @JvmStatic fun main(args: Array<String>) = runBlocking {
        val directory = Path.of(args[0])
        val p = partition()
        val action = args[2]
        val keys = FileTestVaultKeys(Path.of(args[1])) { alias ->
            if (action == "crash-after" && alias.endsWith("_h")) Runtime.getRuntime().halt(DIED_AFTER_REPLACE)
        }
        val f = VaultFixture(directory, keys, transform = { delegate ->
            object : EnterpriseVaultFiles by delegate {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    if (action == "crash-before" && !descriptor) Runtime.getRuntime().halt(DIED_BEFORE_REPLACE)
                    delegate.replaceGeneration(token, generation, descriptor, ciphertext)
                }
            }
        })
        if (action == "read") {
            check(f.store.record(p, A)!!.envelope.contentEquals(byteArrayOf(7)))
        } else {
            f.store.enqueue(EnterprisePendingOperation(p, args[3], A, byteArrayOf(9)))
        }
        Unit
    }
}
