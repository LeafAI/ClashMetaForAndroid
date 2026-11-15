ANDROID_HOME ?= $(HOME)/android-sdk
JAVA_HOME ?= /Applications/Android Studio.app/Contents/jbr/Contents/Home
FLAVOR ?= Meta
ABI ?= arm64-v8a
GRADLE := ./gradlew
BUILD_TOOLS ?= $(ANDROID_HOME)/build-tools/35.0.0
PACKAGE := com.github.metacubex.clash.$(shell echo $(FLAVOR) | tr A-Z a-z)
FLAVOR_DIR := $(shell echo $(FLAVOR) | tr A-Z a-z)

export JAVA_HOME
export ANDROID_HOME
export ANDROID_SDK_ROOT := $(ANDROID_HOME)

.PHONY: all android-debug android-release android-install android-clean android-apk android-verify serveapk submodules go-vet go-test help

all: android-debug android-release

submodules:
	git submodule update --init --depth 1

android-debug: submodules
	@echo "Using JAVA_HOME: $(JAVA_HOME)"
	@echo "Using ANDROID_HOME: $(ANDROID_HOME)"
	$(GRADLE) :app:assemble$(FLAVOR)Debug

android-release: submodules
	@echo "Using JAVA_HOME: $(JAVA_HOME)"
	@echo "Using ANDROID_HOME: $(ANDROID_HOME)"
	$(GRADLE) :app:assemble$(FLAVOR)Release

android-apk: android-debug
	@echo "APK generated at: app/build/outputs/apk/$(FLAVOR_DIR)/debug/"

android-install: android-debug
	adb install -r app/build/outputs/apk/$(FLAVOR_DIR)/debug/*-$(ABI)-debug.apk

android-verify:
	@for apk in app/build/outputs/apk/$(FLAVOR_DIR)/*/*-$(ABI)-*.apk; do \
		echo "== $$apk"; \
		"$(BUILD_TOOLS)/apksigner" verify --print-certs "$$apk" | grep -E "Signer #1 certificate (DN|SHA-256)"; \
	done

go-vet:
	cd core/src/main/golang && \
		CGO_ENABLED=1 GOOS=android GOARCH=arm64 \
		CC=$(ANDROID_HOME)/ndk/29.0.14206865/toolchains/llvm/prebuilt/darwin-x86_64/bin/aarch64-linux-android23-clang \
		go vet -tags foss,with_gvisor,cmfa ./native/config/ ./native/tunnel/ ./native/rulemerge/

go-test:
	cd core/src/main/golang && go test -race -count=1 ./native/rulemerge/

android-clean:
	$(GRADLE) clean

serveapk: android-clean android-release
	@echo "Available IP for HTTP connections(:9980): $(shell ifconfig | grep inet | grep -v inet6 | awk '{print $$2}')"
	@python3 -m http.server 9980 -d $(PWD)/app/build/outputs/apk/$(FLAVOR_DIR)/release

help:
	@echo "Available targets:"
	@echo "  android-debug    - Build debug APKs (FLAVOR=Meta|Alpha, default Meta)"
	@echo "  android-release  - Build release APKs, signed with the key in local.properties"
	@echo "  android-apk      - Build debug APKs and show location"
	@echo "  android-install  - Build debug APK and install to the connected device (ABI=$(ABI))"
	@echo "  android-verify   - Print signing certificate of built APKs"
	@echo "  go-vet           - Vet the native Go packages for android/arm64"
	@echo "  go-test          - Run the native Go unit tests"
	@echo "  submodules       - Fetch the mihomo core submodule"
	@echo "  android-clean    - Clean build artifacts"
	@echo "  serveapk         - Clean, build release and serve it over HTTP on :9980"
	@echo ""
	@echo "Signing: set signing.keystore (and optionally signing.store.password,"
	@echo "signing.key.alias, signing.key.password) in local.properties."
