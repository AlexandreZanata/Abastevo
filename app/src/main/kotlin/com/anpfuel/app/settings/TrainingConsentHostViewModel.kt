package com.anpfuel.app.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import com.anpfuel.data.local.preferences.TrainingConsentOwner
import com.anpfuel.data.local.preferences.TrainingTermsSource
import com.anpfuel.domain.repository.TrainingTermsDocument
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*

data class ConsentHostState(val loaded:Boolean=false,val caller:TrainingConsentCaller?=null,val terms:TrainingTermsDocument?=null)
@HiltViewModel
class TrainingConsentHostViewModel @Inject constructor(private val owner:TrainingConsentOwner,private val terms:TrainingTermsSource):ViewModel() {
    private val mutable=MutableStateFlow(ConsentHostState())
    val state=mutable.asStateFlow()
    private var generation=0L
    fun refresh() {
        val request=++generation
        mutable.value=ConsentHostState() // Drop old review/authority during a lifecycle change.
        viewModelScope.launch {
            val next=withContext(Dispatchers.IO) {
                val document=try { terms.read() } catch(e:CancellationException) {throw e} catch(_:Exception) {null}
                val caller=try {owner.current()} catch(e:CancellationException) {throw e} catch(_:Exception) {null}
                ConsentHostState(true,caller,document)
            }
            if(generation==request) mutable.value=next
        }
    }
}
