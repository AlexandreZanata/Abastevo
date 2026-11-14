package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding

/** Synthetic owning record values used only by host fault/process fixtures. */
internal object ProtectedQueueFixture {
    fun id(value: Int) = "7b0e2a54-0000-4000-8000-" + value.toString().padStart(12, '0')
    fun operation(p: EnterprisePartition = partition(), operationId: String = id(100), value: String = "SYNTHETIC-QUEUE"): EnterpriseOperation =
        EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"$operationId","owner_account_id":"${p.ownerAccount}","api_origin":"${p.origin}","org_id":"${p.organizationId}","membership_id":"$B","enrollment_id":"$C","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"$A","expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"$value"}}""".toByteArray(),
            SyncBinding.of(p.ownerAccount, p.origin, p.organizationId))
    fun ack(operation: EnterpriseOperation): OperationAcknowledgement = OperationAcknowledgement.parse(
        """{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:00:01Z","review_item_id":null,"warnings":[]}""".toByteArray(), operation)
    fun account(f: VaultFixture): EnterpriseQueueAccountLease {
        val identity = requireNotNull(f.identity); val epoch = f.fence.epoch; val background = f.fence.backgroundEpoch
        return EnterpriseQueueAccountLease.capture(identity, epoch, background, id(900)) {
            f.identity == identity && f.fence.epoch == epoch && f.fence.backgroundEpoch == background
        }
    }
    suspend fun attempt(f: VaultFixture, p: EnterprisePartition = partition()): EnterpriseQueueAttempt {
        val account = account(f)
        return account.forEnrollment(f.store.protectedEnrollment(account, p))
    }
}
