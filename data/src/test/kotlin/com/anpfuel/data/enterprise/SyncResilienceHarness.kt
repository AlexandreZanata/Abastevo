package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistProducers
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseProducer
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingProducer
import com.anpfuel.application.enterprise.queue.EnterpriseOperationTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthKeyStore
import com.anpfuel.application.portable.AuthPorts
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.AuthWallClock
import com.anpfuel.application.portable.PortableNonceSource
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.checklist.ProtectedChecklistStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.expense.ExpenseEnvelope
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.fueling.FuelingEnvelope
import com.anpfuel.domain.enterprise.fueling.FuelingOperations
import com.anpfuel.domain.enterprise.fueling.FuelingReadingInput
import com.anpfuel.domain.enterprise.fueling.FuelingSubmitInput
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistAnswer
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBatch
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncLimits
import com.anpfuel.domain.enterprise.trip.TripClockReading
import com.anpfuel.domain.enterprise.trip.TripEnvelope
import com.anpfuel.domain.enterprise.trip.TripOperations
import com.anpfuel.domain.enterprise.trip.TripReadingInput
import com.anpfuel.domain.portable.PortableAuth
import java.io.IOException
import java.lang.reflect.Proxy
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.CREATE_NEW
import java.nio.file.StandardOpenOption.WRITE
import java.nio.channels.FileChannel
import java.time.Instant
import java.util.concurrent.atomic.AtomicInteger
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer

/**
 * EMP-OFFL-013 (EDR-351) edge ledger: the server rows of 10-offline-sync §10.2 that a device observes, applied at the
 * network edge of the real HTTP client. One record per (organization, operation identifier) is committed durably to
 * [directory] (so a child process's commits survive its death), an identical resend returns the stored bytes, another
 * digest is `REJECTED` `OPERATION_ID_REUSED`, a dependency that is not `ACCEPTED` answers `RETRY`
 * `DEPENDENCY_PENDING` (nothing stored), and a soft reference to an entity that is not stored is held
 * `ACCEPTED_FOR_REVIEW` `ORPHAN_REFERENCE`. The real intake is proven by the Go half (`TestSyncResilience*`).
 */
internal class ResilienceLedger(private val directory: Path) {
    enum class Fault { LOSE_ANSWER, DROP_LAST_ACKNOWLEDGEMENT }

    private class Stored(val digest: String, val result: String, val entity: String, val ack: ByteArray)

    /** One-shot transport fault applied to the next intake answer after its operations are committed. */
    @Volatile var fault: Fault? = null
    /** Called after a request's operations are committed and before its answer leaves (the process probe arms its crash here). */
    @Volatile var afterCommit: () -> Unit = {}
    /** New records created by this process. */
    val commits = AtomicInteger()
    val requests: MutableList<Pair<String, List<String>>> = java.util.Collections.synchronizedList(mutableListOf())

    init { Files.createDirectories(directory) }

    private fun file(org: String, operation: String): Path = directory.resolve("$org-$operation.record")

    @Synchronized private fun load(org: String, operation: String): Stored? {
        val path = file(org, operation)
        if (!Files.exists(path)) return null
        val lines = String(Files.readAllBytes(path), Charsets.UTF_8).split('\n')
        check(lines.size == 4)
        return Stored(lines[0], lines[1], lines[2], lines[3].toByteArray())
    }

    @Synchronized private fun commit(org: String, operation: String, value: Stored) {
        FileChannel.open(file(org, operation), CREATE_NEW, WRITE).use { channel ->
            val bytes = java.nio.ByteBuffer.wrap((value.digest + "\n" + value.result + "\n" + value.entity + "\n" + String(value.ack, Charsets.UTF_8)).toByteArray())
            while (bytes.hasRemaining()) check(channel.write(bytes) > 0)
            channel.force(true)
        }
        commits.incrementAndGet()
    }

    /** The stored final acknowledgement bytes of one operation, or null. */
    fun stored(org: String, operation: String): ByteArray? = load(org, operation)?.ack

    /** Every committed record of the organization. */
    private fun files(org: String): List<Path> = Files.newDirectoryStream(directory).use { paths ->
        paths.filter { it.fileName.toString().startsWith("$org-") && it.fileName.toString().endsWith(".record") }
    }

    fun records(org: String): Int = files(org).size

    @Synchronized private fun storedEntity(org: String, entity: String): Boolean = files(org).any { path ->
        load(org, path.fileName.toString().removePrefix("$org-").removeSuffix(".record"))!!.let { it.entity == entity && it.result == "ACCEPTED" }
    }

    /** The canonical acknowledgement form (sorted keys, as the server writes and the device re-encodes it). */
    private fun ack(op: EnterpriseOperation, result: String, code: String?, revision: Long?, received: String?, review: String?): ByteArray =
        ("""{"code":""" + (code?.let { "\"$it\"" } ?: "null") + ""","entity_revision":""" + (revision?.toString() ?: "null") +
            ""","operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","received_at":""" + (received?.let { "\"$it\"" } ?: "null") +
            ""","result":"$result","review_item_id":""" + (review?.let { "\"$it\"" } ?: "null") + ""","schema_version":1,"warnings":[]}""").toByteArray()

    @Synchronized private fun decide(org: String, op: EnterpriseOperation): ByteArray {
        load(org, op.operationId)?.let { existing ->
            if (existing.digest == op.payloadDigest) return existing.ack
            return ack(op, "REJECTED", "OPERATION_ID_REUSED", null, received(), null)
        }
        if (op.dependsOn.any { parent -> load(org, parent)?.result != "ACCEPTED" })
            return ack(op, "RETRY", "DEPENDENCY_PENDING", null, null, null)
        val entity = op.entity.reference.type + ":" + op.entity.reference.id
        val orphan = op.softReferences.any { !storedEntity(org, it.type + ":" + it.id) }
        val value = if (orphan) Stored(op.payloadDigest, "ACCEPTED_FOR_REVIEW", entity,
            ack(op, "ACCEPTED_FOR_REVIEW", "ORPHAN_REFERENCE", null, received(), "7d000000-0000-4000-8000-" + (commits.get() + 1).toString().padStart(12, '0')))
        else Stored(op.payloadDigest, "ACCEPTED", entity, ack(op, "ACCEPTED", null, 1, received(), null))
        commit(org, op.operationId, value)
        return value.ack
    }

    private fun received(): String = Instant.parse("2026-11-16T12:00:00Z").plusSeconds(commits.get().toLong() + 1).toString()

    fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val segments = request.url.pathSegments
        check(segments.size == 5 && segments[0] == "v1" && segments[1] == "orgs" && segments[3] == "sync" && segments[4] == "operations") {
            "Only the signed intake belongs to this ledger"
        }
        val org = EnterprisePartition.requireId(segments[2])
        val raw = Buffer().also { request.body!!.writeTo(it) }.readByteArray()
        val operations = SyncBatch.parse(raw).operations.map { it.parse(SyncBinding.of(A, ORIGIN, org)) }
        requests += org to operations.map { it.operationId }
        val answers = operations.map { decide(org, it) }
        afterCommit()
        val armed = fault
        fault = null
        if (armed == Fault.LOSE_ANSWER) throw IOException("Synthetic connection lost after the intake committed")
        val sent = if (armed == Fault.DROP_LAST_ACKNOWLEDGEMENT) answers.dropLast(1) else answers
        val body = "{\"schema_version\":1,\"acknowledgements\":[" + sent.joinToString(",") { String(it, Charsets.UTF_8) } + "]}"
        return Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(200).message("Synthetic ledger answer")
            .header("Cache-Control", "no-store").header("Content-Type", "application/json")
            .body(body.toResponseBody("application/json".toMediaType())).build()
    }
}

/** The product producers with a count of receipt projections per operation (the projection itself is the real one). */
internal class CountingProducer(private val real: EnterpriseQueueProducer, private val counts: MutableMap<String, Int>) : EnterpriseQueueProducer by real {
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement) {
        synchronized(counts) { counts[operation.operationId] = (counts[operation.operationId] ?: 0) + 1 }
        real.acknowledge(tx, attempt, operation, ack)
    }
}

/**
 * The device half's real graph over one vault directory (EDR-351): protected persistence with exportable file test
 * keys (shared with a child JVM), the protected queue, the product trip, checklist, fueling and expense producers, the
 * queue service, the session source with a real [AuthFlow] whose live session never needs the network, and the real
 * signed HTTP client behind [ledger]. The signature itself is synthetic: the Go half verifies real identity-v1 proofs.
 */
internal class ResilienceDevice(directory: Path, keys: Path, val ledger: ResilienceLedger,
    transform: (EnterpriseVaultFiles) -> EnterpriseVaultFiles = { it }) {
    val f = VaultFixture(directory, FileTestVaultKeys(keys), transform)
    @Volatile var session: PortableAuth.Session? = PortableAuth.Session(A, A, "synthetic-access", "synthetic-refresh", 4000, 8000)
    private val sessions = object : AuthSessionStore {
        override fun load(): PortableAuth.Session? = session
        override fun save(session: PortableAuth.Session) { this@ResilienceDevice.session = session }
        override fun clear() { session = null }
    }
    private inline fun <reified T> unused(): T = Proxy.newProxyInstance(T::class.java.classLoader, arrayOf(T::class.java)) { _, method, _ ->
        throw AssertionError("A live synthetic session never calls ${method.name}")
    } as T
    private val auth = AuthFlow(AuthPorts(sessions, unused<AuthKeyStore>(), AuthWallClock { 1000 }, PortableNonceSource { "synthetic-nonce" }, unused<AuthAccountApi>()))
    val fence: EnterpriseSessionFence get() = f.fence
    val source = EnterpriseOperationSessionSource(ORIGIN, sessions, auth, f.fence, { lease, p -> f.store.protectedEnrollment(lease, p) })
    val queue = ProtectedEnterpriseQueue(f.store)
    val checklists = ProtectedChecklistStore(f.store)
    val projections = mutableMapOf<String, Int>()
    private val hints = EnterpriseTripStartHints()
    private val registry = EnterpriseQueueRegistry(*(EnterpriseTripProducers.all(hints).toList() + EnterpriseChecklistProducers.all().toList() +
        listOf(EnterpriseFuelingProducer(), EnterpriseExpenseProducer())).map { CountingProducer(it, projections) }.toTypedArray())
    val http: OkHttpClient = OkHttpClient.Builder().addInterceptor(Interceptor { ledger.intercept(it) }).build()
    val client = EnterpriseOperationHttpClient(http, source, EnterpriseDeviceSigner { _, _, _ ->
        mapOf("Signature" to "synthetic-signature", "Signature-Created" to "1794571200", "Signature-Expires" to "1794571320",
            "Signature-Keyid" to "synthetic-key", "Signature-Nonce" to "synthetic-nonce")
    }, SyncLimits.DEFAULT)
    fun service(transport: EnterpriseOperationTransport = client) = EnterpriseQueueService(source, queue, transport, registry, { f.now }, { 0L })
    suspend fun attempt(p: EnterprisePartition = VaultFixture.partition()): EnterpriseQueueAttempt = source.attempt(source.account(), p)

    /** Moves the device's monotonic and wall clocks forward (same boot). */
    fun advance(millis: Long) { f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + millis, wallMillis = f.now.wallMillis + millis) }

    /**
     * Delivers until nothing is retryable, advancing the clock over every remaining delay; returns the rounds used.
     * [strict] fails when work is still retryable after [rounds]; otherwise the caller asserts what it observes.
     */
    suspend fun drain(p: EnterprisePartition = VaultFixture.partition(), rounds: Int = 30, strict: Boolean = true): Int {
        val service = service()
        repeat(rounds) { round ->
            val result = service.dispatch(source.deliveryAccount(), p)
            if (result.retryable == 0) return round + 1
            advance(result.retryAfterMillis ?: 1_000)
        }
        if (strict) throw AssertionError("The queue did not drain in $rounds rounds")
        return rounds
    }

    fun close() {
        http.connectionPool.evictAll()
        http.dispatcher.executorService.shutdown()
    }
}

/**
 * The same field day as the Go half's script, through the product's own builders (membership B, enrollment C, the
 * vault fixture's grant): the departure checklist run, the trip start naming it, a logbook stop, a fueling, a toll, the
 * finish and the submission. Identifiers and times are fixed so a child process and its parent build identical bytes.
 */
internal object FieldDay {
    val asset: String = ChecklistStoreFixture.asset
    val run = id(1800)
    val trip = id(1810)
    private val t0: Instant = ChecklistStoreFixture.t0
    private fun declared(minutes: Long) = TimeDeclaration(t0.plusSeconds(minutes * 60), "America/Cuiaba", -14_400)
    private fun binding(p: EnterprisePartition) = SyncBinding.of(p.ownerAccount, p.origin, p.organizationId)
    private fun trip(p: EnterprisePartition, op: Int, minutes: Long) = TripEnvelope(binding(p), B, C, id(op), declared(minutes))

    const val RUN = 1900
    const val START = 1901
    const val STOP = 1902
    const val FUELING = 1903
    const val EXPENSE = 1904
    const val FINISH = 1905
    const val SUBMIT = 1906
    val ALL = listOf(RUN, START, STOP, FUELING, EXPENSE, FINISH, SUBMIT)

    fun start(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation = TripOperations.start(trip(p, START, 10), trip, asset, id(1830),
        TripReadingInput(id(1840), id(1841), id(1850), "ODOMETER", "184320000"), null, TripClockReading("boot-7", 1_000), departureRunId = run)
    fun stop(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation =
        TripOperations.event(trip(p, STOP, 40), id(1860), trip, "STOP", "Pátio sintético", id(START))
    fun fueling(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation = FuelingOperations.submit(
        FuelingEnvelope(binding(p), B, C, id(FUELING), declared(120)), id(1870),
        FuelingSubmitInput(asset, id(1830), "DIESEL_S10", "L", "50000", "32450", null, "6490", null, "PARTIAL", "COMPANY", "FUEL_CARD",
            FuelingReadingInput(id(1840), id(1841), id(1851), "ODOMETER", "184532000"), null, "NOT_FOUND", "Posto sintético"))
    fun expense(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation = ExpenseOperations.submit(
        ExpenseEnvelope(binding(p), B, C, id(EXPENSE), declared(135)), id(1880), asset, "TOLL", "1250", "1250", "EMPLOYEE", null)
    fun finish(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation = TripOperations.finish(trip(p, FINISH, 180), trip,
        TripReadingInput(id(1840), id(1841), id(1852), "ODOMETER", "184610000"), null, TripClockReading("boot-7", 10_801_000), id(START))
    fun submit(p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation = TripOperations.submit(trip(p, SUBMIT, 210), trip, 1, id(FINISH))

    /** The departure run started and answered on the device (its book); no operation yet. */
    suspend fun prepareChecklist(device: ResilienceDevice, p: EnterprisePartition = VaultFixture.partition()) {
        val t = ChecklistStoreFixture.t0
        device.checklists.update(device.attempt(p)) { it.withApplicable(ChecklistStoreFixture.applicable) }
        device.checklists.update(device.attempt(p)) { it.withStart(run, asset, ChecklistContext.DEPARTURE, t) }
        device.checklists.update(device.attempt(p)) { book ->
            (1..3).fold(book) { current, position -> current.withAnswer(run, position, ChecklistAnswer.OK, null, t) }
        }
    }

    /** The completion operation of the open departure run, preparing the run first when the book has none. */
    suspend fun checklist(device: ResilienceDevice, p: EnterprisePartition = VaultFixture.partition()): EnterpriseOperation {
        if (device.checklists.read(device.attempt(p)).book?.run(run) == null) prepareChecklist(device, p)
        return ChecklistStoreFixture.complete(p, device.checklists.read(device.attempt(p)).book!!, run, RUN, ChecklistStoreFixture.t0.plusSeconds(60))
    }

    /** Saves the whole field day through the queue service, in the order a driver records it. */
    suspend fun save(device: ResilienceDevice, p: EnterprisePartition = VaultFixture.partition()): List<EnterpriseOperation> {
        val service = device.service()
        val operations = listOf(checklist(device, p), start(p), stop(p), fueling(p), expense(p), finish(p), submit(p))
        for (operation in operations) check(service.save(p, operation)) { "A field-day operation was not newly saved" }
        return operations
    }
}
