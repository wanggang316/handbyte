# Homebrew cask for the Handbyte macOS app.
#
# This file is the template for the published cask. When an app release
# (tag app/vX.Y.Z) is published, .github/workflows/macos-publish.yml stamps
# `version` and `sha256` with macos/scripts/render-cask.sh and pushes the
# result to wanggang316/homebrew-tap, so that
# `brew install --cask wanggang316/tap/handbyte` installs the latest DMG.
#
# Sparkle updates the app in place after install; `auto_updates true` tells
# Homebrew not to report that as drift.
cask "handbyte" do
  version "0.1.0"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"

  url "https://github.com/wanggang316/handbyte/releases/download/app/v#{version}/Handbyte-#{version}.dmg",
      verified: "github.com/wanggang316/handbyte/"
  name "Handbyte"
  desc "Review and clean caches and build output on your Mac"
  homepage "https://handbyte.gumpw.com"

  livecheck do
    url "https://handbyte.gumpw.com/appcast.xml"
    strategy :sparkle
  end

  auto_updates true
  depends_on macos: ">= :sonoma"

  app "Handbyte.app"

  zap trash: [
    "~/Library/Application Support/Handbyte",
    "~/Library/Caches/com.gumpw.handbyte",
    "~/Library/HTTPStorages/com.gumpw.handbyte",
    "~/Library/Preferences/com.gumpw.handbyte.plist",
    "~/Library/Saved Application State/com.gumpw.handbyte.savedState",
  ]
end
