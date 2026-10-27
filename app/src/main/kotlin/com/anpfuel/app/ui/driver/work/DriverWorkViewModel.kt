package com.anpfuel.app.ui.driver.work

import android.content.Context
import android.provider.Settings
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.local.entity.DriverCostSettingEntity
import com.anpfuel.data.local.entity.DriverDistanceEntity
import com.anpfuel.data.local.entity.DriverEarningEntity
import com.anpfuel.data.local.entity.DriverEarningRevisionEntity
import com.anpfuel.data.local.entity.DriverGoalEntity
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.local.entity.DriverMaintenanceEntity
import com.anpfuel.data.local.entity.DriverReconciliationEntity
import com.anpfuel.data.local.entity.DriverPayoutEntity
import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity
import com.anpfuel.data.repository.DriverLocalStore
import com.anpfuel.domain.driver.DriverClock
import com.anpfuel.domain.driver.DriverShiftAction
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import java.math.BigDecimal
import java.text.DecimalFormatSymbols
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.util.Locale
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay

data class DriverWorkState(
    val loading: Boolean = true,
    val shift: DriverShiftEntity? = null,
    val shiftBlocked: Boolean = false,
    val gaps: List<DriverShiftGapEntity> = emptyList(),
    val earnings: List<DriverEarningEntity> = emptyList(),
    val payouts: List<DriverPayoutEntity> = emptyList(),
    val distances: List<DriverDistanceEntity> = emptyList(),
    val cost: DriverCostSettingEntity? = null,
    val reconciliations: List<DriverReconciliationEntity> = emptyList(),
    val goals: List<DriverGoalEntity> = emptyList(),
    val maintenance: List<DriverMaintenanceEntity> = emptyList(),
    val message: String? = null,
    val activeMillis: Long = 0,
)

@HiltViewModel
class DriverWorkViewModel @Inject constructor(
    private val store: DriverLocalStore,
    private val sessions: AuthSessionStore,
    @ApplicationContext private val context: Context,
) : ViewModel() {
    private val state = MutableStateFlow(DriverWorkState())
    val uiState = state.asStateFlow()
    private val origin = "android"
    private val owner: String get() = sessions.load()?.accountId ?: "local"

    private var initializedOwner: String? = null
    private var checkpointStarted = false

    fun load(vehicleId: String) {
        val ownerAtStart = owner
        viewModelScope.launch {
            try {
                val shift = store.currentShiftChecked(ownerAtStart, origin)
                check(owner == ownerAtStart)
                if (initializedOwner != ownerAtStart && shift?.state == "RUNNING" && shift.vehicleId == vehicleId) {
                    val gaps = store.listShiftGaps(ownerAtStart, origin, shift.shiftId)
                    val (recovered, nextGaps) = DriverWorkShiftTransition.apply(
                        shift, gaps, DriverShiftAction.RECOVER, clock(), UUID.randomUUID().toString(),
                    )
                    check(owner == ownerAtStart)
                    store.updateShift(recovered, nextGaps, shift.version)
                    DriverGpsService.stop(context)
                }
                check(owner == ownerAtStart)
                initializedOwner = ownerAtStart.takeIf { shift == null || shift.vehicleId == vehicleId }
                refresh(vehicleId, ownerAtStart)
                if (!checkpointStarted) {
                    checkpointStarted = true
                    viewModelScope.launch {
                        while (true) {
                            delay(30_000)
                            checkpointActiveShift()
                        }
                    }
                }
            } catch (error: RuntimeException) {
                if (error !is IllegalStateException && error !is IllegalArgumentException) throw error
                state.value = DriverWorkState(loading = false, shiftBlocked = true, message = "Não foi possível recuperar o turno. Os dados permanecem salvos para revisão.")
            }
        }
    }

    internal fun checkpointActiveShift() {
        val active = state.value.shift ?: return
        if (active.state == "RUNNING") transition(active.vehicleId, DriverShiftAction.CHECKPOINT)
    }

    private suspend fun refresh(vehicleId: String, ownerAtStart: String = owner) {
        check(owner == ownerAtStart)
        val shift = store.currentShiftChecked(ownerAtStart, origin)
        check(owner == ownerAtStart)
        if (shift != null && shift.vehicleId != vehicleId) {
            state.value = DriverWorkState(loading = false, shiftBlocked = true, message = "Existe um turno ativo em outro veículo. Abra esse veículo para continuar.")
            return
        }
        val gaps = shift?.let { store.listShiftGaps(ownerAtStart, origin, it.shiftId) }.orEmpty()
        val next = DriverWorkState(
            loading = false, shift = shift, gaps = gaps,
            earnings = store.listEarnings(ownerAtStart, origin, vehicleId),
            payouts = store.listPayouts(ownerAtStart, origin),
            distances = store.listDistances(ownerAtStart, origin, vehicleId),
            cost = store.findCostSetting(ownerAtStart, origin, vehicleId),
            reconciliations = store.listReconciliations(ownerAtStart, origin, vehicleId),
            goals = store.listGoals(ownerAtStart, origin, vehicleId),
            maintenance = store.listMaintenance(ownerAtStart, origin, vehicleId),
            activeMillis = shift?.activeMillis ?: 0,
        )
        check(owner == ownerAtStart)
        state.value = next
    }

    private fun clock(): DriverClock = DriverClock(
        "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0),
        android.os.SystemClock.elapsedRealtime(), System.currentTimeMillis(),
    )

    fun start(vehicleId: String, startKm: String) {
        val metres = parseKm(startKm)?.takeIf { it >= 0 } ?: return fail("Odômetro inicial inválido.")
        val ownerAtStart = owner
        viewModelScope.launch {
            try {
                check(owner == ownerAtStart)
                val now = clock()
                val id = UUID.randomUUID().toString()
                val row = DriverShiftEntity(
                    ownerAtStart, origin, id, vehicleId, ZoneId.systemDefault().id, "RUNNING", 1,
                    now.bootId, now.elapsedMillis, now.utcMillis, 0, 0, null, null, now.utcMillis, now.utcMillis,
                ).copy(startOdometerMetres = metres)
                check(owner == ownerAtStart)
                store.startShift(row)
                refresh(vehicleId, ownerAtStart)
            } catch (_: IllegalStateException) {
                if (owner != ownerAtStart) {
                    initializedOwner = null
                    state.value = DriverWorkState(loading = false, shiftBlocked = true)
                }
                fail("Já existe um turno ativo ou houve conflito. Reabra a tela para revisar.")
            }
        }
    }

    fun pause(vehicleId: String) = transition(vehicleId, DriverShiftAction.PAUSE)
    fun resume(vehicleId: String) = transition(vehicleId, DriverShiftAction.RESUME)
    fun end(vehicleId: String, endKm: String) {
        val metres = parseKm(endKm)?.takeIf { it >= 0 } ?: return fail("Odômetro final inválido.")
        val row = state.value.shift ?: return fail("Nenhum turno ativo.")
        val startMetres = row.startOdometerMetres
        if (startMetres != null && metres < startMetres) return fail("Odômetro final menor que o inicial.")
        if (state.value.gaps.any { !it.reviewed || it.reviewedWorkMillis == null }) return fail("Revise todas as lacunas antes de encerrar o turno.")
        transition(vehicleId, DriverShiftAction.END, endMetres = metres)
    }

    private fun transition(vehicleId: String, action: DriverShiftAction, endMetres: Long? = null, gapId: String? = null, reviewedWorkMillis: Long? = null) {
        val row = state.value.shift ?: return
        val gaps = state.value.gaps
        val ownerAtStart = owner
        viewModelScope.launch {
            try {
                check(row.ownerAccount == ownerAtStart && owner == ownerAtStart)
                val (candidate, nextGaps) = DriverWorkShiftTransition.apply(row, gaps, action, clock(), UUID.randomUUID().toString(), gapId, reviewedWorkMillis)
                val updated = if (candidate.state == "CLOSED") candidate.copy(endOdometerMetres = endMetres) else candidate
                check(owner == ownerAtStart)
                store.updateShift(updated, nextGaps, row.version)
                if (updated.state != "RUNNING") DriverGpsService.stop(context)
                refresh(vehicleId, ownerAtStart)
            } catch (error: RuntimeException) {
                if (error !is IllegalStateException && error !is IllegalArgumentException) throw error
                if (row.ownerAccount == ownerAtStart && owner == ownerAtStart) {
                    try { refresh(vehicleId, ownerAtStart) } catch (_: IllegalStateException) { }
                } else {
                    initializedOwner = null
                    state.value = DriverWorkState(loading = false, shiftBlocked = true)
                }
                fail("O turno mudou ou não pôde ser recuperado. Os dados permanecem salvos para revisão.")
            }
        }
    }

    fun startGps() {
        val shift = state.value.shift ?: return fail("Inicie um turno antes do GPS.")
        if (shift.ownerAccount != owner || shift.state != "RUNNING") return fail("O GPS exige um turno em andamento.")
        DriverGpsService.start(context, shift.shiftId)
    }
    fun stopGps() = DriverGpsService.stop(context)

    fun reviewGap(vehicleId: String, gapId: String, minutes: String) {
        val gap = state.value.gaps.singleOrNull { it.gapId == gapId && (!it.reviewed || it.reviewedWorkMillis == null) } ?: return fail("Lacuna não encontrada.")
        val amount = minutes.toLongOrNull()?.takeIf { it >= 0 && it <= Long.MAX_VALUE / 60_000 }?.times(60_000)
            ?: return fail("Informe os minutos trabalhados na lacuna, inclusive zero quando confirmado.")
        val wallGap = gap.nextUtcMillis - gap.previousUtcMillis
        if (amount > 0 && (wallGap < 0 || amount > wallGap)) return fail("Duração não verificável; mantenha a lacuna pendente.")
        transition(vehicleId, DriverShiftAction.REVIEW_GAP, gapId = gapId, reviewedWorkMillis = amount)
    }

    fun saveEarning(vehicleId: String, platform: String, netInput: String, tipsInput: String, feesInput: String) {
        val net = parseMoney(netInput) ?: return fail("Ganho inválido.")
        val tips = tipsInput.takeIf { it.isNotBlank() }?.let { parseMoney(it) ?: return fail("Gorjeta inválida.") }
        val fees = feesInput.takeIf { it.isNotBlank() }?.let { parseMoney(it) ?: return fail("Taxa inválida.") }
        viewModelScope.launch {
            val now = Instant.now()
            val day = LocalDate.now().atStartOfDay(ZoneId.systemDefault()).toInstant().toEpochMilli()
            val end = day + 86_400_000
            if (store.overlapCount(owner, origin, vehicleId, platform, day, end) > 0) return@launch fail("Já existe ganho nessa janela.")
            val id = UUID.randomUUID().toString()
            val revision = DriverEarningRevisionEntity(owner, origin, id, 1, "{}", "a".repeat(64), System.currentTimeMillis())
            val earning = DriverEarningEntity(
                owner, origin, id, vehicleId, platform, "SUMMARY", "BRL", day, end, ZoneId.systemDefault().id,
                "DETAILED", null, fees, tips, null, null, net, 1, "a".repeat(64), System.currentTimeMillis(),
            )
            val command = DriverLocalCommandEntity(owner, origin, UUID.randomUUID().toString(), "EARNING_SAVE", "a".repeat(64), 0, "PENDING", 0, 0, null, 0, System.currentTimeMillis())
            store.saveEarning(earning, revision, command)
            refresh(vehicleId)
        }
    }

    fun savePayout(vehicleId: String, amountInput: String, kind: String) {
        val amount = parseMoney(amountInput) ?: return fail("Repasse inválido.")
        viewModelScope.launch {
            store.savePayout(DriverPayoutEntity(owner, origin, UUID.randomUUID().toString(), vehicleId, System.currentTimeMillis(), amount, kind, "manual", null, System.currentTimeMillis()))
            refresh(vehicleId)
        }
    }

    fun saveCost(vehicleId: String, ownership: String, amountInput: String, personalInput: String) {
        val amount = amountInput.takeIf { it.isNotBlank() }?.let { parseMoney(it) ?: return fail("Custo inválido.") }
        val personal = personalInput.toIntOrNull()?.takeIf { it in 0..100 } ?: return fail("Fração pessoal inválida.")
        viewModelScope.launch {
            store.saveCostSetting(DriverCostSettingEntity(owner, origin, vehicleId, ownership, amount, personal, LocalDate.now().toString(), System.currentTimeMillis()))
            refresh(vehicleId)
        }
    }


    fun saveReconciliation(vehicleId: String, basis: String, odometerKm: String, gpsKm: String, paidKm: String, personalKm: String) {
        val odo = parseKm(odometerKm) ?: return fail("Odômetro inválido.")
        val gps = gpsKm.takeIf { it.isNotBlank() }?.let { parseKm(it) ?: return fail("GPS inválido.") }
        val paid = parseKm(paidKm) ?: return fail("Km pago inválido.")
        val personal = personalKm.takeIf { it.isNotBlank() }?.let { parseKm(it) ?: return fail("Km pessoal inválido.") }
        val empty = if (personal != null && paid <= odo && (paid + personal) <= odo) odo - paid - personal else null
        viewModelScope.launch {
            store.saveReconciliation(DriverReconciliationEntity(
                owner, origin, vehicleId, LocalDate.now().toString(), basis, odo, gps, paid, empty, personal,
                empty != null, System.currentTimeMillis(),
            ))
            refresh(vehicleId)
        }
    }

    fun saveGoal(vehicleId: String, period: String, metric: String, targetInput: String) {
        val target = parseMoney(targetInput) ?: return fail("Meta inválida.")
        viewModelScope.launch {
            store.saveGoal(DriverGoalEntity(owner, origin, vehicleId, UUID.randomUUID().toString(), period, metric, target, true, System.currentTimeMillis()))
            refresh(vehicleId)
        }
    }

    fun saveMaintenance(vehicleId: String, kind: String, intervalKmInput: String, intervalDaysInput: String, baselineKmInput: String) {
        val intervalKm = intervalKmInput.takeIf { it.isNotBlank() }?.let { parseKm(it) ?: return fail("Intervalo inválido.") }
        val intervalDays = intervalDaysInput.takeIf { it.isNotBlank() }?.toLongOrNull() ?: if (intervalDaysInput.isBlank()) null else return fail("Dias inválidos.")
        val baseline = baselineKmInput.takeIf { it.isNotBlank() }?.let { parseKm(it) ?: return fail("Base inválida.") }
        if (intervalKm == null && intervalDays == null) return fail("Informe km ou dias.")
        val today = LocalDate.now()
        viewModelScope.launch {
            store.saveMaintenance(DriverMaintenanceEntity(
                owner, origin, vehicleId, UUID.randomUUID().toString(), kind, intervalKm, intervalDays,
                baseline, today.toString(), baseline?.let { intervalKm?.let { km -> it + km } }, intervalDays?.let { today.plusDays(it).toString() },
                baseline, today.toString(), System.currentTimeMillis(),
            ))
            refresh(vehicleId)
        }
    }

    fun markService(vehicleId: String, item: DriverMaintenanceEntity, kmInput: String) {
        val km = kmInput.takeIf { it.isNotBlank() }?.let { parseKm(it) } ?: item.lastServiceKm
        val today = LocalDate.now()
        viewModelScope.launch {
            store.saveMaintenance(item.copy(lastServiceKm = km ?: item.lastServiceKm, lastServiceDay = today.toString(),
                nextDueKm = km?.let { v -> item.intervalKm?.let { v + it } } ?: item.nextDueKm,
                nextDueDay = item.intervalDays?.let { today.plusDays(it).toString() } ?: item.nextDueDay,
                updatedAtMillis = System.currentTimeMillis()))
            refresh(vehicleId)
        }
    }

    fun recordedResult(): Long {
        val earned = state.value.earnings.sumOf { it.netCentavos ?: 0L }
        val cost = state.value.cost?.let { recordedCost(it.monthlyAmountCentavos, it.personalFractionPercent) } ?: 0L
        return recordedResult(earned, cost)
    }

    fun perKm(result: Long): String? {
        val metres = state.value.reconciliations.firstOrNull()?.let { it.odometerMetres ?: it.gpsMetres } ?: return null
        return perKm(result, metres)
    }

    private fun fail(message: String) { state.value = state.value.copy(message = message) }
    fun clearMessage() { state.value = state.value.copy(message = null) }

    companion object {
        fun parseMoney(raw: String): Long? = try {
            BigDecimal(raw.trim().replace(".", "").replace(',', '.')).movePointRight(2).longValueExact()
        } catch (_: Exception) { null }
        fun parseKm(raw: String): Long? = try {
            BigDecimal(raw.trim().replace(".", "").replace(',', '.')).movePointRight(3).longValueExact()
        } catch (_: Exception) { null }
        fun recordedCost(monthlyCentavos: Long?, personalPercent: Int): Long =
            monthlyCentavos?.let { it * (100 - personalPercent) / 100 } ?: 0L
        fun recordedResult(earned: Long, cost: Long): Long = earned - cost
        fun perKm(result: Long, metres: Long?): String? {
            if (metres == null || metres <= 0) return null
            val km = metres / 1000
            return if (km <= 0) null else (result / km).toString()
        }
    }
}
