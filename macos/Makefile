.PHONY: build test test-scripts sync-check parity generate app icon bump-version release clean

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

# Redraw the app icon set from scripts/render-app-icon.swift.
icon:
	swift scripts/render-app-icon.swift App/Resources/Assets.xcassets/AppIcon.appiconset

# Set MARKETING_VERSION and the next build number: make bump-version VERSION=0.2.0 [BUILD=N]
bump-version:
	@test -n "$(VERSION)" || { echo "usage: make bump-version VERSION=X.Y.Z [BUILD=N]" >&2; exit 2; }
	scripts/bump-version.sh $(VERSION) $(BUILD)

# Signed Release archive, notarized DMG and Sparkle appcast under dist/.
# VERSION=X.Y.Z asserts that project.yml carries that version (CI passes the tag).
release:
	DERIVED_DATA=$(DERIVED_DATA) scripts/release.sh $(if $(VERSION),--version $(VERSION))

clean:
	rm -rf .build $(DERIVED_DATA) Handbyte.xcodeproj App/Info.plist App/Handbyte.entitlements dist
