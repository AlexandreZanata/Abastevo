package com.anpfuel.app.ui.forum

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.forum.EditForumTopicUseCase
import com.anpfuel.application.usecase.forum.GetOwnerTopicUseCase
import com.anpfuel.application.usecase.forum.RemoveForumTopicUseCase
import com.anpfuel.application.usecase.forum.SubmitForumTopicUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumTopicReceipt
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class ForumTopicValidationError {
    TITLE_EMPTY,
    TITLE_TOO_LONG,
    BODY_EMPTY,
    BODY_TOO_LONG,
    CATEGORY_REQUIRED,
    REGION_INVALID,
}

data class ForumTopicFormUiState(
    val isEditMode: Boolean = false,
    val topicId: String? = null,
    val isLoading: Boolean = false,
    val title: String = "",
    val bodyText: String = "",
    val selectedCategory: ForumCategory? = null,
    val selectedUf: String? = null,
    val municipalityIbge: String = "",
    val selectedFuelProduct: String? = null,
    val publicAlias: String = "",
    val expectedRevision: Long = 1L,
    val isSubmitting: Boolean = false,
    val isRemoving: Boolean = false,
    val isDirty: Boolean = false,
    val showDiscardDialog: Boolean = false,
    val showDeleteDialog: Boolean = false,
    val submissionReceipt: ForumTopicReceipt? = null,
    val isRemoved: Boolean = false,
    val actionError: ForumActionError? = null,
    val validationError: ForumTopicValidationError? = null,
    val isAuthenticated: Boolean = false,
    val requireAuthPrompt: Boolean = false,
    val notFoundError: Boolean = false,
)

@HiltViewModel
class ForumTopicFormViewModel @Inject constructor(
    private val submitForumTopic: SubmitForumTopicUseCase,
    private val editForumTopic: EditForumTopicUseCase,
    private val removeForumTopic: RemoveForumTopicUseCase,
    private val getOwnerTopic: GetOwnerTopicUseCase,
    private val sessionStore: AuthSessionStore? = null,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val topicIdArg: String? = savedStateHandle.get<String>("topicId")?.trim()?.takeIf { it.isNotEmpty() }
    private var lastKnownAccountId: String? = sessionStore?.load()?.accountId
    private var currentOperationId: String = UUID.randomUUID().toString()

    private val _state = MutableStateFlow(
        ForumTopicFormUiState(
            isEditMode = topicIdArg != null,
            topicId = topicIdArg,
            isLoading = topicIdArg != null,
            isAuthenticated = sessionStore?.load() != null,
        )
    )
    val state: StateFlow<ForumTopicFormUiState> = _state.asStateFlow()

    private var loadJob: Job? = null
    private var submitJob: Job? = null
    private var removeJob: Job? = null

    init {
        if (topicIdArg != null) {
            loadTopic(topicIdArg)
        }
    }

    fun checkSessionConsistency() {
        val currentSession = sessionStore?.load()
        val currentAccountId = currentSession?.accountId
        val wasAuthed = _state.value.isAuthenticated
        val isAuthed = currentSession != null

        if (lastKnownAccountId != null && currentAccountId != null && currentAccountId != lastKnownAccountId) {
            // Account changed to a different identity: abort in-flight work and reset private draft to preserve intent safety
            submitJob?.cancel()
            removeJob?.cancel()
            _state.update {
                it.copy(
                    isAuthenticated = isAuthed,
                    isSubmitting = false,
                    isRemoving = false,
                    title = "",
                    bodyText = "",
                    selectedCategory = null,
                    selectedUf = null,
                    municipalityIbge = "",
                    selectedFuelProduct = null,
                    publicAlias = "",
                    isDirty = false,
                    actionError = null,
                    validationError = null,
                )
            }
            lastKnownAccountId = currentAccountId
            currentOperationId = UUID.randomUUID().toString()
        } else if (lastKnownAccountId != currentAccountId) {
            lastKnownAccountId = currentAccountId
            _state.update { it.copy(isAuthenticated = isAuthed) }
        } else if (wasAuthed != isAuthed) {
            _state.update { it.copy(isAuthenticated = isAuthed) }
        }
    }

    fun loadTopic(id: String) {
        loadJob?.cancel()
        _state.update { it.copy(isLoading = true, notFoundError = false, actionError = null) }
        loadJob = viewModelScope.launch {
            try {
                val topic = getOwnerTopic(id)
                _state.update {
                    it.copy(
                        isLoading = false,
                        title = topic.title,
                        bodyText = topic.bodyText,
                        selectedCategory = topic.category,
                        selectedUf = topic.region?.uf,
                        municipalityIbge = topic.region?.municipalityIbge?.toString().orEmpty(),
                        selectedFuelProduct = topic.fuelProduct,
                        publicAlias = topic.publicAlias.orEmpty(),
                        expectedRevision = topic.revision,
                        isDirty = false,
                    )
                }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isLoading = false, notFoundError = true) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isLoading = false, actionError = ForumActionError.Offline) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isLoading = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun updateTitle(newTitle: String) {
        currentOperationId = UUID.randomUUID().toString()
        _state.update {
            it.copy(
                title = newTitle,
                isDirty = true,
                validationError = if (it.validationError == ForumTopicValidationError.TITLE_EMPTY ||
                    it.validationError == ForumTopicValidationError.TITLE_TOO_LONG) null else it.validationError,
            )
        }
    }

    fun updateBodyText(newBody: String) {
        currentOperationId = UUID.randomUUID().toString()
        _state.update {
            it.copy(
                bodyText = newBody,
                isDirty = true,
                validationError = if (it.validationError == ForumTopicValidationError.BODY_EMPTY ||
                    it.validationError == ForumTopicValidationError.BODY_TOO_LONG) null else it.validationError,
            )
        }
    }

    fun updateCategory(category: ForumCategory?) {
        currentOperationId = UUID.randomUUID().toString()
        _state.update {
            it.copy(
                selectedCategory = category,
                isDirty = true,
                validationError = if (it.validationError == ForumTopicValidationError.CATEGORY_REQUIRED) null else it.validationError,
            )
        }
    }

    fun updateUf(uf: String?) {
        val sanitized = uf?.trim()?.uppercase()?.takeIf { it.isNotEmpty() }
        _state.update { it.copy(selectedUf = sanitized, isDirty = true) }
    }

    fun updateMunicipalityIbge(ibge: String) {
        val digitsOnly = ibge.filter { it.isDigit() }.take(7)
        _state.update { it.copy(municipalityIbge = digitsOnly, isDirty = true) }
    }

    fun updateFuelProduct(fuelProduct: String?) {
        _state.update { it.copy(selectedFuelProduct = fuelProduct, isDirty = true) }
    }

    fun updatePublicAlias(alias: String) {
        _state.update { it.copy(publicAlias = alias, isDirty = true) }
    }

    fun onBackPressed(onNavigateBack: () -> Unit) {
        if (_state.value.isDirty) {
            _state.update { it.copy(showDiscardDialog = true) }
        } else {
            onNavigateBack()
        }
    }

    fun confirmDiscard(onNavigateBack: () -> Unit) {
        _state.update {
            it.copy(
                showDiscardDialog = false,
                isDirty = false,
                title = "",
                bodyText = "",
                selectedCategory = null,
                selectedUf = null,
                municipalityIbge = "",
                selectedFuelProduct = null,
                publicAlias = "",
            )
        }
        onNavigateBack()
    }

    fun dismissDiscardDialog() {
        _state.update { it.copy(showDiscardDialog = false) }
    }

    fun dismissActionError() {
        _state.update { it.copy(actionError = null) }
    }

    fun dismissAuthPrompt() {
        _state.update { it.copy(requireAuthPrompt = false) }
    }

    fun dismissSubmissionReceipt() {
        _state.update { it.copy(submissionReceipt = null) }
    }

    fun confirmRemove() {
        _state.update { it.copy(showDeleteDialog = true) }
    }

    fun dismissDeleteDialog() {
        _state.update { it.copy(showDeleteDialog = false) }
    }

    fun submit() {
        checkSessionConsistency()
        val s = _state.value
        val trimmedTitle = s.title.trim()
        val trimmedBody = s.bodyText.trim()

        if (trimmedTitle.isEmpty()) {
            _state.update { it.copy(validationError = ForumTopicValidationError.TITLE_EMPTY) }
            return
        }
        if (trimmedTitle.length > 120) {
            _state.update { it.copy(validationError = ForumTopicValidationError.TITLE_TOO_LONG) }
            return
        }
        if (trimmedBody.isEmpty()) {
            _state.update { it.copy(validationError = ForumTopicValidationError.BODY_EMPTY) }
            return
        }
        if (trimmedBody.length > 4000) {
            _state.update { it.copy(validationError = ForumTopicValidationError.BODY_TOO_LONG) }
            return
        }
        val cat = s.selectedCategory
        if (cat == null || cat == ForumCategory.UNKNOWN) {
            _state.update { it.copy(validationError = ForumTopicValidationError.CATEGORY_REQUIRED) }
            return
        }

        // Region validation
        val ibgeInt = s.municipalityIbge.toIntOrNull()
        val region = if (!s.selectedUf.isNullOrBlank() || ibgeInt != null) {
            ForumRegion(uf = s.selectedUf, municipalityIbge = ibgeInt)
        } else {
            null
        }

        if (!s.isAuthenticated) {
            _state.update { it.copy(requireAuthPrompt = true) }
            return
        }

        submitJob?.cancel()
        _state.update { it.copy(isSubmitting = true, actionError = null, validationError = null) }
        submitJob = viewModelScope.launch {
            try {
                val receipt = if (s.isEditMode && !s.topicId.isNullOrBlank()) {
                    editForumTopic(
                        topicId = s.topicId,
                        expectedRevision = s.expectedRevision,
                        title = trimmedTitle,
                        bodyText = trimmedBody,
                        category = cat,
                        operationId = currentOperationId,
                        region = region,
                        fuelProduct = s.selectedFuelProduct,
                        publicAlias = s.publicAlias.trim().takeIf { it.isNotEmpty() },
                    )
                } else {
                    submitForumTopic(
                        title = trimmedTitle,
                        bodyText = trimmedBody,
                        category = cat,
                        operationId = currentOperationId,
                        region = region,
                        fuelProduct = s.selectedFuelProduct,
                        publicAlias = s.publicAlias.trim().takeIf { it.isNotEmpty() },
                    )
                }
                _state.update {
                    it.copy(
                        isSubmitting = false,
                        submissionReceipt = receipt,
                        expectedRevision = receipt.revision,
                        isDirty = false,
                    )
                }
            } catch (e: ForumException.Conflict) {
                _state.update { it.copy(isSubmitting = false, actionError = ForumActionError.Conflict) }
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isSubmitting = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isSubmitting = false, actionError = ForumActionError.Offline) }
            } catch (e: ForumException.AuthRequired) {
                _state.update { it.copy(isSubmitting = false, requireAuthPrompt = true) }
            } catch (e: ForumException.Invalid) {
                _state.update { it.copy(isSubmitting = false, actionError = ForumActionError.Generic(e.code)) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isSubmitting = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }

    fun executeRemove() {
        val topicId = _state.value.topicId ?: return
        checkSessionConsistency()
        if (!_state.value.isAuthenticated) {
            _state.update { it.copy(requireAuthPrompt = true, showDeleteDialog = false) }
            return
        }

        removeJob?.cancel()
        _state.update { it.copy(isRemoving = true, actionError = null) }
        removeJob = viewModelScope.launch {
            try {
                removeForumTopic(topicId = topicId)
                _state.update {
                    it.copy(
                        isRemoving = false,
                        showDeleteDialog = false,
                        isRemoved = true,
                        isDirty = false,
                    )
                }
            } catch (e: ForumException.Conflict) {
                _state.update { it.copy(isRemoving = false, showDeleteDialog = false, actionError = ForumActionError.Conflict) }
            } catch (e: ForumException.RateLimited) {
                _state.update { it.copy(isRemoving = false, showDeleteDialog = false, actionError = ForumActionError.RateLimited(e.retryAfterSeconds)) }
            } catch (e: ForumException.Offline) {
                _state.update { it.copy(isRemoving = false, showDeleteDialog = false, actionError = ForumActionError.Offline) }
            } catch (e: ForumException.NotFound) {
                _state.update { it.copy(isRemoving = false, showDeleteDialog = false, actionError = ForumActionError.TargetMissing) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _state.update { it.copy(isRemoving = false, showDeleteDialog = false, actionError = ForumActionError.Generic(e.message)) }
            }
        }
    }
}
