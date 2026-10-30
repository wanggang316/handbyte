.PHONY: build test test-scripts sync-check parity generate app release clean

DERIVED_DATA ?= .dd
# Debug build for local runs and CI: ad-hoc identity, universal binary.
# ONLY_ACTIVE_ARCH=NO must be on the command line so it also reaches the
# SPM package targets, which otherwise build the active architecture only.
XCODEBUILD_FLAGS ?= CODE_SIGN_IDENTITY=- CODE_SIGNING_ALLOWED=NO ONLY_ACTIVE_ARCH=NO

build:
	swift build

test:
	swift test

test-scripts:
	bash scripts/tests/release-lib.test.sh

sync-check:
	scripts/sync-check.sh

# Manual: compare hbscan with the Go CLI on this machine. PARITY_ARGS passes
# flags such as --min 100M --no-projects to both scanners.
parity:
	scripts/parity.sh $(PARITY_ARGS)

generate:
	xcodegen generate

app: generate
	xcodebuild -scheme Handbyte -configuration Debug -destination 'platform=macOS' \
		-derivedDataPath $(DERIVED_DATA) build $(XCODEBUILD_FLAGS)

# Signed Release archive, DMG and Sparkle appcast under dist/.
# Pass VERSION=X.Y.Z to override MARKETING_VERSION from project.yml.
release:
	DERIVED_DATA=$(DERIVED_DATA) scripts/release.sh $(if $(VERSION),--version $(VERSION))

clean:
	rm -rf .build $(DERIVED_DATA) Handbyte.xcodeproj App/Info.plist App/Handbyte.entitlements dist
