package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumTopicReceipt
import com.anpfuel.domain.repository.ForumGateway
import java.util.UUID

class SubmitForumTopicUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        title: String,
        bodyText: String,
        category: ForumCategory,
        operationId: String = UUID.randomUUID().toString(),
        region: ForumRegion? = null,
        fuelProduct: String? = null,
        stationId: String? = null,
        publicAlias: String? = null,
    ): ForumTopicReceipt {
        val trimmedTitle = title.trim()
        if (trimmedTitle.isEmpty()) {
            throw ForumException.Invalid("forum.topic-title-empty")
        }
        if (trimmedTitle.length > 120) {
            throw ForumException.Invalid("forum.topic-title-too-long")
        }

        val trimmedBody = bodyText.trim()
        if (trimmedBody.isEmpty()) {
            throw ForumException.Invalid("forum.topic-body-empty")
        }
        if (trimmedBody.length > 4000) {
            throw ForumException.Invalid("forum.topic-body-too-long")
        }

        if (category == ForumCategory.UNKNOWN) {
            throw ForumException.Invalid("forum.topic-category-invalid")
        }

        if (!UUID_REGEX.matches(operationId.trim())) {
            throw ForumException.Invalid("forum.bad-operation-id")
        }

        val validatedRegion = validateRegion(region)
        val validatedFuel = validateFuelProduct(fuelProduct)
        val validatedStationId = validateStationId(stationId)
        val trimmedAlias = publicAlias?.trim()?.takeIf { it.isNotEmpty() }

        return gateway.createTopic(
            title = trimmedTitle,
            bodyText = trimmedBody,
            category = category,
            operationId = operationId.trim(),
            region = validatedRegion,
            fuelProduct = validatedFuel,
            stationId = validatedStationId,
            publicAlias = trimmedAlias,
        )
    }

    companion object {
        val UUID_REGEX = Regex(
            "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
        )

        val VALID_UFS: Map<String, Int> = mapOf(
            "RO" to 11, "AC" to 12, "AM" to 13, "RR" to 14, "PA" to 15, "AP" to 16, "TO" to 17,
            "MA" to 21, "PI" to 22, "CE" to 23, "RN" to 24, "PB" to 25, "PE" to 26, "AL" to 27, "SE" to 28, "BA" to 29,
            "MG" to 31, "ES" to 32, "RJ" to 33, "SP" to 35,
            "PR" to 41, "SC" to 42, "RS" to 43,
            "MS" to 50, "MT" to 51, "GO" to 52, "DF" to 53,
        )

        val VALID_FUELS = setOf(
            "GASOLINE_REGULAR",
            "GASOLINE_PREMIUM",
            "ETHANOL",
            "DIESEL_S10",
            "DIESEL_S500",
            "GLP",
            "GNV",
        )

        fun validateRegion(region: ForumRegion?): ForumRegion? {
            if (region == null) return null
            val uf = region.uf?.trim()?.uppercase()?.takeIf { it.isNotEmpty() }
            val ibge = region.municipalityIbge

            if (uf == null && ibge == null) return null

            var expectedPrefix = 0
            if (uf != null) {
                val prefix = VALID_UFS[uf] ?: throw ForumException.Invalid("forum.bad-region-uf")
                expectedPrefix = prefix
            }

            if (ibge != null) {
                if (ibge !in 1000000..9999999) {
                    throw ForumException.Invalid("forum.bad-region-ibge")
                }
                if (expectedPrefix > 0) {
                    val actualPrefix = ibge / 100000
                    if (actualPrefix != expectedPrefix) {
                        throw ForumException.Invalid("forum.bad-region-prefix")
                    }
                }
            }

            return ForumRegion(uf = uf, municipalityIbge = ibge)
        }

        fun validateFuelProduct(fuel: String?): String? {
            val trimmed = fuel?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            if (!VALID_FUELS.contains(trimmed)) {
                throw ForumException.Invalid("forum.bad-fuel-product")
            }
            return trimmed
        }

        fun validateStationId(stationId: String?): String? {
            val trimmed = stationId?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            if (!UUID_REGEX.matches(trimmed)) {
                throw ForumException.Invalid("forum.bad-station-id")
            }
            return trimmed
        }
    }
}
