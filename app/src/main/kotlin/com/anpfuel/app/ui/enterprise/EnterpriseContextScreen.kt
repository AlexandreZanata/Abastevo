package com.anpfuel.app.ui.enterprise

import android.app.Activity
import android.app.KeyguardManager
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.anpfuel.app.BuildConfig
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureUi
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureViewModel
import com.anpfuel.app.ui.enterprise.attachment.rememberSystemReceiptPickers
import com.anpfuel.app.ui.enterprise.checklist.ChecklistActions
import com.anpfuel.app.ui.enterprise.checklist.ChecklistHomePanel
import com.anpfuel.app.ui.enterprise.checklist.ChecklistRunPanel
import com.anpfuel.app.ui.enterprise.checklist.ChecklistViewModel
import com.anpfuel.app.ui.enterprise.checklist.ChecklistsHomeCard
import com.anpfuel.app.ui.enterprise.checklist.departureRun
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsHomeCard
import com.anpfuel.app.ui.enterprise.expense.ExpensePanel
import com.anpfuel.app.ui.enterprise.expense.ExpenseViewModel
import com.anpfuel.app.ui.enterprise.expense.ExpensesHomeCard
import com.anpfuel.app.ui.enterprise.fueling.FuelingPanel
import com.anpfuel.app.ui.enterprise.fueling.FuelingViewModel
import com.anpfuel.app.ui.enterprise.fueling.FuelingsHomeCard
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsPanel
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsViewModel
import com.anpfuel.app.ui.enterprise.trip.TripPanel
import com.anpfuel.app.ui.enterprise.trip.TripViewModel
import com.anpfuel.app.ui.enterprise.trip.TripsHomeCard
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseContextState
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.checklist.ChecklistViewState
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnterpriseContextScreen(
    onPersonal: () -> Unit,
    onSignIn: () -> Unit,
    onInvitation: () -> Unit,
    viewModel: EnterpriseContextViewModel = hiltViewModel(),
    syncViewModel: EnterpriseSyncViewModel = hiltViewModel(),
    assetsViewModel: MyAssetsViewModel = hiltViewModel(),
    tripViewModel: TripViewModel = hiltViewModel(),
    receiptViewModel: ReceiptCaptureViewModel = hiltViewModel(),
    fuelingViewModel: FuelingViewModel = hiltViewModel(),
    checklistViewModel: ChecklistViewModel = hiltViewModel(),
    expenseViewModel: ExpenseViewModel = hiltViewModel(),
    /** EMP-ROUTE-004: opens the app's own route preview with an approved destination query; last, so positional callers stay valid. */
    onNavigate: (String) -> Unit = {},
) {
    // These are cheap local invalidation flows; keep private presentation current
    // while stopped rather than briefly resuming an earlier confirmed context.
    val state by viewModel.state.collectAsState()
    val connected by viewModel.connected.collectAsState()
    val syncState by syncViewModel.state.collectAsState()
    val assetsState by assetsViewModel.state.collectAsState()
    var showAssets by remember { mutableStateOf(false) }
    var assetsFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    val tripState by tripViewModel.state.collectAsState()
    // EMP-DOC-004: one capture sheet owner; its unsaved photo stays in memory only (B-BR-EMP-453).
    val receiptState by receiptViewModel.state.collectAsState()
    val receiptPickers = rememberSystemReceiptPickers(receiptViewModel)
    var showTrips by remember { mutableStateOf(false) }
    var tripsFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    val fuelingState by fuelingViewModel.state.collectAsState()
    var showFuelings by remember { mutableStateOf(false) }
    var fuelingsFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    val expenseState by expenseViewModel.state.collectAsState()
    var showExpenses by remember { mutableStateOf(false) }
    var expensesFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    val checklistState by checklistViewModel.state.collectAsState()
    var showChecklists by remember { mutableStateOf(false) }
    var checklistsFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    val renderedGeneration = syncState.generation
    val renderedSelected = state.selected
    var showSync by remember { mutableStateOf(false) }
    var openedFor by remember { mutableStateOf<EnterpriseContextSummary?>(null) }
    var resumed by remember { mutableStateOf(false) }
    // Old UI callbacks retain their rendered primitive, never the newer delegate.
    val renderedEpoch = viewModel.epochs.collectAsState().value
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var choosing by remember { mutableStateOf(false) }
    var recovery by remember { mutableStateOf<Pair<EnterpriseContextSummary, Long>?>(null) }
    var protectionNotice by remember { mutableStateOf<Int?>(null) }
    val androidContext = LocalContext.current
    val unlock = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        // Unlock never resumes an old request or grants authority. Selecting
        // again starts a fresh owner/proof ceremony after lifecycle invalidation.
        viewModel.unconfirm()
        if (result.resultCode == Activity.RESULT_OK) protectionNotice = R.string.enterprise_protection_unlock_done
    }
    fun currentGesture(): Boolean = syncViewModel.isCurrent(renderedEpoch, renderedGeneration, allowBusy = true) &&
        viewModel.state.value.selected == renderedSelected
    fun closeAssets() {
        assetsViewModel.withdraw()
        showAssets = false
        assetsFor = null
    }
    // SCR-TRIP-01 (EMP-TRIP-005): withdrawn with the same gestures and lifecycle as "Meus ativos".
    fun closeTrips() {
        tripViewModel.withdraw()
        // An unsaved photo never survives its context (B-BR-EMP-453).
        receiptViewModel.reset()
        // The trip start's departure line borrows the checklist owner (EMP-CHECK-002); it leaves with the trips.
        if (!showChecklists) checklistViewModel.withdraw()
        showTrips = false
        tripsFor = null
    }
    // SCR-FUEL-01/02 (EMP-FUEL-011): withdrawn with the same gestures and lifecycle as the trips.
    fun closeFuelings() {
        fuelingViewModel.withdraw()
        receiptViewModel.reset()
        showFuelings = false
        fuelingsFor = null
    }
    // SCR-EXP-01 (EMP-EXP-004): withdrawn with the same gestures and lifecycle as the fuelings.
    fun closeExpenses() {
        expenseViewModel.withdraw()
        receiptViewModel.reset()
        showExpenses = false
        expensesFor = null
    }
    // Checklists (EMP-CHECK-002): withdrawn with the same gestures and lifecycle as the trips.
    fun closeChecklists() {
        checklistViewModel.withdraw()
        showChecklists = false
        checklistsFor = null
    }
    fun withdrawForGesture(): Boolean {
        if (!currentGesture()) return false
        syncViewModel.withdraw()
        showSync = false
        openedFor = null
        closeAssets()
        closeTrips()
        closeFuelings()
        closeExpenses()
        closeChecklists()
        return true
    }
    fun requestUnlock() {
        if (!withdrawForGesture()) return
        viewModel.unconfirm()
        recovery = null
        val manager = androidContext.getSystemService(KeyguardManager::class.java)
        if (manager?.isDeviceSecure != true) { protectionNotice = R.string.enterprise_protection_device_lock_required; return }
        @Suppress("DEPRECATION")
        val intent = manager.createConfirmDeviceCredentialIntent(androidContext.getString(R.string.enterprise_protection_unlock_title),
            androidContext.getString(R.string.enterprise_protection_unlock_copy))
        if (intent == null) { protectionNotice = R.string.enterprise_context_unavailable; return }
        try { unlock.launch(intent) }
        catch (_: Exception) { protectionNotice = R.string.enterprise_context_unavailable }
    }
    val personal = { if (withdrawForGesture()) { viewModel.selectPersonal(); onPersonal() } }
    val signIn = { if (withdrawForGesture()) { viewModel.unconfirm(); onSignIn() } }
    val invitation = { if (withdrawForGesture()) { viewModel.unconfirm(); onInvitation() } }
    val syncMatches = resumed && renderedSelected != null && syncState.epoch == renderedEpoch &&
        syncState.context?.partition == renderedSelected.partition && syncState.context?.membershipId == renderedSelected.membershipId
    fun syncBack() {
        if (!currentGesture()) return
        if (syncMatches && syncState.detail != null) syncViewModel.backToList(renderedEpoch, renderedGeneration)
        else if (withdrawForGesture()) {
            renderedSelected?.let { syncViewModel.open(it, renderedEpoch, syncViewModel.state.value.generation) }
        }
    }
    // "Meus ativos" (EMP-FLEET-013) is shown only for the exact selected partition, member and epoch.
    val assetsMatches = resumed && renderedSelected != null && assetsState.epoch == renderedEpoch &&
        assetsState.context?.partition == renderedSelected.partition && assetsState.context?.membershipId == renderedSelected.membershipId
    // Trips are shown only for the exact selected partition, member and epoch; their writes keep their creation partition.
    val tripsMatch = resumed && renderedSelected != null && tripState.epoch == renderedEpoch &&
        tripState.context?.partition == renderedSelected.partition && tripState.context?.membershipId == renderedSelected.membershipId
    // Fuelings likewise: the exact selected partition, member and epoch only.
    val fuelingsMatch = resumed && renderedSelected != null && fuelingState.epoch == renderedEpoch &&
        fuelingState.context?.partition == renderedSelected.partition && fuelingState.context?.membershipId == renderedSelected.membershipId
    // Expenses likewise: the exact selected partition, member and epoch only.
    val expensesMatch = resumed && renderedSelected != null && expenseState.epoch == renderedEpoch &&
        expenseState.context?.partition == renderedSelected.partition && expenseState.context?.membershipId == renderedSelected.membershipId
    // Checklists are shown only for the exact selected partition, member and epoch; their writes keep their creation partition.
    val checklistMatch = resumed && renderedSelected != null && checklistState.epoch == renderedEpoch &&
        checklistState.context?.partition == renderedSelected.partition && checklistState.context?.membershipId == renderedSelected.membershipId
    // A source loss clears the owner; keep only its safe message and withhold every checklist fact.
    val renderedChecklist = if (checklistMatch) checklistState else ChecklistViewState(error = checklistState.error)
    fun checklistActions(selected: EnterpriseContextSummary) = ChecklistActions(
        onRefresh = { if (checklistMatch) checklistViewModel.refresh(renderedEpoch, checklistState.generation) },
        onRetry = {
            val asset = checklistState.assetId
            val moment = checklistState.checklistContext
            if (asset != null && moment != null) checklistViewModel.open(selected, asset, moment, renderedEpoch)
        },
        onStart = { if (checklistMatch) checklistViewModel.start(renderedEpoch, checklistState.generation) },
        onAnswer = { run, position, answer, note -> if (checklistMatch) checklistViewModel.answer(run, position, answer, note, renderedEpoch, checklistState.generation) },
        onComplete = { run, acknowledged -> if (checklistMatch) checklistViewModel.complete(run, acknowledged, renderedEpoch, checklistState.generation) },
        onSend = { if (checklistMatch) checklistViewModel.sendNow(renderedEpoch, checklistState.generation) },
    )
    SecureAccountWindow()
    DisposableEffect(viewModel, syncViewModel, assetsViewModel, tripViewModel, fuelingViewModel, checklistViewModel, expenseViewModel) {
        onDispose { syncViewModel.withdraw(); assetsViewModel.withdraw(); tripViewModel.withdraw(); fuelingViewModel.withdraw()
            checklistViewModel.withdraw(); expenseViewModel.withdraw(); viewModel.unconfirm() }
    }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) {
        resumed = true; syncViewModel.invalidateIfStale(); assetsViewModel.invalidateIfStale(); tripViewModel.invalidateIfStale()
        fuelingViewModel.invalidateIfStale(); checklistViewModel.invalidateIfStale(); expenseViewModel.invalidateIfStale(); viewModel.invalidateIfStale()
    }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
        resumed = false; showSync = false; openedFor = null; recovery = null; syncViewModel.withdraw(); closeAssets(); closeTrips(); closeFuelings()
        closeExpenses(); closeChecklists(); viewModel.unconfirm()
    }
    LaunchedEffect(renderedSelected, renderedEpoch, resumed, choosing, state.busy, state.selectionConfirmed) {
        val selected = renderedSelected
        if (viewModel.state.value.selected != selected || viewModel.epochs.value != renderedEpoch) return@LaunchedEffect
        if (openedFor != null && (openedFor?.partition != selected?.partition || openedFor?.membershipId != selected?.membershipId)) {
            showSync = false
            openedFor = null
        }
        if (assetsFor != null && (assetsFor?.partition != selected?.partition || assetsFor?.membershipId != selected?.membershipId)) closeAssets()
        if (tripsFor != null && (tripsFor?.partition != selected?.partition || tripsFor?.membershipId != selected?.membershipId)) closeTrips()
        if (fuelingsFor != null && (fuelingsFor?.partition != selected?.partition || fuelingsFor?.membershipId != selected?.membershipId)) closeFuelings()
        if (expensesFor != null && (expensesFor?.partition != selected?.partition || expensesFor?.membershipId != selected?.membershipId)) closeExpenses()
        if (checklistsFor != null && (checklistsFor?.partition != selected?.partition || checklistsFor?.membershipId != selected?.membershipId)) closeChecklists()
        if (!resumed || selected == null) { syncViewModel.withdraw(); showSync = false; openedFor = null }
        else if (!choosing && !state.busy && resumed && lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) &&
            !viewModel.state.value.busy && viewModel.state.value.selected == selected && viewModel.epochs.value == renderedEpoch) {
            // An old effect may not borrow a newer status generation for its old selector.
            val current = syncViewModel.state.value
            if (current.epoch != renderedEpoch || current.context?.partition != selected.partition || current.context?.membershipId != selected.membershipId) {
                syncViewModel.open(selected, renderedEpoch, current.generation)
            }
        }
    }
    LaunchedEffect(renderedEpoch, state.confirmationTarget) {
        recovery?.let { if (it.second != renderedEpoch || it.first != state.confirmationTarget) recovery = null }
    }
    LaunchedEffect(state.selected, state.selectionConfirmed) {
        if (state.selectionConfirmed && state.selected != null) choosing = false
    }
    LaunchedEffect(state.unlockRequired, state.erasedWorkAcknowledgementRequired) {
        if (state.unlockRequired || state.erasedWorkAcknowledgementRequired) choosing = false
    }
    BackHandler {
        when {
            showChecklists && resumed -> closeChecklists()
            showTrips && resumed -> if (tripsMatch && tripState.openTripId != null) tripViewModel.openTrip(null, renderedEpoch, tripState.generation)
                else closeTrips()
            showFuelings && resumed -> if (fuelingsMatch && fuelingState.openFuelingId != null) fuelingViewModel.openFueling(null, renderedEpoch, fuelingState.generation)
                else closeFuelings()
            showExpenses && resumed -> if (expensesMatch && expenseState.openExpenseId != null) expenseViewModel.openExpense(null, renderedEpoch, expenseState.generation)
                else closeExpenses()
            showAssets && resumed -> if (assetsMatches && assetsState.detail != null) assetsViewModel.backToList(renderedEpoch, assetsState.generation)
                else closeAssets()
            showSync && resumed && openedFor?.partition == renderedSelected?.partition &&
                openedFor?.membershipId == renderedSelected?.membershipId -> syncBack()
            choosing -> choosing = false
            state.selected != null -> if (withdrawForGesture()) choosing = true
            else -> personal()
        }
    }
    AnpScaffold(
        modifier = Modifier.fillMaxSize().testTag("enterprise-context-screen"),
        topBar = {
            Column {
                AnpTopAppBar(title = { Text(stringResource(R.string.enterprise_context_home)) })
                state.selected?.let { selected ->
                    Text(
                        selected.organizationName,
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)
                            .testTag("enterprise-context-header-name").semantics { heading() },
                    )
                }
            }
        },
    ) { padding ->
        if (showChecklists && resumed && renderedSelected != null && !choosing &&
            checklistsFor?.partition == renderedSelected.partition && checklistsFor?.membershipId == renderedSelected.membershipId) ChecklistHomePanel(
            assets = if (assetsMatches) assetsState else assetsState.copy(snapshot = null, detailAssetId = null,
                error = assetsState.error ?: EnterpriseAccessError.STALE),
            state = renderedChecklist,
            connected = connected,
            actions = checklistActions(renderedSelected),
            onOpenRun = { asset, moment -> checklistViewModel.open(renderedSelected, asset, moment, renderedEpoch) },
            onBack = { closeChecklists() },
            modifier = Modifier.fillMaxSize().padding(padding),
        ) else if (showExpenses && resumed && renderedSelected != null && !choosing &&
            expensesFor?.partition == renderedSelected.partition && expensesFor?.membershipId == renderedSelected.membershipId) ExpensePanel(
            // A source loss clears the owner; keep only its safe message and withhold every expense fact.
            state = if (expensesMatch) expenseState else expenseState.copy(expenses = emptyList(), openExpenseId = null,
                error = expenseState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            assets = if (assetsMatches) assetsState else assetsState.copy(snapshot = null, detailAssetId = null),
            onBack = { closeExpenses() },
            onRetry = { expenseViewModel.open(renderedSelected, renderedEpoch) },
            onOpen = { id -> if (expensesMatch) expenseViewModel.openExpense(id, renderedEpoch, expenseState.generation) },
            onPick = { if (currentGesture()) assetsViewModel.open(renderedSelected, renderedEpoch) },
            onChoose = { id, warning -> if (assetsMatches) assetsViewModel.choose(id, warning, renderedEpoch, assetsState.generation) else null },
            onSave = { choice, form, draft -> if (expensesMatch) expenseViewModel.record(choice, form, draft, renderedEpoch, expenseState.generation) },
            onSend = { if (expensesMatch) expenseViewModel.sendNow(renderedEpoch, expenseState.generation) },
            modifier = Modifier.fillMaxSize().padding(padding),
            capture = if (receiptViewModel.enabled) ReceiptCaptureUi(receiptState, receiptPickers, receiptViewModel::reset) else null,
            onAttach = { expense, draft -> if (expensesMatch) expenseViewModel.attach(expense, draft, renderedEpoch, expenseState.generation) },
        ) else if (showFuelings && resumed && renderedSelected != null && !choosing &&
            fuelingsFor?.partition == renderedSelected.partition && fuelingsFor?.membershipId == renderedSelected.membershipId) FuelingPanel(
            // A source loss clears the owner; keep only its safe message and withhold every fueling fact.
            state = if (fuelingsMatch) fuelingState else fuelingState.copy(fuelings = emptyList(), openFuelingId = null,
                error = fuelingState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            assets = if (assetsMatches) assetsState else assetsState.copy(snapshot = null, detailAssetId = null),
            onBack = { closeFuelings() },
            onRetry = { fuelingViewModel.open(renderedSelected, renderedEpoch) },
            onOpen = { id -> if (fuelingsMatch) fuelingViewModel.openFueling(id, renderedEpoch, fuelingState.generation) },
            onPick = { if (currentGesture()) assetsViewModel.open(renderedSelected, renderedEpoch) },
            onChoose = { id, warning -> if (assetsMatches) assetsViewModel.choose(id, warning, renderedEpoch, assetsState.generation) else null },
            onSave = { choice, form, draft -> if (fuelingsMatch) fuelingViewModel.record(choice, form, draft, renderedEpoch, fuelingState.generation) },
            warnings = { form -> fuelingViewModel.warnings(form) },
            onSend = { if (fuelingsMatch) fuelingViewModel.sendNow(renderedEpoch, fuelingState.generation) },
            modifier = Modifier.fillMaxSize().padding(padding),
            capture = if (receiptViewModel.enabled) ReceiptCaptureUi(receiptState, receiptPickers, receiptViewModel::reset) else null,
            onAttach = { fueling, draft -> if (fuelingsMatch) fuelingViewModel.attach(fueling, draft, renderedEpoch, fuelingState.generation) },
        ) else if (showTrips && resumed && renderedSelected != null && !choosing &&
            tripsFor?.partition == renderedSelected.partition && tripsFor?.membershipId == renderedSelected.membershipId) TripPanel(
            // A source loss clears the owner; keep only its safe message and withhold every trip fact.
            state = if (tripsMatch) tripState else tripState.copy(trips = emptyList(), openTripId = null, missions = null,
                error = tripState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            assets = if (assetsMatches) assetsState else assetsState.copy(snapshot = null, detailAssetId = null),
            onBack = { closeTrips() },
            onRetry = { tripViewModel.open(renderedSelected, renderedEpoch) },
            onOpenTrip = { id -> if (tripsMatch) tripViewModel.openTrip(id, renderedEpoch, tripState.generation) },
            onPick = { if (currentGesture()) assetsViewModel.open(renderedSelected, renderedEpoch) },
            onChoose = { id, warning -> if (assetsMatches) assetsViewModel.choose(id, warning, renderedEpoch, assetsState.generation) else null },
            // EMP-TRIP-019: the start names the chosen vehicle's latest valid, non-blocking departure run shown on the form.
            onStart = { choice, label, departure ->
                if (tripsMatch) tripViewModel.start(choice, label, departure, renderedEpoch, tripState.generation,
                    if (BuildConfig.ENTERPRISE_CHECKLIST_UI_ENABLED) departureRun(renderedChecklist, choice.assetId) else null)
            },
            onLog = { trip, type, text -> if (tripsMatch) tripViewModel.log(trip, type, text, renderedEpoch, tripState.generation) },
            onFinish = { trip, final -> if (tripsMatch) tripViewModel.finish(trip, final, renderedEpoch, tripState.generation) },
            finishWarnings = { trip, final -> if (tripsMatch) tripViewModel.finishWarnings(trip, final) else emptyList() },
            onReady = { trip -> if (tripsMatch) tripViewModel.markReady(trip, renderedEpoch, tripState.generation) },
            onSend = { if (tripsMatch) tripViewModel.sendNow(renderedEpoch, tripState.generation) },
            capture = if (receiptViewModel.enabled) ReceiptCaptureUi(receiptState, receiptPickers, receiptViewModel::reset) else null,
            onAttach = { owner, target, draft -> if (tripsMatch) tripViewModel.attach(owner, target, draft, renderedEpoch, tripState.generation) },
            modifier = Modifier.fillMaxSize().padding(padding),
            checklist = if (BuildConfig.ENTERPRISE_CHECKLIST_UI_ENABLED) renderedChecklist else null,
            onDepartureCheck = { asset -> checklistViewModel.open(renderedSelected, asset, ChecklistContext.DEPARTURE, renderedEpoch) },
            departureContent = if (!BuildConfig.ENTERPRISE_CHECKLIST_UI_ENABLED) null else { _, label, onDone ->
                ChecklistRunPanel(renderedChecklist, connected, label, checklistActions(renderedSelected), onBack = onDone,
                    modifier = Modifier.fillMaxSize().padding(padding))
            },
            tracking = if (!BuildConfig.ENTERPRISE_TRACKING_UI_ENABLED) null else com.anpfuel.app.ui.enterprise.trip.TripTrackingActions(
                onStart = { trip, device -> if (tripsMatch) tripViewModel.startTracking(trip, device, renderedEpoch, tripState.generation) },
                onStop = { trip -> if (tripsMatch) tripViewModel.stopTracking(trip, renderedEpoch, tripState.generation) },
                onRefresh = { if (tripsMatch) tripViewModel.refreshTrackingPolicy(renderedEpoch, tripState.generation) },
            ),
            // EMP-TRIP-021: the missions inbox, answers, planned starts and the trip lifecycle, with the same exact-partition guards.
            missionsEnabled = BuildConfig.ENTERPRISE_MISSION_UI_ENABLED,
            onRefreshMissions = { if (tripsMatch) tripViewModel.refreshMissions(renderedEpoch, tripState.generation) },
            onRespond = { mission, response, reason -> if (tripsMatch) tripViewModel.respond(mission, response, reason, renderedEpoch, tripState.generation) },
            onStartMission = { mission, choice, label, departure ->
                if (tripsMatch) tripViewModel.startMission(mission, choice, label, departure, renderedEpoch, tripState.generation,
                    if (BuildConfig.ENTERPRISE_CHECKLIST_UI_ENABLED) departureRun(renderedChecklist, choice.assetId) else null)
            },
            onLifecycle = { trip, command, reason -> if (tripsMatch) tripViewModel.lifecycle(trip, command, reason, renderedEpoch, tripState.generation) },
            // EMP-ROUTE-004 (B-BR-EMP-532..535): the visible partition's own approved destination only; nothing is recorded or sent.
            onNavigate = if (BuildConfig.ENTERPRISE_MISSION_UI_ENABLED) { query -> if (tripsMatch) onNavigate(query) } else null,
        ) else if (showAssets && resumed && renderedSelected != null && !choosing &&
            assetsFor?.partition == renderedSelected.partition && assetsFor?.membershipId == renderedSelected.membershipId) MyAssetsPanel(
            // A source loss clears the owner; keep only its safe message and withhold every cached fact.
            state = if (assetsMatches) assetsState else assetsState.copy(snapshot = null, detailAssetId = null,
                error = assetsState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            onBack = {
                if (assetsMatches && assetsState.detail != null) assetsViewModel.backToList(renderedEpoch, assetsState.generation)
                else closeAssets()
            },
            onRefresh = { if (assetsMatches) assetsViewModel.refresh(renderedEpoch, assetsState.generation) },
            onRetry = { assetsViewModel.open(renderedSelected, renderedEpoch) },
            onOpen = { id -> if (assetsMatches) assetsViewModel.detail(id, renderedEpoch, assetsState.generation) },
            modifier = Modifier.fillMaxSize().padding(padding),
        ) else if (showSync && resumed && renderedSelected != null && !choosing &&
            openedFor?.partition == renderedSelected.partition && openedFor?.membershipId == renderedSelected.membershipId) EnterpriseSyncPanel(
            // A source loss can intentionally clear the flow owner. Keep its safe
            // recovery message visible while withholding every previous private fact.
            state = if (syncMatches) syncState else syncState.copy(items = emptyList(), totals = null,
                detail = null, active = emptySet(), hasMore = false, manualAttention = 0, manualWaitingMillis = null,
                error = syncState.error ?: EnterpriseAccessError.STALE),
            connected = connected,
            onBack = ::syncBack,
            onRefresh = { if (currentGesture()) {
                if (syncMatches) syncViewModel.refresh(renderedEpoch, renderedGeneration)
                else syncViewModel.open(renderedSelected, renderedEpoch, renderedGeneration)
            } },
            onMore = { if (currentGesture()) syncViewModel.loadMore(renderedEpoch, renderedGeneration) },
            onMode = { if (currentGesture()) syncViewModel.changeMode(it, renderedEpoch, renderedGeneration) },
            onDetail = { if (currentGesture()) syncViewModel.detail(it, renderedEpoch, renderedGeneration) },
            onManualCheck = { if (currentGesture()) syncViewModel.manualCheck(renderedEpoch, renderedGeneration) },
            onDismiss = { identity, outcome -> if (currentGesture()) syncViewModel.dismiss(identity, outcome, renderedEpoch, renderedGeneration) },
            modifier = Modifier.fillMaxSize().padding(padding),
        ) else Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.enterprise_test_environment), style = MaterialTheme.typography.labelLarge)
            protectionNotice?.let { Text(stringResource(it), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) }
            val selected = state.selected
            if (selected == null) {
                ContextChooser(state, connected, renderedEpoch, viewModel, personal, signIn, invitation, ::withdrawForGesture)
            } else {
                Column(Modifier.testTag("enterprise-home"), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    Text(
                        stringResource(R.string.enterprise_context_current, selected.organizationName),
                        style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.testTag("enterprise-context-name").semantics { heading(); liveRegion = LiveRegionMode.Polite },
                    )
                    ContextStatus(state, connected)
                    ContextCard {
                        Text(stringResource(R.string.enterprise_context_empty_home))
                        Text(stringResource(R.string.enterprise_context_ownership), style = MaterialTheme.typography.bodyMedium)
                    }
                    if (BuildConfig.ENTERPRISE_TRIP_UI_ENABLED) TripsHomeCard(onOpen = {
                        if (currentGesture() && !viewModel.state.value.busy) {
                            tripViewModel.open(selected, renderedEpoch)
                            // The picker and the finish meters read the protected reference cache, offline first.
                            if (BuildConfig.ENTERPRISE_FLEET_UI_ENABLED) assetsViewModel.open(selected, renderedEpoch)
                            tripsFor = selected
                            showTrips = true
                        }
                    })
                    if (BuildConfig.ENTERPRISE_CHECKLIST_UI_ENABLED) ChecklistsHomeCard(onOpen = {
                        if (currentGesture() && !viewModel.state.value.busy) {
                            // The asset is chosen with the FLEET-013 picker from the protected reference cache, offline first.
                            assetsViewModel.open(selected, renderedEpoch)
                            checklistsFor = selected
                            showChecklists = true
                        }
                    })
                    if (BuildConfig.ENTERPRISE_FUELING_UI_ENABLED) FuelingsHomeCard(onOpen = {
                        if (currentGesture() && !viewModel.state.value.busy) {
                            fuelingViewModel.open(selected, renderedEpoch)
                            // The picker reads the protected reference cache, offline first.
                            if (BuildConfig.ENTERPRISE_FLEET_UI_ENABLED) assetsViewModel.open(selected, renderedEpoch)
                            fuelingsFor = selected
                            showFuelings = true
                        }
                    })
                    if (BuildConfig.ENTERPRISE_EXPENSE_UI_ENABLED) ExpensesHomeCard(onOpen = {
                        if (currentGesture() && !viewModel.state.value.busy) {
                            expenseViewModel.open(selected, renderedEpoch)
                            // The picker reads the protected reference cache, offline first.
                            if (BuildConfig.ENTERPRISE_FLEET_UI_ENABLED) assetsViewModel.open(selected, renderedEpoch)
                            expensesFor = selected
                            showExpenses = true
                        }
                    })
                    if (BuildConfig.ENTERPRISE_FLEET_UI_ENABLED) MyAssetsHomeCard(onOpen = {
                        if (currentGesture() && !viewModel.state.value.busy) {
                            assetsViewModel.open(selected, renderedEpoch)
                            assetsFor = selected
                            showAssets = true
                        }
                    })
                    EnterpriseSyncHomeStatus(if (syncMatches && syncState.error == null && !syncState.unlockRequired && !syncState.unrecoverable) syncState.totals else null,
                        onOpen = {
                            if (currentGesture() && !viewModel.state.value.busy && syncViewModel.open(selected, renderedEpoch, renderedGeneration)) {
                                openedFor = selected
                                showSync = true
                            }
                        })
                    if (!state.selectionConfirmed) Button(
                        onClick = { if (withdrawForGesture()) viewModel.select(selected, renderedEpoch) },
                        enabled = connected && !state.busy,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-reconfirm"),
                    ) { Text(stringResource(R.string.enterprise_context_reconfirm)) }
                    TextButton(
                        onClick = { if (withdrawForGesture()) choosing = true },
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-switch"),
                    ) { Text(stringResource(R.string.enterprise_context_switch)) }
                    FreshContextSignIn(state, signIn)
                }
            }
            if (state.unlockRequired) {
                Text(stringResource(R.string.enterprise_protection_unlock_required))
                Button(onClick = ::requestUnlock, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-protection-unlock")) {
                    Text(stringResource(R.string.enterprise_protection_unlock_action))
                }
            }
            state.confirmationTarget?.let { target -> if (state.erasedWorkAcknowledgementRequired) {
                Text(stringResource(R.string.enterprise_protection_recovery_required), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
                Button(onClick = { if (withdrawForGesture()) recovery = target to renderedEpoch }, enabled = connected && !state.busy,
                    modifier = Modifier.fillMaxWidth().testTag("enterprise-protection-recover")) {
                    Text(stringResource(R.string.enterprise_protection_recovery_action, target.organizationName))
                }
            } }
        }
    }
    recovery?.let { (target, epoch) -> AlertDialog(
        onDismissRequest = { recovery = null },
        title = { Text(stringResource(R.string.enterprise_protection_recovery_title)) },
        text = { Text(stringResource(R.string.enterprise_protection_recovery_copy)) },
        confirmButton = { TextButton(onClick = {
            recovery = null
            if (viewModel.state.value.confirmationTarget == target && epoch == renderedEpoch && withdrawForGesture())
                viewModel.select(target, epoch, acknowledgeErasedWork = true)
        }, enabled = connected && !state.busy) { Text(stringResource(R.string.enterprise_protection_recovery_confirm)) } },
        dismissButton = { TextButton(onClick = { recovery = null }) { Text(stringResource(R.string.action_cancel)) } },
    ) }
    if (choosing && state.selected != null) ModalBottomSheet(
        onDismissRequest = { choosing = false },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    ) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            ContextChooser(state, connected, renderedEpoch, viewModel, personal, signIn, invitation, ::withdrawForGesture)
            TextButton(onClick = { choosing = false }, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.enterprise_context_close))
            }
        }
    }
}

@Composable
private fun ContextChooser(
    state: EnterpriseContextState,
    connected: Boolean,
    renderedEpoch: Long,
    viewModel: EnterpriseContextViewModel,
    personal: () -> Unit,
    onSignIn: () -> Unit,
    onInvitation: () -> Unit,
    beforeContextChange: () -> Boolean,
) {
    Column(Modifier.testTag("enterprise-context-chooser"), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Text(stringResource(R.string.enterprise_context_title), style = MaterialTheme.typography.titleLarge, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_context_intro))
        Button(onClick = personal, modifier = Modifier.fillMaxWidth().testTag("enterprise-context-personal")) {
            Text(stringResource(R.string.enterprise_context_personal))
        }
        ContextStatus(state, connected, catalog = true)
        if (!state.signedIn) Text(stringResource(R.string.enterprise_context_sign_in))
        else if (state.contexts.isEmpty() && state.catalogConfirmed && !state.busy) Text(stringResource(R.string.enterprise_context_empty))
        for (context in state.contexts) ContextOption(
            context,
            connected && state.signedIn && !state.busy,
            onSelect = { if (beforeContextChange()) viewModel.select(context, renderedEpoch) },
        )
        Button(
            onClick = { if (beforeContextChange()) viewModel.refresh(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-refresh"),
        ) { Text(stringResource(R.string.enterprise_context_refresh)) }
        if (state.nextAfterOrganizationId != null) TextButton(
            onClick = { if (beforeContextChange()) viewModel.loadMore(renderedEpoch) },
            enabled = connected && state.signedIn && !state.busy,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-next"),
        ) { Text(stringResource(R.string.enterprise_context_next)) }
        FreshContextSignIn(state, onSignIn)
        TextButton(onClick = onInvitation, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.enterprise_context_invitation))
        }
    }
}

@Composable
private fun ContextOption(context: EnterpriseContextSummary, enabled: Boolean, onSelect: () -> Unit) {
    ContextCard {
        Text(context.organizationName, style = MaterialTheme.typography.titleMedium)
        Button(
            onClick = onSelect,
            enabled = enabled,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-context-option-${context.partition.organizationId}"),
        ) { Text(stringResource(R.string.enterprise_context_select, context.organizationName)) }
    }
}

@Composable
private fun ContextStatus(state: EnterpriseContextState, connected: Boolean, catalog: Boolean = false) {
    if (!connected) Text(stringResource(R.string.enterprise_context_offline))
    val unconfirmed = if (catalog) state.contexts.isNotEmpty() && !state.catalogConfirmed
        else state.selected != null && !state.selectionConfirmed
    if (unconfirmed) Text(
        stringResource(R.string.enterprise_context_unconfirmed),
        modifier = Modifier.testTag("enterprise-context-unconfirmed").semantics { liveRegion = LiveRegionMode.Polite },
    )
    else if (!catalog && state.selected != null && state.selectionConfirmed) Text(stringResource(R.string.enterprise_context_confirmed))
    if (state.busy) {
        CircularProgressIndicator()
        Text(stringResource(R.string.enterprise_context_busy), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
    }
    state.error?.let { error -> Text(
        stringResource(when (error) {
            EnterpriseAccessError.ACCESS_DENIED -> R.string.enterprise_context_denied
            EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED -> R.string.enterprise_invitation_fresh_required
            EnterpriseAccessError.STALE -> R.string.enterprise_context_unconfirmed
            EnterpriseAccessError.RATE_LIMITED -> R.string.enterprise_invitation_limited
            else -> R.string.enterprise_context_unavailable
        }),
        color = MaterialTheme.colorScheme.error,
        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
    ) }
}

@Composable
private fun FreshContextSignIn(state: EnterpriseContextState, onSignIn: () -> Unit) {
    if (!state.signedIn || state.error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED)) Button(
        onClick = onSignIn,
        enabled = !state.busy,
        modifier = Modifier.fillMaxWidth().testTag("enterprise-context-sign-in"),
    ) { Text(stringResource(R.string.enterprise_invitation_sign_in)) }
}

@Composable
private fun ContextCard(content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill),
        border = AnpContainerDefaults.borderStroke(fill),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
    }
}
