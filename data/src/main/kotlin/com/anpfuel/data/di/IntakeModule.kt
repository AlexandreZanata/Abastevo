package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.StationIntakeHttpClient
import com.anpfuel.domain.repository.IntakeReceipt
import com.anpfuel.domain.repository.IntakeStatus
import com.anpfuel.domain.repository.IntakeSummary
import com.anpfuel.domain.repository.StationIntakeGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton
import org.json.JSONArray
import org.json.JSONObject

/**
 * P27-T04 intake bindings (origin shared from P34-T02).
 */
@Module
@InstallIn(SingletonComponent::class)
object IntakeModule {

    @Provides
    @Singleton
    fun provideStationIntakeGateway(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): StationIntakeGateway =
        StationIntakeGatewayImpl(
            StationIntakeHttpClient(
                client = OkHttpClientFactory.create(),
                baseUrl = environment.origin,
            ),
        )
}

private class StationIntakeGatewayImpl(
    private val client: StationIntakeHttpClient,
) : StationIntakeGateway {

    override fun submit(
        familyId: String,
        accessToken: String,
        clientSubmissionId: String,
        proposalJson: String,
    ): IntakeReceipt {
        val raw = client.submit(familyId, accessToken, clientSubmissionId, proposalJson)
        val id = JSONObject(raw).optString("id", "")
        if (id.isBlank()) throw java.io.IOException("intake submit failed: missing id")
        return IntakeReceipt(id)
    }

    override fun mine(familyId: String, accessToken: String): List<IntakeSummary> {
        val raw = client.mine(familyId, accessToken)
        val items = JSONObject(raw).optJSONArray("items") ?: JSONArray()
        return (0 until items.length()).map { index ->
            val item = items.getJSONObject(index)
            IntakeSummary(
                id = item.optString("id", ""),
                state = item.optString("state", "pending"),
            )
        }
    }

    override fun status(
        familyId: String,
        accessToken: String,
        id: String,
    ): IntakeStatus {
        val raw = client.status(familyId, accessToken, id)
        val doc = JSONObject(raw)
        return IntakeStatus(
            id = doc.optString("id", id),
            state = doc.optString("state", "pending"),
        )
    }

    override fun cancel(
        familyId: String,
        accessToken: String,
        id: String,
    ) {
        client.cancel(familyId, accessToken, id)
    }
}
