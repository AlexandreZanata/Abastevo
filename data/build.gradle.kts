plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.hilt)
    alias(libs.plugins.ksp)
}

android {
    namespace = "com.anpfuel.data"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        buildConfigField("boolean", "QUALITY_CACHE_ENABLED", providers.gradleProperty("qualityCacheEnabled").orElse("true").get().toBooleanStrict().toString())
        buildConfigField("boolean", "QUALITY_OUTBOX_ENABLED", providers.gradleProperty("qualityOutboxEnabled").orElse("true").get().toBooleanStrict().toString())
        // Map distribution origin/region/keys are build-time operational
        // config; empty means "not configured" (UI shows the pending state,
        // never a fake download). Keys are public verification keys only.
        buildConfigField("String", "MAP_DIST_BASE_URL", "\"${providers.gradleProperty("mapDistBaseUrl").orElse("").get()}\"")
        buildConfigField("String", "MAP_DIST_REGION", "\"${providers.gradleProperty("mapDistRegion").orElse("").get()}\"")
        buildConfigField("String", "MAP_DIST_KEYS", "\"${providers.gradleProperty("mapDistKeys").orElse("").get()}\"")
        minSdk = libs.versions.minSdk.get().toInt()
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        testInstrumentationRunnerArguments["clearPackageData"] = "true"
        externalNativeBuild {
            cmake {
                cppFlags += listOf("-std=c99")
                // Keep 16 KB page alignment consistent with the MAP-14 engine gate.
                arguments += listOf("-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON")
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/jni/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    testOptions {
        execution = "ANDROIDX_TEST_ORCHESTRATOR"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    // BuildConfig.DEBUG feeds the location release guard (P16-T02):
    // debug injection exists only in debuggable builds.
    buildFeatures {
        buildConfig = true
    }

    sourceSets {
        getByName("main").assets.srcDir("$rootDir/backend/internal/platform/legalterms/text")
        getByName("androidTest") {
            assets.srcDir("$projectDir/schemas")
            assets.srcDir("$projectDir/samples")
            assets.srcDir("src/main/assets")
        }
        getByName("test") {
            assets.srcDir("src/main/assets")
            resources.srcDir("$projectDir/fixtures")
            resources.srcDir("$rootDir/backend/internal/platform/legalterms/text")
        }
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

ksp {
    arg("room.schemaLocation", "$projectDir/schemas")
}

dependencies {
    implementation(project(":domain"))
    implementation(project(":application"))

    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    implementation(libs.kotlinx.coroutines.android)

    implementation(libs.okhttp)
    implementation(libs.jsoup)

    // P37-PC02 dependency/license review: docs/mobile/p37-image-ocr.md
    implementation("com.google.android.gms:play-services-mlkit-text-recognition:19.0.1")
    implementation("com.google.android.gms:play-services-base:18.11.0")
    implementation("androidx.exifinterface:exifinterface:1.3.7")

    implementation(libs.hilt.android)
    ksp(libs.hilt.compiler)

    implementation(libs.datastore.preferences)

    implementation(libs.work.runtime.ktx)
    implementation(libs.hilt.work)
    ksp(libs.hilt.work.compiler)

    testImplementation(libs.junit.jupiter)
    testImplementation(libs.mockk)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation("org.json:json:20240303")
    testImplementation(libs.kxml2)
    testImplementation(libs.poi.ooxml)
    testImplementation("com.squareup.okhttp3:mockwebserver:${libs.versions.okhttp.get()}")

    androidTestImplementation("androidx.test:core:1.6.1")
    androidTestImplementation(libs.room.testing)
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.work:work-testing:${libs.versions.work.get()}")
    androidTestImplementation("com.squareup.okhttp3:mockwebserver:${libs.versions.okhttp.get()}")
    androidTestImplementation(libs.mockk)
    androidTestImplementation("io.mockk:mockk-android:${libs.versions.mockk.get()}")
    androidTestImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(project(":application"))
    androidTestUtil("androidx.test:orchestrator:1.5.1")
}

tasks.withType<Test> {
    useJUnitPlatform()
}

afterEvaluate {
    tasks.register<Test>("parserMemoryTest") {
        description = "Runs parser memory POC with a 64MB heap"
        group = "verification"

        val unitTest = tasks.named<Test>("testDebugUnitTest").get()
        testClassesDirs = unitTest.testClassesDirs
        classpath = unitTest.classpath

        useJUnitPlatform {
            includeTags("parser-memory")
        }

        maxHeapSize = "64m"
        jvmArgs("-XX:+HeapDumpOnOutOfMemoryError")
    }
}

// Each synthetic Go harness owns a fresh TLS/database fixture. Never reuse a skipped/cached run.
tasks.withType<Test> {
    inputs.property("consentInteropFixture", System.getenv("ANPFUEL_ANDROID_CONSENT_FIXTURE").orEmpty())
}
