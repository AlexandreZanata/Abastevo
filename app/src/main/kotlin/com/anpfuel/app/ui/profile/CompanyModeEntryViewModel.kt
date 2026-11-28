package com.anpfuel.app.ui.profile

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.enterprise.CompanyModeEntryRule
import com.anpfuel.application.enterprise.EnterpriseContextFlow
import com.anpfuel.application.enterprise.EnterpriseContextGateway
import com.anpfuel.application.enterprise.EnterpriseMembershipHints
import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import javax.inject.Named
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * B-BR-EMP-ENTRY-01 — whether Perfil's "Company mode" entry is open. It reads
 * the remembered link, records what a confirmed company list proves, and reads
 * the list itself only while the link is unknown and no company is in use.
 */
@HiltViewModel
class CompanyModeEntryViewModel @Inject constructor(
    private val contexts: EnterpriseContextFlow,
    private val gateway: EnterpriseContextGateway,
    private val hints: EnterpriseMembershipHints,
    @Named("enterpriseConnectivity") private val network: NetworkConnectivityGateway,
) : ViewModel() {

    private val _enabled = MutableStateFlow(resolve())
    val enabled: StateFlow<Boolean> = _enabled.asStateFlow()
    private var probed = false

    init {
        viewModelScope.launch {
            contexts.state.collect { state ->
                val binding = gateway.currentBinding()
                if (binding != null) {
                    CompanyModeEntryRule.linkToRecord(state)?.let { hints.record(binding.accountId, binding.origin, it) }
                }
                _enabled.value = resolve()
            }
        }
    }

    /** Perfil resumed: re-evaluate and, once per screen, read the list when the link is unknown. */
    fun refresh() {
        _enabled.value = resolve()
        val binding = gateway.currentBinding()
        val hint = binding?.let { hints.linked(it.accountId, it.origin) }
        if (probed || !CompanyModeEntryRule.shouldProbe(binding, hint, contexts.state.value)) return
        probed = true
        viewModelScope.launch {
            val online = runCatching { network.observeIsConnected().first() }.getOrDefault(false)
            if (online) contexts.refresh()
        }
    }

    private fun resolve(): Boolean {
        val binding = gateway.currentBinding()
        val hint = binding?.let { hints.linked(it.accountId, it.origin) }
        return CompanyModeEntryRule.enabled(binding, hint, contexts.state.value)
    }
}
