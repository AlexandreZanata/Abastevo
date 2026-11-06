package com.anpfuel.app.ui.forum

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.forum.GetForumCategoriesUseCase
import com.anpfuel.application.usecase.forum.GetForumFeedUseCase
import com.anpfuel.application.usecase.forum.SubmitForumTopicUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumPostSummary
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ForumFeedUiState(
    val categories: List<ForumCategoryItem> = emptyList(),
    val selectedCategory: ForumCategory? = null,
    val filters: ForumFeedFilters = ForumFeedFilters(),
    val availableUfs: List<String> = emptyList(),
    val availableFuels: List<String> = emptyList(),
    val items: List<ForumPostSummary> = emptyList(),
    val loading: Boolean = true,
    val refreshing: Boolean = false,
    val loadingMore: Boolean = false,
    val nextCursor: String? = null,
    val error: ForumUiError? = null,
    val isOffline: Boolean = false,
) {
    val hasActiveFilters: Boolean
        get() = filters.isActive
}

enum class ForumUiError {
    DISABLED,
    OFFLINE,
    GENERIC,
}

@HiltViewModel
class ForumFeedViewModel @Inject constructor(
    private val getForumCategories: GetForumCategoriesUseCase,
    private val getForumFeed: GetForumFeedUseCase,
) : ViewModel() {

    private val _state = MutableStateFlow(
        ForumFeedUiState(
            availableUfs = SubmitForumTopicUseCase.VALID_UFS.keys.sorted(),
            availableFuels = SubmitForumTopicUseCase.VALID_FUELS.sorted(),
        ),
    )
    val state: StateFlow<ForumFeedUiState> = _state.asStateFlow()

    private var feedJob: Job? = null
    private var moreJob: Job? = null

    init {
        loadInitial()
    }

    fun loadInitial() {
        feedJob?.cancel()
        feedJob = viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            val categoryItems: List<ForumCategoryItem> = try {
                getForumCategories().items
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, error = ForumUiError.DISABLED) }
                return@launch
            } catch (e: ForumException.Offline) {
                emptyList()
            } catch (e: Exception) {
                emptyList()
            }

            try {
                val page = getForumFeed(category = _state.value.selectedCategory, kind = null, limit = 20, cursor = null, filters = _state.value.filters)
                _state.update {
                    it.copy(
                        categories = if (categoryItems.isNotEmpty()) categoryItems else it.categories,
                        items = page.items,
                        nextCursor = page.nextCursor,
                        loading = false,
                        refreshing = false,
                        error = null,
                        isOffline = false,
                    )
                }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumUiError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        categories = if (categoryItems.isNotEmpty()) categoryItems else it.categories,
                        loading = false,
                        refreshing = false,
                        isOffline = true,
                        error = if (it.items.isEmpty()) ForumUiError.OFFLINE else null,
                    )
                }
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        categories = if (categoryItems.isNotEmpty()) categoryItems else it.categories,
                        loading = false,
                        refreshing = false,
                        error = if (it.items.isEmpty()) ForumUiError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun selectCategory(category: ForumCategory?) {
        if (_state.value.selectedCategory == category && !_state.value.loading) return
        feedJob?.cancel()
        moreJob?.cancel()
        _state.update {
            it.copy(
                selectedCategory = category,
                items = emptyList(),
                nextCursor = null,
                loading = true,
                error = null,
                isOffline = false,
            )
        }
        feedJob = viewModelScope.launch {
            try {
                val page = getForumFeed(category = category, kind = null, limit = 20, cursor = null, filters = _state.value.filters)
                _state.update {
                    it.copy(
                        items = page.items,
                        nextCursor = page.nextCursor,
                        loading = false,
                        error = null,
                        isOffline = false,
                    )
                }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, error = ForumUiError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        loading = false,
                        isOffline = true,
                        error = if (it.items.isEmpty()) ForumUiError.OFFLINE else null,
                    )
                }
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        loading = false,
                        error = if (it.items.isEmpty()) ForumUiError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun updateFilters(filters: ForumFeedFilters) {
        if (_state.value.filters == filters && !_state.value.loading) return
        feedJob?.cancel()
        moreJob?.cancel()
        _state.update {
            it.copy(
                filters = filters,
                items = emptyList(),
                nextCursor = null,
                loading = true,
                error = null,
                isOffline = false,
            )
        }
        feedJob = viewModelScope.launch {
            try {
                val page = getForumFeed(
                    category = _state.value.selectedCategory,
                    kind = null,
                    limit = 20,
                    cursor = null,
                    filters = filters,
                )
                _state.update {
                    it.copy(
                        items = page.items,
                        nextCursor = page.nextCursor,
                        loading = false,
                        error = null,
                        isOffline = false,
                    )
                }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, error = ForumUiError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        loading = false,
                        isOffline = true,
                        error = if (it.items.isEmpty()) ForumUiError.OFFLINE else null,
                    )
                }
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        loading = false,
                        error = if (it.items.isEmpty()) ForumUiError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun clearFilters() {
        updateFilters(ForumFeedFilters())
    }

    fun refresh() {
        feedJob?.cancel()
        moreJob?.cancel()
        _state.update { it.copy(refreshing = true, error = null) }
        feedJob = viewModelScope.launch {
            val categoryItems: List<ForumCategoryItem> = try {
                getForumCategories().items
            } catch (_: Exception) {
                _state.value.categories
            }
            try {
                val page = getForumFeed(category = _state.value.selectedCategory, kind = null, limit = 20, cursor = null, filters = _state.value.filters)
                _state.update {
                    it.copy(
                        categories = if (categoryItems.isNotEmpty()) categoryItems else it.categories,
                        items = page.items,
                        nextCursor = page.nextCursor,
                        loading = false,
                        refreshing = false,
                        error = null,
                        isOffline = false,
                    )
                }
            } catch (e: ForumException.Disabled) {
                _state.update { it.copy(loading = false, refreshing = false, error = ForumUiError.DISABLED) }
            } catch (e: ForumException.Offline) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        isOffline = true,
                        error = if (it.items.isEmpty()) ForumUiError.OFFLINE else null,
                    )
                }
            } catch (e: Exception) {
                _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        error = if (it.items.isEmpty()) ForumUiError.GENERIC else null,
                    )
                }
            }
        }
    }

    fun loadMore() {
        val cursor = _state.value.nextCursor ?: return
        if (_state.value.loading || _state.value.loadingMore || _state.value.refreshing) return
        moreJob?.cancel()
        moreJob = viewModelScope.launch {
            _state.update { it.copy(loadingMore = true) }
            try {
                val page = getForumFeed(
                    category = _state.value.selectedCategory,
                    kind = null,
                    limit = 20,
                    cursor = cursor,
                    filters = _state.value.filters,
                )
                val merged = (_state.value.items + page.items).distinctBy { it.id }
                _state.update {
                    it.copy(
                        items = merged,
                        nextCursor = page.nextCursor,
                        loadingMore = false,
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(loadingMore = false) }
            }
        }
    }
}
