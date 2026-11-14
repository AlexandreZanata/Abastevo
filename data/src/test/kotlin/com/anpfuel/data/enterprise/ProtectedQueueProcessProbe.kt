package com.anpfuel.data.enterprise

import com.anpfuel.data.enterprise.ProtectedQueueFixture.ack
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.operation
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import java.nio.file.Path
import kotlinx.coroutines.runBlocking

/** Independent host process only; exportable synthetic keys do not stand in for Android Keystore. */
object ProtectedQueueProcessProbe {
    const val BEFORE_DURABLE_REPLACE = 81
    const val AFTER_TRUSTED_HEAD = 82
    @JvmStatic fun main(args: Array<String>): Unit = runBlocking {
        val action = args[2]
        val keys = FileTestVaultKeys(Path.of(args[1])) { alias ->
            if (action.endsWith("-after") && alias.endsWith("_h")) Runtime.getRuntime().halt(AFTER_TRUSTED_HEAD)
        }
        val f = VaultFixture(Path.of(args[0]), keys) { real -> object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (action.endsWith("-before") && !descriptor) Runtime.getRuntime().halt(BEFORE_DURABLE_REPLACE)
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        } }
        val queue = ProtectedEnterpriseQueue(f.store); val captured = attempt(f)
        val operation = operation(operationId = args[3])
        if (action.startsWith("save-")) {
            queue.save(captured, operation) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(8))) }
        } else {
            check(action.startsWith("ack-"))
            queue.acknowledge(captured, operation, ack(operation)) { it.put(EnterpriseLocalRecord(partition(), A, byteArrayOf(9))) }
        }
        error("Synthetic process did not reach its requested crash boundary")
    }
}
