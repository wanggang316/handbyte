package scanner

// Preset describes a known cleanable location on macOS.
//
// Path is the directory to inspect. If ExpandChildren is true, the scanner
// lists each direct child as a separate Item so the user can pick per app;
// otherwise the directory is reported as one Item.
type Preset struct {
	Path           string
	ExpandChildren bool
	Category       Category
	Safety         Safety
	Description    string
	Source         string
}

// Presets is the curated list of known cleanable macOS paths.
//
// Paths use ~ for the home directory; the scanner expands them. Anything that
// does not exist on the current machine is silently skipped.
var Presets = []Preset{
	// Xcode — biggest wins on most dev macs.
	{
		Path:           "~/Library/Developer/Xcode/DerivedData",
		Category:       CatXcode,
		Safety:         SafetyAuto,
		Description:    "Xcode build intermediates. Regenerated on next build.",
		Source:         "xcode-derived",
		ExpandChildren: true,
	},
	{
		Path:           "~/Library/Developer/Xcode/iOS DeviceSupport",
		Category:       CatXcode,
		Safety:         SafetyAuto,
		Description:    "Symbols for connected iOS devices. Re-downloaded on reconnect.",
		Source:         "xcode-devicesupport",
		ExpandChildren: true,
	},
	{
		Path:           "~/Library/Developer/Xcode/watchOS DeviceSupport",
		Category:       CatXcode,
		Safety:         SafetyAuto,
		Description:    "Symbols for connected watchOS devices.",
		Source:         "xcode-devicesupport",
		ExpandChildren: true,
	},
	{
		Path:           "~/Library/Developer/Xcode/tvOS DeviceSupport",
		Category:       CatXcode,
		Safety:         SafetyAuto,
		Description:    "Symbols for connected tvOS devices.",
		Source:         "xcode-devicesupport",
		ExpandChildren: true,
	},
	{
		Path:        "~/Library/Developer/Xcode/Archives",
		Category:    CatXcode,
		Safety:      SafetyCaution,
		Description: "App archives (.xcarchive). Needed for re-signing or symbolication of shipped builds.",
		Source:      "xcode-archives",
	},
	{
		Path:        "~/Library/Developer/CoreSimulator/Caches",
		Category:    CatXcode,
		Safety:      SafetyAuto,
		Description: "iOS Simulator caches.",
		Source:      "coresim-cache",
	},

	// Developer language caches.
	{
		Path:        "~/Library/Caches/go-build",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Go build cache. Regenerated on next `go build`.",
		Source:      "go-build",
	},
	{
		Path:        "~/go/pkg/mod/cache/download",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Go module download cache. Re-downloaded on demand.",
		Source:      "go-mod-cache",
	},
	{
		Path:        "~/.cargo/registry",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Cargo registry & source cache. Re-downloaded on next build.",
		Source:      "cargo-registry",
	},
	{
		Path:        "~/.cargo/git",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Cargo git dependency cache.",
		Source:      "cargo-git",
	},
	{
		Path:           "~/.rustup/toolchains",
		Category:       CatDevCache,
		Safety:         SafetyReview,
		Description:    "Installed Rust toolchains. Removing one forces re-install on next use.",
		Source:         "rustup",
		ExpandChildren: true,
	},
	{
		Path:        "~/.npm/_cacache",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "npm content-addressable cache.",
		Source:      "npm-cache",
	},
	{
		Path:        "~/Library/pnpm/store",
		Category:    CatDevCache,
		Safety:      SafetyReview,
		Description: "pnpm content-addressable store. Shared across projects; rebuilds on `pnpm install`.",
		Source:      "pnpm-store",
	},
	{
		Path:        "~/.yarn/berry/cache",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Yarn Berry cache.",
		Source:      "yarn-cache",
	},
	{
		Path:        "~/Library/Caches/Yarn",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Classic Yarn cache.",
		Source:      "yarn-cache",
	},
	{
		Path:        "~/.gradle/caches",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Gradle dependency & build cache.",
		Source:      "gradle-cache",
	},
	{
		Path:        "~/.m2/repository",
		Category:    CatDevCache,
		Safety:      SafetyReview,
		Description: "Maven local repository. Re-downloaded on demand.",
		Source:      "maven-cache",
	},
	{
		Path:        "~/Library/Caches/org.swift.swiftpm",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Swift Package Manager cache.",
		Source:      "swiftpm-cache",
	},
	{
		Path:        "~/Library/Caches/ms-playwright",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Playwright browser binaries. Re-fetched by `npx playwright install`.",
		Source:      "playwright",
	},
	{
		Path:        "~/Library/Caches/Cypress",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Cypress test runner binaries.",
		Source:      "cypress",
	},
	{
		Path:        "~/Library/Caches/Homebrew",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Homebrew download cache. `brew cleanup` does the same.",
		Source:      "brew-cache",
	},
	{
		Path:        "~/Library/Caches/JetBrains",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "JetBrains IDE caches (indexes, logs). Rebuilt on next launch.",
		Source:      "jetbrains",
	},
	{
		Path:        "~/Library/Caches/com.openai.codex",
		Category:    CatDevCache,
		Safety:      SafetyAuto,
		Description: "Codex CLI cache.",
		Source:      "codex",
	},
	{
		Path:           "~/.cache",
		Category:       CatDevCache,
		Safety:         SafetyReview,
		Description:    "XDG user cache (huggingface, uv, pip, etc.). Per-tool, generally rebuildable.",
		Source:         "xdg-cache",
		ExpandChildren: true,
	},

	// Per-application caches. ExpandChildren so the user picks per app.
	{
		Path:           "~/Library/Caches",
		ExpandChildren: true,
		Category:       CatSystemCache,
		Safety:         SafetyAuto,
		Description:    "Per-application caches under ~/Library/Caches. Apps rebuild as needed.",
		Source:         "library-caches",
	},
	{
		Path:           "~/Library/Logs",
		ExpandChildren: true,
		Category:       CatTrash,
		Safety:         SafetyAuto,
		Description:    "Per-application logs.",
		Source:         "library-logs",
	},
	{
		Path:           "~/Library/Containers",
		ExpandChildren: true,
		Category:       CatSystemCache,
		Safety:         SafetyCaution,
		Description:    "Sandboxed app containers. Holds app state — only delete if you know the app.",
		Source:         "containers",
	},

	// VMs & container runtimes — caution, but commonly large.
	{
		Path:        "~/Library/Group Containers/HUAQ24HBR6.dev.orbstack/data",
		Category:    CatVMs,
		Safety:      SafetyCaution,
		Description: "OrbStack VM/container data. Use OrbStack UI to remove individual containers first.",
		Source:      "orbstack",
	},
	{
		Path:        "~/Library/Application Support/com.apple.container",
		Category:    CatVMs,
		Safety:      SafetyCaution,
		Description: "Apple Container framework data.",
		Source:      "apple-container",
	},
	{
		Path:        "~/Library/Application Support/Claude/vm_bundles",
		Category:    CatVMs,
		Safety:      SafetyCaution,
		Description: "Claude desktop VM bundles.",
		Source:      "claude-vm",
	},
	{
		Path:        "~/.docker",
		Category:    CatVMs,
		Safety:      SafetyCaution,
		Description: "Docker config & data. Use `docker system prune` first.",
		Source:      "docker",
	},

	// User-state stuff.
	{
		Path:        "~/.Trash",
		Category:    CatTrash,
		Safety:      SafetyReview,
		Description: "Files already in the Trash.",
		Source:      "trash",
	},
	{
		Path:        "~/Downloads",
		Category:    CatOther,
		Safety:      SafetyCaution,
		Description: "Downloads folder. Review before deleting.",
		Source:      "downloads",
	},
}

// ProjectArtifactPatterns are directory names commonly produced by build
// systems. The scanner walks user project roots and reports each match as
// a candidate cleanup target.
type ProjectArtifact struct {
	Name        string
	Category    Category
	Safety      Safety
	Description string
}

var ProjectArtifactPatterns = []ProjectArtifact{
	{"node_modules", CatProjectBuild, SafetyReview, "Node.js dependencies. Restored by `npm/pnpm/yarn install`."},
	{"target", CatProjectBuild, SafetyReview, "Rust build output. Rebuilt by `cargo build`."},
	{"build", CatProjectBuild, SafetyReview, "Generic build output."},
	{".build", CatProjectBuild, SafetyReview, "Swift Package Manager build output."},
	{"dist", CatProjectBuild, SafetyReview, "Dist/output folder (frontend bundles, etc.)."},
	{".next", CatProjectBuild, SafetyReview, "Next.js build cache."},
	{".nuxt", CatProjectBuild, SafetyReview, "Nuxt build cache."},
	{".turbo", CatProjectBuild, SafetyReview, "Turborepo cache."},
	{".parcel-cache", CatProjectBuild, SafetyReview, "Parcel bundler cache."},
	{".gradle", CatProjectBuild, SafetyReview, "Per-project Gradle cache."},
	{".dart_tool", CatProjectBuild, SafetyReview, "Dart/Flutter tool cache."},
	{"DerivedData", CatProjectBuild, SafetyReview, "Per-project Xcode DerivedData."},
	{"Pods", CatProjectBuild, SafetyReview, "CocoaPods install. Restored by `pod install`."},
	{"__pycache__", CatProjectBuild, SafetyAuto, "Python bytecode cache."},
	{".pytest_cache", CatProjectBuild, SafetyAuto, "pytest cache."},
	{".mypy_cache", CatProjectBuild, SafetyAuto, "mypy cache."},
	{".ruff_cache", CatProjectBuild, SafetyAuto, "ruff cache."},
	{".venv", CatProjectBuild, SafetyReview, "Python virtualenv. Recreated by `python -m venv`."},
	{"venv", CatProjectBuild, SafetyReview, "Python virtualenv."},
}

// DefaultProjectRoots are the directories scanned for ProjectArtifactPatterns
// by default. Override via --roots flag.
var DefaultProjectRoots = []string{
	"~/dev",
	"~/Projects",
	"~/Code",
	"~/workspace",
}
