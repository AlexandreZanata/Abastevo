package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterpriseContactAuthority
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseContextProtection
import com.anpfuel.application.enterprise.EnterprisePersistence
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentService
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentStore
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentTransport
import com.anpfuel.application.enterprise.attachment.EnterpriseReceipts
import com.anpfuel.application.enterprise.attachment.ReceiptOriginals
import com.anpfuel.application.enterprise.checklist.ChecklistFlow
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistGateway
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistProducers
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistService
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistStore
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceCacheStore
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceService
import com.anpfuel.application.enterprise.fleet.EnterpriseReferenceTransport
import com.anpfuel.application.enterprise.fleet.MyAssetsFlow
import com.anpfuel.application.enterprise.queue.EnterpriseOperationQueue
import com.anpfuel.application.enterprise.queue.EnterpriseOperationTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseReviewQueue
import com.anpfuel.application.enterprise.queue.EnterpriseReviewService
import com.anpfuel.application.enterprise.queue.EnterpriseReviewTransport
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.sync.EnterpriseSyncFlow
import com.anpfuel.application.enterprise.sync.EnterpriseSyncStatusStore
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseProducer
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseService
import com.anpfuel.application.enterprise.expense.EnterpriseExpenseStore
import com.anpfuel.application.enterprise.expense.ExpenseFlow
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingProducer
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingService
import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingStore
import com.anpfuel.application.enterprise.fueling.FuelingFlow
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.application.enterprise.trip.EnterpriseTripService
import com.anpfuel.application.enterprise.trip.EnterpriseTripStartHints
import com.anpfuel.application.enterprise.trip.EnterpriseTripStore
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.application.enterprise.trip.TripRelatedRecords
import com.anpfuel.application.enterprise.trip.TripTracking
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingPolicyGateway
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingProducers
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingService
import com.anpfuel.application.enterprise.tracking.EnterpriseTrackingStore
import com.anpfuel.application.enterprise.tracking.TrackingDeviceObserver
import com.anpfuel.application.enterprise.tracking.TrackingRuntime
import com.anpfuel.application.enterprise.tracking.TrackingStartGate
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.BuildConfig
import com.anpfuel.data.enterprise.*
import com.anpfuel.data.enterprise.attachment.AndroidReceiptOriginals
import com.anpfuel.data.enterprise.attachment.EnterpriseAttachmentHttpClient
import com.anpfuel.data.enterprise.attachment.ProtectedAttachmentStore
import com.anpfuel.data.enterprise.checklist.ProtectedChecklistStore
import com.anpfuel.data.enterprise.fleet.EnterpriseReferenceHttpClient
import com.anpfuel.data.enterprise.fleet.ProtectedReferenceCacheStore
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.data.enterprise.expense.ProtectedExpenseStore
import com.anpfuel.data.enterprise.fueling.ProtectedFuelingStore
import com.anpfuel.data.enterprise.trip.ProtectedTripStore
import com.anpfuel.data.enterprise.tracking.ProtectedTrackingStore
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.domain.enterprise.sync.SyncLimits
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import java.time.Instant
import java.time.ZoneId
import java.util.UUID
import java.util.concurrent.TimeUnit
import javax.inject.Named
import javax.inject.Singleton
import kotlin.random.Random
import okhttp3.OkHttpClient

/** Signed contact and protected custody share source-owned account and business authority boundaries. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseModule {
    @Provides
    @Singleton
    fun sessionFence(): EnterpriseSessionFence = EnterpriseSessionFence()

    @Provides
    @Singleton
    fun vaultFiles(@ApplicationContext context: Context): EnterpriseVaultFiles =
        AtomicEnterpriseVaultFiles(File(context.noBackupFilesDir, "enterprise-sealed-v1"))

    @Provides
    @Singleton
    fun vaultKeys(): EnterpriseVaultKeys = AndroidEnterpriseVaultKeys()

    @Provides
    @Singleton
    fun erasure(files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys, fence: EnterpriseSessionFence): EnterpriseVaultErasure =
        EnterpriseVaultErasure(files, keys, fence)

    @Provides
    @Singleton
    fun vaultJanitor(@ApplicationContext context: Context, files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys,
        fence: EnterpriseSessionFence, erasure: EnterpriseVaultErasure): EnterpriseVaultJanitor =
        EnterpriseVaultJanitor(files, keys, fence, erasure, AndroidEnterpriseDeviceClock(context)::read)

    @Provides
    @Singleton
    fun contactAuthority(client: EnterpriseDeviceContactClient): EnterpriseContactAuthority = client

    @Provides
    @Singleton
    fun contactSigner(proof: PhotoProofTransport): EnterpriseDeviceSigner = EnterpriseIdentityV1ContactSigner(proof)

    @Provides
    @Singleton
    fun signedContact(
        @Named("apiOrigin") environment: ApiEnvironment, sessions: AuthSessionStore, auth: AuthFlow,
        fence: EnterpriseSessionFence, signer: EnterpriseDeviceSigner,
    ): EnterpriseDeviceContactClient = EnterpriseDeviceContactClient(
        OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
        environment.origin, sessions, auth, fence, signer,
    )

    @Provides
    @Singleton
    fun contextProtection(store: ProtectedEnterprisePersistence): EnterpriseContextProtection = store

    @Provides
    @Singleton
    fun protectedStore(
        @ApplicationContext context: Context,
        files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys, fence: EnterpriseSessionFence,
        erasure: EnterpriseVaultErasure, sessions: AuthSessionStore,
        @Named("apiOrigin") environment: ApiEnvironment,
        authority: EnterpriseContactAuthority,
    ): ProtectedEnterprisePersistence = ProtectedEnterprisePersistence(
        files, keys, fence, erasure,
        identity = {
            runCatching {
                sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, environment.origin, it.familyId) }
            }.getOrNull()
        },
        clock = AndroidEnterpriseDeviceClock(context)::read,
        authority = authority,
    )

    @Provides
    @Singleton
    fun persistence(store: ProtectedEnterprisePersistence): EnterprisePersistence = store

    @Provides
    @Singleton
    fun operationQueue(store: ProtectedEnterprisePersistence): ProtectedEnterpriseQueue = ProtectedEnterpriseQueue(store)

    @Provides
    @Singleton
    fun typedOperationQueue(queue: ProtectedEnterpriseQueue): EnterpriseOperationQueue = queue

    @Provides
    @Singleton
    fun operationSource(
        @Named("apiOrigin") environment: ApiEnvironment, sessions: AuthSessionStore, auth: AuthFlow,
        fence: EnterpriseSessionFence, store: ProtectedEnterprisePersistence,
    ): EnterpriseOperationSessionSource = EnterpriseOperationSessionSource(
        environment.origin, sessions, auth, fence, store::protectedEnrollment,
    )

    @Provides
    @Singleton
    fun queueSource(source: EnterpriseOperationSessionSource): EnterpriseQueueSource = source

    @Provides
    @Singleton
    fun queueLimits(): SyncLimits = SyncLimits.DEFAULT

    @Provides
    @Singleton
    fun operationClient(source: EnterpriseOperationSessionSource, signer: EnterpriseDeviceSigner,
        limits: SyncLimits): EnterpriseOperationHttpClient = EnterpriseOperationHttpClient(
        OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
        source, signer, limits,
    )

    @Provides
    @Singleton
    fun operationTransport(client: EnterpriseOperationHttpClient): EnterpriseOperationTransport = client

    /** The protected queue's separate review-only port (EDR-120); delivery ports stay unchanged. */
    @Provides
    @Singleton
    fun reviewQueue(queue: ProtectedEnterpriseQueue): EnterpriseReviewQueue = queue

    @Provides
    @Singleton
    fun reviewClient(source: EnterpriseOperationSessionSource, signer: EnterpriseDeviceSigner): EnterpriseReviewHttpClient =
        EnterpriseReviewHttpClient(
            OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
            source, signer,
        )

    @Provides
    @Singleton
    fun reviewTransport(client: EnterpriseReviewHttpClient): EnterpriseReviewTransport = client

    @Provides
    @Singleton
    fun reviewService(@ApplicationContext context: Context, source: EnterpriseQueueSource, queue: EnterpriseReviewQueue,
        transport: EnterpriseReviewTransport, limits: SyncLimits, activity: EnterpriseDeliveryActivity): EnterpriseReviewService =
        EnterpriseReviewService(source, queue, transport, AndroidEnterpriseDeviceClock(context)::read,
            entropy = { bound -> Random.Default.nextLong(bound) }, activity = activity, limits = limits)

    /** EMP-FLEET-013: the signed reference feed on the device proof plane (EDR-225). */
    @Provides
    @Singleton
    fun referenceClient(source: EnterpriseOperationSessionSource, signer: EnterpriseDeviceSigner): EnterpriseReferenceHttpClient =
        EnterpriseReferenceHttpClient(
            OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
            source, signer,
        )

    @Provides
    @Singleton
    fun referenceTransport(client: EnterpriseReferenceHttpClient): EnterpriseReferenceTransport = client

    /** The protected generation's sealed reference cache, never a plaintext table (EDR-224). */
    @Provides
    @Singleton
    fun referenceCache(store: ProtectedEnterprisePersistence): EnterpriseReferenceCacheStore = ProtectedReferenceCacheStore(store)

    @Provides
    @Singleton
    fun referenceService(@ApplicationContext context: Context, source: EnterpriseQueueSource, cache: EnterpriseReferenceCacheStore,
        transport: EnterpriseReferenceTransport): EnterpriseReferenceService =
        EnterpriseReferenceService(source, cache, transport, AndroidEnterpriseDeviceClock(context)::read)

    /** Each visible "Meus ativos" owner is independent, like the sync status owner. */
    @Provides
    fun myAssetsFlow(@ApplicationContext context: Context, source: EnterpriseQueueSource, service: EnterpriseReferenceService,
        fence: EnterpriseSessionFence): MyAssetsFlow = MyAssetsFlow(source, service, fence, AndroidEnterpriseDeviceClock(context)::read)

    /** In-memory start labels for the trip producers; never persisted elsewhere or used as authority (EDR-242). */
    @Provides
    @Singleton
    fun tripStartHints(): EnterpriseTripStartHints = EnterpriseTripStartHints()

    /**
     * Owning offline business producers are registered only after their own acceptance: the four EMP-TRIP-005
     * `trip.*` producers (EDR-242) are the first, then the EMP-CHECK-002 `check.run.complete` producer (EDR-283), then the
     * six EMP-GPS-002 `gps.session.*` producers (EDR-322). Their queued work keeps its delivery whatever the UI flags say.
     */
    @Provides
    @Singleton
    fun queueRegistry(hints: EnterpriseTripStartHints): EnterpriseQueueRegistry =
        // EMP-FUEL-011 adds the `fueling.submit` producer (EDR-295), EMP-EXP-004 the `expense.submit` producer (EDR-336),
        // EMP-GPS-002 the `gps.session.*` producers (EDR-321).
        EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), *EnterpriseChecklistProducers.all(), EnterpriseFuelingProducer(),
            EnterpriseExpenseProducer(), *EnterpriseTrackingProducers.all())

    /** The sealed trip book and the entries it names, never a plaintext table (EDR-242). */
    @Provides
    @Singleton
    fun tripStore(store: ProtectedEnterprisePersistence): EnterpriseTripStore = ProtectedTripStore(store)

    @Provides
    @Singleton
    fun tripService(@ApplicationContext context: Context, source: EnterpriseQueueSource, delivery: EnterpriseQueueService,
        store: EnterpriseTripStore, hints: EnterpriseTripStartHints): EnterpriseTripService = EnterpriseTripService(
        source, { partition, operation -> delivery.save(partition, operation) }, store, hints,
        AndroidEnterpriseDeviceClock(context)::read, { ZoneId.systemDefault() },
    )

    /** Each visible trip owner is independent, like "Meus ativos"; "Enviar agora" is one dispatch of its partition. */
    @Provides
    fun tripFlow(@ApplicationContext context: Context, source: EnterpriseQueueSource, service: EnterpriseTripService,
        fence: EnterpriseSessionFence, delivery: EnterpriseQueueService, receipts: EnterpriseReceipts, fuelings: EnterpriseFuelingService,
        expenses: EnterpriseExpenseService, tracking: EnterpriseTrackingService, runtime: TrackingRuntime, device: TrackingDeviceObserver): TripFlow =
        // EMP-TRIP-020: the closing conference reads the same partition's own fuelings and expenses (EDR-339);
        // EMP-GPS-002: the tracking hooks of the open trip (EDR-323).
        TripFlow(source, service, fence, receipts, TripRelatedRecords(fuelings::fuelings, expenses::expenses),
            // EMP-GPS-010: the cause observed at recovery and the one session this phone collects (B-BR-EMP-544/547).
            TripTracking(tracking, runtime::running, { AndroidEnterpriseDeviceClock(context).read().boot }, device::observe,
                { runtime.collector?.runningSessionId })) { account, partition ->
            delivery.dispatch(account, partition)
        }

    /** EMP-FUEL-011: the partition's retained `fueling.submit` entries, never a plaintext table (EDR-295). */
    @Provides
    @Singleton
    fun fuelingStore(store: ProtectedEnterprisePersistence): EnterpriseFuelingStore = ProtectedFuelingStore(store)

    @Provides
    @Singleton
    fun fuelingService(source: EnterpriseQueueSource, delivery: EnterpriseQueueService, store: EnterpriseFuelingStore): EnterpriseFuelingService =
        EnterpriseFuelingService(source, { partition, operation -> delivery.save(partition, operation) }, store, { ZoneId.systemDefault() })

    /** Each visible fueling owner is independent, like the trip owner; its receipts go through EMP-DOC-004. */
    @Provides
    fun fuelingFlow(source: EnterpriseQueueSource, service: EnterpriseFuelingService, fence: EnterpriseSessionFence,
        delivery: EnterpriseQueueService, receipts: EnterpriseReceipts): FuelingFlow =
        FuelingFlow(source, service, fence, receipts) { account, partition -> delivery.dispatch(account, partition) }

    /** EMP-EXP-004: the partition's retained `expense.submit` entries, never a plaintext table (EDR-336). */
    @Provides
    @Singleton
    fun expenseStore(store: ProtectedEnterprisePersistence): EnterpriseExpenseStore = ProtectedExpenseStore(store)

    @Provides
    @Singleton
    fun expenseService(source: EnterpriseQueueSource, delivery: EnterpriseQueueService, store: EnterpriseExpenseStore): EnterpriseExpenseService =
        EnterpriseExpenseService(source, { partition, operation -> delivery.save(partition, operation) }, store, { ZoneId.systemDefault() })

    /** Each visible expense owner is independent, like the fueling owner; its receipts go through EMP-DOC-004. */
    @Provides
    fun expenseFlow(source: EnterpriseQueueSource, service: EnterpriseExpenseService, fence: EnterpriseSessionFence,
        delivery: EnterpriseQueueService, receipts: EnterpriseReceipts): ExpenseFlow =
        ExpenseFlow(source, service, fence, receipts) { account, partition -> delivery.dispatch(account, partition) }

    /** The sealed tracking book, its trip book and the entries they name, never a plaintext table (EDR-322). */
    @Provides
    @Singleton
    fun trackingStore(store: ProtectedEnterprisePersistence): EnterpriseTrackingStore = ProtectedTrackingStore(store)

    @Provides
    @Singleton
    fun trackingService(source: EnterpriseQueueSource, delivery: EnterpriseQueueService, store: EnterpriseTrackingStore,
        gateway: EnterpriseTrackingPolicyGateway): EnterpriseTrackingService = EnterpriseTrackingService(
        source, { partition, operation -> delivery.save(partition, operation) }, store, gateway, { ZoneId.systemDefault() },
    )

    /** The process's one tracking runtime: the visible start gate (elapsed time) and the running collector (EDR-323). */
    @Provides
    @Singleton
    fun trackingRuntime(): TrackingRuntime = TrackingRuntime(TrackingStartGate { android.os.SystemClock.elapsedRealtime() })

    /** The sealed checklist book and the entries it names, never a plaintext table (EDR-283). */
    @Provides
    @Singleton
    fun checklistStore(store: ProtectedEnterprisePersistence): EnterpriseChecklistStore = ProtectedChecklistStore(store)

    @Provides
    @Singleton
    fun checklistService(source: EnterpriseQueueSource, delivery: EnterpriseQueueService, store: EnterpriseChecklistStore,
        gateway: EnterpriseChecklistGateway): EnterpriseChecklistService = EnterpriseChecklistService(
        source, { partition, operation -> delivery.save(partition, operation) }, store, gateway, { ZoneId.systemDefault() },
    )

    /** Each visible checklist owner is independent, like the trip owner; "Enviar agora" is one dispatch of its partition. */
    @Provides
    fun checklistFlow(source: EnterpriseQueueSource, service: EnterpriseChecklistService, fence: EnterpriseSessionFence,
        delivery: EnterpriseQueueService): ChecklistFlow = ChecklistFlow(source, service, fence) { account, partition -> delivery.dispatch(account, partition) }

    @Provides
    @Singleton
    fun deliveryActivity(): EnterpriseDeliveryActivity = EnterpriseDeliveryActivity()

    @Provides
    @Singleton
    fun syncStatus(store: ProtectedEnterprisePersistence): EnterpriseSyncStatusStore = ProtectedEnterpriseSyncStatusStore(store)

    /** Each visible status owner is independent; no shared UI selection or retained view state. */
    @Provides
    fun syncFlow(source: EnterpriseQueueSource, store: EnterpriseSyncStatusStore,
        delivery: EnterpriseQueueService, activity: EnterpriseDeliveryActivity,
        fence: EnterpriseSessionFence, reviews: EnterpriseReviewService): EnterpriseSyncFlow =
        EnterpriseSyncFlow(source, store, delivery, activity, fence, reviews)

    @Provides
    @Singleton
    fun queueService(@ApplicationContext context: Context, source: EnterpriseQueueSource,
        queue: EnterpriseOperationQueue, transport: EnterpriseOperationTransport, registry: EnterpriseQueueRegistry,
        limits: SyncLimits, activity: EnterpriseDeliveryActivity): EnterpriseQueueService = EnterpriseQueueService(
        source, queue, transport, registry, AndroidEnterpriseDeviceClock(context)::read,
        entropy = { bound -> Random.Default.nextLong(bound) }, limits = limits, activity = activity,
    )

    /** EMP-OFFL-005: attachment bytes on the device proof plane and the capability-bound ingress (EDR-270). */
    @Provides
    @Singleton
    fun attachmentClient(source: EnterpriseOperationSessionSource, signer: EnterpriseDeviceSigner): EnterpriseAttachmentHttpClient =
        EnterpriseAttachmentHttpClient(
            OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(30, TimeUnit.SECONDS).writeTimeout(30, TimeUnit.SECONDS).build(),
            source, signer,
        )

    @Provides
    @Singleton
    fun attachmentTransport(client: EnterpriseAttachmentHttpClient): EnterpriseAttachmentTransport = client

    @Provides
    @Singleton
    fun attachmentStore(store: ProtectedEnterprisePersistence): EnterpriseAttachmentStore = ProtectedAttachmentStore(store)

    /** Off unless `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED` (EDR-267): real receipts stay legally gated (OQ-04). */
    @Provides
    @Singleton
    fun attachmentService(@ApplicationContext context: Context, source: EnterpriseQueueSource, store: EnterpriseAttachmentStore,
        transport: EnterpriseAttachmentTransport): EnterpriseAttachmentService = EnterpriseAttachmentService(
        source, store, transport, AndroidEnterpriseDeviceClock(context)::read, Instant::now,
        entropy = { bound -> Random.Default.nextLong(bound) }, newId = { UUID.randomUUID().toString() },
        enabled = BuildConfig.ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED,
    )

    /** EMP-DOC-004: the thin capture face of the attachment queue, off with it (EDR-293). */
    @Provides
    @Singleton
    fun receipts(attachments: EnterpriseAttachmentService): EnterpriseReceipts =
        EnterpriseReceipts(attachments, BuildConfig.ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED)

    /** EMP-DOC-004: the private camera file, bounded reads and the decoded legibility sample (EDR-291/292). */
    @Provides
    @Singleton
    fun receiptOriginals(@ApplicationContext context: Context): ReceiptOriginals = AndroidReceiptOriginals(context)

    @Provides
    @Singleton
    fun operationRunner(source: EnterpriseQueueSource, store: ProtectedEnterprisePersistence,
        service: EnterpriseQueueService, reviews: EnterpriseReviewService, attachments: EnterpriseAttachmentService): EnterpriseOperationWorkRunner = EnterpriseOperationWorkRunner(
        source, store::discover, { account, partition -> service.dispatch(account, partition) },
        { account, partition -> reviews.poll(account, partition) },
        if (BuildConfig.ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED) { account, partition -> attachments.deliver(account, partition) } else null,
    )

    @Provides
    @Singleton
    fun localContext(
        auth: AuthFlow, @Named("apiOrigin") environment: ApiEnvironment, persistence: EnterprisePersistence,
        fence: EnterpriseSessionFence,
    ): EnterpriseLocalContext =
        EnterpriseLocalContext(
            ownerAccount = {
                when (val state = auth.rehydrate()) {
                    is AuthFlow.AuthState.Active -> state.session.accountId
                    is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
                    else -> null
                }
            },
            serverOrigin = { environment.origin },
            persistence = persistence,
            accessEpoch = { fence.epoch },
        )
}
