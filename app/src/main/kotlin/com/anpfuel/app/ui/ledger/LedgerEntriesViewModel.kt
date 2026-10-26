package com.anpfuel.app.ui.ledger

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.ledger.DeclareLedgerGapUseCase
import com.anpfuel.application.usecase.ledger.DeleteLedgerEntryUseCase
import com.anpfuel.application.usecase.ledger.EnqueueLedgerCommandUseCase
import com.anpfuel.application.usecase.ledger.EstimatedEfficiencyUseCase
import com.anpfuel.application.usecase.ledger.GetLedgerEntryDraftUseCase
import com.anpfuel.application.usecase.ledger.LedgerEntryRow
import com.anpfuel.application.usecase.ledger.ListLedgerEntriesUseCase
import com.anpfuel.application.usecase.ledger.ResetLedgerSegmentUseCase
import com.anpfuel.application.usecase.ledger.SaveLedgerDraftUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.fromVehicleProduct
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.LedgerGapIntent
import com.anpfuel.domain.premium.LedgerSegmentReset
import com.anpfuel.domain.premium.LedgerSessionContext
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.premium.RefuelDraft
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.Propulsion
import dagger.hilt.android.lifecycle.HiltViewModel
import java.math.BigDecimal
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import java.text.DecimalFormatSymbols
import com.anpfuel.domain.premium.LedgerDistance
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class LedgerEntryFormState(
    val entryId: String? = null,
    val dataset: LedgerDataset = LedgerDataset.REFUEL,
    val dateInput: String = "",
    val timeInput: String = "",
    val quantityInput: String = "",
    val totalInput: String = "",
    val unitPriceInput: String = "",
    val rateInput: String = "",
    val socStartInput: String = "",
    val socEndInput: String = "",
    val tariffIdInput: String = "",
    val tariffRevisionInput: String = "",
    val context: LedgerSessionContext? = null,
    val fill: LedgerFill = LedgerFill.FULL,
    val category: ExpenseCategory = ExpenseCategory.OTHER,
    val odometerInput: String = "",
    val stationInput: String = "",
    val noteInput: String = "",
    val validationErrorKey: LedgerEntryValidationError? = null,
)

enum class LedgerEntryValidationError {
    DATE_INVALID,
    QUANTITY_INVALID,
    TOTAL_INVALID,
    UNIT_PRICE_INVALID,
    RATE_INVALID,
    SOC_INVALID,
    ODOMETER_INVALID,
    STATION_INVALID,
    FUEL_UNAVAILABLE,
    CHARGE_UNAVAILABLE,
    TARIFF_INVALID,
}

data class LedgerEntriesUiState(
    val isLoading: Boolean = true,
    val vehicleId: DomainId? = null,
    val vehicleName: String = "",
    val vehicleHasFuel: Boolean = true,
    val vehicleIsElectric: Boolean = false,
    val entries: List<LedgerEntryRow> = emptyList(),
    val estimateInput: String = "",
    val estimateSaved: String? = null,
    val estimateError: Boolean = false,
    val showForm: Boolean = false,
    val form: LedgerEntryFormState = LedgerEntryFormState(),
    val showDeleteConfirm: Boolean = false,
    val pendingDelete: Pair<LedgerDataset, String>? = null,
    val showGapDialog: Boolean = false,
    val gapTarget: LedgerEntryRow? = null,
    val gapBeforeInput: String = "",
    val gapNoteInput: String = "",
    val showResetConfirm: Boolean = false,
    val isSaving: Boolean = false,
    val errorMessage: String? = null,
)

@HiltViewModel
class LedgerEntriesViewModel @Inject constructor(
    private val listEntriesUseCase: ListLedgerEntriesUseCase,
    private val getDraftUseCase: GetLedgerEntryDraftUseCase,
    private val saveDraftUseCase: SaveLedgerDraftUseCase,
    private val enqueueCommandUseCase: EnqueueLedgerCommandUseCase,
    private val deleteEntryUseCase: DeleteLedgerEntryUseCase,
    private val declareGapUseCase: DeclareLedgerGapUseCase,
    private val resetSegmentUseCase: ResetLedgerSegmentUseCase,
    private val efficiencyUseCase: EstimatedEfficiencyUseCase,
    private val getVehicleUseCase: GetVehicleUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(LedgerEntriesUiState())
    val uiState: StateFlow<LedgerEntriesUiState> = _uiState.asStateFlow()

    private var vehicle: Vehicle? = null
    private var inputLocale: Locale = Locale.getDefault()

    fun load(vehicleId: DomainId, locale: Locale) {
        inputLocale = locale
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null, vehicleId = vehicleId) }
            runCatching {
                vehicle = getVehicleUseCase(vehicleId)
                val entries = listEntriesUseCase(vehicleId)
                val estimate = vehicle?.let { efficiencyUseCase.current(it) }
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        vehicleName = vehicle?.displayName.orEmpty(),
                        vehicleHasFuel = vehicle?.fuelProduct != null,
                        vehicleIsElectric = vehicle?.propulsion == Propulsion.BEV ||
                            vehicle?.propulsion == Propulsion.PHEV,
                        entries = entries,
                        estimateSaved = estimate,
                        estimateInput = estimate.orEmpty(),
                    )
                }
            }.onFailure { error ->
                _uiState.update { it.copy(isLoading = false, errorMessage = error.message) }
            }
        }
    }

    fun startAdd(dataset: LedgerDataset) {
        // BEV carries no fuel: refuel entries are refused up front with
        // an explicit reason instead of a silently broken form. Charge
        // entries need an electric vehicle for the same reason.
        if (dataset == LedgerDataset.REFUEL && vehicle?.fuelProduct == null) {
            _uiState.update {
                it.copy(
                    showForm = true,
                    form = LedgerEntryFormState(
                        dataset = dataset,
                        validationErrorKey = LedgerEntryValidationError.FUEL_UNAVAILABLE,
                    ),
                )
            }
            return
        }
        if (dataset == LedgerDataset.CHARGE && _uiState.value.vehicleIsElectric == false) {
            _uiState.update {
                it.copy(
                    showForm = true,
                    form = LedgerEntryFormState(
                        dataset = dataset,
                        validationErrorKey = LedgerEntryValidationError.CHARGE_UNAVAILABLE,
                    ),
                )
            }
            return
        }
        val now = java.time.ZonedDateTime.now()
        _uiState.update {
            it.copy(
                showForm = true,
                form = LedgerEntryFormState(
                    dataset = dataset,
                    dateInput = now.toLocalDate().toString(),
                    timeInput = now.toLocalTime().format(DateTimeFormatter.ofPattern("HH:mm")),
                ),
            )
        }
    }

    fun startEdit(row: LedgerEntryRow) {
        viewModelScope.launch {
            val stored = getDraftUseCase(row.dataset, row.recordId) ?: return@launch
            val draft = stored.draft
            _uiState.update {
                it.copy(
                    showForm = true,
                    form = when (draft) {
                        is RefuelDraft -> LedgerEntryFormState(
                            entryId = draft.id, dataset = LedgerDataset.REFUEL,
                            dateInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalDate().toString(),
                            timeInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalTime()
                                .format(DateTimeFormatter.ofPattern("HH:mm")),
                            quantityInput = milliToInput(draft.quantityMilli),
                            totalInput = centavosToInput(draft.totalCentavos),
                            unitPriceInput = draft.unitPriceMilli?.let { milliToInput(it) }.orEmpty(),
                            fill = draft.fill,
                            odometerInput = draft.odometerMetres?.let { LedgerDistance.kilometresInput(it, DecimalFormatSymbols.getInstance(inputLocale).decimalSeparator) }.orEmpty(),
                            stationInput = draft.stationId.orEmpty(),
                            noteInput = draft.note.orEmpty(),
                        )
                        is ExpenseDraft -> LedgerEntryFormState(
                            entryId = draft.id, dataset = LedgerDataset.EXPENSE,
                            dateInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalDate().toString(),
                            timeInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalTime()
                                .format(DateTimeFormatter.ofPattern("HH:mm")),
                            totalInput = centavosToInput(draft.amountCentavos),
                            category = draft.category,
                            odometerInput = draft.odometerMetres?.let { LedgerDistance.kilometresInput(it, DecimalFormatSymbols.getInstance(inputLocale).decimalSeparator) }.orEmpty(),
                            noteInput = draft.note.orEmpty(),
                        )
                    is ChargeDraft -> LedgerEntryFormState(
                        entryId = draft.id, dataset = LedgerDataset.CHARGE,
                        dateInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalDate().toString(),
                        timeInput = draft.occurredAt.atZone(ZoneId.systemDefault()).toLocalTime()
                            .format(DateTimeFormatter.ofPattern("HH:mm")),
                        quantityInput = draft.energyWh?.let { whToKwhInput(it) }.orEmpty(),
                        totalInput = draft.totalCentavos?.let { centavosToInput(it) }.orEmpty(),
                        socStartInput = draft.socStart?.toString().orEmpty(),
                        socEndInput = draft.socEnd?.toString().orEmpty(),
                        tariffIdInput = draft.tariffId.orEmpty(),
                        tariffRevisionInput = draft.tariffRevision.orEmpty(),
                        context = stored.context,
                        odometerInput = draft.odometerMetres?.let { LedgerDistance.kilometresInput(it, DecimalFormatSymbols.getInstance(inputLocale).decimalSeparator) }.orEmpty(),
                        noteInput = draft.note.orEmpty(),
                    )
                    },
                )
            }
        }
    }

    fun dismissForm() {
        _uiState.update { it.copy(showForm = false, form = LedgerEntryFormState()) }
    }

    fun onDatasetChanged(dataset: LedgerDataset) {
        _uiState.update { it.copy(form = it.form.copy(dataset = dataset, validationErrorKey = null)) }
    }

    fun onDateChanged(value: String) = updateForm { copy(dateInput = value) }
    fun onTimeChanged(value: String) = updateForm { copy(timeInput = value) }
    fun onQuantityChanged(value: String) = updateForm { copy(quantityInput = value) }
    fun onTotalChanged(value: String) = updateForm { copy(totalInput = value) }
    fun onUnitPriceChanged(value: String) = updateForm { copy(unitPriceInput = value) }
    fun onRateChanged(value: String) = updateForm { copy(rateInput = value) }
    fun onSocStartChanged(value: String) = updateForm { copy(socStartInput = value) }
    fun onSocEndChanged(value: String) = updateForm { copy(socEndInput = value) }
    fun onTariffIdChanged(value: String) = updateForm { copy(tariffIdInput = value) }
    fun onTariffRevisionChanged(value: String) = updateForm { copy(tariffRevisionInput = value) }
    fun onContextChanged(context: LedgerSessionContext?) {
        _uiState.update { it.copy(form = it.form.copy(context = context, validationErrorKey = null)) }
    }
    fun onFillChanged(fill: LedgerFill) {
        _uiState.update { it.copy(form = it.form.copy(fill = fill, validationErrorKey = null)) }
    }
    fun onCategoryChanged(category: ExpenseCategory) {
        _uiState.update { it.copy(form = it.form.copy(category = category, validationErrorKey = null)) }
    }
    fun onOdometerChanged(value: String) = updateForm { copy(odometerInput = value) }
    fun onStationChanged(value: String) = updateForm { copy(stationInput = value) }
    fun onNoteChanged(value: String) = updateForm { copy(noteInput = value) }

    private inline fun updateForm(change: LedgerEntryFormState.() -> LedgerEntryFormState) {
        _uiState.update { it.copy(form = it.form.change().copy(validationErrorKey = null)) }
    }

    /** Tariff simulation for the charge form: kWh × rate, display only. */
    fun simulatedChargeTotal(): String? {
        val form = _uiState.value.form
        if (form.dataset != LedgerDataset.CHARGE) return null
        val wh = parseKwhToWh(form.quantityInput) ?: return null
        val rateMilli = form.rateInput.ifBlank { return null }.let { parseRateMilli(it) } ?: return null
        val total = BigDecimal.valueOf(wh).multiply(BigDecimal.valueOf(rateMilli))
            .divide(BigDecimal(1_000_000), 2, java.math.RoundingMode.HALF_UP)
        return total.stripTrailingZeros().toPlainString()
    }

    fun saveDraft() {
        if (_uiState.value.isSaving) {
            // Double-tap guard: the in-flight save owns the entry id;
            // a second tap is ignored instead of forking a duplicate.
            return
        }
        val form = _uiState.value.form
        val vehicleId = _uiState.value.vehicleId ?: return
        val currentVehicle = vehicle ?: return
        val occurred = parseOccurred(form) ?: run {
            failForm(LedgerEntryValidationError.DATE_INVALID)
            return
        }
        _uiState.update { it.copy(isSaving = true) }
        viewModelScope.launch {
            try {
                runCatching {
                    val id = form.entryId ?: UUID.randomUUID().toString()
                    // Edits keep the stored revision so corrections chain
                    // instead of forking; new entries start at zero.
                    val revision = form.entryId?.let {
                        getDraftUseCase(form.dataset, it)?.expectedRevision ?: 0L
                    } ?: 0L
                    val draft = buildDraft(id, currentVehicle, occurred, form) ?: return@runCatching
                    // Session context rides the draft only; command payloads
                    // never carry it (backend parity, no auto-post surface).
                    val draftContext = if (form.dataset == LedgerDataset.CHARGE) form.context else null
                    saveDraftUseCase(draft, revision, draftContext)
                    _uiState.update {
                        it.copy(
                            showForm = false,
                            form = LedgerEntryFormState(),
                            entries = listEntriesUseCase(vehicleId),
                        )
                    }
                }.onFailure { error ->
                    if (error is PremiumValidationException) {
                        failForm(LedgerEntryValidationError.TOTAL_INVALID)
                    } else {
                        _uiState.update { it.copy(errorMessage = error.message) }
                    }
                }
            } finally {
                _uiState.update { it.copy(isSaving = false) }
            }
        }
    }

    private fun buildDraft(
        id: String,
        vehicle: Vehicle,
        occurred: Instant,
        form: LedgerEntryFormState,
    ): LedgerDraft? {
        val timezone = ZoneId.systemDefault().id
        return when (form.dataset) {
            LedgerDataset.REFUEL -> {
                val fuel = vehicle.fuelProduct?.let {
                    com.anpfuel.domain.premium.PremiumFuel.fromVehicleProduct(it)
                } ?: run {                    failForm(LedgerEntryValidationError.FUEL_UNAVAILABLE)
                    return null
                }
                val quantity = parseMilli(form.quantityInput) ?: run {
                    failForm(LedgerEntryValidationError.QUANTITY_INVALID)
                    return null
                }
                val total = parseCentavos(form.totalInput) ?: run {
                    failForm(LedgerEntryValidationError.TOTAL_INVALID)
                    return null
                }
                val unitPrice = form.unitPriceInput.ifBlank { null }?.let {
                    parseMilli(it) ?: run {
                        failForm(LedgerEntryValidationError.UNIT_PRICE_INVALID)
                        return null
                    }
                }
                val odo = if (form.odometerInput.isBlank()) {
                    null
                } else {
                    parseOdometer(form) ?: return null
                }
                val station = if (form.stationInput.isBlank()) {
                    null
                } else {
                    parseStation(form) ?: return null
                }
                RefuelDraft(
                    id = id, vehicleId = vehicle.id.value, occurredAt = occurred,
                    timezone = timezone, fuel = fuel,
                    quantityMilli = quantity, totalCentavos = total, unitPriceMilli = unitPrice,
                    odometerMetres = odo, fill = form.fill, stationId = station,
                    note = form.noteInput.ifBlank { null },
                )
            }
            LedgerDataset.EXPENSE -> {
                val total = parseCentavos(form.totalInput) ?: run {
                    failForm(LedgerEntryValidationError.TOTAL_INVALID)
                    return null
                }
                val odo = if (form.odometerInput.isBlank()) {
                    null
                } else {
                    parseOdometer(form) ?: return null
                }
                ExpenseDraft(
                    id = id, vehicleId = vehicle.id.value, occurredAt = occurred,
                    timezone = timezone, amountCentavos = total, category = form.category,
                    odometerMetres = odo, note = form.noteInput.ifBlank { null },
                )
            }
            LedgerDataset.CHARGE -> {
                if (vehicle.propulsion != Propulsion.BEV && vehicle.propulsion != Propulsion.PHEV) {
                    failForm(LedgerEntryValidationError.CHARGE_UNAVAILABLE)
                    return null
                }
                val energy = form.quantityInput.ifBlank { null }?.let {
                    parseKwhToWh(it) ?: run {
                        failForm(LedgerEntryValidationError.QUANTITY_INVALID)
                        return null
                    }
                }
                if (form.rateInput.isNotBlank() && parseRateMilli(form.rateInput) == null) {
                    failForm(LedgerEntryValidationError.RATE_INVALID)
                    return null
                }
                val socStart = parseSoc(form.socStartInput) ?: run {
                    if (form.socStartInput.isNotBlank()) {
                        failForm(LedgerEntryValidationError.SOC_INVALID)
                        return null
                    }
                    null
                }
                val socEnd = parseSoc(form.socEndInput) ?: run {
                    if (form.socEndInput.isNotBlank()) {
                        failForm(LedgerEntryValidationError.SOC_INVALID)
                        return null
                    }
                    null
                }
                val tariffId = form.tariffIdInput.ifBlank { null }
                val tariffRevision = form.tariffRevisionInput.ifBlank { null }
                if ((tariffId == null) != (tariffRevision == null)) {
                    failForm(LedgerEntryValidationError.TARIFF_INVALID)
                    return null
                }
                if (tariffId != null) {
                    try {
                        java.util.UUID.fromString(tariffId.trim())
                    } catch (_: Exception) {
                        failForm(LedgerEntryValidationError.TARIFF_INVALID)
                        return null
                    }
                    if (tariffRevision!!.isBlank() || tariffRevision.length > 128) {
                        failForm(LedgerEntryValidationError.TARIFF_INVALID)
                        return null
                    }
                }
                val total = form.totalInput.ifBlank { null }?.let {
                    parseCentavos(it) ?: run {
                        failForm(LedgerEntryValidationError.TOTAL_INVALID)
                        return null
                    }
                }
                val odo = if (form.odometerInput.isBlank()) {
                    null
                } else {
                    parseOdometer(form) ?: return null
                }
                ChargeDraft(
                    id = id, vehicleId = vehicle.id.value, occurredAt = occurred,
                    timezone = timezone, energyWh = energy, totalCentavos = total,
                    socStart = socStart, socEnd = socEnd, odometerMetres = odo,
                    feesCentavos = null, tariffId = tariffId?.trim(), tariffRevision = tariffRevision,
                    note = form.noteInput.ifBlank { null },
                )
            }
        }
    }

    private fun parseSoc(input: String): Int? {
        if (input.isBlank()) return null
        // Blank means absent; non-blank must be a 0..100 integer.
        return input.trim().toIntOrNull()?.takeIf { it in 0..100 }
    }

    private fun parseOdometer(form: LedgerEntryFormState): Long? {
        if (form.odometerInput.isBlank()) return null
        return LedgerDistance.parseKilometres(form.odometerInput, if (form.dataset == LedgerDataset.CHARGE) 1_000_000_000L else com.anpfuel.domain.premium.PremiumBounds.MAX_DISTANCE_METRES) ?: run {
            failForm(LedgerEntryValidationError.ODOMETER_INVALID)
            null
        }
    }

    private fun parseStation(form: LedgerEntryFormState): String? {
        if (form.stationInput.isBlank()) return null
        return try {
            java.util.UUID.fromString(form.stationInput.trim())
            form.stationInput.trim()
        } catch (_: Exception) {
            failForm(LedgerEntryValidationError.STATION_INVALID)
            null
        }
    }

    fun queueEntry(row: LedgerEntryRow) {
        viewModelScope.launch {
            runCatching {
                enqueueCommandUseCase(row.dataset, row.recordId)
                _uiState.value.vehicleId?.let { _uiState.update { s -> s.copy(entries = listEntriesUseCase(it)) } }
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun requestDelete(row: LedgerEntryRow) {
        _uiState.update { it.copy(showDeleteConfirm = true, pendingDelete = row.dataset to row.recordId) }
    }

    fun dismissDelete() {
        _uiState.update { it.copy(showDeleteConfirm = false, pendingDelete = null) }
    }

    fun confirmDelete() {
        val pending = _uiState.value.pendingDelete ?: return
        val vehicleId = _uiState.value.vehicleId ?: return
        viewModelScope.launch {
            runCatching {
                deleteEntryUseCase(pending.first, pending.second)
                _uiState.update {
                    it.copy(
                        showDeleteConfirm = false,
                        pendingDelete = null,
                        entries = listEntriesUseCase(vehicleId),
                    )
                }
            }.onFailure { error ->
                _uiState.update { it.copy(showDeleteConfirm = false, pendingDelete = null, errorMessage = error.message) }
            }
        }
    }

    fun openGapDialog(row: LedgerEntryRow) {
        _uiState.update { it.copy(showGapDialog = true, gapTarget = row, gapBeforeInput = "", gapNoteInput = "") }
    }

    fun dismissGapDialog() {
        _uiState.update { it.copy(showGapDialog = false, gapTarget = null) }
    }

    fun onGapBeforeChanged(value: String) {
        _uiState.update { it.copy(gapBeforeInput = value) }
    }

    fun onGapNoteChanged(value: String) {
        _uiState.update { it.copy(gapNoteInput = value) }
    }

    fun declareGap() {
        val target = _uiState.value.gapTarget ?: return
        val vehicleId = _uiState.value.vehicleId ?: return
        val before = _uiState.value.gapBeforeInput.ifBlank { null }
        if (before != null) {
            try {
                java.util.UUID.fromString(before.trim())
            } catch (_: Exception) {
                _uiState.update { it.copy(errorMessage = "invalid-gap-reference") }
                return
            }
        }
        val intent = try {
            LedgerGapIntent(
                afterRecordId = target.recordId,
                beforeRecordId = before?.trim(),
                note = _uiState.value.gapNoteInput.ifBlank { null },
            )
        } catch (e: PremiumValidationException) {
            _uiState.update { it.copy(errorMessage = e.code) }
            return
        }
        viewModelScope.launch {
            runCatching {
                declareGapUseCase(vehicleId, intent)
                _uiState.update { it.copy(showGapDialog = false, gapTarget = null, entries = listEntriesUseCase(vehicleId)) }
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun requestReset() {
        _uiState.update { it.copy(showResetConfirm = true) }
    }

    fun dismissReset() {
        _uiState.update { it.copy(showResetConfirm = false) }
    }

    fun confirmReset() {
        val vehicleId = _uiState.value.vehicleId ?: return
        viewModelScope.launch {
            runCatching {
                resetSegmentUseCase(vehicleId, LedgerSegmentReset("reset"))
                _uiState.update { it.copy(showResetConfirm = false, entries = listEntriesUseCase(vehicleId)) }
            }.onFailure { error ->
                _uiState.update { it.copy(showResetConfirm = false, errorMessage = error.message) }
            }
        }
    }

    fun onEstimateChanged(value: String) {
        _uiState.update { it.copy(estimateInput = value, estimateError = false) }
    }

    fun saveEstimate() {
        viewModelScope.launch {
            runCatching {
                val current = vehicle ?: return@runCatching
                val saved = efficiencyUseCase.set(current, _uiState.value.estimateInput)
                _uiState.update { it.copy(estimateSaved = saved, estimateInput = saved, estimateError = false) }
            }.onFailure { error ->
                if (error is PremiumValidationException) {
                    _uiState.update { it.copy(estimateError = true) }
                } else {
                    _uiState.update { it.copy(errorMessage = error.message) }
                }
            }
        }
    }

    fun clearEstimate() {
        viewModelScope.launch {
            runCatching {
                val current = vehicle ?: return@runCatching
                efficiencyUseCase.clear(current)
                _uiState.update { it.copy(estimateSaved = null, estimateInput = "", estimateError = false) }
            }.onFailure { error ->
                _uiState.update { it.copy(errorMessage = error.message) }
            }
        }
    }

    fun dismissError() {
        _uiState.update { it.copy(errorMessage = null) }
    }

    private fun failForm(key: LedgerEntryValidationError) {
        _uiState.update { it.copy(form = it.form.copy(validationErrorKey = key)) }
    }

    private fun parseOccurred(form: LedgerEntryFormState): Instant? {
        return try {
            val date = LocalDate.parse(form.dateInput.trim())
            val time = LocalTime.parse(form.timeInput.trim())
            date.atTime(time).atZone(ZoneId.systemDefault()).toInstant()
        } catch (_: Exception) {
            null
        }
    }

    internal fun parseMilli(input: String): Long? {
        val scaled = try {
            BigDecimal(input.trim().replace(',', '.')).movePointRight(3)
        } catch (_: Exception) {
            return null
        }
        if (scaled.stripTrailingZeros().scale() > 0 || scaled <= BigDecimal.ZERO) return null
        return try { scaled.longValueExact() } catch (_: ArithmeticException) { null }
    }

    internal fun parseCentavos(input: String): Long? {
        val scaled = try {
            BigDecimal(input.trim().replace(',', '.')).movePointRight(2)
        } catch (_: Exception) {
            return null
        }
        if (scaled.stripTrailingZeros().scale() > 0 || scaled < BigDecimal.ZERO) return null
        return try { scaled.longValueExact() } catch (_: ArithmeticException) { null }
    }

    internal fun parseKwhToWh(input: String): Long? {
        val scaled = try {
            BigDecimal(input.trim().replace(',', '.')).movePointRight(3)
        } catch (_: Exception) {
            return null
        }
        if (scaled.stripTrailingZeros().scale() > 0 || scaled <= BigDecimal.ZERO ||
            scaled > BigDecimal(1_000_000_000)
        ) {
            return null
        }
        return try { scaled.longValueExact() } catch (_: ArithmeticException) { null }
    }

    internal fun parseRateMilli(input: String): Long? {
        val scaled = try {
            BigDecimal(input.trim().replace(',', '.')).movePointRight(3)
        } catch (_: Exception) {
            return null
        }
        if (scaled.stripTrailingZeros().scale() > 0 || scaled < BigDecimal.ZERO) return null
        return try { scaled.longValueExact() } catch (_: ArithmeticException) { null }
    }

    private fun milliToInput(milli: Long): String =
        BigDecimal.valueOf(milli, 3).stripTrailingZeros().toPlainString()

    private fun centavosToInput(centavos: Long): String =
        BigDecimal.valueOf(centavos, 2).stripTrailingZeros().toPlainString()

    private fun whToKwhInput(wh: Long): String =
        BigDecimal.valueOf(wh, 3).stripTrailingZeros().toPlainString()
}
