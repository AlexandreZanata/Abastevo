package com.anpfuel.data.remote.premium

import com.anpfuel.domain.premium.PremiumSource
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.premium.PremiumTier
import com.anpfuel.domain.premium.StoreProvider
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PremiumStatusHttpClientTest {

    private val active = """
        {"policy_version":1,"state":"ACTIVE","reason":"verified",
         "capabilities":["cloud.sync","vehicles.multiple"],
         "valid_until":"2026-11-30T23:59:59Z","retained_data_access":true,
         "payments_configured":false,
         "management_url":"https://play.google.com/store/account/subscriptions"}
    """.trimIndent()

    private fun client(server: MockWebServer) =
        PremiumStatusHttpClient(OkHttpClient(), server.url("/").toString())

    @Test fun `status sends only session proof and parses verified capabilities`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(active))
            val status = runBlocking { client(server).status("11111111-1111-4111-8111-111111111111", "token-1") }
            assertEquals(PremiumState.ACTIVE, status.state)
            assertTrue(status.canUsePaidCapabilities)
            assertTrue(status.hasCapability("vehicles.multiple"))
            assertEquals("2026-11-30T23:59:59Z", status.validUntil)
            assertEquals(PremiumStatus.MANAGEMENT_URL, status.managementUrl)
            val request = server.takeRequest(1, TimeUnit.SECONDS)!!
            assertEquals("no-store", request.getHeader("Cache-Control"))
            assertEquals("application/json; charset=utf-8", request.getHeader("Content-Type"))
            val body = request.body.readUtf8()
            assertTrue(body.contains("11111111-1111-4111-8111-111111111111"))
            assertTrue(body.contains("token-1"))
            assertFalse(body.contains("account_id"))
            assertFalse(body.contains("is_premium"))
        }
    }

    @Test fun `unknown state stays explicit and does not grant access`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(active.replace("\"ACTIVE\"", "\"FUTURE_STATE\"")))
            val status = runBlocking {
                client(server).status("11111111-1111-4111-8111-111111111111", "token-1")
            }
            assertEquals(PremiumState.UNKNOWN, status.state)
            assertFalse(status.canUsePaidCapabilities)
        }
    }

    @Test fun `null expiry and inactive state preserve free retained data`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(
                MockResponse().setBody(
                    active.replace("\"ACTIVE\"", "\"EXPIRED\"")
                        .replace("\"2026-11-30T23:59:59Z\"", "null")
                        .replace("\"cloud.sync\",\"vehicles.multiple\"", ""),
                ),
            )
            val status = runBlocking {
                client(server).status("11111111-1111-4111-8111-111111111111", "token-1")
            }
            assertEquals(PremiumState.EXPIRED, status.state)
            assertNull(status.validUntil)
            assertTrue(status.retainedDataAccess)
            assertFalse(status.canUsePaidCapabilities)
        }
    }

    @Test fun `refused and unavailable responses are typed without bodies`() = runTest {
        MockWebServer().use { server ->
            val api = client(server)
            server.enqueue(MockResponse().setResponseCode(401))
            assertThrows(PremiumStatusException.Denied::class.java) {
                runBlocking { api.status("11111111-1111-4111-8111-111111111111", "t") }
            }
            server.enqueue(MockResponse().setResponseCode(503))
            assertThrows(PremiumStatusException.Unavailable::class.java) {
                runBlocking { api.status("11111111-1111-4111-8111-111111111111", "t") }
            }
        }
    }

    @Test fun `corrupt oversized and foreign bodies fail closed`() = runTest {
        MockWebServer().use { server ->
            val api = client(server)
            val id = "11111111-1111-4111-8111-111111111111"
            server.enqueue(MockResponse().setBody(active.replace("\"retained_data_access\":true", "\"retained_data_access\":false")))
            assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
            server.enqueue(MockResponse().setBody(active.replace(MANAGEMENT, "https://evil.example")))
            assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
            server.enqueue(MockResponse().setBody(active.replace("\"cloud.sync\",\"vehicles.multiple\"", "\"invented.capability\"")))
            assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
            server.enqueue(MockResponse().setBody("x".repeat(64 * 1024 + 1)))
            assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
            server.enqueue(MockResponse().setBody("{"))
            assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
        }
    }

    @Test fun `store offers and tiers are parsed with their own validation`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(active.replace("\"management_url\"", "$OFFERS,$TIERS,\"management_url\"")))
            val status = runBlocking { client(server).status("11111111-1111-4111-8111-111111111111", "token-1") }
            val play = status.playOffer!!
            assertEquals(StoreProvider.PLAY, play.provider)
            assertEquals("a".repeat(64), play.accountBinding)
            assertEquals(listOf("monthly", "yearly"), play.products.single().planIds)
            assertEquals("$MANAGEMENT?package=com.anpfuel.app", play.managementUrl)
            assertEquals(2, status.storeOffers.size)
            assertEquals(
                listOf(
                    PremiumTier("PRO", PremiumSource.PLAY, "2026-11-30T23:59:59Z"),
                    PremiumTier("PRO", PremiumSource.ADMIN, null),
                    PremiumTier("PRO", PremiumSource.UNKNOWN, null),
                ),
                status.tiers,
            )
        }
    }

    @Test fun `absent offers keep the status readable without a purchase channel`() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody(active))
            val status = runBlocking { client(server).status("11111111-1111-4111-8111-111111111111", "token-1") }
            assertTrue(status.storeOffers.isEmpty())
            assertTrue(status.tiers.isEmpty())
            assertNull(status.playOffer)
        }
    }

    @Test fun `malformed or foreign offers are dropped and never become a purchase channel`() = runTest {
        val id = "11111111-1111-4111-8111-111111111111"
        val badOffers = listOf(
            OFFERS.replace("\"${"a".repeat(64)}\"", "\"SHORT\""),
            OFFERS.replace("https://play.google.com/store/account/subscriptions?package=com.anpfuel.app", "https://pay.example/checkout"),
            OFFERS.replace("\"provider\":\"play\"", "\"provider\":\"stripe\""),
            OFFERS.replace("\"plan_ids\":[\"monthly\",\"yearly\"]", "\"plan_ids\":\"monthly\""),
            OFFERS.replace("\"abastevo_premium\"", "7"),
            "\"store_offers\":{\"provider\":\"play\"}",
            "\"store_offers\":[\"play\"]",
        )
        MockWebServer().use { server ->
            val api = client(server)
            badOffers.forEach { offers ->
                server.enqueue(MockResponse().setBody(active.replace("\"management_url\"", "$offers,\"management_url\"")))
                val status = runBlocking { api.status(id, "t") }
                assertNull(status.playOffer, offers)
                assertEquals(PremiumState.ACTIVE, status.state)
            }
            val duplicated = OFFERS.replaceFirst("[{", "[$PLAY_OFFER,{")
            server.enqueue(MockResponse().setBody(active.replace("\"management_url\"", "$duplicated,\"management_url\"")))
            assertNull(runBlocking { api.status(id, "t") }.playOffer)
        }
    }

    @Test fun `corrupt tiers fail closed like other core fields`() = runTest {
        val id = "11111111-1111-4111-8111-111111111111"
        MockWebServer().use { server ->
            val api = client(server)
            listOf(
                "\"tiers\":{}",
                "\"tiers\":[{\"tier\":\"\",\"source\":\"web\",\"valid_until\":null}]",
                "\"tiers\":[{\"tier\":\"PRO\",\"source\":7,\"valid_until\":null}]",
                "\"tiers\":[{\"tier\":\"PRO\",\"source\":\"web\",\"valid_until\":\"tomorrow\"}]",
            ).forEach { tiers ->
                server.enqueue(MockResponse().setBody(active.replace("\"management_url\"", "$tiers,\"management_url\"")))
                assertThrows(PremiumStatusException.Invalid::class.java) { runBlocking { api.status(id, "t") } }
            }
        }
    }

    private companion object {
        const val MANAGEMENT = "https://play.google.com/store/account/subscriptions"
        val PLAY_OFFER = """{"provider":"play","products":[{"product_id":"abastevo_premium","plan_ids":["monthly","yearly"]}],""" +
            """"account_binding":"${"a".repeat(64)}",""" +
            """"management_url":"https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"}"""
        val OFFERS = """"store_offers":[$PLAY_OFFER,""" +
            """{"provider":"app_store","products":[{"product_id":"com.anpfuel.app.premium.monthly","plan_ids":[]}],""" +
            """"account_binding":"1a2b3c4d-1111-4111-8111-abcdefabcdef",""" +
            """"management_url":"https://apps.apple.com/account/subscriptions"}]"""
        const val TIERS = """"tiers":[{"tier":"PRO","source":"play","valid_until":"2026-11-30T23:59:59Z"},""" +
            """{"tier":"PRO","source":"admin","valid_until":null},{"tier":"PRO","source":"future","valid_until":null}]"""
    }
}
