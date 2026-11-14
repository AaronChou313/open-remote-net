#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
: "${ANDROID_HOME:?Set ANDROID_HOME to the Android SDK directory}"
: "${ANDROID_NDK_HOME:=$ANDROID_HOME/ndk/28.2.13676358}"
export ANDROID_NDK_HOME
mobile_version=v0.0.0-20261011034528-197770444867
go install "golang.org/x/mobile/cmd/gomobile@$mobile_version"
go install "golang.org/x/mobile/cmd/gobind@$mobile_version"
PATH="$(go env GOPATH)/bin:$PATH"
export PATH
mkdir -p android/app/libs android/app/src/main/assets dist
python3 agent/packaging/notices.py
cp dist/THIRD_PARTY_NOTICES.txt android/app/src/main/assets/THIRD_PARTY_NOTICES.txt
gomobile bind -target=android/arm64 -androidapi=26 -trimpath -ldflags='-s -w' -javapkg=top.openremotehub.orn.core -o android/app/libs/orn.aar ./agent/mobile
(cd android && ./gradlew --no-daemon assembleDebug lintDebug)
cp android/app/build/outputs/apk/debug/app-debug.apk dist/orn-android-arm64-debug.apk
