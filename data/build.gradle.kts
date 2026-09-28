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
        minSdk = libs.versions.minSdk.get().toInt()
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        testInstrumentationRunnerArguments["clearPackageData"] = "true"
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

    sourceSets {
        getByName("androidTest") {
            assets.srcDir("$projectDir/schemas")
            assets.srcDir("$projectDir/samples")
            assets.srcDir("src/main/assets")
        }
        getByName("test") {
            assets.srcDir("src/main/assets")
            resources.srcDir("$projectDir/fixtures")
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
    androidTestImplementation(libs.mockk)
    androidTestImplementation("io.mockk:mockk-android:${libs.versions.mockk.get()}")
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
