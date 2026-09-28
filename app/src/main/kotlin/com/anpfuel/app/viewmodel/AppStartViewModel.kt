package com.anpfuel.app.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.navigation.Routes
import com.anpfuel.application.usecase.navigation.ResolveAppStartDestinationUseCase
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.model.AppStartDestination
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

@HiltViewModel
class AppStartViewModel @Inject constructor(
    private val resolveAppStartDestinationUseCase: ResolveAppStartDestinationUseCase,
    private val autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
) : ViewModel() {

    private val _startDestination = MutableStateFlow<String?>(null)
    val startDestination: StateFlow<String?> = _startDestination.asStateFlow()

    init {
        viewModelScope.launch {
            val destination = resolveAppStartDestinationUseCase()
            _startDestination.value = destination.toRoute()
            if (destination == AppStartDestination.HOME) {
                autoDownloadLatestWeekUseCase(SyncRequestSource.SCHEDULED)
            }
        }
    }

    private fun AppStartDestination.toRoute(): String = when (this) {
        AppStartDestination.ONBOARDING -> Routes.ONBOARDING
        AppStartDestination.HOME -> Routes.HOME
        AppStartDestination.WEEK_PICKER -> Routes.WEEK_PICKER
    }
}
