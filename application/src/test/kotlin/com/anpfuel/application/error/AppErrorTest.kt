package com.anpfuel.application.error

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.MethodSource

class AppErrorTest {

    @ParameterizedTest
    @MethodSource("documentedErrors")
    fun documentedErrorCodesMatchUserBusinessLogic(error: AppError) {
        assertFalse(error.code.isBlank())
        assertFalse(error.i18nKey.isBlank())
    }

    @Test
    fun syncNoNewDataIsInformational() {
        assertTrue(AppError.SyncNoNewData.isInformational)
    }

    @Test
    fun syncFailuresAreNotInformational() {
        assertFalse(AppError.SyncNetworkError.isInformational)
        assertFalse(AppError.SyncParseError.isInformational)
    }

    companion object {
        @JvmStatic
        fun documentedErrors(): List<AppError> = listOf(
            AppError.SyncNetworkError,
            AppError.SyncParseError,
            AppError.SyncNoNewData,
            AppError.SearchNoResults,
            AppError.StationDetailNotSynced,
            AppError.StorageFull,
        )
    }
}
