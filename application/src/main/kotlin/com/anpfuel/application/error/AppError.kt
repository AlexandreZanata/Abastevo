package com.anpfuel.application.error

/**
 * Structured application errors mapped to i18n keys in `:app` (see user-business-logic.md).
 */
sealed class AppError {

    abstract val code: String
    abstract val i18nKey: String

    open val isInformational: Boolean get() = false

    data object SyncNetworkError : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY

        const val CODE = "SYNC_NETWORK_ERROR"
        const val I18N_KEY = "error_sync_network"
    }

    data object SyncParseError : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY

        const val CODE = "SYNC_PARSE_ERROR"
        const val I18N_KEY = "error_sync_parse"
    }

    data object SyncNoNewData : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY
        override val isInformational: Boolean = true

        const val CODE = "SYNC_NO_NEW_DATA"
        const val I18N_KEY = "info_sync_up_to_date"
    }

    data object SearchNoResults : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY

        const val CODE = "SEARCH_NO_RESULTS"
        const val I18N_KEY = "error_search_no_results"
    }

    data object StationDetailNotSynced : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY

        const val CODE = "STATION_DETAIL_NOT_SYNCED"
        const val I18N_KEY = "error_station_detail_missing"
    }

    data object StorageFull : AppError() {
        override val code: String = CODE
        override val i18nKey: String = I18N_KEY

        const val CODE = "STORAGE_FULL"
        const val I18N_KEY = "error_storage_full"
    }
}
