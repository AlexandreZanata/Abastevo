package com.anpfuel.app.ui.stations

/**
 * P32-T02 — Private claim-status presentation.
 */
enum class ClaimStatus {
    DRAFT,
    SUBMITTED,
    NEEDS_INFO,
    UNDER_REVIEW,
    APPEALABLE,
}

data class ClaimStatusUi(val label: String)

object ClaimStatusUiMapper {

    fun toUi(status: ClaimStatus): ClaimStatusUi = when (status) {
        ClaimStatus.DRAFT -> ClaimStatusUi(label = "Rascunho privado")
        ClaimStatus.SUBMITTED -> ClaimStatusUi(label = "Enviado · aguardando análise")
        ClaimStatus.NEEDS_INFO -> ClaimStatusUi(label = "Ação necessária: enviar documentos")
        ClaimStatus.UNDER_REVIEW -> ClaimStatusUi(label = "Em análise")
        ClaimStatus.APPEALABLE -> ClaimStatusUi(label = "Recurso disponível")
    }
}
