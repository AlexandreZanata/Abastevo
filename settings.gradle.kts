pluginManagement {
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "anp-fuel-prices"

include(":app")
include(":domain")
include(":application")
include(":data")
// The production map tab always ships the accepted native engine.
// Fall back to the versioned MAP-14 candidate when the property is absent.
if (providers.gradleProperty("comapsSdkDir").isPresent ||
    file("/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22").isDirectory) {
    include(":maps:engine")
}
