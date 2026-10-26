package com.anpfuel.app.ui.driver.work

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import java.util.Locale

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DriverWorkScreen(
    vehicleId: String,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: DriverWorkViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(vehicleId) { viewModel.load(vehicleId) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(), containerColor = MaterialTheme.colorScheme.background,
        topBar = { AnpTopAppBar(title = { Text("Turno e ganhos") }, onNavigateUp = onNavigateBack) },
    ) { padding ->
        when {
            state.loading -> LoadingState(modifier = Modifier.padding(padding))
            else -> Column(
                modifier = Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                state.message?.let { message ->
                    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(12.dp)) {
                            Text(message)
                            OutlinedButton(onClick = viewModel::clearMessage) { Text("Fechar") }
                        }
                    }
                }
                ShiftCard(state, vehicleId, viewModel)
                EarningsCard(vehicleId, viewModel)
                PayoutCard(vehicleId, viewModel)
                CostCard(vehicleId, state, viewModel)
                SummaryCard(state)
            }
        }
    }
}

@Composable
private fun ShiftCard(state: DriverWorkState, vehicleId: String, vm: DriverWorkViewModel) {
    var startKm by remember { mutableStateOf("") }
    var endKm by remember { mutableStateOf("") }
    val shift = state.shift
    Card(modifier = Modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Turno manual", style = MaterialTheme.typography.titleMedium)
            if (shift == null || shift.state == "CLOSED") {
                Text("Nenhum turno ativo. O hodômetro é obrigatório para abrir.")
                OutlinedTextField(startKm, { startKm = it }, label = { Text("Hodômetro inicial (km)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
                Button(onClick = { vm.start(vehicleId, startKm) }, modifier = Modifier.fillMaxWidth()) { Text("Iniciar turno") }
            } else {
                Text("Estado: ${shift.state} · ${(state.activeMillis / 60000)} min trabalhados")
                RowButtons(
                    onPause = { vm.pause(vehicleId) }, onResume = { vm.resume(vehicleId) },
                    onEnd = { vm.end(vehicleId, endKm) }, state = shift.state,
                )
                OutlinedTextField(endKm, { endKm = it }, label = { Text("Hodômetro final (km)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
                OutlinedButton(onClick = vm::startGps, modifier = Modifier.fillMaxWidth()) { Text("Ligar GPS opcional") }
                OutlinedButton(onClick = vm::stopGps, modifier = Modifier.fillMaxWidth()) { Text("Parar GPS opcional") }
                state.gaps.filter { !it.reviewed }.forEach { gap ->
                    OutlinedButton(onClick = { vm.reviewGap(vehicleId, gap.gapId) }, modifier = Modifier.fillMaxWidth()) {
                        Text("Revisar lacuna: ${gap.reason}")
                    }
                }
            }
        }
    }
}

@Composable
private fun RowButtons(onPause: () -> Unit, onResume: () -> Unit, onEnd: () -> Unit, state: String) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        when (state) {
            "RUNNING" -> Button(onClick = onPause, modifier = Modifier.fillMaxWidth()) { Text("Pausar") }
            "PAUSED" -> Button(onClick = onResume, modifier = Modifier.fillMaxWidth()) { Text("Retomar") }
        }
        OutlinedButton(onClick = onEnd, modifier = Modifier.fillMaxWidth()) { Text("Encerrar turno") }
    }
}

@Composable
private fun EarningsCard(vehicleId: String, vm: DriverWorkViewModel) {
    var platform by remember { mutableStateOf("UBER") }
    var net by remember { mutableStateOf("") }
    var tips by remember { mutableStateOf("") }
    var fees by remember { mutableStateOf("") }
    Card(modifier = Modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Ganhos", style = MaterialTheme.typography.titleMedium)
            OutlinedTextField(platform, { platform = it }, label = { Text("Plataforma") }, singleLine = true)
            OutlinedTextField(net, { net = it }, label = { Text("Líquido (R$)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
            OutlinedTextField(tips, { tips = it }, label = { Text("Gorjetas (R$)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
            OutlinedTextField(fees, { fees = it }, label = { Text("Taxas (R$)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
            Button(onClick = { vm.saveEarning(vehicleId, platform, net, tips, fees) }, modifier = Modifier.fillMaxWidth()) { Text("Salvar ganho") }
        }
    }
}

@Composable
private fun PayoutCard(vehicleId: String, vm: DriverWorkViewModel) {
    var amount by remember { mutableStateOf("") }
    Card(modifier = Modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Repasse recebido", style = MaterialTheme.typography.titleMedium)
            OutlinedTextField(amount, { amount = it }, label = { Text("Valor (R$)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
            Button(onClick = { vm.savePayout(vehicleId, amount, "PAYOUT") }, modifier = Modifier.fillMaxWidth()) { Text("Registrar repasse") }
        }
    }
}

@Composable
private fun CostCard(vehicleId: String, state: DriverWorkState, vm: DriverWorkViewModel) {
    var ownership by remember { mutableStateOf(state.cost?.ownership ?: "OWNED") }
    var amount by remember { mutableStateOf("") }
    var personal by remember { mutableStateOf("0") }
    Card(modifier = Modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Custo do veículo", style = MaterialTheme.typography.titleMedium)
            OutlinedTextField(ownership, { ownership = it }, label = { Text("OWNED, LEASED ou FINANCED") }, singleLine = true)
            OutlinedTextField(amount, { amount = it }, label = { Text("Valor mensal (R$)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal))
            OutlinedTextField(personal, { personal = it }, label = { Text("Uso pessoal (%)") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number))
            Text("Estimativa separada do gasto real; o combustível não é tratado como consumo do turno.")
            Button(onClick = { vm.saveCost(vehicleId, ownership, amount, personal) }, modifier = Modifier.fillMaxWidth()) { Text("Salvar configuração") }
        }
    }
}

@Composable
private fun SummaryCard(state: DriverWorkState) {
    val earned = state.earnings.sumOf { it.netCentavos ?: 0L }
    val payouts = state.payouts.sumOf { it.amountCentavos }
    Card(modifier = Modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("Resumo local", style = MaterialTheme.typography.titleMedium)
            Text("Ganhos: R$ ${"%.2f".format(Locale("pt", "BR"), earned / 100.0)}")
            Text("Repasses: R$ ${"%.2f".format(Locale("pt", "BR"), payouts / 100.0)}")
            Text("Distâncias registradas: ${state.distances.size}")
            Text("O repasse não duplica o ganho.")
        }
    }
}
