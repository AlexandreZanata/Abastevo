// Compile the Google ID SDK behind a Java boundary. Its latest stable AAR
// carries Kotlin metadata newer than the app's pinned Kotlin compiler.
plugins {
    alias(libs.plugins.android.library)
}

android {
    namespace = "com.anpfuel.app.credential.google"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        minSdk = libs.versions.minSdk.get().toInt()
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    implementation(libs.credentials.core)
    implementation(libs.google.identity)
}
