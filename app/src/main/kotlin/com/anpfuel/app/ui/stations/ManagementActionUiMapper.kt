package com.anpfuel.app.ui.stations

import com.anpfuel.domain.profile.ManagementAction

/**
 * P32-T03 — Management action presentation with review limitations.
 */
data class ManagementActionUi(
    val label: String,
    val limitation: String,
)

object ManagementActionUiMapper {

    fun toUi(action: ManagementAction): ManagementActionUi = when (action) {
        ManagementAction.EditBusiness -> ManagementActionUi(
            label = "Editar dados do negócio",
            limitation = "Sujeito a análise independente",
        )
        ManagementAction.ReplyReview -> ManagementActionUi(
            label = "Responder avaliação oficial",
            limitation = "Limite de 280 caracteres",
        )
        ManagementAction.InviteManager -> ManagementActionUi(
            label = "Convidar responsável",
            limitation = "Sujeito a análise independente",
        )
        ManagementAction.ContestAccess -> ManagementActionUi(
            label = "Contestar acesso",
            limitation = "Decisão do servidor",
        )
        ManagementAction.RequestReverification -> ManagementActionUi(
            label = "Solicitar nova verificação",
            limitation = "Decisão do servidor",
        )
    }
}
