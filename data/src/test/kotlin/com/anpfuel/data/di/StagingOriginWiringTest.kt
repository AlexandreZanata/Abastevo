package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import dagger.Provides
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P36-T01 staging account/provider wiring guard.
 *
 * Every backend HTTP client binding must resolve its base URL from the
 * single shared [ApiEnvironment] origin (staging during P34–P38
 * construction), never from a per-module constant. The construction
 * origin itself must be the explicit staging selection; release stays
 * separate and separately certified. Provider credentials (Google/Apple
 * client IDs, secrets, `google-services.json`) must not exist in Git —
 * enforced by `scan-secrets.sh` plus the provider checklist in the task
 * evidence, not by this unit suite.
 */
class StagingOriginWiringTest {

    private val backendModules = listOf(
        AuthModule,
        AnonymousModule,
        CommunityModule,
        ContributionModule,
        FeedbackModule,
        DirectoryModule,
        IntakeModule,
    )

    @Test
    fun `construction origin is the explicit staging selection`() {
        assertEquals(ApiEnvironment.STAGING, NetworkModule.provideApiEnvironment())
        assertTrue(ApiEnvironment.STAGING.origin.startsWith("https://"))
    }

    @Test
    fun `every backend client binding takes the shared origin`() {
        val offenders = mutableListOf<String>()
        for (module in backendModules) {
            val methods = module.javaClass.declaredMethods
                .filter { it.isAnnotationPresent(Provides::class.java) }
                .filter { method ->
                    val name = method.returnType.simpleName
                    (name.contains("Client") || name.contains("Api") || name.contains("Gateway")) &&
                        name != "OkHttpClient"
                }
            assertTrue(methods.isNotEmpty(), "${module.javaClass.simpleName} has no client bindings")
            for (method in methods) {
                val takesOrigin = method.parameterTypes.any { it == ApiEnvironment::class.java }
                if (!takesOrigin) {
                    offenders += "${module.javaClass.simpleName}.${method.name}"
                }
            }
        }
        assertTrue(offenders.isEmpty(), "bindings missing shared origin: $offenders")
    }

    @Test
    fun `preview placeholder never resolves`() {
        // RFC 2606 `.invalid` must stay non-resolving reference values only.
        for (placeholder in listOf(
            AuthModule.PREVIEW_BASE_URL,
            AnonymousModule.PREVIEW_BASE_URL,
            CommunityModule.PREVIEW_BASE_URL,
            ContributionModule.PREVIEW_BASE_URL,
            ApiEnvironment.PREVIEW.origin,
        )) {
            assertTrue(placeholder.endsWith(".invalid"), "placeholder resolves: $placeholder")
        }
    }
}
