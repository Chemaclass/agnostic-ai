// agnostic-ai JetBrains plugin build.
//
// Uses the IntelliJ Platform Gradle Plugin v2 (current recommended
// stack). Targets the IDE described by `platformVersion` in
// gradle.properties; that single knob controls both the dependency we
// compile against and the IDE the run/test tasks launch.

import org.jetbrains.intellij.platform.gradle.IntelliJPlatformType
import org.jetbrains.intellij.platform.gradle.TestFrameworkType

plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.25"
    id("org.jetbrains.intellij.platform") version "2.16.0"
}

group = "com.agnosticai"
version = providers.gradleProperty("pluginVersion").get()

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        create(IntelliJPlatformType.IntellijIdeaCommunity, providers.gradleProperty("platformVersion").get())
        // Bundled plugins the agnostic-ai plugin depends on. JSON
        // support ships in the platform itself (no separate plugin),
        // so only YAML is listed here.
        bundledPlugins("org.jetbrains.plugins.yaml")
        // Required by the instrumentCode task on IntelliJ Platform 2024.2+.
        instrumentationTools()
        pluginVerifier()
        zipSigner()
        testFramework(TestFrameworkType.Platform)
    }
    testImplementation("junit:junit:4.13.2")
}

intellijPlatform {
    pluginConfiguration {
        ideaVersion {
            sinceBuild = providers.gradleProperty("pluginSinceBuild")
            untilBuild = providers.gradleProperty("pluginUntilBuild")
        }
    }
    pluginVerification {
        ides {
            recommended()
        }
    }
}

kotlin {
    jvmToolchain(21)
}

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(21)
    }
}

tasks {
    wrapper {
        gradleVersion = providers.gradleProperty("gradleVersion").get()
    }
    publishPlugin {
        // Set JETBRAINS_MARKETPLACE_TOKEN in the env to publish.
        token = providers.environmentVariable("JETBRAINS_MARKETPLACE_TOKEN")
    }
}
