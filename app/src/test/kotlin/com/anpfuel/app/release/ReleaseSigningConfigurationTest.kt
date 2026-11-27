package com.anpfuel.app.release

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Path
import kotlin.io.path.readText

class ReleaseSigningConfigurationTest {

    @Test
    fun signingPropertiesExampleExists() {
        val example = readProjectFile("signing.properties.example")

        assertTrue(example.contains("storeFile="), "signing.properties.example must define storeFile")
        assertTrue(example.contains("keyAlias="), "signing.properties.example must define keyAlias")
        assertTrue(example.contains("CHANGE_ME") || example.contains("anpfuel-dev-keystore"))
    }

    @Test
    fun appBuildGradleWiresReleaseSigning() {
        val buildGradle = readProjectFile("app/build.gradle.kts")

        assertTrue(buildGradle.contains("signingConfigs"), "app/build.gradle.kts must define signingConfigs")
        assertTrue(buildGradle.contains("signing.properties"), "app/build.gradle.kts must read signing.properties")
        assertTrue(buildGradle.contains("bundleRelease") || buildGradle.contains("assembleRelease"))
        assertTrue(buildGradle.contains("ANPFUEL_KEYSTORE_TYPE"), "app/build.gradle.kts must allow overriding keystore type")
    }

    @Test
    fun appBuildGradleSupportsPlayStoreDistributionId() {
        val buildGradle = readProjectFile("app/build.gradle.kts")

        assertTrue(buildGradle.contains("androidDistribution"), "app/build.gradle.kts must read androidDistribution")
        assertTrue(buildGradle.contains("\"play\" -> \"com.abastevo\""), "play distribution must map to com.abastevo")
        assertTrue(buildGradle.contains("\"com.anpfuel.app\""), "staging distribution must retain com.anpfuel.app")
    }

    @Test
    fun releaseBuildValidationScriptExists() {
        val script = readProjectFile("scripts/validate-release-build.sh")

        assertTrue(script.contains("assembleRelease"), "validate-release-build.sh must build APK")
        assertTrue(script.contains("bundleRelease"), "validate-release-build.sh must build AAB")
        assertTrue(script.contains("apksigner"), "validate-release-build.sh must verify APK signature")
    }

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        val fromAppModule = moduleRoot.resolve(relativePath)
        if (fromAppModule.toFile().exists()) {
            return fromAppModule.readText()
        }
        return moduleRoot.parent.resolve(relativePath).readText()
    }
}
