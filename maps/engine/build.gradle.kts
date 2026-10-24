plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
}

val comapsSdkDir = providers.gradleProperty("comapsSdkDir").orNull
    ?: error("comapsSdkDir Gradle property is required for :maps:engine")

android {
    namespace = "app.organicmaps.sdk"
    compileSdk = libs.versions.compileSdk.get().toInt()
    defaultConfig {
        minSdk = libs.versions.minSdk.get().toInt()
        consumerProguardFiles("consumer-rules.pro")
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
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")
}
