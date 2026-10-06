package com.anpfuel.application.usecase.intake

import com.anpfuel.domain.repository.IntakeReceipt
import com.anpfuel.domain.repository.IntakeStatus
import com.anpfuel.domain.repository.StationIntakeGateway
import io.mockk.every
import io.mockk.mockk
import java.io.IOException
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Test

class StationIntakeUseCasesTest {

    private val gateway: StationIntakeGateway = mockk()

    @Test
    fun `submit maps outcomes honestly`() {
        val useCase = SubmitStationSuggestionUseCase(gateway)
        assertInstanceOf(
            SubmitSuggestionOutcome.SessionInvalid::class.java,
            useCase.invoke("", "tok", "key", "{}"),
        )
        assertInstanceOf(
            SubmitSuggestionOutcome.Invalid::class.java,
            useCase.invoke("fam", "tok", "", "{}"),
        )
        every { gateway.submit(any(), any(), any(), any()) } returns IntakeReceipt("sug-1")
        assertInstanceOf(
            SubmitSuggestionOutcome.Submitted::class.java,
            useCase.invoke("fam", "tok", "key", """{"display_name": "X"}"""),
        )
        every { gateway.submit(any(), any(), any(), any()) } throws IOException("intake call failed: HTTP 401 /v1/stations/suggestions")
        assertInstanceOf(
            SubmitSuggestionOutcome.SessionInvalid::class.java,
            useCase.invoke("fam", "tok", "key", """{}"""),
        )
        every { gateway.submit(any(), any(), any(), any()) } throws IOException("intake call failed: HTTP 409 /v1/stations/suggestions")
        assertInstanceOf(
            SubmitSuggestionOutcome.Conflict::class.java,
            useCase.invoke("fam", "tok", "key", """{}"""),
        )
        every { gateway.submit(any(), any(), any(), any()) } throws IOException("timeout")
        assertInstanceOf(
            SubmitSuggestionOutcome.Unavailable::class.java,
            useCase.invoke("fam", "tok", "key", """{}"""),
        )
    }

    @Test
    fun `status maps found missing session and transport`() {
        val useCase = GetSuggestionStatusUseCase(gateway)
        every { gateway.status(any(), any(), any()) } returns IntakeStatus("s", "approved")
        val found = useCase.invoke("fam", "tok", "s")
        assertInstanceOf(SuggestionStatusOutcome.Found::class.java, found)
        every { gateway.status(any(), any(), any()) } throws IOException("intake call failed: HTTP 404 /v1/stations/suggestions/s/status")
        assertInstanceOf(
            SuggestionStatusOutcome.NotFound::class.java,
            useCase.invoke("fam", "tok", "s"),
        )
    }

    @Test
    fun `cancel maps closed missing and session`() {
        val useCase = CancelOwnedSuggestionUseCase(gateway)
        every { gateway.cancel(any(), any(), any()) } returns Unit
        assertInstanceOf(
            CancelSuggestionOutcome.Cancelled::class.java,
            useCase.invoke("fam", "tok", "s"),
        )
        every { gateway.cancel(any(), any(), any()) } throws IOException("intake call failed: HTTP 409 /v1/stations/suggestions/s/cancel")
        assertInstanceOf(
            CancelSuggestionOutcome.Closed::class.java,
            useCase.invoke("fam", "tok", "s"),
        )
        assertInstanceOf(
            CancelSuggestionOutcome.SessionInvalid::class.java,
            useCase.invoke("", "tok", "s"),
        )
    }
}
