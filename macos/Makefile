.PHONY: build test sync-check generate app clean

DERIVED_DATA ?= .dd
# Debug build for local runs and CI: ad-hoc identity, universal binary.
# ONLY_ACTIVE_ARCH=NO must be on the command line so it also reaches the
# SPM package targets, which otherwise build the active architecture only.
XCODEBUILD_FLAGS ?= CODE_SIGN_IDENTITY=- CODE_SIGNING_ALLOWED=NO ONLY_ACTIVE_ARCH=NO

build:
	swift build

test:
	swift test

sync-check:
	scripts/sync-check.sh

generate:
	xcodegen generate

app: generate
	xcodebuild -scheme Handbyte -configuration Debug -destination 'platform=macOS' \
		-derivedDataPath $(DERIVED_DATA) build $(XCODEBUILD_FLAGS)

clean:
	rm -rf .build $(DERIVED_DATA) Handbyte.xcodeproj App/Info.plist App/Handbyte.entitlements dist
