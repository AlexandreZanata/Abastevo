package com.anpfuel.application.error

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException

class AppErrorResolverTest {

    @Test
    fun mapsIOExceptionToSyncNetworkError() {
        assertEquals(
            AppError.SyncNetworkError,
            AppErrorResolver.fromThrowable(IOException("connection reset")),
        )
    }

    @Test
    fun mapsUnknownHostToSyncNetworkError() {
        assertEquals(
            AppError.SyncNetworkError,
            AppErrorResolver.fromThrowable(UnknownHostException("gov.br")),
        )
    }

    @Test
    fun mapsSocketTimeoutToSyncNetworkError() {
        assertEquals(
            AppError.SyncNetworkError,
            AppErrorResolver.fromThrowable(SocketTimeoutException("timeout")),
        )
    }

    @Test
    fun mapsParseFailuresToSyncParseError() {
        assertEquals(
            AppError.SyncParseError,
            AppErrorResolver.fromThrowable(IllegalStateException("Invalid XLSX sheet layout")),
        )
    }

    @Test
    fun mapsBr001DomainExceptionToSyncParseError() {
        val error = AppErrorResolver.fromDomainException(
            DomainException("BR-001: SurveyWeek range invalid in filename"),
        )

        assertEquals(AppError.SyncParseError, error)
    }

    @Test
    fun mapsBr002DomainExceptionToSyncParseError() {
        val error = AppErrorResolver.fromDomainException(
            DomainException("BR-002: Unknown ANP fuel product label"),
        )

        assertEquals(AppError.SyncParseError, error)
    }

    @Test
    fun returnsNullForUnmappedDomainException() {
        assertNull(
            AppErrorResolver.fromDomainException(
                DomainException("BR-015: Cannot start sync while another job is active"),
            ),
        )
    }

    @Test
    fun unwrapsCauseChainBeforeMapping() {
        val wrapped = IOException("download failed", SocketTimeoutException("read timed out"))

        assertEquals(AppError.SyncNetworkError, AppErrorResolver.fromThrowable(wrapped))
    }

    @Test
    fun mapsStorageFullMessageToStorageFull() {
        assertEquals(
            AppError.StorageFull,
            AppErrorResolver.fromInfrastructureThrowable(
                IOException("SQLITE_FULL: database or disk is full"),
            ),
        )
    }

    @Test
    fun mapsNoSpaceLeftMessageToStorageFull() {
        assertEquals(
            AppError.StorageFull,
            AppErrorResolver.fromInfrastructureThrowable(
                IOException("no space left on device"),
            ),
        )
    }
}
