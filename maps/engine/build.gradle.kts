plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.hilt)
    alias(libs.plugins.ksp)
}

import java.security.MessageDigest
val pinnedCandidateRevision = "9cdd3cda00e1025a"
val comapsSdkDir = rootProject.extra["comapsSdkDirResolved"] as String

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

verifyCandidateSha256("classes.jar", "d856d1ed9d5cc7eac0ca74210d447eb59a3f5232bee9b97e2e2f29d076c02ec9")
verifyCandidateSha256("jni/arm64-v8a/liborganicmaps.so", "bb2aed7bdbc26e52fc90d0cfc0cf05e01b2e7aab96119b7f2e1d01533ba1c45b")
verifyCandidateSha256("jni/armeabi-v7a/liborganicmaps.so", "7d61b573c6a15bcb1bd7de463ec420a02b716f88c872099ff7d8e901f3fba842")

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
    implementation(project(":domain"))
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
