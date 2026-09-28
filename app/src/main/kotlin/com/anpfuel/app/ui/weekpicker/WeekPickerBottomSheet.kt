package com.anpfuel.app.ui.weekpicker

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.unit.dp
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.accessibility.headingSemantics
import com.anpfuel.app.ui.components.SurveyWeekChip

@Composable
fun SurveyWeekChipAction(
    onWeekChanged: () -> Unit,
    modifier: Modifier = Modifier,
    hasTrailingAction: Boolean = false,
    activeSurveyWeekViewModel: ActiveSurveyWeekViewModel = hiltViewModel(),
) {
    val uiState by activeSurveyWeekViewModel.uiState.collectAsStateWithLifecycle()
    var sheetVisible by rememberSaveable { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        activeSurveyWeekViewModel.load()
    }

    val chipModifier = if (hasTrailingAction) {
        modifier
    } else {
        modifier.padding(end = 12.dp)
    }

    uiState.surveyWeek?.let { surveyWeek ->
        SurveyWeekChip(
            surveyWeek = surveyWeek,
            isLatest = uiState.isLatest,
            onClick = { sheetVisible = true },
            modifier = chipModifier,
        )
    }

    WeekPickerBottomSheet(
        visible = sheetVisible,
        onDismiss = { sheetVisible = false },
        onWeekSelected = {
            sheetVisible = false
            activeSurveyWeekViewModel.load()
            onWeekChanged()
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeekPickerBottomSheet(
    visible: Boolean,
    onDismiss: () -> Unit,
    onWeekSelected: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: WeekPickerViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    val maxSheetHeight = LocalConfiguration.current.screenHeightDp.dp * 0.92f

    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { destination ->
            when (destination) {
                WeekPickerNavigation.Completed -> onWeekSelected()
            }
        }
    }

    if (visible) {
        ModalBottomSheet(
            onDismissRequest = onDismiss,
            sheetState = sheetState,
            modifier = modifier,
            contentWindowInsets = { WindowInsets(0) },
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = maxSheetHeight)
                    .fillMaxHeight()
                    .navigationBarsPadding()
                    .padding(bottom = 16.dp),
            ) {
                Text(
                    text = stringResource(R.string.week_picker_title),
                    style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier
                        .padding(horizontal = 16.dp, vertical = 12.dp)
                        .headingSemantics(),
                )
                WeekPickerContent(
                    uiState = uiState,
                    onRetryCatalog = viewModel::loadCatalog,
                    onUseLatestWeek = viewModel::useLatestWeek,
                    onSelectWeek = viewModel::onWeekRowTapped,
                    onRetrySync = viewModel::retrySync,
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f),
                )
            }
        }
    }

    uiState.pendingConfirmation?.let { entry ->
        WeekPickerConfirmDialog(
            entry = entry,
            locale = locale,
            onConfirm = viewModel::confirmPendingWeek,
            onDismiss = viewModel::dismissPendingConfirmation,
        )
    }
}
