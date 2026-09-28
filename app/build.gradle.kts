import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.hilt)
    alias(libs.plugins.ksp)
}

fun loadSigningProperties(): Properties? {
    val signingFile = rootProject.file("signing.properties")
    if (!signingFile.exists()) {
        return null
    }
    return Properties().apply {
        signingFile.inputStream().use { load(it) }
    }
}

fun signingProperty(name: String, envVar: String): String? {
    val fromFile = loadSigningProperties()?.getProperty(name)?.takeIf { it.isNotBlank() }
    if (fromFile != null) {
        return fromFile
    }
    return System.getenv(envVar)?.takeIf { it.isNotBlank() }
}

fun hasReleaseSigningConfig(): Boolean {
    val storeFilePath = signingProperty("storeFile", "ANPFUEL_KEYSTORE_PATH")
    return storeFilePath != null
}

android {
    namespace = "com.anpfuel.app"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        applicationId = "com.anpfuel.app"
        minSdk = libs.versions.minSdk.get().toInt()
        targetSdk = libs.versions.targetSdk.get().toInt()
        versionCode = 4
        versionName = "3.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        testInstrumentationRunnerArguments["clearPackageData"] = "true"
    }

    testOptions {
        execution = "ANDROIDX_TEST_ORCHESTRATOR"
    }

    signingConfigs {
        create("release") {
            val storeFilePath = signingProperty("storeFile", "ANPFUEL_KEYSTORE_PATH")
                ?: return@create
            storeFile = rootProject.file(storeFilePath)
            storePassword = signingProperty("storePassword", "ANPFUEL_KEYSTORE_PASSWORD")
            keyAlias = signingProperty("keyAlias", "ANPFUEL_KEY_ALIAS")
            keyPassword = signingProperty("keyPassword", "ANPFUEL_KEY_PASSWORD")
        }
    }

    buildTypes {
        debug {
            // Sideloaded release builds signed with the release keystore cannot be
            // upgraded in place by a debug APK (INSTALL_FAILED_UPDATE_INCOMPATIBLE).
            // The suffixed application id lets the debug build coexist with them.
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            if (hasReleaseSigningConfig()) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    packaging {
        resources {
            excludes += setOf(
                "META-INF/LICENSE.md",
                "META-INF/LICENSE-notice.md",
            )
        }
    }
}

dependencies {
    implementation(project(":domain"))
    implementation(project(":application"))
    implementation(project(":data"))

    implementation(platform(libs.compose.bom))
    implementation(libs.bundles.compose)
    implementation(libs.activity.compose)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.lifecycle.viewmodel.compose)
    implementation(libs.navigation.compose)
    implementation(libs.hilt.navigation.compose)
    implementation(libs.compose.material.icons.extended)
    implementation(libs.compose.foundation)

    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)

    testImplementation(libs.junit.jupiter)
    testImplementation(libs.mockk)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.turbine)

    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.compose.material3)
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test:rules:1.6.1")
    androidTestImplementation(libs.kotlinx.coroutines.android)
    androidTestImplementation(libs.room.runtime)
    androidTestImplementation(libs.room.ktx)

    androidTestUtil("androidx.test:orchestrator:1.5.1")

    implementation(libs.hilt.android)
    ksp(libs.hilt.compiler)
    implementation(libs.hilt.work)
    ksp(libs.hilt.work.compiler)
    implementation(libs.work.runtime.ktx)
}

tasks.withType<Test> {
    useJUnitPlatform()
}

private val maxReleaseApkBytes = 15L * 1024 * 1024

tasks.register("verifyReleaseApkSize") {
    group = "verification"
    description = "Builds release APK and fails when uncompressed size exceeds 15 MB (Phase 9.2.2)"
    dependsOn("assembleRelease")
    doLast {
        val apkDir = layout.buildDirectory.dir("outputs/apk/release").get().asFile
        val apk = apkDir.listFiles()
            ?.firstOrNull { it.isFile && it.extension == "apk" }
            ?: throw GradleException("Release APK not found in ${apkDir.absolutePath}")
        val sizeBytes = apk.length()
        val sizeMb = sizeBytes / (1024.0 * 1024.0)
        logger.lifecycle("Release APK: ${apk.name} — ${"%.2f".format(sizeMb)} MB ($sizeBytes bytes)")
        if (sizeBytes > maxReleaseApkBytes) {
            throw GradleException(
                "Release APK exceeds 15 MB limit: ${"%.2f".format(sizeMb)} MB",
            )
        }
    }
}
