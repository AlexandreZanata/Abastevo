plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.hilt)
    alias(libs.plugins.ksp)
}

import java.security.MessageDigest
import java.util.Properties

val localProps = Properties().apply {
    val lp = rootProject.file("local.properties")
    if (lp.isFile) {
        lp.inputStream().use { load(it) }
    }
}
val pinnedCandidateRevision = "d2ec37881ede4e22"
val bundledCandidate = rootProject.layout.buildDirectory.dir("comaps/$pinnedCandidateRevision").get().asFile
val configuredSdkDir = providers.gradleProperty("comapsSdkDir").orNull
    ?: localProps.getProperty("comaps.sdk.dir")
    ?: localProps.getProperty("comapsSdkDir")
    ?: providers.environmentVariable("COMAPS_SDK_DIR").orNull
if (configuredSdkDir == null) {
    providers.exec {
        commandLine("python3", rootProject.file("scripts/maps/provision-comaps-aar.py"),
            "--aar", rootProject.file("maps/engine/vendor/sdk-release.aar"),
            "--output", bundledCandidate)
    }.result.get().assertNormalExitValue()
}
val defaultCandidate = bundledCandidate.absolutePath
val comapsSdkDir = (
    providers.gradleProperty("comapsSdkDir").orNull
        ?: localProps.getProperty("comaps.sdk.dir")
        ?: localProps.getProperty("comapsSdkDir")
        ?: providers.environmentVariable("COMAPS_SDK_DIR").orNull
        ?: defaultCandidate
    ).trim()

val sdkRoot = file(comapsSdkDir)
if (!sdkRoot.isDirectory) {
    error(
        """
        CoMaps Android SDK candidate ($pinnedCandidateRevision) was not found at: $comapsSdkDir
        Provision the verified MAP-14 candidate unpack directory and configure one of:
          1. -PcomapsSdkDir=/path/to/abastevo-map-engine/$pinnedCandidateRevision
          2. comaps.sdk.dir=/path/to/abastevo-map-engine/$pinnedCandidateRevision in local.properties
          3. COMAPS_SDK_DIR=/path/to/abastevo-map-engine/$pinnedCandidateRevision environment variable
        See docs/backend/evidence/maps/MAP-14.md for candidate provenance and SHA-256 digests.
        """.trimIndent()
    )
}

fun verifyCandidateSha256(relativePath: String, expectedSha256: String) {
    val target = sdkRoot.resolve(relativePath)
    if (!target.isFile) {
        error("CoMaps SDK candidate is incomplete: missing $relativePath under $comapsSdkDir (see docs/backend/evidence/maps/MAP-14.md)")
    }
    val digest = MessageDigest.getInstance("SHA-256")
    target.inputStream().use { input ->
        val buf = ByteArray(64 * 1024)
        while (true) {
            val read = input.read(buf)
            if (read <= 0) break
            digest.update(buf, 0, read)
        }
    }
    val actual = digest.digest().joinToString("") { "%02x".format(it) }
    if (!actual.equals(expectedSha256, ignoreCase = true)) {
        error(
            "CoMaps SDK candidate SHA-256 mismatch for $relativePath: expected $expectedSha256, got $actual " +
                "(candidate revision $pinnedCandidateRevision)"
        )
    }
}

verifyCandidateSha256("classes.jar", "074122f4de6c6c8c930bd5fae50be0626c527abb8498dae36f0176ce20ee3ca8")
verifyCandidateSha256("jni/arm64-v8a/liborganicmaps.so", "df51c422e3e6163e7079a53875933232577bb939396738cf05c0edb8cc509f21")
verifyCandidateSha256("jni/armeabi-v7a/liborganicmaps.so", "c911390d5906f97f82ec4876e56f90dbe05d9d24487897f81907a40b89dc1788")

android {
    namespace = "app.organicmaps.sdk"
    compileSdk = libs.versions.compileSdk.get().toInt()
    defaultConfig {
        minSdk = libs.versions.minSdk.get().toInt()
        consumerProguardFiles("consumer-rules.pro")
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        isCoreLibraryDesugaringEnabled = true
    }
    kotlinOptions { jvmTarget = "17" }
    // Consume the actual release BuildConfig from classes.jar; do not duplicate it.
    buildFeatures { buildConfig = false }
    sourceSets {
        getByName("main") {
            res.srcDir("$comapsSdkDir/res")
            assets.srcDir("$comapsSdkDir/assets")
            jniLibs.srcDir("$comapsSdkDir/jni")
        }
    }
    androidResources {
        noCompress += listOf("txt", "bin", "html", "png", "json", "mwm", "ttf", "sdf", "ui", "config", "csv", "spv", "obj")
    }
}

dependencies {
    implementation(files("$comapsSdkDir/classes.jar"))
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.lifecycle.viewmodel.compose)
    implementation(libs.activity.compose)
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.preference:preference:1.2.1")
    implementation("androidx.lifecycle:lifecycle-process:2.8.7")
    implementation("androidx.fragment:fragment-ktx:1.8.5")
    implementation("androidx.recyclerview:recyclerview:1.3.2")
    implementation("androidx.documentfile:documentfile:1.0.1")
    implementation("androidx.customview:customview:1.1.0")
    implementation("androidx.media:media:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
    implementation(libs.hilt.android)
    implementation(libs.hilt.navigation.compose)
    ksp(libs.hilt.compiler)
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")
    testImplementation(libs.junit.jupiter)
    testImplementation(libs.mockk)
    testImplementation(libs.kotlinx.coroutines.test)
    debugImplementation(libs.compose.ui.test.manifest)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.compose.ui.test.manifest)
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
}

tasks.withType<Test> { useJUnitPlatform() }
