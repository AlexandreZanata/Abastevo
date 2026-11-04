package com.anpfuel.app.ui.driver.work

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.compose.ui.platform.LocalContext
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import java.util.Locale
import java.util.Date
import java.text.DateFormat

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DriverWorkScreen(
    vehicleId: String,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: DriverWorkViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (granted) viewModel.startGps() }
    val startGps = {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED) {
            viewModel.startGps()
        } else permission.launch(Manifest.permission.ACCESS_FINE_LOCATION)
    }
    LaunchedEffect(vehicleId) { viewModel.load(vehicleId) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(), containerColor = MaterialTheme.colorScheme.background,
        topBar = { AnpTopAppBar(title = { Text(stringResource(R.string.driver_hub_open_work)) }, onNavigateUp = onNavigateBack) },
    ) { padding ->
        when {
            state.loading -> LoadingState(modifier = Modifier.padding(padding))
            else -> Column(
                modifier = Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(20.dp),
            ) {
                state.message?.let { message ->
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer),
                    ) {
                        Column(Modifier.padding(16.dp)) {
                            Text(message, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
                            OutlinedButton(onClick = viewModel::clearMessage) { Text("Fechar") }
                        }
                    }
                }
                ShiftCard(state, vehicleId, viewModel, startGps, viewModel::stopGps)
                EarningsCard(vehicleId, viewModel)
                PayoutCard(vehicleId, viewModel)
                WorkHeading("Conferir resultados")
                SummaryCard(state)
                ResultCard(state, viewModel)
                ReconciliationCard(vehicleId, state, viewModel)
                WorkHeading("Planejamento")
                CostCard(vehicleId, state, viewModel)
                GoalCard(vehicleId, state, viewModel)
                MaintenanceCard(vehicleId, state, viewModel)
            }
        }
    }
}

@Composable
private fun WorkHeading(title: String) {
    Text(
        title,
        modifier = Modifier.semantics { heading() },
        style = MaterialTheme.typography.titleMedium,
    )
}

@Composable
private fun WorkSection(
    title: String,
    description: String? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val surface = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = surface),
        border = AnpContainerDefaults.borderStroke(surface),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            WorkHeading(title)
            description?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            content()
        }
    }
}

@Composable
private fun WorkField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    keyboard: KeyboardType = KeyboardType.Decimal,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        modifier = Modifier.fillMaxWidth(),
        label = { Text(label) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = keyboard),
    )
}

private val workButton = Modifier.fillMaxWidth().heightIn(min = 48.dp)

@Composable
private fun ShiftCard(state: DriverWorkState, vehicleId: String, vm: DriverWorkViewModel, onStartGps: () -> Unit, onStopGps: () -> Unit) {
    var startKm by remember(vehicleId) { mutableStateOf("") }
    var endKm by remember(state.shift?.shiftId) { mutableStateOf("") }
    val shift = state.shift
    val status = when {
        state.shiftBlocked -> "Controle de turno indisponível. Resolva o conflito para continuar."
        shift == null || shift.state == "CLOSED" -> "Nenhum turno ativo. Informe a quilometragem atual do veículo para começar."
        else -> {
            val label = if (shift.state == "PAUSED") "Turno pausado" else "Turno em andamento"
            "$label · ${state.activeMillis / 60000} min registrados"
        }
    }
    WorkSection("Controle do turno", status) {
        if (state.shiftBlocked) return@WorkSection
        if (shift == null || shift.state == "CLOSED") {
            WorkField(startKm, { startKm = it }, "Hodômetro inicial (km)")
            Button(onClick = { vm.start(vehicleId, startKm) }, modifier = workButton, enabled = startKm.isNotBlank()) {
                Text("Iniciar turno")
            }
        } else {
            when (shift.state) {
                "RUNNING" -> Button(onClick = { vm.pause(vehicleId) }, modifier = workButton) { Text("Pausar") }
                "PAUSED" -> Button(onClick = { vm.resume(vehicleId) }, modifier = workButton) { Text("Retomar") }
            }
            HorizontalDivider()
            WorkHeading("Encerramento")
            Text("Informe a leitura do painel ao terminar.", style = MaterialTheme.typography.bodyMedium)
            WorkField(endKm, { endKm = it }, "Hodômetro final (km)")
            OutlinedButton(onClick = { vm.end(vehicleId, endKm) }, modifier = workButton, enabled = endKm.isNotBlank()) {
                Text("Encerrar turno")
            }
            state.gaps.filter { !it.reviewed || it.reviewedWorkMillis == null }.forEach { gap ->
                key(gap.gapId) {
                    var reviewedMinutes by remember(gap.gapId) { mutableStateOf("") }
                    val formatter = DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT)
                    val previous = formatter.format(Date(gap.previousUtcMillis))
                    val next = formatter.format(Date(gap.nextUtcMillis))
                    Text("Lacuna pendente: ${gap.reason}. Revise o tempo antes de encerrar.")
                    Text("Último registro: $previous · Retomada: $next (horário local)")
                    if (gap.nextUtcMillis < gap.previousUtcMillis) Text("Relógio regressivo: duração da lacuna desconhecida.")
                    WorkField(reviewedMinutes, { reviewedMinutes = it }, "Minutos trabalhados confirmados", KeyboardType.Number)
                    OutlinedButton(
                        onClick = { vm.reviewGap(vehicleId, gap.gapId, reviewedMinutes); reviewedMinutes = "" },
                        modifier = workButton,
                    ) {
                        Text("Confirmar revisão da lacuna")
                    }
                }
            }
            HorizontalDivider()
            WorkHeading("Distância por GPS")
            Text("Opcional. Inicie o registro durante o turno para estimar a distância.", style = MaterialTheme.typography.bodyMedium)
            OutlinedButton(onClick = onStartGps, modifier = workButton, enabled = shift.state == "RUNNING") {
                Text("Iniciar registro por GPS")
            }
            OutlinedButton(onClick = onStopGps, modifier = workButton) { Text("Parar registro por GPS") }
        }
    }
}

@Composable
private fun EarningsCard(vehicleId: String, vm: DriverWorkViewModel) {
    var platform by remember(vehicleId) { mutableStateOf("UBER") }
    var net by remember(vehicleId) { mutableStateOf("") }
    var tips by remember(vehicleId) { mutableStateOf("") }
    var fees by remember(vehicleId) { mutableStateOf("") }
    WorkSection("Registrar ganhos", "Informe a plataforma e o valor líquido do dia. Preencha gorjetas e taxas se quiser detalhar o ganho.") {
        WorkField(platform, { platform = it }, "Plataforma", KeyboardType.Text)
        WorkField(net, { net = it }, "Valor líquido (R$)")
        WorkField(tips, { tips = it }, "Gorjetas (R$, opcional)")
        WorkField(fees, { fees = it }, "Taxas (R$, opcional)")
        Button(
            onClick = { vm.saveEarning(vehicleId, platform, net, tips, fees) },
            modifier = workButton,
            enabled = platform.isNotBlank() && net.isNotBlank(),
        ) {
            Text("Salvar ganho")
        }
    }
}

@Composable
private fun PayoutCard(vehicleId: String, vm: DriverWorkViewModel) {
    var amount by remember { mutableStateOf("") }
    WorkSection("Repasse recebido", "Registre o dinheiro transferido pela plataforma. O repasse fica separado dos ganhos.") {
        WorkField(amount, { amount = it }, "Valor do repasse (R$)")
        Button(onClick = { vm.savePayout(vehicleId, amount, "PAYOUT") }, modifier = workButton, enabled = amount.isNotBlank()) {
            Text("Registrar repasse")
        }
    }
}

@Composable
private fun CostCard(vehicleId: String, state: DriverWorkState, vm: DriverWorkViewModel) {
    var ownership by remember { mutableStateOf(state.cost?.ownership ?: "OWNED") }
    var amount by remember { mutableStateOf("") }
    var personal by remember { mutableStateOf("0") }
    WorkSection("Custo do veículo") {
        WorkField(ownership, { ownership = it }, "OWNED, LEASED ou FINANCED", KeyboardType.Text)
        WorkField(amount, { amount = it }, "Valor mensal (R$)")
        WorkField(personal, { personal = it }, "Uso pessoal (%)", KeyboardType.Number)
        Text("Estimativa separada do gasto real; o combustível não é tratado como consumo do turno.")
        Button(onClick = { vm.saveCost(vehicleId, ownership, amount, personal) }, modifier = workButton) { Text("Salvar configuração") }
    }
}

@Composable
private fun SummaryCard(state: DriverWorkState) {
    val earned = state.earnings.sumOf { it.netCentavos ?: 0L }
    val payouts = state.payouts.sumOf { it.amountCentavos }
    WorkSection("Resumo local") {
        Text("Ganhos: R$ ${"%.2f".format(Locale("pt", "BR"), earned / 100.0)}")
        Text("Repasses: R$ ${"%.2f".format(Locale("pt", "BR"), payouts / 100.0)}")
        Text("Distâncias registradas: ${state.distances.size}")
        Text("Ganhos e repasses são registrados separadamente.")
    }
}

@Composable
private fun ReconciliationCard(vehicleId: String, state: DriverWorkState, vm: DriverWorkViewModel) {
    var basis by remember { mutableStateOf("ODOMETER") }
    var odo by remember { mutableStateOf("") }
    var gps by remember { mutableStateOf("") }
    var paid by remember { mutableStateOf("") }
    var personal by remember { mutableStateOf("") }
    WorkSection("Conciliação de distância") {
        WorkField(basis, { basis = it }, "Base: ODOMETER ou GPS_ESTIMATE", KeyboardType.Text)
        WorkField(odo, { odo = it }, "Hodômetro (km)")
        WorkField(gps, { gps = it }, "GPS (km, opcional)")
        WorkField(paid, { paid = it }, "Km pagos")
        WorkField(personal, { personal = it }, "Km pessoais")
        Button(onClick = { vm.saveReconciliation(vehicleId, basis, odo, gps, paid, personal) }, modifier = workButton) { Text("Salvar conciliação") }
        state.reconciliations.firstOrNull()?.let {
            Text("Última: total ${it.odometerMetres ?: it.gpsMetres ?: 0} m · pagos ${it.paidMetres ?: 0} m · vazios ${it.emptyMetres ?: 0} m · ${if (it.complete) "completa" else "incompleta"}")
        }
    }
}

@Composable
private fun ResultCard(state: DriverWorkState, vm: DriverWorkViewModel) {
    val result = vm.recordedResult()
    WorkSection("Resultado registrado") {
        Text("Ganhos: R$ ${"%.2f".format(Locale("pt", "BR"), state.earnings.sumOf { it.netCentavos ?: 0L } / 100.0)}")
        Text("Custo configurado (estimado): R$ ${"%.2f".format(Locale("pt", "BR"), (state.cost?.monthlyAmountCentavos ?: 0L) / 100.0)}")
        Text("Resultado estimado: R$ ${"%.2f".format(Locale("pt", "BR"), result / 100.0)}")
        Text("R\$/km: ${vm.perKm(result) ?: "indisponível sem distância conciliada"}")
        Text("O resultado usa os ganhos registrados e o custo estimado.")
    }
}

@Composable
private fun GoalCard(vehicleId: String, state: DriverWorkState, vm: DriverWorkViewModel) {
    var period by remember { mutableStateOf("DAILY") }
    var target by remember { mutableStateOf("") }
    WorkSection("Metas") {
        WorkField(period, { period = it }, "DAILY ou WEEKLY", KeyboardType.Text)
        WorkField(target, { target = it }, "Meta (R$)")
        Button(onClick = { vm.saveGoal(vehicleId, period, "RESULT", target) }, modifier = workButton) { Text("Salvar meta") }
        state.goals.firstOrNull()?.let { Text("${it.period}: R$ ${"%.2f".format(Locale("pt", "BR"), it.targetCentavos / 100.0)} · progresso usa resultado local") }
    }
}

@Composable
private fun MaintenanceCard(vehicleId: String, state: DriverWorkState, vm: DriverWorkViewModel) {
    var kind by remember { mutableStateOf("OIL") }
    var interval by remember { mutableStateOf("") }
    var days by remember { mutableStateOf("") }
    var baseline by remember { mutableStateOf("") }
    var serviceKm by remember { mutableStateOf("") }
    WorkSection("Manutenção") {
        WorkField(kind, { kind = it }, "Tipo (OIL, TIRES, ...)", KeyboardType.Text)
        WorkField(interval, { interval = it }, "Intervalo (km)")
        WorkField(days, { days = it }, "Intervalo (dias)", KeyboardType.Number)
        WorkField(baseline, { baseline = it }, "Km atual/base")
        Button(onClick = { vm.saveMaintenance(vehicleId, kind, interval, days, baseline) }, modifier = workButton) { Text("Salvar plano") }
        state.maintenance.forEach { item ->
            Text("${item.kind}: próximo ${item.nextDueKm ?: "-"} km / ${item.nextDueDay ?: "-"}")
        }
        WorkField(serviceKm, { serviceKm = it }, "Km do serviço realizado")
        state.maintenance.firstOrNull()?.let { item ->
            OutlinedButton(onClick = { vm.markService(vehicleId, item, serviceKm) }, modifier = workButton) { Text("Marcar serviço realizado") }
        }
    }
}
