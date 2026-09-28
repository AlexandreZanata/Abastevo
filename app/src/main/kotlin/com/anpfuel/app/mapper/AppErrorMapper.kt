package com.anpfuel.app.mapper

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.application.error.AppError

object AppErrorMapper {

    @StringRes
    fun toStringRes(error: AppError): Int = when (error) {
        AppError.SyncNetworkError -> R.string.error_sync_network
        AppError.SyncParseError -> R.string.error_sync_parse
        AppError.SyncNoNewData -> R.string.info_sync_up_to_date
        AppError.SearchNoResults -> R.string.error_search_no_results
        AppError.StationDetailNotSynced -> R.string.error_station_detail_missing
        AppError.StorageFull -> R.string.error_storage_full
    }
}
