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
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.local.entity.DriverPayoutEntity
import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity
import com.anpfuel.data.repository.DriverLocalStore
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

data class DriverWorkState(
    val loading: Boolean = true,
    val shift: DriverShiftEntity? = null,
    val gaps: List<DriverShiftGapEntity> = emptyList(),
    val earnings: List<DriverEarningEntity> = emptyList(),
    val payouts: List<DriverPayoutEntity> = emptyList(),
    val distances: List<DriverDistanceEntity> = emptyList(),
    val cost: DriverCostSettingEntity? = null,
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

    fun load(vehicleId: String) {
        viewModelScope.launch {
            val shift = store.currentShift(owner, origin)
            val gaps = shift?.let { store.listShiftGaps(owner, origin, it.shiftId) }.orEmpty()
            state.value = DriverWorkState(
                loading = false, shift = shift, gaps = gaps,
                earnings = store.listEarnings(owner, origin, vehicleId),
                payouts = store.listPayouts(owner, origin),
                distances = store.listDistances(owner, origin, vehicleId),
                cost = store.findCostSetting(owner, origin, vehicleId),
                activeMillis = shift?.activeMillis ?: 0,
            )
        }
    }

    private fun clock(): Pair<String, Long> =
        "boot-" + Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, 0) to System.currentTimeMillis()

    private fun elapsed(row: DriverShiftEntity, nowElapsed: Long): Long {
        val last = row.lastElapsedMillis ?: return 0
        return (nowElapsed - last).coerceAtLeast(0)
    }

    fun start(vehicleId: String, startKm: String) {
        val metres = parseKm(startKm) ?: return fail("Odômetro inicial inválido.")
        viewModelScope.launch {
            val (boot, utc) = clock()
            val id = UUID.randomUUID().toString()
            val row = DriverShiftEntity(
                owner, origin, id, vehicleId, ZoneId.systemDefault().id, "RUNNING", 1,
                boot, android.os.SystemClock.elapsedRealtime(), utc, 0, 0, null, null, utc, utc,
            ).copy(startOdometerMetres = metres)
            store.saveShift(row, emptyList())
            load(vehicleId)
        }
    }

    fun pause(vehicleId: String) = mutate(vehicleId, "PAUSED", end = false)
    fun resume(vehicleId: String) = mutate(vehicleId, "RUNNING", end = false)
    fun end(vehicleId: String, endKm: String) {
        val metres = parseKm(endKm) ?: return fail("Odômetro final inválido.")
        val row = state.value.shift ?: return fail("Nenhum turno ativo.")
        val start = row.startOdometerMetres
        if (start != null && metres < start) return fail("Odômetro final menor que o inicial.")
        mutate(vehicleId, "CLOSED", end = false, endMetres = metres)
    }

    private fun mutate(vehicleId: String, stateName: String, end: Boolean, endMetres: Long? = null) {
        val row = state.value.shift ?: return fail("Nenhum turno ativo.")
        viewModelScope.launch {
            val (boot, utc) = clock()
            val nowElapsed = android.os.SystemClock.elapsedRealtime()
            val previousElapsed = row.lastElapsedMillis
            val previousBoot = row.lastBootId
            val worked = row.activeMillis + if (row.state == "RUNNING") elapsed(row, nowElapsed) else 0
            var gaps = state.value.gaps
            var nextState = stateName
            if (previousBoot != null && previousBoot != boot) {
                nextState = "RECOVERY_REQUIRED"
                gaps = gaps + DriverShiftGapEntity(
                    owner, origin, row.shiftId, UUID.randomUUID().toString(), "boot-changed",
                    previousBoot, previousElapsed ?: 0, row.lastUtcMillis ?: utc, boot, nowElapsed, utc,
                    false, null, utc,
                )
            }
            val updated = row.copy(
                state = nextState, version = row.version + 1, lastBootId = boot, lastElapsedMillis = nowElapsed,
                lastUtcMillis = utc, activeMillis = worked, updatedAtMillis = utc, endOdometerMetres = endMetres ?: row.endOdometerMetres,
            )
            store.saveShift(updated, gaps)
            load(vehicleId)
        }
    }

    fun reviewGap(vehicleId: String, gapId: String) {
        val row = state.value.shift ?: return
        viewModelScope.launch {
            val gaps = state.value.gaps.map { if (it.gapId == gapId) it.copy(reviewed = true) else it }
            store.saveShift(row.copy(state = "PAUSED", version = row.version + 1, updatedAtMillis = System.currentTimeMillis()), gaps)
            load(vehicleId)
        }
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
            load(vehicleId)
        }
    }

    fun savePayout(vehicleId: String, amountInput: String, kind: String) {
        val amount = parseMoney(amountInput) ?: return fail("Repasse inválido.")
        viewModelScope.launch {
            store.savePayout(DriverPayoutEntity(owner, origin, UUID.randomUUID().toString(), vehicleId, System.currentTimeMillis(), amount, kind, "manual", null, System.currentTimeMillis()))
            load(vehicleId)
        }
    }

    fun saveCost(vehicleId: String, ownership: String, amountInput: String, personalInput: String) {
        val amount = amountInput.takeIf { it.isNotBlank() }?.let { parseMoney(it) ?: return fail("Custo inválido.") }
        val personal = personalInput.toIntOrNull()?.takeIf { it in 0..100 } ?: return fail("Fração pessoal inválida.")
        viewModelScope.launch {
            store.saveCostSetting(DriverCostSettingEntity(owner, origin, vehicleId, ownership, amount, personal, LocalDate.now().toString(), System.currentTimeMillis()))
            load(vehicleId)
        }
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
    }
}
