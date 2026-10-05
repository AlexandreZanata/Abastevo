package com.anpfuel.app.capture

import android.content.Context
import com.anpfuel.application.port.CaptureOcrFlagProvider
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.usecase.capture.ConfirmPriceCaptureUseCase
import com.anpfuel.domain.portable.PortablePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T04: ViewModel gates on flag/permission/cancel and never
 * auto-confirms or uploads.
 */
class CaptureOcrViewModelTest {

    private fun viewModel(
        enabled: Boolean,
        hasPermission: Boolean,
        ocrText: (String) -> List<PortablePriceOcr.OcrCandidate> =
            PortablePriceOcr::parseCandidates,
    ): CaptureOcrViewModel {
        val flags = object : CaptureOcrFlagProvider {
            override fun isEnabled(): Boolean = enabled
        }
        val ocr = object : OcrPort {
            override fun candidatesFromText(text: String) = ocrText(text)
        }
        val useCase = ConfirmPriceCaptureUseCase(flags, ocr)
        val context = mockk<Context>(relaxed = true)
        val permissions = CameraPermissionHandler(context)
        val handler = mockk<CameraPermissionHandler>()
        every { handler.hasCameraPermission() } returns hasPermission
        return CaptureOcrViewModel(useCase, handler, flags)
    }

    @Test
    fun `denied permission surfaces PermissionDenied`() {
        val vm = viewModel(enabled = true, hasPermission = false)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.PermissionDenied)
    }

    @Test
    fun `cancelled capture surfaces Cancelled`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = true, ocrText = "R$ 5,89")
        assertTrue(vm.state.value is CaptureOcrUiState.Cancelled)
    }

    @Test
    fun `multi-price capture needs human choice and confirms explicitly`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "R$ 6,19\nR$ 5,89")
        val pending = vm.state.value
        assertTrue(pending is CaptureOcrUiState.NeedsConfirmation)
        assertTrue((pending as CaptureOcrUiState.NeedsConfirmation).candidates.size == 2)

        vm.onConfirm(pending.candidates.first(), null, "STANDARD", true)
        assertTrue(vm.state.value is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirm(pending.candidates[1], FuelProduct.GASOLINE_REGULAR, null, true)
        assertTrue(vm.state.value is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirm(pending.candidates[1], FuelProduct.GASOLINE_REGULAR, "STANDARD", true)
        val done = vm.state.value
        assertTrue(done is CaptureOcrUiState.Confirmed)
        assertTrue((done as CaptureOcrUiState.Confirmed).candidate.priceMilli == 5890L)
        assertTrue(done.conditionKind == "STANDARD")
    }

    @Test
    fun `manual price confirms and invalid input keeps candidates`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.onCaptureResult(cancelled = false, ocrText = "5,89")
        val pending = vm.state.value
        assertTrue(pending is CaptureOcrUiState.NeedsConfirmation)

        vm.onConfirmManual("nove", FuelProduct.ETHANOL, "APP", true)
        val kept = vm.state.value
        assertTrue(kept is CaptureOcrUiState.NeedsConfirmation)
        assertTrue((kept as CaptureOcrUiState.NeedsConfirmation).candidates.size == 1)

        vm.onConfirmManual("5,89", FuelProduct.ETHANOL, "APP", true)
        val done = vm.state.value
        assertTrue(done is CaptureOcrUiState.Confirmed)
        val confirmed = done as CaptureOcrUiState.Confirmed
        assertTrue(confirmed.candidate.priceMilli == 5890L)
        assertTrue(confirmed.candidate.manualEntry)
        assertTrue(confirmed.product == FuelProduct.ETHANOL)
        assertTrue(confirmed.conditionKind == "APP")
    }

    @Test
    fun `valid target binds and legacy cnpj is invalid`() {
        val vm = viewModel(enabled = true, hasPermission = true)
        vm.bindTarget("d6c74c23-63db-4c24-a2e5-408cb23bad26", "GASOLINE_REGULAR")
        assertTrue(vm.target.value?.stationId == "d6c74c23-63db-4c24-a2e5-408cb23bad26")
        assertTrue(!vm.targetInvalid.value)

        vm.bindTarget("04218406000104", "GASOLINE_REGULAR")
        assertTrue(vm.target.value == null)
        assertTrue(vm.targetInvalid.value)

        vm.bindTarget(null, null)
        assertTrue(vm.target.value == null)
        assertTrue(!vm.targetInvalid.value)
    }
}
