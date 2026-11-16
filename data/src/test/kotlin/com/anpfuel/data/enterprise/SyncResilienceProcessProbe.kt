package com.anpfuel.data.enterprise

import java.nio.file.Path
import kotlinx.coroutines.runBlocking

/**
 * EMP-OFFL-013 independent host process (B-BR-EMP-542): "save" records the field day through the product producers and
 * dies right after the last save reported success; "deliver" sends the queue and dies after the edge ledger committed,
 * at the first durable acknowledgement write. Exportable synthetic keys never stand in for Android Keystore.
 */
object SyncResilienceProcessProbe {
    const val SAVED = 83
    const val KILLED_BEFORE_ACKNOWLEDGEMENT = 84

    @JvmStatic fun main(args: Array<String>): Unit = runBlocking {
        val ledger = ResilienceLedger(Path.of(args[2]))
        val committed = java.util.concurrent.atomic.AtomicBoolean(false)
        ledger.afterCommit = { committed.set(true) }
        val device = ResilienceDevice(Path.of(args[0]), Path.of(args[1]), ledger) { real -> object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (committed.get() && !descriptor) Runtime.getRuntime().halt(KILLED_BEFORE_ACKNOWLEDGEMENT)
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        } }
        when (args[3]) {
            "save" -> {
                FieldDay.save(device)
                Runtime.getRuntime().halt(SAVED)
            }
            "deliver" -> device.service().dispatch(device.source.deliveryAccount(), VaultFixture.partition())
            else -> error("Unknown synthetic stage")
        }
        error("Synthetic process did not reach its requested crash boundary")
    }
}
