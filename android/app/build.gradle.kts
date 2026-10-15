import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// version from the git tag (make apk VERSION=vX.Y.Z), e.g. 1.23.0 -> code 12300
val appVersion: String = (project.findProperty("appVersion") as String?)?.removePrefix("v") ?: "0.0.1"
val appVersionCode = appVersion.split(".").map { it.toIntOrNull() ?: 0 }
    .let { (it.getOrElse(0) { 0 } * 10000) + (it.getOrElse(1) { 0 } * 100) + it.getOrElse(2) { 0 } }

// release signing: ~/Android/keys/xiaomi-cam.properties (storeFile, storePassword, keyAlias,
// keyPassword) - outside the repo
val signProps = Properties().apply {
    val f = File(System.getProperty("user.home"), "Android/keys/xiaomi-cam.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

android {
    namespace = "pl.lawinski.xiaomicam"
    compileSdk = 35

    defaultConfig {
        applicationId = "pl.lawinski.xiaomicam"
        minSdk = 26
        targetSdk = 35
        versionCode = maxOf(appVersionCode, 1)
        versionName = appVersion
    }

    signingConfigs {
        if (signProps.containsKey("storeFile")) {
            create("release") {
                storeFile = File(signProps.getProperty("storeFile"))
                storePassword = signProps.getProperty("storePassword")
                keyAlias = signProps.getProperty("keyAlias")
                keyPassword = signProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
    }

    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86", "x86_64")
            isUniversalApk = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging { jniLibs { useLegacyPackaging = false } }
}

dependencies {
    implementation(files("libs/xccore.aar"))

    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")

    val media3 = "1.5.1"
    implementation("androidx.media3:media3-exoplayer:$media3")
    implementation("androidx.media3:media3-exoplayer-rtsp:$media3")
    implementation("androidx.media3:media3-ui:$media3")
}
