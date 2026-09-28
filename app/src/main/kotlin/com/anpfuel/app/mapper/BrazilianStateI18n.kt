package com.anpfuel.app.mapper

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.BrazilianState

object BrazilianStateI18n {

    @StringRes
    fun toStringRes(state: BrazilianState): Int = when (state) {
        BrazilianState.ACRE -> R.string.state_name_acre
        BrazilianState.ALAGOAS -> R.string.state_name_alagoas
        BrazilianState.AMAPA -> R.string.state_name_amapa
        BrazilianState.AMAZONAS -> R.string.state_name_amazonas
        BrazilianState.BAHIA -> R.string.state_name_bahia
        BrazilianState.CEARA -> R.string.state_name_ceara
        BrazilianState.DISTRICT_FEDERAL -> R.string.state_name_distrito_federal
        BrazilianState.ESPIRITO_SANTO -> R.string.state_name_espirito_santo
        BrazilianState.GOIAS -> R.string.state_name_goias
        BrazilianState.MARANHAO -> R.string.state_name_maranhao
        BrazilianState.MATO_GROSSO -> R.string.state_name_mato_grosso
        BrazilianState.MATO_GROSSO_DO_SUL -> R.string.state_name_mato_grosso_do_sul
        BrazilianState.MINAS_GERAIS -> R.string.state_name_minas_gerais
        BrazilianState.PARA -> R.string.state_name_para
        BrazilianState.PARAIBA -> R.string.state_name_paraiba
        BrazilianState.PARANA -> R.string.state_name_parana
        BrazilianState.PERNAMBUCO -> R.string.state_name_pernambuco
        BrazilianState.PIAUI -> R.string.state_name_piaui
        BrazilianState.RIO_DE_JANEIRO -> R.string.state_name_rio_de_janeiro
        BrazilianState.RIO_GRANDE_DO_NORTE -> R.string.state_name_rio_grande_do_norte
        BrazilianState.RIO_GRANDE_DO_SUL -> R.string.state_name_rio_grande_do_sul
        BrazilianState.RONDONIA -> R.string.state_name_rondonia
        BrazilianState.RORAIMA -> R.string.state_name_roraima
        BrazilianState.SANTA_CATARINA -> R.string.state_name_santa_catarina
        BrazilianState.SAO_PAULO -> R.string.state_name_sao_paulo
        BrazilianState.SERGIPE -> R.string.state_name_sergipe
        BrazilianState.TOCANTINS -> R.string.state_name_tocantins
    }
}
