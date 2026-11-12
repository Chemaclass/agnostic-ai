// agnostic-ai JetBrains plugin build.
//
// Uses the IntelliJ Platform Gradle Plugin v2 (current recommended
// stack). Targets the IDE described by `platformVersion` in
// gradle.properties; that single knob controls both the dependency we
// compile against and the IDE the run/test tasks launch.

import org.jetbrains.intellij.platform.gradle.IntelliJPlatformType
import org.jetbrains.intellij.platform.gradle.TestFrameworkType
import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.dsl.KotlinVersion
import org.jetbrains.kotlin.gradle.tasks.KotlinJvmCompile

plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.3.21"
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

// Gradle and the compilers run on JDK 25. IDE 2024.2 runs plugins on
// Java 21 with Kotlin stdlib 1.9, so the output targets those. The
// targets go on the tasks because the toolchain's own target overrides
// the extension.
kotlin {
    jvmToolchain(25)
}

tasks.withType<KotlinJvmCompile>().configureEach {
    compilerOptions {
        jvmTarget = JvmTarget.JVM_21
        apiVersion = KotlinVersion.KOTLIN_1_9
        freeCompilerArgs.add("-Xjdk-release=21")
    }
}

tasks.withType<JavaCompile>().configureEach {
    options.release = 21
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
